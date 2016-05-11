package syncbox

import (
	"bufio"
	"encoding/json"
	"io"
	"net"
)

// Hub is the network reading/writing hub for request/reponse,
// it's the lowest level entry point for network connection
type Hub struct {
	*Logger
	Conn             *net.TCPConn
	InboundRequest   chan []byte
	OutboundRequest  chan []byte
	InboundResponse  chan []byte
	OutboundResponse chan []byte
	ErrorHandler     ErrorHandler
}

// NewHub instantiates a Hub
func NewHub(conn *net.TCPConn, eHandler ErrorHandler) *Hub {
	hub := &Hub{
		Conn:             conn,
		InboundRequest:   make(chan []byte),
		OutboundRequest:  make(chan []byte),
		InboundResponse:  make(chan []byte),
		OutboundResponse: make(chan []byte),
		ErrorHandler:     eHandler,
		Logger:           NewLogger(DefaultAppPrefix, GlobalLogInfo, GlobalLogDebug, GlobalLogDebug),
	}
	hub.Setup()
	return hub
}

// Setup runs up two goroutines to wait for inbound and outbound data
func (hub *Hub) Setup() {
	go hub.waitInbound()
	go hub.waitOutbound()
}

func (hub *Hub) waitInbound() {
	for {
		reader := bufio.NewReader(hub.Conn)
		message, err := reader.ReadBytes(ByteDelim)
		if err != nil {
			if err == io.EOF {
				hub.LogDebug("peer socket closed\n")
				hub.Conn.Close()
				return
			}
			hub.ErrorHandler(err)
			continue
		}
		if len(message) == 0 {
			hub.LogDebug("peer socket closed\n")
			hub.Conn.Close()
			return
		}
		switch message[0] {
		case RequestPrefix:
			hub.InboundRequest <- message[1:len(message)]
		case ResponsePrefix:
			hub.InboundResponse <- message[1:len(message)]
		default:
			hub.ErrorHandler(ErrorUnknownRequestType)
			continue
		}
	}
}

func (hub *Hub) waitOutbound() {
	for {
		select {
		case reqBytes := <-hub.OutboundRequest:
			reqBytes = append(reqBytes, ByteDelim)
			data := append([]byte{byte(RequestPrefix)}, reqBytes...)
			_, err := (*hub.Conn).Write(data)
			if err != nil {
				hub.ErrorHandler(err)
				continue
			}
		case resBytes := <-hub.OutboundResponse:
			resBytes = append(resBytes, ByteDelim)
			data := append([]byte{byte(ResponsePrefix)}, resBytes...)
			_, err := (*hub.Conn).Write(data)
			if err != nil {
				hub.ErrorHandler(err)
				continue
			}
		}
	}
}

// ReceiveRequest blocks until is a inbound request
func (hub *Hub) ReceiveRequest() (*Request, error) {
	bytes := <-hub.InboundRequest
	var req Request
	err := json.Unmarshal(bytes, &req)
	if err != nil {
		hub.LogDebug("error on json Unmarshal in ReceiveRequest: %v\n", err)
		return nil, err
	}
	return &req, nil
}

// ReceiveResponse blocks until there is a inbound response
func (hub *Hub) ReceiveResponse() (*Response, error) {
	bytes := <-hub.InboundResponse
	var res Response
	err := json.Unmarshal(bytes, &res)
	if err != nil {
		hub.LogDebug("error on json Unmarshal in ReceiveResponse: %v\n", err)
		return nil, err
	}
	return &res, nil
}

// SendRequest sends a request to the hub
func (hub *Hub) SendRequest(req *Request) error {
	bytes, err := json.Marshal(req)
	if err != nil {
		hub.LogDebug("error on json Marshal in SendRequest: %v\n", err)
		return err
	}
	hub.OutboundRequest <- bytes
	return nil
}

// SendResponse sends a response to the hub
func (hub *Hub) SendResponse(res *Response) error {
	bytes, err := json.Marshal(res)
	if err != nil {
		hub.LogDebug("error on json Marshal in SendResponse: %v\n", err)
		return err
	}
	hub.OutboundResponse <- bytes
	return nil
}

// SendRequestForResponse sends a request and waits for response
func (hub *Hub) SendRequestForResponse(req *Request) (*Response, error) {
	err := hub.SendRequest(req)
	if err != nil {
		hub.LogDebug("error on SendRequest in SendRequestForResponse: %v\n", err)
		return nil, err
	}
	res, err := hub.ReceiveResponse()
	if err != nil {
		hub.LogDebug("error on ReceiveResponse in SendRequestForResponse: %v\n", err)
		return nil, err
	}
	return res, nil
}

// SendIdentityRequest sends a request with data type of user identity
func (hub *Hub) SendIdentityRequest(username string) (*Response, error) {
	eReq := IdentityRequest{
		Username: username,
	}
	eReqJSON, err := json.Marshal(eReq)
	if err != nil {
		hub.LogDebug("error on json Marshal in SendIdentityRequest: %v\n", err)
		return nil, err
	}
	req := &Request{
		DataType: TypeIdentity,
		Data:     eReqJSON,
	}
	res, err := hub.SendRequestForResponse(req)
	if err != nil {
		hub.LogDebug("error on SendRequestForResponse in SendIdentityRequest: %v\n", err)
		return nil, err
	}
	return res, nil
}

// SendDigestRequest sends a request with data type file tree digest
func (hub *Hub) SendDigestRequest(username string, dir *Dir) (*Response, error) {
	dReq := DigestRequest{
		Username: username,
		Dir:      dir,
	}
	dReqJSON, err := json.Marshal(dReq)
	if err != nil {
		hub.LogDebug("error on json Marshal in SendDigestRequest: %v\n", err)
		return nil, err
	}
	req := &Request{
		DataType: TypeDigest,
		Data:     dReqJSON,
	}
	res, err := hub.SendRequestForResponse(req)
	if err != nil {
		hub.LogDebug("error on SendRequestForResponse in SendDigestRequest: %v\n", err)
		return nil, err
	}
	return res, nil
}

// SendSyncRequest sends a request of data type file operation request
func (hub *Hub) SendSyncRequest(action string, file *File) (*Response, error) {
	sReq := SyncRequest{
		Action: action,
		File:   file,
	}
	sReqJSON, err := json.Marshal(sReq)
	if err != nil {
		hub.LogDebug("error on json Marshal in SendSyncRequest: %v\n", err)
		return nil, err
	}
	req := &Request{
		DataType: TypeSyncRequest,
		Data:     sReqJSON,
	}
	res, err := hub.SendRequestForResponse(req)
	if err != nil {
		hub.LogDebug("error on SendRequestForResponse in SendSyncRequest: %v\n", err)
		return nil, err
	}
	return res, nil
}

// SendFileRequest sends a request of data type of file content
func (hub *Hub) SendFileRequest(file *File, content []byte) (*Response, error) {
	fReq := FileRequest{
		File:    file,
		Content: content,
	}
	fReqJSON, err := json.Marshal(fReq)
	if err != nil {
		hub.LogDebug("error on json Marshal in SendFileRequest: %v\n", err)
		return nil, err
	}
	req := &Request{
		DataType: TypeFile,
		Data:     fReqJSON,
	}
	res, err := hub.SendRequestForResponse(req)
	if err != nil {
		hub.LogDebug("error on SendRequestForResponse in SendFileRequest: %v\n", err)
		return nil, err
	}
	return res, nil
}
