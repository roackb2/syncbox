package syncbox

import (
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strings"
)

// constants
const (
	// LocalHost local host string literal
	IPLocalHost = "localhost"
	IPAnywhere  = "0.0.0.0"

	DefaultServerPort = "8000"
	DefaultClientPort = ""
)

// variables
var (
	ServerHost = os.Getenv("SB_SERVER_HOST")
)

// Connector is the base structure for server and client connection
type Connector struct {
	*Logger
	ServerHost string
	ServerPort string
	ServerAddr *net.TCPAddr
}

// ServerConnector structure for server connection
type ServerConnector struct {
	*Connector
	Clients map[*net.TCPAddr]*Peer
}

// ClientConnector structure for client connection
type ClientConnector struct {
	*Connector
	ClientPort string
	Peer       *Peer
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

// RequestHandler function type for server to handle requests
// RequestHandler should be called as goroutine
type RequestHandler func(*Peer) error

// ErrorHandler function type for server to deal with errors when handling connections
type ErrorHandler func(error)

// NewServerConnector instantiate server connector
func NewServerConnector() (*ServerConnector, error) {
	addr, err := net.ResolveTCPAddr("tcp", IPAnywhere+":"+DefaultServerPort)
	if err != nil {
		return nil, err
	}
	return &ServerConnector{
		Connector: &Connector{
			ServerHost: IPAnywhere,
			ServerPort: DefaultServerPort,
			ServerAddr: addr,
			Logger:     NewDefaultLogger(),
		},
		Clients: make(map[*net.TCPAddr]*Peer),
	}, nil
}

// NewClientConnector instantiate client connector
func NewClientConnector() (*ClientConnector, error) {
	fmt.Printf("server host ip: %v\n", ServerHost)
	addr, err := net.ResolveTCPAddr("tcp", ServerHost+":"+DefaultServerPort)
	if err != nil {
		return nil, err
	}
	return &ClientConnector{
		Connector: &Connector{
			ServerHost: ServerHost,
			ServerPort: DefaultServerPort,
			ServerAddr: addr,
			Logger:     NewDefaultLogger(),
		},
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

func (sc *ServerConnector) closeConn(conn *net.TCPConn, addr *net.TCPAddr) {
	conn.Close()
	delete(sc.Clients, addr)
}

func (cc *ClientConnector) closeConn(conn *net.TCPConn) {
	conn.Close()
}

// Listen listen on port
func (sc *ServerConnector) Listen(handler ConnectionHandler) error {
	ln, err := net.ListenTCP("tcp", sc.ServerAddr)
	if err != nil {
		sc.LogDebug("error on listening: %v\n", err)
		return err
	}
	fmt.Printf("server listening on %v:%v\n", sc.ServerHost, sc.ServerPort)
	for {
		conn, err := ln.AcceptTCP()
		if err != nil {
			sc.LogDebug("error on accept: %v\n", err)
			return err
		}
		addr := conn.RemoteAddr().(*net.TCPAddr)
		hub := NewHub(conn, handler.HandleError)
		peer := NewPeer(hub, "", "", addr, nil)
		sc.Clients[addr] = peer

		go func() {
			err := hub.WaitInbound()
			if err != nil {
				sc.LogDebug("error on WaitInbound: %v\n", err)
				handler.HandleError(err)
			}
			sc.closeConn(conn, addr)
		}()

		go func() {
			err := hub.WaitOutbound()
			if err != nil {
				sc.LogDebug("error on WaitOutbound: %v\n", err)
				handler.HandleError(err)
			}
			sc.closeConn(conn, addr)
		}()

		go func() {
			err := handler.HandleRequest(peer)
			if err != nil {
				sc.LogDebug("error on handle connection: %v\n", err)
				handler.HandleError(err)
			}
			sc.closeConn(conn, addr)
		}()
	}
}

// Dial dials to server
func (cc *ClientConnector) Dial(handler ConnectionHandler) error {
	clientAddr, err := net.ResolveTCPAddr("tcp", ":"+cc.ClientPort)
	if err != nil {
		cc.LogDebug("error on resolve client address: %v\n", err)
		return err
	}
	conn, err := net.DialTCP("tcp", clientAddr, cc.ServerAddr)
	if err != nil {
		cc.LogDebug("error on dial: %v\n", err)
		return err
	}
	hub := NewHub(conn, handler.HandleError)
	addr := conn.RemoteAddr().(*net.TCPAddr)
	cc.Peer = NewPeer(hub, "", "", addr, nil)

	go func() {
		err := hub.WaitInbound()
		if err != nil {
			cc.LogDebug("error on WaitInbound: %v\n", err)
			handler.HandleError(err)
		}
		cc.closeConn(conn)
	}()

	go func() {
		err := hub.WaitOutbound()
		if err != nil {
			cc.LogDebug("error on WaitOutbound: %v\n", err)
			handler.HandleError(err)
		}
		cc.closeConn(conn)
	}()

	go func() {
		err := handler.HandleRequest(cc.Peer)
		if err != nil {
			cc.LogDebug("error on handle connection: %v\n", err)
			handler.HandleError(err)
		}
		cc.closeConn(conn)
	}()
	return nil
}

// HandleRequest boilerplates connection handling
func HandleRequest(peer *Peer, handler ConnectionHandler) error {
	for {
		req, err := peer.Hub.ReceiveRequest()
		if err != nil {
			if err == ErrorEmptyContent || err == io.EOF {
				// peer socket is closed
				return nil
			}
			handler.LogError("error on receiving message: %v\n", err)
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

func getDockerMachineDefaultIP() (string, error) {
	cmd := "docker-machine"
	args := []string{"ip", "default"}
	bytes, err := exec.Command(cmd, args...).Output()
	if err != nil {
		return "", err
	}
	output := strings.Trim(string(bytes), "\n")
	return output, err
}
