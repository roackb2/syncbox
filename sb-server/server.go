package main

import (
	"encoding/json"
	"fmt"

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
	server.LogDebug("reborned dir: \n%v\n", dReq.Dir)

	err := server.Storage.CreateBucket(dReq.Username)
	if err != nil {
		server.LogDebug("error on creating bucket: %v\n", err)
		return hub.SendResponse(&syncbox.Response{
			Status:  syncbox.StatusBad,
			Message: syncbox.MessageDeny,
		})
	}

	dirBytes, err := json.Marshal(dReq.Dir)
	if err != nil {
		server.LogError("error marshal dir: %v\n", err)
		return hub.SendResponse(&syncbox.Response{
			Status:  syncbox.StatusBad,
			Message: syncbox.MessageDeny,
		})
	}

	if err := server.Storage.CreateObject(dReq.Username, syncbox.DigestFileName, string(dirBytes)); err != nil {
		server.LogError("error creating object: %v\n", err)
		return hub.SendResponse(&syncbox.Response{
			Status:  syncbox.StatusBad,
			Message: syncbox.MessageDeny,
		})
	}

	if err := hub.SendResponse(&syncbox.Response{
		Status:  syncbox.StatusOK,
		Message: syncbox.MessageAccept,
	}); err != nil {
		server.LogError("error on sending response:%v\n", err)
		return err
	}

	// if err := hub.SendSyncRequest(syncbox., file *syncbox.File)

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
		return err
	}
	return hub.SendResponse(&syncbox.Response{
		Status:  syncbox.StatusOK,
		Message: syncbox.MessageAccept,
	})
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
