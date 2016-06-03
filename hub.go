package syncbox

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net"
	"strings"
	"time"
)

// constants about connection retry
const (
	SendMessageRestPeriod = 2 * time.Second
	SendMessageMaxRetry   = 10
)

// Hub is the network reading/writing hub for request/reponse,
// it's the lowest level entry point for network connection
type Hub struct {
	*Logger
	Conn                   *net.TCPConn
	InboundRequest         chan []byte
	OutboundRequest        chan []byte
	InboundResponse        chan []byte
	OutboundResponse       chan []byte
	InboundRequestError    chan error
	InboundResponseError   chan error
	OutboundRequestFinish  chan bool
	OutboundRequestError   chan error
	OutboundResponseFinish chan bool
	OutboundResponseError  chan error
	ErrorHandler           ErrorHandler
}

// NewHub instantiates a Hub
func NewHub(conn *net.TCPConn, eHandler ErrorHandler) *Hub {
	hub := &Hub{
		Conn:                   conn,
		InboundRequest:         make(chan []byte),
		OutboundRequest:        make(chan []byte),
		InboundResponse:        make(chan []byte),
		OutboundResponse:       make(chan []byte),
		InboundRequestError:    make(chan error),
		InboundResponseError:   make(chan error),
		OutboundRequestFinish:  make(chan bool),
		OutboundRequestError:   make(chan error),
		OutboundResponseFinish: make(chan bool),
		OutboundResponseError:  make(chan error),
		ErrorHandler:           eHandler,
		Logger:                 NewDefaultLogger(),
	}
	return hub
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
	var packets []*Packet
	lastProgress := 0
	for {
		buffer := make([]byte, PacketTotalSize, PacketTotalSize)
		readSize := 0
		// one read perhaps won't read the full packet, loop until a full packet is read
		for readSize < PacketTotalSize {
			n, err := (*hub.Conn).Read(buffer[readSize:len(buffer)])
			if err != nil {
				hub.LogDebug("error on readPackets: %v\n", err)
				return nil, err
			}
			readSize += n
		}
		var bufferArr [PacketTotalSize]byte
		copy(bufferArr[:], buffer)
		packet := RebornPacket(bufferArr)
		packets = append(packets, packet)
		size, err := packet.GetSize()
		if err != nil {
			return nil, err
		}
		sequence, err := packet.GetSequence()
		if err != nil {
			return nil, err
		}
		progress := int(math.Floor(float64(sequence) / float64(size) * 100))
		if size > 10000 && (progress%10 == 0) && progress != lastProgress {
			hub.LogInfo("progress reading inbound message: %v%%\n", progress)
			lastProgress = progress
		}
		if sequence >= size-1 {
			break
		}
	}
	data := Deserialize(packets)
	data = bytes.TrimRight(data, string([]byte{0})) // trim trailing zero char in last packet
	return data, nil
}

// WaitInbound waits for inbound message and dispatch to InboundRequest or InboundResponse channel accordingly,
// this should be run as goroutine/
// It returns error if an error is considered as connection level, such as EOF or unknonw message type,
// and leave for the connectors to deal with error,
// otherwise it sends the error to InboundRequestError or InboundResponseError channel accordingly.
func (hub *Hub) WaitInbound() error {
	for {
		message, err := hub.readPackets()
		if len(message) > 0 {
			message = message[0 : len(message)-1]
		}
		if err != nil {
			if err == io.EOF {
				hub.LogDebug("peer socket closed\n")
				// hub.Conn.Close()
				return ErrorPeerSocketClosed
			}
			hub.LogVerbose("error in waitInbound: %v\n", err)
			// hub.ErrorHandler(err)
			// continue
		}
		if len(message) == 0 {
			hub.LogDebug("peer socket closed\n")
			// hub.Conn.Close()
			return ErrorPeerSocketClosed
		}
		prefix := message[0]
		if err != nil {
			switch prefix {
			case RequestPrefix:
				hub.LogVerbose("inbound request error: %v\n", err)
				hub.InboundRequestError <- err
			case ResponsePrefix:
				hub.LogVerbose("inbound response error: %v\n", err)
				hub.InboundResponseError <- err
			default:
				// hub.ErrorHandler(errors.New("unknown message type: " + string(prefix)))
				return errors.New("unknown message type: " + string(prefix))
			}
			continue
		}
		message = message[1:len(message)]
		switch prefix {
		case RequestPrefix:
			hub.LogVerbose("inbound request message: %v\n", string(message))
			hub.InboundRequest <- message
		case ResponsePrefix:
			hub.LogVerbose("inbound response message: %v\n", string(message))
			hub.InboundResponse <- message
		default:
			// hub.ErrorHandler(errors.New("unknown message type: " + string(prefix)))
			// continue
			return errors.New("unknown message type: " + string(prefix))
		}
	}
}

// WaitOutbound waits on the OutboundRequest and OutboundResponse channel and send message to the connection.
// this Should be run as goroutine.
// It returns error if an error is considered as connection level, such as EOF or unknonw message type,
// and leave for the connectors to deal with error,
// otherwise it sends the error to OutboundRequestError or OutboundResponseError channel accordingly.
func (hub *Hub) WaitOutbound() error {
	for {
		select {
		case message := <-hub.OutboundRequest:
			hub.LogVerbose("outbound request message: %v\n", string(message))
			message = append(message, ByteDelim)                      // appends delim
			message = append([]byte{byte(RequestPrefix)}, message...) // unshift request prefix
			err := hub.writePackets(message)
			if err != nil {
				hub.LogDebug("error on writePackets in waitOutbound: %v\n", err)
				hub.OutboundRequestError <- err
				// hub.ErrorHandler(err)
			} else {
				hub.OutboundRequestFinish <- true
			}
		case message := <-hub.OutboundResponse:
			hub.LogVerbose("outbound response message: %v\n", string(message))
			message = append(message, ByteDelim)                       // append delim
			message = append([]byte{byte(ResponsePrefix)}, message...) // unshift response prefix
			err := hub.writePackets(message)
			if err != nil {
				hub.LogDebug("error on writePackets in waitOutbound: %v\n", err)
				hub.OutboundResponseError <- err
				// hub.ErrorHandler(err)
			} else {
				hub.OutboundResponseFinish <- true
			}
		}
	}
}

// ReceiveRequest blocks until there is a inbound request
func (hub *Hub) ReceiveRequest() (*Request, error) {
	select {
	case bytes := <-hub.InboundRequest:
		var req Request
		err := json.Unmarshal(bytes, &req)
		if err != nil {
			hub.LogDebug("error on json Unmarshal in ReceiveRequest: %v\n", err)
			return nil, err
		}
		return &req, nil
	case err := <-hub.InboundRequestError:
		return nil, err
	}

}

// ReceiveResponse blocks until there is a inbound response,
func (hub *Hub) ReceiveResponse() (*Response, error) {
	select {
	case bytes := <-hub.InboundResponse:
		var res Response
		err := json.Unmarshal(bytes, &res)
		if err != nil {
			hub.LogDebug("error on json Unmarshal in ReceiveResponse: %v\n", err)
			return nil, err
		}
		return &res, nil
	case err := <-hub.InboundResponseError:
		return nil, err
	}

}

func (hub *Hub) sendRequest(req *Request) error {
	bytes, err := json.Marshal(req)
	if err != nil {
		hub.LogDebug("error on json Marshal in SendRequest: %v\n", err)
		return err
	}
	hub.OutboundRequest <- bytes
	select {
	case <-hub.OutboundRequestFinish:
		return nil
	case err := <-hub.OutboundRequestError:
		return err
	}
}

func (hub *Hub) sendResponse(req *Request, res *Response) error {
	res.RequestID = req.ID
	bytes, err := json.Marshal(res)
	if err != nil {
		hub.LogDebug("error on json Marshal in SendResponse: %v\n", err)
		return err
	}
	hub.OutboundResponse <- bytes
	select {
	case <-hub.OutboundResponseFinish:
		return nil
	case err := <-hub.OutboundResponseError:
		return err
	}
}

// SendRequest sends a request to the hub,
// it retries finite number if there is error sending request
// and try to reconnect if connection is broken
func (hub *Hub) SendRequest(handler ConnectionHandler, req *Request) error {
	var err error
	for i := 0; i < SendMessageMaxRetry; i++ {
		err = hub.sendRequest(req)
		if err != nil {
			hub.LogDebug("error SendRequest,\n request id: %v,\n error: %v,\n retry count: %v\n ", req.ID, err, i)
			if err == ErrorPeerSocketClosed || strings.HasSuffix(err.Error(), "use of closed network connection") {
				if dialErr := handler.Dial(handler, hub.Conn.RemoteAddr().(*net.TCPAddr)); dialErr != nil {
					hub.LogDebug("error on retry Dial in SendRequest: %v\n", dialErr)
					time.Sleep(SendMessageRestPeriod)
				}
			} else {
				time.Sleep(SendMessageRestPeriod)
			}
		} else {
			break
		}
	}
	return err
}

// SendResponse sends a response to the hub
// it retries finite number if there is error sending response,
// and try to reconnect if connection is broken
func (hub *Hub) SendResponse(handler ConnectionHandler, req *Request, res *Response) error {
	var err error
	for i := 0; i < SendMessageMaxRetry; i++ {
		err = hub.sendResponse(req, res)
		if err != nil {
			hub.LogDebug("error SendResponse,\n request id: %v,\n error: %v,\n retry count: %v\n ", req.ID, err, i)
			if err == ErrorPeerSocketClosed || strings.HasSuffix(err.Error(), "use of closed network connection") {
				hub.LogDebug("connection closed, trying to dial\n")
				if dialErr := handler.Dial(handler, hub.Conn.RemoteAddr().(*net.TCPAddr)); dialErr != nil {
					hub.LogDebug("error on retry Dial in SendResponse: %v\n", dialErr)
					time.Sleep(SendMessageRestPeriod)
				}
			} else {
				time.Sleep(SendMessageRestPeriod)
			}
		} else {
			break
		}
	}
	return err
}

// SendRequestForResponse sends a request and waits for response,
func (hub *Hub) SendRequestForResponse(handler ConnectionHandler, req *Request) (*Response, error) {
	err := hub.SendRequest(handler, req)
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
func (hub *Hub) SendIdentityRequest(handler ConnectionHandler, username string, password string, device string) (*Response, error) {
	eReq := IdentityRequest{
		Username: username,
	}
	eReqJSON, err := json.Marshal(eReq)
	if err != nil {
		hub.LogDebug("error on json Marshal in SendIdentityRequest: %v\n", err)
		return nil, err
	}
	req := NewRequest(username, password, device, TypeIdentity, eReqJSON)
	hub.LogDebug("SendIdentityRequest called,\n request id: %v,\n username: %v, password: %v, device: %v\n", req.ID, username, password, device)

	res, err := hub.SendRequestForResponse(handler, req)
	if err != nil {
		hub.LogDebug("error on SendRequestForResponse in SendIdentityRequest: %v\n", err)
		return nil, err
	}
	return res, nil
}

// SendDigestRequest sends a request with data type file tree digest
func (hub *Hub) SendDigestRequest(handler ConnectionHandler, username string, password string, device string, dir *Dir) (*Response, error) {
	dReq := DigestRequest{
		Dir: dir,
	}
	dReqJSON, err := json.Marshal(dReq)
	if err != nil {
		hub.LogDebug("error on json Marshal in SendDigestRequest:%v\n", err)
		return nil, err
	}
	req := NewRequest(username, password, device, TypeDigest, dReqJSON)
	hub.LogDebug("SendDigestRequest called,\n request id: %v,\n username: %v, password: %v, device: %v,\n dir checksum: %v\n", req.ID, username, password, device, dir.ContentChecksum)

	res, err := hub.SendRequestForResponse(handler, req)
	if err != nil {
		hub.LogDebug("error on SendRequestForResponse in SendDigestRequest: %v\n", err)
		return nil, err
	}
	return res, nil
}

// SendSyncRequest sends a request of data type file operation request
func (hub *Hub) SendSyncRequest(handler ConnectionHandler, username string, password string, device string, unrootPath string, action string, file *File) (*Response, error) {
	sReq := SyncRequest{
		Action:     action,
		File:       file,
		UnrootPath: unrootPath,
	}
	sReqJSON, err := json.Marshal(sReq)
	if err != nil {
		hub.LogDebug("error on json Marshal in SendSyncRequest: %v\n", err)
		return nil, err
	}
	req := NewRequest(username, password, device, TypeSyncRequest, sReqJSON)
	hub.LogDebug("SendSyncRequest called,\n request id: %v,\n username: %v, password: %v, device: %v,\n unrootPath: %v,\n action: %v,\n file checksum: %v\n", req.ID, username, password, device, unrootPath, action, file.ContentChecksum)
	res, err := hub.SendRequestForResponse(handler, req)
	if err != nil {
		hub.LogDebug("error on SendRequestForResponse in SendSyncRequest: %v\n", err)
		return nil, err
	}
	return res, nil
}

// SendFileRequest sends a request of data type of file content
func (hub *Hub) SendFileRequest(handler ConnectionHandler, username string, password string, device string, unrootPath string, file *File, content []byte) (*Response, error) {
	fReq := FileRequest{
		File:       file,
		UnrootPath: unrootPath,
		Content:    content,
	}
	fReqJSON, err := json.Marshal(fReq)
	if err != nil {
		hub.LogDebug("error on json Marshal in SendFileRequest: %v\n", err)
		return nil, err
	}
	req := NewRequest(username, password, device, TypeFile, fReqJSON)
	hub.LogDebug("SendFileRequest called,\n request id: %v,\n username: %v, password: %v, device: %v,\n unrootPath: %v,\n file checksum: %v,\n content length: %v\n", req.ID, username, password, device, unrootPath, file.ContentChecksum, len(content))
	res, err := hub.SendRequestForResponse(handler, req)
	if err != nil {
		hub.LogDebug("error on SendRequestForResponse in SendFileRequest: %v\n", err)
		return nil, err
	}
	return res, nil
}
