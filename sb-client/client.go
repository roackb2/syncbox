package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"strings"
	"time"
	// "time"

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
	OldDir *syncbox.Dir
	NewDir *syncbox.Dir
}

// NewClient instantiates a Client
func NewClient() (*Client, error) {
	logger := syncbox.NewLogger(syncbox.DefaultAppPrefix, syncbox.GlobalLogInfo, syncbox.GlobalLogDebug, syncbox.GlobalLogDebug)

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

	return &Client{
		Logger:          logger,
		ClientConnector: connector,
		Cmd:             cmd,
		OldDir: &syncbox.Dir{
			Object: &syncbox.Object{
				ContentChecksum: syncbox.Checksum([16]byte{}),
			},
		},
		NewDir: &syncbox.Dir{
			Object: &syncbox.Object{
				ContentChecksum: syncbox.Checksum([16]byte{}),
			},
		},
	}, nil
}

// Start runs a client main program
func (client *Client) Start() error {
	err := client.Dial(client)
	if err != nil {
		client.LogDebug("error on dial: %v\n", err)
		return err
	}
	for i := 0; i < MaxScanCount; i++ {
		time.Sleep(ScanPeriod)
		if err := client.Scan(); err != nil {
			if err == syncbox.ErrorEmptyContent || err == io.EOF {
				// peer socket is closed
				return syncbox.ErrorPeerSocketClosed
			}
			client.LogError("error on scan: %v\n", err)
			return err
		}
	}
	return nil
}

// HandleRequest implements the ConnectionHandler interface
func (client *Client) HandleRequest(hub *syncbox.Hub) error {
	return syncbox.HandleRequest(hub, client)
}

// HandleError implements the ConnectionHandler interface
func (client *Client) HandleError(err error) {
	client.LogError("error: %v\n", err)
}

// ProcessIdentity implements the ConnectionHandler interface
func (client *Client) ProcessIdentity(req *syncbox.Request, hub *syncbox.Hub) error {
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
func (client *Client) ProcessDigest(req *syncbox.Request, hub *syncbox.Hub) error {
	data := req.Data
	dReq := syncbox.DigestRequest{}
	if err := json.Unmarshal(data, &dReq); err != nil {
		return err
	}
	return hub.SendResponse(&syncbox.Response{
		Status:  syncbox.StatusOK,
		Message: syncbox.MessageAccept,
	})
}

// ProcessSync implements the ConnectionHandler interface
func (client *Client) ProcessSync(req *syncbox.Request, hub *syncbox.Hub) error {
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
func (client *Client) ProcessFile(req *syncbox.Request, hub *syncbox.Hub) error {
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
	client.LogInfo("scanning files\nold dir checksum: %v\nnew dir checksum: %v\n", client.OldDir.ContentChecksum, client.NewDir.ContentChecksum)

	if hasOldDigest && client.OldDir.ContentChecksum == client.NewDir.ContentChecksum {
		// nothing else need to do
		return nil
	}

	if err := client.WriteDigest(); err != nil {
		client.LogError("error on writing digest file: %v\n", err)
		return err
	}

	client.LogInfo("sending digest request to server")
	res, err := client.ClientConnector.Hub.SendDigestRequest(client.Username, client.NewDir)
	if err != nil {
		client.LogError("error on send: %v\n", err)
		return err
	}

	client.LogInfo("response: \n%v\n", res)
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
