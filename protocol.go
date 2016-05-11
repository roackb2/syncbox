package syncbox

import (
	"encoding/json"
	"fmt"
)

// constants for protocol
const (
	RequestPrefix  = 'q'
	ResponsePrefix = 's'

	ByteDelim   = '\n'
	StringDelim = "\n"

	TypeIdentity    = "IDENTITY"
	TypeDigest      = "DIGEST"
	TypeSyncRequest = "SYNC-REQUEST"
	TypeFile        = "FILE"

	StatusOK  = 200
	StatusBad = 400

	MessageAccept = "ACCEPT"
	MessageDeny   = "DENY"
)

// Request structure for request
type Request struct {
	DataType string
	Data     []byte
}

// Response structure for response
type Response struct {
	Status  int
	Message string
	Data    []byte
}

// IdentityRequest is the Request data type of user identity
type IdentityRequest struct {
	Username string
}

// DigestRequest is the Request data type of a file tree digest
type DigestRequest struct {
	Username string
	Dir      *Dir
}

// SyncRequest is the Request data type of a file CRUD request
type SyncRequest struct {
	Action string
	File   *File
}

// FileRequest is the Request data type of CRUD on file content
type FileRequest struct {
	File    *File
	Content []byte
}

func (req *Request) String() string {
	str := fmt.Sprintf("DataType: %v\n", req.DataType)
	str += fmt.Sprintf("Data: %v\n", req.Data)
	return str
}

func (res *Response) String() string {
	str := fmt.Sprintf("Status: %v\n", res.Status)
	str += fmt.Sprintf("Message: %v\n", res.Message)
	str += fmt.Sprintf("Data: %v\n", res.Data)
	return str
}

// ToJSON converts request to JSON string
func (req *Request) ToJSON() (string, error) {
	jsonBytes, err := json.Marshal(req)
	if err != nil {
		return "", err
	}
	return string(jsonBytes), nil
}

// RebornRequest reborn request from JSON string
func RebornRequest(jsonStr string) (*Request, error) {
	jsonBytes := []byte(jsonStr)
	restoredReq := Request{}
	if err := json.Unmarshal(jsonBytes, &restoredReq); err != nil {
		return nil, err
	}
	return &restoredReq, nil
}

// ToJSON converts response to JSON string
func (res *Response) ToJSON() (string, error) {
	jsonBytes, err := json.Marshal(res)
	if err != nil {
		return "", err
	}
	return string(jsonBytes), nil
}

// RebornResponse reborn response from JSON string
func RebornResponse(jsonStr string) (*Response, error) {
	jsonBytes := []byte(jsonStr)
	restoredRes := Response{}
	if err := json.Unmarshal(jsonBytes, &restoredRes); err != nil {
		return nil, err
	}
	return &restoredRes, nil
}
