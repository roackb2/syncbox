package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"net"
	"os"
	"path"
	"strings"
	"time"

	"github.com/roackb2/syncbox"
)

// constants about scan
const (
	MaxScanCount = math.MaxInt32
	ScanPeriod   = 2 * time.Second
)

func main() {
	client, err := NewClient()
	if err != nil {
		fmt.Printf("error on new client: %v\n", err)
		return
	}
	err = client.Start()
	if err != nil {
		client.LogError("error on start client: %v\n", err)
	}
}

// Client is the main instance to be run at client program
type Client struct {
	*syncbox.Logger
	*syncbox.Cmd
	*syncbox.ClientConnector
	OldDir  *syncbox.Dir
	NewDir  *syncbox.Dir
	Device  string
	fileOps int
}

// NewClient instantiates a Client
func NewClient() (*Client, error) {
	logger := syncbox.NewDefaultLogger()

	connector, err := syncbox.NewClientConnector()
	if err != nil {
		logger.LogDebug("error on new client connector: %v\n", err)
		return nil, err
	}

	cmd, err := syncbox.ParseCommand()
	if err != nil {
		logger.LogDebug("error on parsing command: %v\n", err)
		return nil, err
	}
	logger.LogInfo("command:\n%v\n", cmd)

	interfaces, err := net.Interfaces()
	if err != nil {
		logger.LogDebug("error on get net interfaces: %v\n", err)
		return nil, err
	}
	macAddr := ""
	for _, inter := range interfaces {
		if inter.Name == "en0" {
			macAddr = inter.HardwareAddr.String()
		}
	}

	return &Client{
		Logger:          logger,
		ClientConnector: connector,
		Cmd:             cmd,
		OldDir:          syncbox.NewEmptyDir(),
		NewDir:          syncbox.NewEmptyDir(),
		Device:          macAddr,
		fileOps:         0,
	}, nil
}

// IncreaseFileOp increase the fileOps variable
func (client *Client) IncreaseFileOp() {
	client.fileOps++
}

// DecreaseFileOp decrease the fileOps variable
func (client *Client) DecreaseFileOp() {
	client.fileOps--
}

// CouldScan examines whether fileOps is zero to determine is there any syncing operations not finished
func (client *Client) CouldScan() bool {
	client.LogDebug("fileOps: %v\n", client.fileOps)
	return client.fileOps == 0
}

// Start runs a client main program
func (client *Client) Start() error {
	if err := os.RemoveAll(client.TmpDir); err != nil && !strings.HasSuffix(err.Error(), "no such file or directory") {
		return err
	}
	if err := os.Mkdir(client.TmpDir, 0777); err != nil && !strings.HasSuffix(err.Error(), "file exists") {
		client.LogDebug("error on creating temp dir: %v\n", err)
		return err
	}
	err := client.Dial(client)
	if err != nil {
		client.LogDebug("error on dial: %v\n", err)
		return err
	}
	for i := 0; i < MaxScanCount; i++ {
		time.Sleep(ScanPeriod)
		if client.CouldScan() {
			if err := client.Scan(); err != nil {
				if err == syncbox.ErrorEmptyContent || err == io.EOF {
					// peer socket is closed
					return syncbox.ErrorPeerSocketClosed
				}
				client.LogError("error on scan: %v\n", err)
				return err
			}
		}
	}
	return nil
}

// HandleRequest implements the ConnectionHandler interface
func (client *Client) HandleRequest(peer *syncbox.Peer) error {
	return syncbox.HandleRequest(peer, client)
}

// HandleError implements the ConnectionHandler interface
func (client *Client) HandleError(err error) {
	client.LogError("error: %v\n", err)
}

// ProcessIdentity implements the ConnectionHandler interface
func (client *Client) ProcessIdentity(req *syncbox.Request, peer *syncbox.Peer, eHandler syncbox.ErrorHandler) {
	data := req.Data
	iReq := syncbox.IdentityRequest{}
	if err := json.Unmarshal(data, &iReq); err != nil {
		client.LogDebug("error on Unmarshal in ProcessIdentity: %v\n", err)
		eHandler(err)
	}
	client.LogDebug("sending response in ProcessIdentity, request id: %v\n", req.ID)
	if err := peer.SendResponse(req, &syncbox.Response{
		Status:  syncbox.StatusOK,
		Message: syncbox.MessageAccept,
	}); err != nil {
		client.LogDebug("error on SendResponse in ProcessIdentity: %v\n", err)
		eHandler(err)
	}
}

// ProcessDigest implements the ConnectionHandler interface
func (client *Client) ProcessDigest(req *syncbox.Request, peer *syncbox.Peer, eHandler syncbox.ErrorHandler) {
	data := req.Data
	dReq := syncbox.DigestRequest{}
	if err := json.Unmarshal(data, &dReq); err != nil {
		client.LogDebug("error on Unmarshal in ProcessDigest: %v\n", err)
		eHandler(err)
	}
	client.LogVerbose("client ProcessDigest called, req: %v\n", dReq)

	clientDir := *(client.NewDir)
	if err := syncbox.Compare(&clientDir, dReq.Dir, client, peer); err != nil {
		client.LogDebug("error on Compare in ProcessDigest: %v\n", err)
		eHandler(err)
	}

	if err := client.cleanTempDir(); err != nil {
		client.LogDebug("error on cleanTempDir in ProcessDigest: %v\n", err)
		eHandler(err)
	}

	// update the client side file tree representation and digest file to newest status,
	// to prevent ping pong sync
	client.OldDir = dReq.Dir
	client.NewDir = dReq.Dir
	if err := client.WriteDigest(); err != nil {
		client.LogError("error on writing digest file in ProcessDigest: %v\n", err)
		eHandler(err)
	}

	client.LogDebug("sending response in ProcessDigest, request id: %v\n", req.ID)
	if err := peer.SendResponse(req, &syncbox.Response{
		Status:  syncbox.StatusOK,
		Message: syncbox.MessageAccept,
	}); err != nil {
		client.LogDebug("error on SendResponse in ProcessDigest: %v\n", err)
		eHandler(err)
	}
}

// ProcessSync implements the ConnectionHandler interface
func (client *Client) ProcessSync(req *syncbox.Request, peer *syncbox.Peer, eHandler syncbox.ErrorHandler) {
	data := req.Data
	sReq := syncbox.SyncRequest{}
	if err := json.Unmarshal(data, &sReq); err != nil {
		client.LogDebug("error on Unmarshal in ProcessSync: %v\n", err)
		eHandler(err)
	}
	// client.LogDebug("client ProcessSync called, req: %v\n", sReq)
	switch sReq.Action {
	case syncbox.ActionGet:
		path := sReq.File.Path
		fileBytes, err := ioutil.ReadFile(path)
		if err != nil {
			client.LogDebug("error reading file: %v\n", err)
			eHandler(err)
		}

		client.LogDebug("sending response in ProcessSync, request id: %v\n", req.ID)
		if err := peer.SendResponse(req, &syncbox.Response{
			Status:  syncbox.StatusOK,
			Message: syncbox.MessageAccept,
		}); err != nil {
			client.LogDebug("error on SendResponse in ProcessSync: %v\n", err)
			eHandler(err)
		}
		// client.LogDebug("before SendFileRequest")
		res, err := peer.SendFileRequest(client.Username, client.Password, client.Device, sReq.UnrootPath, sReq.File, fileBytes)
		if err != nil {
			client.LogDebug("error on SendFileRequest in ProcessSync: %v\n", err)
			eHandler(err)
		}
		client.LogDebug("response of SendFileRequest:\n%v\n", res)
	}
}

// ProcessFile implements the ConnectionHandler interface
func (client *Client) ProcessFile(req *syncbox.Request, peer *syncbox.Peer, eHandler syncbox.ErrorHandler) {
	data := req.Data
	dReq := syncbox.FileRequest{}
	if err := json.Unmarshal(data, &dReq); err != nil {
		client.LogDebug("error on Unmarshal in ProcessFile: %v\n", err)
		eHandler(err)
	}

	// server.LogDebug("filename: %v\ncontent: %v\n", filename, content)
	filePath := client.rebornPath(dReq.UnrootPath)
	client.LogVerbose("path in ProcessFile: %v\n", filePath)
	if err := ioutil.WriteFile(filePath, dReq.Content, dReq.File.Mode); err != nil {
		client.LogDebug("error on CreateObject in ProcessFile: %v\n", err)
		eHandler(err)
	}
	client.DecreaseFileOp()

	client.LogDebug("sending response in ProcessFile, request id: %v\n", req.ID)
	if err := peer.SendResponse(req, &syncbox.Response{
		Status:  syncbox.StatusOK,
		Message: syncbox.MessageAccept,
	}); err != nil {
		client.LogDebug("error on SendResponse in ProcessFile: %v\n", err)
		eHandler(err)
	}
}

// AddFile implements the Syncer interface
func (client *Client) AddFile(rootPath string, unrootPath string, file *syncbox.File, peer *syncbox.Peer) error {
	client.LogVerbose("AddFile, rootPath: %v, unrootPath: %v, file path: %v", rootPath, unrootPath, file.Path)
	client.IncreaseFileOp()
	hasTempFile := true
	fileOriginPath := client.rebornPath(unrootPath)
	fileTmpPath := path.Join(client.TmpDir, syncbox.ChecksumToNumString(file.ContentChecksum))
	if err := os.Rename(fileTmpPath, fileOriginPath); err != nil {
		if strings.HasSuffix(err.Error(), "no such file or directory") {
			hasTempFile = false
		} else {
			client.LogDebug("error on Rename in AddFile: %v\n", err)
			return err
		}
	}
	if hasTempFile {
		client.DecreaseFileOp()
	} else {
		res, err := peer.SendSyncRequest(client.Username, client.Password, client.Device, unrootPath, syncbox.ActionGet, file)
		if err != nil {
			client.LogDebug("error on SendSyncRequest in AddFile: %v\n", err)
			return err
		}
		client.LogDebug("response of SendSyncRequest in AddFile:\n%v\n", res)
	}
	return nil
}

// DeleteFile implements the Syncer interface
func (client *Client) DeleteFile(rootPath string, unrootPath string, file *syncbox.File, peer *syncbox.Peer) error {
	client.LogVerbose("DeleteFile, rootPath: %v, unrootPath: %v, file path: %v", rootPath, unrootPath, file.Path)
	client.IncreaseFileOp()
	fileOriginPath := client.rebornPath(unrootPath)
	fileTmpPath := path.Join(client.TmpDir, syncbox.ChecksumToNumString(file.ContentChecksum))
	client.LogVerbose("fileOriginPath in DeleteFile: %v\n", fileOriginPath)
	if err := os.Rename(fileOriginPath, fileTmpPath); err != nil && err != os.ErrExist {
		return err
	}
	client.DecreaseFileOp()
	return nil
}

// AddDir implements the Syncer interface
func (client *Client) AddDir(rootPath string, unrootPath string, dir *syncbox.Dir, peer *syncbox.Peer) error {
	if err := os.Mkdir(client.rebornPath(unrootPath), dir.Mode); err != nil {
		return err
	}
	return syncbox.WalkSubDir(rootPath, dir, peer, client.AddFile, client.AddDir)
}

// DeleteDir implements the Syncer interface
func (client *Client) DeleteDir(rootPath string, unrootPath string, dir *syncbox.Dir, peer *syncbox.Peer) error {
	if err := syncbox.WalkSubDir(rootPath, dir, peer, client.DeleteFile, client.DeleteDir); err != nil {
		return err
	}
	return os.RemoveAll(client.rebornPath(unrootPath))
}

// Scan through the target, write digest file on disk and send to server
func (client *Client) Scan() error {
	hasOldDigest := true

	oldDirBytes, err := ioutil.ReadFile(client.RootDir + "/" + syncbox.DigestFileName)
	if err != nil {
		if strings.HasSuffix(err.Error(), "no such file or directory") {
			hasOldDigest = false
		} else {
			client.LogError("error open digest file: %v\n", err)
			return err
		}
	}

	if hasOldDigest {
		if err = json.Unmarshal(oldDirBytes, client.OldDir); err != nil {
			client.LogError("error on Unmarshal old dir: %v\n", err)
			return err
		}
	}

	client.NewDir, _, err = syncbox.Build(client.RootDir)
	if err != nil {
		client.LogError("error on scanning: %v\n", err)
		return err
	}
	// client.LogDebug("new dir:\n%v\n", client.NewDir)
	client.LogInfo("scanning files\nold dir checksum: %v\nnew dir checksum: %v\n", client.OldDir.ContentChecksum, client.NewDir.ContentChecksum)

	if hasOldDigest && client.OldDir.ContentChecksum == client.NewDir.ContentChecksum {
		// nothing else need to do
		return nil
	}

	if err := client.WriteDigest(); err != nil {
		client.LogError("error on writing digest file: %v\n", err)
		return err
	}

	// client.LogInfo("sending digest request to server")
	res, err := client.ClientConnector.Peer.SendDigestRequest(client.Username, client.Password, client.Device, client.NewDir)
	if err != nil {
		client.LogError("error on send: %v\n", err)
		return err
	}

	client.LogInfo("response of SendDigestRequest:\n%v\n", res)
	return nil
}

// WriteDigest writes digest file to the watching directory
func (client *Client) WriteDigest() error {
	jsonStr, err := client.NewDir.ToJSON()
	if err != nil {
		client.LogDebug("error on json formatting: %v\n", err)
		return err
	}

	err = ioutil.WriteFile(client.RootDir+"/"+syncbox.DigestFileName, []byte(jsonStr), 0644)
	if err != nil {
		client.LogDebug("error on writing digest file: %v\n", err)
		return err
	}
	return nil
}

func (client *Client) rebornPath(unrootPath string) string {
	return client.RootDir + unrootPath
}

func (client *Client) cleanTempDir() error {
	if err := os.RemoveAll(client.TmpDir); err != nil {
		return err
	}
	if err := os.Mkdir(client.TmpDir, 0777); err != nil && err != os.ErrExist {
		return err
	}
	return nil
}
