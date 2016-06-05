package syncbox

import (
	"fmt"
	"net"
	"os"
	"strings"
	"time"
)

// constants
const (
	// LocalHost local host string literal
	IPLocalHost = "localhost"
	IPAnywhere  = "0.0.0.0"

	DefaultServerPort = "8000"
	DefaultClientPort = ""

	SendMessageRestPeriod  = 2 * time.Second
	SendMessageMaxRetry    = 10
	OperationTimeoutPeriod = 10 * time.Second
)

// variables
var (
	ServerHost = os.Getenv("SB_SERVER_HOST")
)

// RequestHandler function type for server to handle requests
// this should be called as goroutine
type RequestHandler func(*Peer) error

// ErrorHandler function type for server to deal with errors when handling connections
type ErrorHandler func(error)

// Callback is arbitrary function that returns error if there's one
type Callback func() error

// CouldCloseConn represents an interface that supports closing a connection
type CouldCloseConn interface {
	CloseConn(*net.TCPConn)
}

// Connector is the base structure for server and client connection
type Connector struct {
	CouldCloseConn
	*Logger
	ServerHost       string
	ServerPort       string
	ServerDialAddr   *net.TCPAddr
	ServerListenAddr *net.TCPAddr
}

// ServerConnector structure for server connection
type ServerConnector struct {
	*Connector
	ServerLocalAddr *net.TCPAddr
	Clients         map[*net.TCPAddr]*Peer
}

// ClientConnector structure for client connection
type ClientConnector struct {
	*Connector
	ClientPort       string
	ServerRemoteAddr *net.TCPAddr
	ClientLocalAddr  *net.TCPAddr
	Peer             *Peer
}

// Peer is a representation of a connection
type Peer struct {
	*Hub
	Username string
	Password string
	Device   string
	Address  *net.TCPAddr
	RefGraph *RefGraph
}

// ConnectionHandler is the interface to specify methods that should be implemented as a connection handler
type ConnectionHandler interface {
	Dial(handler ConnectionHandler, addr *net.TCPAddr) error
	HandleRequest(*Peer) error
	HandleError(error)
	ProcessIdentity(*Request, *Peer, ErrorHandler)
	ProcessDigest(*Request, *Peer, ErrorHandler)
	ProcessSync(*Request, *Peer, ErrorHandler)
	ProcessFile(*Request, *Peer, ErrorHandler)
	LogInfo(string, ...interface{})
	LogDebug(string, ...interface{})
	LogError(string, ...interface{})
	LogVerbose(string, ...interface{})
}

// NewConnector instantiates a connector
func NewConnector() (*Connector, error) {
	serverListenAddr, err := net.ResolveTCPAddr("tcp", IPAnywhere+":"+DefaultServerPort)
	if err != nil {
		return nil, err
	}
	serverDialAddr, err := net.ResolveTCPAddr("tcp", ServerHost+":"+DefaultServerPort)
	if err != nil {
		return nil, err
	}
	return &Connector{
		ServerHost:       ServerHost,
		ServerPort:       DefaultServerPort,
		ServerDialAddr:   serverDialAddr,
		ServerListenAddr: serverListenAddr,
		Logger:           NewDefaultLogger(),
	}, nil
}

// NewServerConnector instantiate server connector
func NewServerConnector() (*ServerConnector, error) {
	connector, err := NewConnector()
	if err != nil {
		return nil, err
	}
	return &ServerConnector{
		Connector: connector,
		Clients:   make(map[*net.TCPAddr]*Peer),
	}, nil
}

// NewClientConnector instantiate client connector
func NewClientConnector() (*ClientConnector, error) {
	fmt.Printf("server host ip: %v\n", ServerHost)
	connector, err := NewConnector()
	if err != nil {
		return nil, err
	}
	return &ClientConnector{
		Connector:  connector,
		ClientPort: DefaultClientPort,
	}, nil
}

// NewPeer instantiates a Peer
func NewPeer(hub *Hub, username string, device string, addr *net.TCPAddr, rg *RefGraph) *Peer {
	return &Peer{
		Hub:      hub,
		Username: username,
		Device:   device,
		Address:  addr,
		RefGraph: rg,
	}
}

// CloseConn implements the CouldCloseConn interface
func (sc *ServerConnector) CloseConn(conn *net.TCPConn) {
	sc.LogDebug("close connection of %v\n", conn.RemoteAddr())
	conn.Close()
	delete(sc.Clients, conn.RemoteAddr().(*net.TCPAddr))
}

// CloseConn implements the CouldCloseConn interface
func (cc *ClientConnector) CloseConn(conn *net.TCPConn) {
	cc.LogDebug("close connection of %v\n", conn.RemoteAddr())
	conn.Close()
}

// SetupConnection setups methods that handles a request, including receiving message,
// waiting for outbound message, dispatch response to corresponding request and handle request
func (connector *Connector) SetupConnection(handler ConnectionHandler, peer *Peer) {
	go func() {
		err := peer.Hub.WaitInbound()
		if err != nil {
			connector.LogDebug("error on WaitInbound: %v\n", err)
			handler.HandleError(err)
		}
		connector.CloseConn(peer.Hub.Conn)
	}()

	go func() {
		err := peer.Hub.WaitOutbound()
		if err != nil {
			connector.LogDebug("error on WaitOutbound: %v\n", err)
			handler.HandleError(err)
		}
		connector.CloseConn(peer.Hub.Conn)
	}()

	go func() {
		err := peer.Hub.DispatchResponse()
		if err != nil {
			connector.LogDebug("error on DispatchResponse: %v\n", err)
			handler.HandleError(err)
		}
		connector.CloseConn(peer.Hub.Conn)
	}()

	go func() {
		err := handler.HandleRequest(peer)
		if err != nil {
			connector.LogDebug("error on handle connection: %v\n", err)
			handler.HandleError(err)
		}
		connector.CloseConn(peer.Hub.Conn)
	}()
}

// Dial dials to client, should be used when try to reconnect to peer
func (sc *ServerConnector) Dial(handler ConnectionHandler, clientDialAddr *net.TCPAddr) error {
	conn, err := net.DialTCP("tcp", nil, clientDialAddr)
	if err != nil {
		sc.LogDebug("error on dial: %v\n", err)
		return err
	}
	addr := conn.RemoteAddr().(*net.TCPAddr)
	hub := NewHub(conn, handler.HandleError)
	peer := NewPeer(hub, "", "", addr, nil)
	sc.Clients[addr] = peer
	sc.SetupConnection(handler, peer)
	return nil
}

// Listen listen on port
func (sc *ServerConnector) Listen(handler ConnectionHandler) error {
	ln, err := net.ListenTCP("tcp", sc.ServerListenAddr)
	if err != nil {
		sc.LogDebug("error on listening: %v\n", err)
		return err
	}
	fmt.Printf("server listening on %v\n", sc.ServerListenAddr)
	for {
		conn, err := ln.AcceptTCP()
		if err != nil {
			sc.LogDebug("error on accept: %v\n", err)
			return err
		}
		addr := conn.RemoteAddr().(*net.TCPAddr)
		sc.LogDebug("accepted connection: %v\n", addr)
		hub := NewHub(conn, handler.HandleError)
		peer := NewPeer(hub, "", "", addr, nil)
		sc.Clients[addr] = peer
		sc.SetupConnection(handler, peer)
	}
}

// Dial dials to server, serverAddr should be passed as nil,
// it's only for  compatibility of the ConnectionHandler interface
func (cc *ClientConnector) Dial(handler ConnectionHandler, serverAddr *net.TCPAddr) error {
	conn, err := net.DialTCP("tcp", nil, cc.ServerDialAddr)
	if err != nil {
		cc.LogDebug("error on dial: %v\n", err)
		return err
	}
	cc.ClientLocalAddr = conn.LocalAddr().(*net.TCPAddr)
	cc.ServerRemoteAddr = conn.RemoteAddr().(*net.TCPAddr)
	hub := NewHub(conn, handler.HandleError)
	cc.Peer = NewPeer(hub, "", "", cc.ServerRemoteAddr, nil)
	cc.SetupConnection(handler, cc.Peer)
	return nil
}

// Listen listens on the port, in case that connection closed and server try to reconnect to client.
// This should be called after client Dial, to get the port that used for dialing
func (cc *ClientConnector) Listen(handler ConnectionHandler) error {
	ln, err := net.ListenTCP("tcp", cc.ClientLocalAddr)
	if err != nil {
		cc.LogDebug("error on listening: %v\n", err)
		return err
	}
	fmt.Printf("client listening on %v\n", cc.ClientLocalAddr)
	for {
		conn, err := ln.AcceptTCP()
		if err != nil {
			cc.LogDebug("error on accept: %v\n", err)
			return err
		}
		serverAddr := conn.RemoteAddr().(*net.TCPAddr)
		cc.LogDebug("accepted connection: %v\n", serverAddr)
		hub := NewHub(conn, handler.HandleError)
		cc.Peer = NewPeer(hub, "", "", serverAddr, nil)
		cc.SetupConnection(handler, cc.Peer)
	}
}

// HandleRequest boilerplates connection handling.
// It should returns error only when the error should cause connnection to be closed,
// otherwise should just continue for next loop to process incoming requests.
func HandleRequest(peer *Peer, handler ConnectionHandler) error {
	for {
		req, err := peer.Hub.ReceiveRequest()
		if err != nil {
			if err == ErrorPeerSocketClosed {
				// peer socket is closed
				return nil
			}
			handler.LogError("error on receiving request: %v\n", err)
			continue
		}
		peer.Username = req.Username
		peer.Password = req.Password
		peer.Device = req.Device
		handler.LogVerbose("request data type: %v\n", req.DataType)
		switch req.DataType {
		case TypeIdentity:
			go handler.ProcessIdentity(req, peer, handler.HandleError)
		case TypeDigest:
			go handler.ProcessDigest(req, peer, handler.HandleError)
		case TypeSyncRequest:
			go handler.ProcessSync(req, peer, handler.HandleError)
		case TypeFile:
			go handler.ProcessFile(req, peer, handler.HandleError)
		default:
			handler.LogDebug("data type: %v\n", req.DataType)
			return ErrorUnknownRequestType
		}
	}
}

// SendWithRetry sends message with retry, it also try to  dial if connection is broken
func SendWithRetry(handler ConnectionHandler, callback Callback, addr *net.TCPAddr) error {
	var err error
	for i := 0; i < SendMessageMaxRetry; i++ {
		if err = callback(); err != nil {
			handler.LogDebug("error in SendWithRetry: %v,\n retry count: %v\n", err, i)
			if err == ErrorPeerSocketClosed || strings.HasSuffix(err.Error(), "use of closed network connection") {
				if dialErr := handler.Dial(handler, addr); dialErr != nil {
					handler.LogDebug("error on retry Dial in SendWithRetry: %v\n", dialErr)
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
