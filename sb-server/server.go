package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/roackb2/syncbox"
)

func main() {
	server, err := NewServer()
	if err != nil {
		fmt.Printf("error on new server: %v\n", err)
		return
	}
	if err := server.Start(); err != nil {
		server.LogError("error on server start: %v\n", err)
		return
	}
}

// Server is the main class to be run at server program
type Server struct {
	*syncbox.Logger
	*syncbox.DB
	*syncbox.ServerConnector
	*syncbox.Storage
}

// NewServer instantiates server
func NewServer() (*Server, error) {
	logger := syncbox.NewDefaultLogger()
	db, err := syncbox.NewDB(syncbox.UserTable{}, syncbox.FileTable{}, syncbox.FileRefTable{})
	if err != nil {
		logger.LogDebug("error on connecting database:%v\n", err)
		return nil, err
	}
	connector, err := syncbox.NewServerConnector()
	if err != nil {
		logger.LogDebug("error on new server connector: %v\n", err)
		return nil, err
	}
	storage := syncbox.NewStorage()
	server := &Server{
		Logger:          logger,
		DB:              db,
		ServerConnector: connector,
		Storage:         storage,
	}
	return server, nil
}

// Start starts to run a server program
func (server *Server) Start() error {
	err := server.Listen(server)
	if err != nil {
		server.LogDebug("error on listen:%v\n", err)
		return err
	}
	return nil
}

// HandleRequest implements the ConnectionHandler interface
func (server *Server) HandleRequest(peer *syncbox.Peer) error {
	return syncbox.HandleRequest(peer, server)
}

// HandleError implements the ConnectionHandler interface
func (server *Server) HandleError(err error) {
	if err == syncbox.ErrorPeerSocketClosed {
		server.LogInfo("%v\n", err)
	} else {
		server.LogError("error: %v\n", err)
	}
}

// ProcessIdentity implements the ConnectionHandler interface
func (server *Server) ProcessIdentity(req *syncbox.Request, peer *syncbox.Peer, eHandler syncbox.ErrorHandler) {
	if peer.Username != "" && peer.RefGraph == nil {
		rg, err := syncbox.NewRefGraph(peer.Username, peer.Password, server.DB)
		if err != nil {
			server.LogDebug("error on NewRefGraph in ProcessDigest: %v\n", err)
			eHandler(err)
		}
		peer.RefGraph = rg
	}
	data := req.Data
	iReq := syncbox.IdentityRequest{}
	if err := json.Unmarshal(data, &iReq); err != nil {
		server.LogDebug("error on Unmarshal in ProcessIdentity: %v\n", err)
		eHandler(err)
	}
	server.LogDebug("sending response in ProcessIdentity, request id: %v\n", req.ID)
	if err := peer.SendResponse(server, req, &syncbox.Response{
		Status:  syncbox.StatusOK,
		Message: syncbox.MessageAccept,
	}); err != nil {
		server.LogDebug("error on SendResponse in ProcessIdentity: %v\n", err)
		eHandler(err)
	}
}

// ProcessDigest implements the ConnectionHandler interface
func (server *Server) ProcessDigest(req *syncbox.Request, peer *syncbox.Peer, eHandler syncbox.ErrorHandler) {
	hasServerDigest := true
	serverDir := syncbox.NewEmptyDir()
	if peer.Username != "" && peer.RefGraph == nil {
		rg, err := syncbox.NewRefGraph(peer.Username, peer.Password, server.DB)
		if err != nil {
			server.LogDebug("error on NewRefGraph in ProcessDigest: %v\n", err)
			eHandler(err)
		}
		peer.RefGraph = rg
	}
	data := req.Data
	dReq := syncbox.DigestRequest{}
	if err := json.Unmarshal(data, &dReq); err != nil {
		server.LogDebug("error on Unmarshal in ProcessDigest: %v\n", err)
		eHandler(err)
	}
	server.LogVerbose("server ProcessDigest called, req\n%v\n", dReq)

	// create a bucket for the user, if not exists
	err := server.Storage.CreateBucket(req.Username)
	if err != nil {
		server.LogDebug("error on creating bucket in ProcessDigest: %v\n", err)
		eHandler(err)
	}
	server.LogInfo("server completes create bucket in ProcessDigest\n")

	// reborn the directory of the request
	dirBytes, err := json.Marshal(dReq.Dir)
	if err != nil {
		server.LogDebug("error Marshal in ProcessDigest: %v\n", err)
		eHandler(err)
	}
	server.LogVerbose("dirBytes after json Marshal: %v\n", dirBytes)

	// get the server side digest file for the user
	serverDigestBytes, err := server.Storage.GetObject(req.Username, syncbox.DigestFileName)
	if err != nil {
		if strings.HasPrefix(err.Error(), "NoSuchKey: The specified key does not exist.") {
			hasServerDigest = false
		} else {
			server.LogDebug("error on GetObject in ProcessDigest: %v\n", err)
			eHandler(err)
		}
	}
	server.LogVerbose("serverDigestBytes:\n%v\n", serverDigestBytes)

	// reborn old directory from digest file,  if it exists
	if hasServerDigest {
		if err := json.Unmarshal(serverDigestBytes, serverDir); err != nil {
			server.LogDebug("error on Unmarshal in ProcessDigest: %v\n", err)
			eHandler(err)
		}
	}
	server.LogVerbose("serverDir:\n%v\n", serverDir)

	// send response to user before Compare
	server.LogDebug("sending response in ProcessDigest, request id: %v\n", req.ID)
	if err := peer.SendResponse(server, req, &syncbox.Response{
		Status:  syncbox.StatusOK,
		Message: syncbox.MessageAccept,
	}); err != nil {
		server.LogDebug("error on SendResponse in ProcessDigest: %v\n", err)
		eHandler(err)
	}

	if serverDir.ModTime.Before(dReq.Dir.ModTime) {
		// if client side digest is newer, compare the server side and client side directory, sync files tree on server with client status,
		// and broadcast to other connections to sync their file tree with original peer status
		if err := syncbox.Compare(serverDir, dReq.Dir, server, peer); err != nil {
			server.LogDebug("error on Compare in ProcessDigest: %v\n", err)
			eHandler(err)
		}
		for addr, clientPeer := range server.Clients {
			if clientPeer.Username == peer.Username && addr != peer.Address {
				res, innerErr := clientPeer.SendDigestRequest(server, syncbox.SyncboxServerUsername, syncbox.SyncboxServerPwd, syncbox.SyncboxServerDevice, dReq.Dir)
				if innerErr != nil {
					server.LogError("error on SendDigestRequest in ProcessDigest: %v\v", innerErr)
					eHandler(innerErr)
				}
				server.LogInfo("SendDigestRequest result: %v\n", res)
			}
		}
	} else {
		// otherwise, tell the original peer to update its file tree with server status
		res, innerErr := peer.SendDigestRequest(server, syncbox.SyncboxServerUsername, syncbox.SyncboxServerPwd, syncbox.SyncboxServerDevice, serverDir)
		if innerErr != nil {
			server.LogError("error on SendDigestRequest in ProcessDigest: %v\v", innerErr)
			eHandler(innerErr)
		}
		server.LogInfo("SendDigestRequest result: %v\n", res)
	}

	server.LogVerbose("before creating digest file object, dirBytes:\n%v\n", string(dirBytes))
	// put the digest file to S3
	if err := server.Storage.CreateObject(req.Username, syncbox.DigestFileName, string(dirBytes)); err != nil {
		server.LogError("error on CreateObject in ProcessDigest: %v\n", err)
		eHandler(err)
	}

	// clean up objects in S3 if no refs on the files
	noRefFiles, err := peer.RefGraph.GetNoRefFiles()
	if err != nil {
		server.LogError("error on GetNoRefFiles in ProcessDigest: %v\n", err)
		eHandler(err)
	}
	for _, fileRec := range noRefFiles {
		if err := server.DeleteObject(peer.Username, fileRec.Checksum); err != nil {
			server.LogError("error on DeleteObject in ProcessDigest: %v\n", err)
			eHandler(err)
		}
		if err := peer.RefGraph.DeleteFileRecord(fileRec); err != nil {
			server.LogError("error on DeleteFileRecord in ProcessDigest: %v\n", err)
			eHandler(err)
		}
	}
	server.LogInfo("server finish cleaning up no ref files in ProcessDigest\n")
}

// ProcessSync implements the ConnectionHandler interface
func (server *Server) ProcessSync(req *syncbox.Request, peer *syncbox.Peer, eHandler syncbox.ErrorHandler) {
	data := req.Data
	sReq := syncbox.SyncRequest{}
	if err := json.Unmarshal(data, &sReq); err != nil {
		server.LogDebug("error on Unmarshal in ProcessSync: %v\n", err)
		eHandler(err)
	}

	switch sReq.Action {
	case syncbox.ActionGet:
		fileBytes, err := server.GetObject(req.Username, syncbox.ChecksumToNumString(sReq.File.ContentChecksum))
		if err != nil {
			server.LogDebug("error on GetObject in ProcessSync: %v\n", err)
			eHandler(err)
		}

		server.LogDebug("sending response in ProcessSync, request id: %v\n", req.ID)
		if err := peer.SendResponse(server, req, &syncbox.Response{
			Status:  syncbox.StatusOK,
			Message: syncbox.MessageAccept,
		}); err != nil {
			server.LogDebug("error on SendResponse in ProcessSync: %v\n", err)
			eHandler(err)
		}

		res, err := peer.SendFileRequest(server, syncbox.SyncboxServerUsername, syncbox.SyncboxServerPwd, syncbox.SyncboxServerDevice, sReq.UnrootPath, sReq.File, fileBytes)
		if err != nil {
			server.LogDebug("error on SendFileRequest in ProcessSync: %v\n", err)
			eHandler(err)
		}
		server.LogInfo("response of SendFileRequest:\n%v\n", res)
	}
}

// ProcessFile implements the ConnectionHandler interface
// should executes the steps to save file to s3
func (server *Server) ProcessFile(req *syncbox.Request, peer *syncbox.Peer, eHandler syncbox.ErrorHandler) {
	data := req.Data
	dReq := syncbox.FileRequest{}
	if err := json.Unmarshal(data, &dReq); err != nil {
		server.LogDebug("error Unmarshal data in ProcessFile: %v\n", err)
		eHandler(err)
	}
	// server.LogDebug("server ProcessFile called, req: %v\n", dReq)

	filename := syncbox.ChecksumToNumString(dReq.File.ContentChecksum)
	content := string(dReq.Content)
	// server.LogDebug("filename: %v\ncontent: %v\n", filename, content)
	if err := server.Storage.CreateObject(req.Username, filename, content); err != nil {
		server.LogDebug("error on CreateObject in ProcessFile: %v\n", err)
		eHandler(err)
	}

	server.LogDebug("sending response in ProcessFile, request id: %v\n", req.ID)
	if err := peer.SendResponse(server, req, &syncbox.Response{
		Status:  syncbox.StatusOK,
		Message: syncbox.MessageAccept,
	}); err != nil {
		server.LogDebug("error on SendResponse in ProcessFile: %v\n", err)
		eHandler(err)
	}
}

// AddFile implements the Syncer interface
// should send a FileRequest to client to get file content, and save to S3
func (server *Server) AddFile(rootPath string, unrootPath string, file *syncbox.File, peer *syncbox.Peer) error {
	server.LogVerbose("AddFile, rootPath: %v, unrootPath: %v, file path: %v", rootPath, unrootPath, file.Path)
	duplicate := false
	// firstly add a file record to database, see if there are duplicates
	if err := peer.RefGraph.AddFileRecord(file); err != nil {
		duplicate, _ = regexp.MatchString("Error \\d+: Duplicate entry '.*' for key 'checksum'", err.Error())
		if !duplicate {
			server.LogDebug("error on AddFileRecord in AddFile: %v\n", err)
			return err
		}
	}

	// if no duplicate, send a sync request to client to get file and save to s3
	if !duplicate {
		res, err := peer.SendSyncRequest(server, syncbox.SyncboxServerUsername, syncbox.SyncboxServerPwd, syncbox.SyncboxServerDevice, unrootPath, syncbox.ActionGet, file)
		if err != nil {
			server.LogDebug("error on SendSyncRequest in AddFile: %v\n", err)
			return err
		}
		server.LogDebug("response of SendSyncRequest in AddFile:\n%v\n", res)
	}

	// no matter whether there are duplicates, it's needed to add a file ref record to database
	if err := peer.RefGraph.AddFileRefRecord(file, unrootPath, peer.Device); err != nil {
		duplicate, _ = regexp.MatchString("Error \\d+: Duplicate entry '.*' for key 'PRIMARY'", err.Error())
		if !duplicate {
			server.LogDebug("error on AddFileRefRecord in AddFile: %v\n", err)
			return err
		}
	}

	return nil
}

// DeleteFile implements the Syncer interface
// should delete the file ref in database
func (server *Server) DeleteFile(rootPath string, unrootPath string, file *syncbox.File, peer *syncbox.Peer) error {
	server.LogVerbose("DeleteFile, rootPath: %v, unrootPath: %v, file path: %v", rootPath, unrootPath, file.Path)
	if err := peer.RefGraph.DeleteFileRefRecord(file); err != nil && err != syncbox.ErrorNoFileRecords {
		server.LogDebug("error on DeleteRef in DeleteFile: %v\n", err)
		return err
	}

	return nil
}

// AddDir implements the Syncer interface
// should walk through the directory recursively and call AddFile on files
func (server *Server) AddDir(rootPath string, unrootPath string, dir *syncbox.Dir, peer *syncbox.Peer) error {
	return syncbox.WalkSubDir(rootPath, dir, peer, server.AddFile, server.AddDir)
}

// DeleteDir implements the Syncer interface
// should walk through the directory recursively and call DeleteFile on files
func (server *Server) DeleteDir(rootPath string, unrootPath string, dir *syncbox.Dir, peer *syncbox.Peer) error {
	return syncbox.WalkSubDir(rootPath, dir, peer, server.DeleteFile, server.DeleteDir)
}
