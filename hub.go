package syncbox

import (
	"bytes"
	"encoding/json"
	"errors"
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

func (hub *Hub) writePackets(bytes []byte) error {
	packets, err := Serialize(bytes)
	if err != nil {
		return err
	}
	for _, packet := range packets {
		data := packet.ToBytes()
		dataSlice := make([]byte, PacketTotalSize)
		copy(dataSlice, data[:])
		_, err := (*hub.Conn).Write(dataSlice)
		if err != nil {
			hub.LogDebug("error on writePackets: %v\n", err)
			return err
		}
	}
	return nil
}

func (hub *Hub) readPackets() ([]byte, error) {
	var packets []Packet
	for {
		buffer := make([]byte, PacketTotalSize)
		_, err := (*hub.Conn).Read(buffer)
		if err != nil {
			hub.LogDebug("error on readPackets: %v\n", err)
			return nil, err
		}
		var bufferArr [PacketTotalSize]byte
		copy(bufferArr[:], buffer)
		packet := RebornPacket(bufferArr)
		packets = append(packets, *packet)
		size, err := packet.GetSize()
		if err != nil {
			return nil, err
		}
		sequence, err := packet.GetSequence()
		if err != nil {
			return nil, err
		}
		if sequence >= size-1 {
			break
		}
	}
	data := Deserialize(packets)
	data = bytes.Trim(data, string([]byte{0})) // trim trailing zero char in last packet
	return data, nil
}

func (hub *Hub) waitInbound() {
	for {
		message, err := hub.readPackets()
		if len(message) > 0 {
			message = message[0 : len(message)-1]
		}
		if err != nil {
			if err == io.EOF {
				hub.LogDebug("peer socket closed\n")
				hub.Conn.Close()
				return
			}
			hub.LogDebug("error in waitInbound: %v\n", err)
			hub.ErrorHandler(err)
			continue
		}
		if len(message) == 0 {
			hub.LogDebug("peer socket closed\n")
			hub.Conn.Close()
			return
		}
		prefix := message[0]
		message = message[1:len(message)]
		switch prefix {
		case RequestPrefix:
			// hub.LogDebug("inbound request message: %v\n", string(message))
			hub.InboundRequest <- message
		case ResponsePrefix:
			// hub.LogDebug("inbound response message: %v\n", string(message))
			hub.InboundResponse <- message
		default:
			hub.ErrorHandler(errors.New("unknown message type: " + string(prefix)))
			continue
		}
	}
}

func (hub *Hub) waitOutbound() {
	for {
		select {
		case message := <-hub.OutboundRequest:
			// hub.LogDebug("outbound request message: %v\n", string(message))
			message = append(message, ByteDelim)                      // appends delim
			message = append([]byte{byte(RequestPrefix)}, message...) // unshift request prefix
			err := hub.writePackets(message)
			if err != nil {
				hub.LogDebug("error on writePackets in waitOutbound: %v\n", err)
				hub.ErrorHandler(err)
			}
		case message := <-hub.OutboundResponse:
			// hub.LogDebug("outbound response message: %v\n", string(message))
			message = append(message, ByteDelim)                       // append delim
			message = append([]byte{byte(ResponsePrefix)}, message...) // unshift response prefix
			err := hub.writePackets(message)
			if err != nil {
				hub.LogDebug("error on writePackets in waitOutbound: %v\n", err)
				hub.ErrorHandler(err)
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
	// hub.LogDebug("beofore ReceiveResponse in SendRequestForResponse")
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
	// hub.LogDebug("SendIdentityRequest called, req: %v\n", req)
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
		Dir: dir,
	}
	dReqJSON, err := json.Marshal(dReq)
	if err != nil {
		hub.LogDebug("error on json Marshal in SendDigestRequest: %v\n", err)
		return nil, err
	}
	req := &Request{
		Username: username,
		DataType: TypeDigest,
		Data:     dReqJSON,
	}
	// hub.LogDebug("SendDigestRequest called, req: %v\n", req)
	res, err := hub.SendRequestForResponse(req)
	if err != nil {
		hub.LogDebug("error on SendRequestForResponse in SendDigestRequest: %v\n", err)
		return nil, err
	}
	return res, nil
}

// SendSyncRequest sends a request of data type file operation request
func (hub *Hub) SendSyncRequest(username string, action string, file *File) (*Response, error) {
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
		Username: username,
		DataType: TypeSyncRequest,
		Data:     sReqJSON,
	}
	// hub.LogDebug("SendSyncRequest called, req: %v\n", req)
	res, err := hub.SendRequestForResponse(req)
	if err != nil {
		hub.LogDebug("error on SendRequestForResponse in SendSyncRequest: %v\n", err)
		return nil, err
	}
	return res, nil
}

// SendFileRequest sends a request of data type of file content
func (hub *Hub) SendFileRequest(username string, file *File, content []byte) (*Response, error) {
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
		Username: username,
		DataType: TypeFile,
		Data:     fReqJSON,
	}
	// hub.LogDebug("SendFileRequest called, req: %v\n", req)
	res, err := hub.SendRequestForResponse(req)
	if err != nil {
		hub.LogDebug("error on SendRequestForResponse in SendFileRequest: %v\n", err)
		return nil, err
	}
	return res, nil
}
