package syncbox

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net"
	"time"
)

// MessageQueueItem represents an item of the message queue
type MessageQueueItem struct {
	Packets      []*Packet
	LastProgress int
}

// Hub is the network reading/writing hub for request/reponse,
// it's the lowest level entry point for network connection
type Hub struct {
	*Logger
	Conn                 *net.TCPConn
	InboundMessage       chan []byte
	InboundMessageError  chan error
	InboundRequest       chan []byte
	InboundRequestError  chan error
	InboundResponse      chan []byte
	InboundResponseError chan error
	MessageQueue         map[[PacketIDSIze]byte]*MessageQueueItem
	RequestQueue         map[string]chan *Response
	ErrorHandler         ErrorHandler
}

// NewHub instantiates a Hub
func NewHub(conn *net.TCPConn, eHandler ErrorHandler) *Hub {
	hub := &Hub{
		Conn:                 conn,
		InboundMessage:       make(chan []byte),
		InboundMessageError:  make(chan error),
		InboundRequest:       make(chan []byte),
		InboundRequestError:  make(chan error),
		InboundResponse:      make(chan []byte),
		InboundResponseError: make(chan error),
		MessageQueue:         make(map[[PacketIDSIze]byte]*MessageQueueItem),
		RequestQueue:         make(map[string]chan *Response),
		ErrorHandler:         eHandler,
		Logger:               NewDefaultLogger(),
	}
	return hub
}

// Setup runs the goroutines necessary for a hub to communicate via channels
func (hub *Hub) Setup() error {
	errChan := make(chan error)
	go func() {
		if err := hub.ReceivePackets(); err != nil {
			hub.LogDebug("error on ReceivePackets in Setup: %v\n", err)
			errChan <- err
		}
	}()
	go func() {
		if err := hub.ReceiveMessage(); err != nil {
			hub.LogDebug("error on ReceiveMessage in Setup: %v\n", err)
			errChan <- err
		}
	}()
	go func() {
		if err := hub.DispatchResponse(); err != nil {
			hub.LogDebug("error on DispatchResponse in Setup: %v\n", err)
			errChan <- err
		}
	}()
	return <-errChan
}

func (hub *Hub) sendPackets(bytes []byte) error {
	packets, err := Serialize(bytes)
	if err != nil {
		return err
	}
	for _, packet := range packets {
		hub.LogVerbose("packet to send: %v\n", packet)
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

func (hub *Hub) sendMessage(message []byte, prefix rune) error {
	message = append(message, ByteDelim) // appends delim
	switch prefix {
	case RequestPrefix:
		message = append([]byte{byte(RequestPrefix)}, message...) // unshift request prefix
		hub.LogVerbose("outbound request message: %v\n", string(message))
		return hub.sendPackets(message)
	case ResponsePrefix:
		message = append([]byte{byte(ResponsePrefix)}, message...) // unshift request prefix
		hub.LogVerbose("outbound response message: %v\n", string(message))
		return hub.sendPackets(message)
	default:
		return errors.New("unknown message type: " + string(prefix))
	}
}

func (hub *Hub) handlePacketFullness(size int64, sequence int64, packet *Packet, item *MessageQueueItem) {
	full := true
	var i int64
	for i = 0; i < size; i++ {
		if item.Packets[i] == nil {
			full = false
		}
	}
	if full {
		data := Deserialize(item.Packets)
		data = bytes.TrimRight(data, string([]byte{0})) // trim trailing zero char in last packet
		hub.InboundMessage <- data
		delete(hub.MessageQueue, packet.MessageID)
	}
}

// ReceivePackets waits to read from the connection of the hub,
// this should be run as goroutine.
func (hub *Hub) ReceivePackets() error {
	for {
		buffer := make([]byte, PacketTotalSize, PacketTotalSize)
		readSize := 0
		// one read perhaps won't read the full packet, loop until a full packet is read
		for readSize < PacketTotalSize {
			n, err := (*hub.Conn).Read(buffer[readSize:len(buffer)])
			if err != nil {
				hub.LogDebug("error on ReceivePackets: %v\n", err)
				hub.InboundMessageError <- err
			}
			readSize += n
		}
		var bufferArr [PacketTotalSize]byte
		copy(bufferArr[:], buffer)
		packet := RebornPacket(bufferArr)
		hub.LogVerbose("packet received: %v\n", packet)

		size, err := packet.GetSize()
		if err != nil {
			hub.InboundMessageError <- err
		}
		sequence, err := packet.GetSequence()
		if err != nil {
			hub.InboundMessageError <- err
		}
		item, exists := hub.MessageQueue[packet.MessageID]
		if exists {
			item.Packets[sequence] = packet
			progress := int(math.Floor(float64(sequence) / float64(size) * 100))
			if size > 10000 && (progress%10 == 0) && progress != item.LastProgress {
				hub.LogInfo("progress reading inbound message: %v%%\n", progress)
			}
			hub.handlePacketFullness(size, sequence, packet, item)
		} else {
			packets := make([]*Packet, size, size)
			packets[sequence] = packet
			item = &MessageQueueItem{
				Packets:      packets,
				LastProgress: 0,
			}
			hub.MessageQueue[packet.MessageID] = item
			hub.handlePacketFullness(size, sequence, packet, item)
		}
	}
}

// ReceiveMessage waits for inbound message and dispatch to InboundRequest or InboundResponse channel accordingly,
// this should be run as goroutine/
// It returns error if an error is considered as connection level, such as EOF or unknonw message type,
// and leave for the connectors to deal with error,
// otherwise it sends the error to InboundRequestError or InboundResponseError channel accordingly.
func (hub *Hub) ReceiveMessage() error {
	for {
		// message, err := hub.readPackets()
		select {
		case message := <-hub.InboundMessage:
			hub.LogVerbose("message received: %v\n", message)
			if len(message) > 0 {
				message = message[0 : len(message)-1]
			}
			if len(message) == 0 {
				hub.LogDebug("peer socket closed\n")
				return ErrorPeerSocketClosed
			}
			prefix := message[0]
			message = message[1:len(message)]
			switch prefix {
			case RequestPrefix:
				hub.LogVerbose("inbound request message: %v\n", string(message))
				hub.InboundRequest <- message
			case ResponsePrefix:
				hub.LogVerbose("inbound response message: %v\n", string(message))
				hub.InboundResponse <- message
			default:
				return errors.New("unknown message type: " + string(prefix))
			}
		case err := <-hub.InboundMessageError:
			if err == io.EOF {
				hub.LogDebug("peer socket closed\n")
				return ErrorPeerSocketClosed
			}
			hub.LogVerbose("error in waitInbound: %v\n", err)
			continue
		}
	}
}

// DispatchResponse waits to receive response and dispatch to waiting request.
// This should be called as goroutine.
// It should returns error only when the error should cause connnection to be closed,
// otherwise should just continue for next loop to process incoming response.
func (hub *Hub) DispatchResponse() error {
	for {
		res, err := hub.ReceiveResponse()
		if err != nil {
			if err == ErrorPeerSocketClosed {
				// peer socket is closed
				return nil
			}
			hub.LogError("error on receiving response: %v\n", err)
			continue
		}
		id := res.RequestID
		resChan, exists := hub.RequestQueue[id]
		if exists {
			resChan <- res
			delete(hub.RequestQueue, id)
		} else {
			hub.LogDebug("request not found in DispatchResponse, request id: %v\n", id)
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

// SendRequest sends a request to the hub,
func (hub *Hub) SendRequest(req *Request) error {
	bytes, err := json.Marshal(req)
	if err != nil {
		hub.LogDebug("error on json Marshal in SendRequest: %v\n", err)
		return err
	}
	return hub.sendMessage(bytes, RequestPrefix)
}

// SendResponse sends a response to the hub
func (hub *Hub) SendResponse(req *Request, res *Response) error {
	res.RequestID = req.ID
	bytes, err := json.Marshal(res)
	if err != nil {
		hub.LogDebug("error on json Marshal in SendResponse: %v\n", err)
		return err
	}
	return hub.sendMessage(bytes, ResponsePrefix)
}

// SendRequestForResponse sends a request and waits for response,
// it returns ErrorTimeout if waits too long for the response
func (hub *Hub) SendRequestForResponse(req *Request) (*Response, error) {
	err := hub.SendRequest(req)
	if err != nil {
		hub.LogDebug("error on SendRequest in SendRequestForResponse: %v\n", err)
		return nil, err
	}
	resChan := make(chan *Response)
	hub.RequestQueue[req.ID] = resChan
	select {
	case res := <-resChan:
		return res, nil
	case <-time.After(OperationTimeoutPeriod):
		return nil, ErrorTimeout
	}
}

// SendIdentityRequest sends a request with data type of user identity
func (hub *Hub) SendIdentityRequest(username string, password string, device string) (*Response, error) {
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

	res, err := hub.SendRequestForResponse(req)
	if err != nil {
		hub.LogDebug("error on SendRequestForResponse in SendIdentityRequest: %v\n", err)
		return nil, err
	}
	return res, nil
}

// SendDigestRequest sends a request with data type file tree digest
func (hub *Hub) SendDigestRequest(username string, password string, device string, dir *Dir) (*Response, error) {
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

	res, err := hub.SendRequestForResponse(req)
	if err != nil {
		hub.LogDebug("error on SendRequestForResponse in SendDigestRequest: %v\n", err)
		return nil, err
	}
	return res, nil
}

// SendSyncRequest sends a request of data type file operation request
func (hub *Hub) SendSyncRequest(username string, password string, device string, unrootPath string, action string, file *File) (*Response, error) {
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
	res, err := hub.SendRequestForResponse(req)
	if err != nil {
		hub.LogDebug("error on SendRequestForResponse in SendSyncRequest: %v\n", err)
		return nil, err
	}
	return res, nil
}

// SendFileRequest sends a request of data type of file content
func (hub *Hub) SendFileRequest(username string, password string, device string, unrootPath string, file *File, content []byte) (*Response, error) {
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
	res, err := hub.SendRequestForResponse(req)
	if err != nil {
		hub.LogDebug("error on SendRequestForResponse in SendFileRequest: %v\n", err)
		return nil, err
	}
	return res, nil
}
