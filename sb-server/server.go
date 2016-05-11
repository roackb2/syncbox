package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

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
	logger := syncbox.NewLogger(syncbox.DefaultAppPrefix, syncbox.GlobalLogInfo, syncbox.GlobalLogDebug, syncbox.GlobalLogDebug)
	db, err := syncbox.NewDB(&syncbox.UserTable{}, &syncbox.FileTable{}, &syncbox.FileRefTable{})
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
func (server *Server) HandleRequest(hub *syncbox.Hub) error {
	return syncbox.HandleRequest(hub, server)
}

// HandleError implements the ConnectionHandler interface
func (server *Server) HandleError(err error) {
	server.LogError("error: %v\n", err)
}

// ProcessIdentity implements the ConnectionHandler interface
func (server *Server) ProcessIdentity(req *syncbox.Request, hub *syncbox.Hub) error {
	data := req.Data
	iReq := syncbox.IdentityRequest{}
	if err := json.Unmarshal(data, &iReq); err != nil {
		return err
	}
	return hub.SendResponse(&syncbox.Response{
		Status:  syncbox.StatusOK,
		Message: syncbox.MessageAccept,
	})
}

// ProcessDigest implements the ConnectionHandler interface
func (server *Server) ProcessDigest(req *syncbox.Request, hub *syncbox.Hub) error {
	data := req.Data
	dReq := syncbox.DigestRequest{}
	if err := json.Unmarshal(data, &dReq); err != nil {
		return err
	}
	// server.LogDebug("reborned dir: \n%v\n", dReq.Dir)

	// create a bucket for the user, if not exists
	err := server.Storage.CreateBucket(req.Username)
	if err != nil {
		server.LogDebug("error on creating bucket: %v\n", err)
		return err
		// return hub.SendResponse(&syncbox.Response{
		// 	Status:  syncbox.StatusBad,
		// 	Message: syncbox.MessageDeny,
		// })
	}

	// reborn the directory of the request
	dirBytes, err := json.Marshal(dReq.Dir)
	if err != nil {
		server.LogDebug("error marshal dir: %v\n", err)
		return err
		// return hub.SendResponse(&syncbox.Response{
		// 	Status:  syncbox.StatusBad,
		// 	Message: syncbox.MessageDeny,
		// })
	}
	// server.LogDebug("dir bytes: %v\n", dirBytes)

	// get the server side digest file for the user
	oldDigestBytes, err := server.Storage.GetObject(req.Username, syncbox.DigestFileName)
	if err != nil {
		server.LogDebug("error on GetObject in ProcessDigest: %v\n", err)
		return err
		// return hub.SendResponse(&syncbox.Response{
		// 	Status:  syncbox.StatusBad,
		// 	Message: syncbox.MessageDeny,
		// })
	}
	// server.LogDebug("old digest bytes: %v\n", oldDigestBytes)

	// reborn old directory from digest file
	oldDir := &syncbox.Dir{}
	if err := json.Unmarshal(oldDigestBytes, oldDir); err != nil {
		server.LogDebug("error on Unmarshal in ProcessDigest: %v\n", err)
		return err
		// return hub.SendResponse(&syncbox.Response{
		// 	Status:  syncbox.StatusBad,
		// 	Message: syncbox.MessageDeny,
		// })
	}
	// server.LogDebug("old dir: %v\n", oldDir)

	// send response to user before Compare
	if err := hub.SendResponse(&syncbox.Response{
		Status:  syncbox.StatusOK,
		Message: syncbox.MessageAccept,
	}); err != nil {
		server.LogError("error on sending response:%v\n", err)
		return err
	}

	time.Sleep(time.Second)

	// compare the server side and client side directory, and sync them
	if err := syncbox.Compare("", oldDir, dReq.Dir, server, hub); err != nil {
		server.LogDebug("error on Compare in ProcessDigest: %v\n", err)
		return err
		// return hub.SendResponse(&syncbox.Response{
		// 	Status:  syncbox.StatusBad,
		// 	Message: syncbox.MessageDeny,
		// })
	}
	// server.LogDebug("finish Compare")

	// put the digest file to S3
	if err := server.Storage.CreateObject(req.Username, syncbox.DigestFileName, string(dirBytes)); err != nil {
		server.LogError("error creating object: %v\n", err)
		return hub.SendResponse(&syncbox.Response{
			Status:  syncbox.StatusBad,
			Message: syncbox.MessageDeny,
		})
	}

	return nil
}

// ProcessSync implements the ConnectionHandler interface
func (server *Server) ProcessSync(req *syncbox.Request, hub *syncbox.Hub) error {
	data := req.Data
	sReq := syncbox.SyncRequest{}
	if err := json.Unmarshal(data, &sReq); err != nil {
		return err
	}
	return hub.SendResponse(&syncbox.Response{
		Status:  syncbox.StatusOK,
		Message: syncbox.MessageAccept,
	})
}

// ProcessFile implements the ConnectionHandler interface
func (server *Server) ProcessFile(req *syncbox.Request, hub *syncbox.Hub) error {
	data := req.Data
	dReq := syncbox.FileRequest{}
	if err := json.Unmarshal(data, &dReq); err != nil {
		server.LogDebug("error Unmarshal data in ProcessFile: %v\n", err)
		return err
	}

	filenmae := strconv.FormatInt(syncbox.ReadInt64(dReq.File.ContentChecksum[:]), 10)
	content := string(dReq.Content)
	if err := server.Storage.CreateObject(req.Username, filenmae, content); err != nil {
		server.LogDebug("error on CreateObject in ProcessFile: %v\n", err)
		return err
	}

	return hub.SendResponse(&syncbox.Response{
		Status:  syncbox.StatusOK,
		Message: syncbox.MessageAccept,
	})
}

// AddFile implements the Syncer interface
func (server *Server) AddFile(path string, file *syncbox.File, hub *syncbox.Hub) error {
	// TODO: should send a FileRequest to client to get file content, and save to S3
	res, err := hub.SendSyncRequest(syncbox.SyncboxServerUsernam, syncbox.ActionGet, file)
	if err != nil {
		server.LogDebug("error on SendSyncRequest in AddFile: %v\n", err)
		return err
	}
	server.LogDebug("response for SendSyncRequest in AddFile: %v\n", res)

	return nil
}

// DeleteFile implements the Syncer interface
func (server *Server) DeleteFile(path string, file *syncbox.File, hub *syncbox.Hub) error {
	// TODO: should delete the file ref in database, and delete file on S3 if no file refs on that file
	return nil
}

// AddDir implements the Syncer interface
func (server *Server) AddDir(path string, file *syncbox.Dir, hub *syncbox.Hub) error {
	// TODO: should walk through the directory recursively and call AddFile on files
	return nil
}

// DeleteDir implements the Syncer interface
func (server *Server) DeleteDir(path string, file *syncbox.Dir, hub *syncbox.Hub) error {
	// TODO: should wak through the directory recursively and call DeleteFile on files
	return nil
}

// GetFile implements the Syncer interface
func (server *Server) GetFile(path string, file *syncbox.File, hub *syncbox.Hub) error {
	// noop
	return nil
}

func walkSubDir(path string, dir *syncbox.Dir, manipulator syncbox.FileManipulator) error {
	for _, dir := range dir.Dirs {
		err := walkSubDir(path, dir, manipulator)
		if err != nil {
			return err
		}
	}
	for _, file := range dir.Files {
		err := manipulator(path, file)
		if err != nil {
			return err
		}
	}
	return nil
}
