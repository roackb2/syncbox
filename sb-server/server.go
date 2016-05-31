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
	server.LogError("error: %v\n", err)
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
	// server.LogDebug("server ProcessIdentity called, req: %v\n", iReq)
	if err := peer.SendResponse(&syncbox.Response{
		Status:  syncbox.StatusOK,
		Message: syncbox.MessageAccept,
	}); err != nil {
		server.LogDebug("error on SendResponse in ProcessIdentity: %v\n", err)
		eHandler(err)
	}
}

// ProcessDigest implements the ConnectionHandler interface
func (server *Server) ProcessDigest(req *syncbox.Request, peer *syncbox.Peer, eHandler syncbox.ErrorHandler) {
	hasOldDigest := true
	oldDir := syncbox.NewEmptyDir()
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
	// server.LogDebug("server ProcessDigest called, req: %v\n", dReq)
	// server.LogDebug("reborned dir: \n%v\n", dReq.Dir)

	// create a bucket for the user, if not exists
	err := server.Storage.CreateBucket(req.Username)
	if err != nil {
		server.LogDebug("error on creating bucket in ProcessDigest: %v\n", err)
		eHandler(err)
	}

	// reborn the directory of the request
	dirBytes, err := json.Marshal(dReq.Dir)
	if err != nil {
		server.LogDebug("error Marshal in ProcessDigest: %v\n", err)
		eHandler(err)
	}
	// server.LogDebug("dir bytes: %v\n", dirBytes)

	// get the server side digest file for the user
	oldDigestBytes, err := server.Storage.GetObject(req.Username, syncbox.DigestFileName)
	if err != nil {
		if strings.HasPrefix(err.Error(), "NoSuchKey: The specified key does not exist.") {
			hasOldDigest = false
		} else {
			server.LogDebug("error on GetObject in ProcessDigest: %v\n", err)
			eHandler(err)
		}
	}
	// server.LogDebug("old digest bytes: %v\n", oldDigestBytes)

	// reborn old directory from digest file,  if it exists
	if hasOldDigest {
		if err := json.Unmarshal(oldDigestBytes, oldDir); err != nil {
			server.LogDebug("error on Unmarshal in ProcessDigest: %v\n", err)
			eHandler(err)
		}
	}

	// server.LogDebug("old dir: %v\n", oldDir)

	// send response to user before Compare
	if err := peer.SendResponse(&syncbox.Response{
		Status:  syncbox.StatusOK,
		Message: syncbox.MessageAccept,
	}); err != nil {
		server.LogDebug("error on SendResponse in ProcessDigest: %v\n", err)
		eHandler(err)
	}

	// compare the server side and client side directory, and sync them
	if err := syncbox.Compare(oldDir, dReq.Dir, server, peer); err != nil {
		server.LogDebug("error on Compare in ProcessDigest: %v\n", err)
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
	}

	// put the digest file to S3
	if err := server.Storage.CreateObject(req.Username, syncbox.DigestFileName, string(dirBytes)); err != nil {
		server.LogError("error on CreateObject in ProcessDigest: %v\n", err)
		eHandler(err)
	}
}

// ProcessSync implements the ConnectionHandler interface
func (server *Server) ProcessSync(req *syncbox.Request, peer *syncbox.Peer, eHandler syncbox.ErrorHandler) {
	data := req.Data
	sReq := syncbox.SyncRequest{}
	if err := json.Unmarshal(data, &sReq); err != nil {
		server.LogDebug("error on Unmarshal in ProcessSync: %v\n", err)
		eHandler(err)
	}
	// server.LogDebug("server ProcessSync called, req: %v\n", sReq)
	if err := peer.SendResponse(&syncbox.Response{
		Status:  syncbox.StatusOK,
		Message: syncbox.MessageAccept,
	}); err != nil {
		server.LogDebug("error on SendResponse in ProcessSync: %v\n", err)
		eHandler(err)
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

	if err := peer.SendResponse(&syncbox.Response{
		Status:  syncbox.StatusOK,
		Message: syncbox.MessageAccept,
	}); err != nil {
		server.LogDebug("error on SendResponse in ProcessFile: %v\n", err)
		eHandler(err)
	}
}

// AddFile implements the Syncer interface
// should send a FileRequest to client to get file content, and save to S3
func (server *Server) AddFile(path string, file *syncbox.File, peer *syncbox.Peer) error {
	duplicate := false
	// firstly add a file record to database, see if there are duplicates
	if err := peer.RefGraph.AddFileRecord(file, path, peer.Device); err != nil {
		duplicate, _ = regexp.MatchString("Error \\d+: Duplicate entry '\\d+' for key 'checksum'", " Error 1062: Duplicate entry '338333539836370388' for key 'checksum'")
		if !duplicate {
			server.LogDebug("error on AddRef in AddFile: %v\n", err)
			return err
		}
	}

	// if no duplicate, send a sync request to client to get file and save to s3
	if !duplicate {
		res, err := peer.SendSyncRequest(syncbox.SyncboxServerUsername, syncbox.SyncboxServerPwd, syncbox.ActionGet, file)
		if err != nil {
			server.LogDebug("error on SendSyncRequest in AddFile: %v\n", err)
			return err
		}
		server.LogDebug("response for SendSyncRequest in AddFile: %v\n", res)
	}

	// no matter whether there are duplicates, it's needed to add a file ref record to database
	if err := peer.RefGraph.AddFileRefRecord(file, path, peer.Device); err != nil {
		return err
	}

	return nil
}

// DeleteFile implements the Syncer interface
// should delete the file ref in database
func (server *Server) DeleteFile(path string, file *syncbox.File, peer *syncbox.Peer) error {
	if err := peer.RefGraph.DeleteFileRefRecord(file, peer.Device, path); err != nil {
		server.LogDebug("error on DeleteRef in DeleteFile: %v\n", err)
		return err
	}

	return nil
}

// AddDir implements the Syncer interface
// should walk through the directory recursively and call AddFile on files
func (server *Server) AddDir(path string, dir *syncbox.Dir, peer *syncbox.Peer) error {
	return walkSubDir(path, dir, peer, server.AddFile)
}

// DeleteDir implements the Syncer interface
// should walk through the directory recursively and call DeleteFile on files
func (server *Server) DeleteDir(path string, dir *syncbox.Dir, peer *syncbox.Peer) error {
	return walkSubDir(path, dir, peer, server.DeleteFile)
}

// GetFile implements the Syncer interface
// noop
func (server *Server) GetFile(path string, file *syncbox.File, peer *syncbox.Peer) error {
	return nil
}

func walkSubDir(path string, dir *syncbox.Dir, peer *syncbox.Peer, manipulator syncbox.FileManipulator) error {
	for _, dir := range dir.Dirs {
		err := walkSubDir(path, dir, peer, manipulator)
		if err != nil {
			return err
		}
	}
	for _, file := range dir.Files {
		err := manipulator(path, file, peer)
		if err != nil {
			return err
		}
	}
	return nil
}
