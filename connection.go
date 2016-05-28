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
			Logger:     NewLogger(DefaultAppPrefix, GlobalLogInfo, GlobalLogError, GlobalLogDebug),
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
			Logger:     NewLogger(DefaultAppPrefix, GlobalLogInfo, GlobalLogError, GlobalLogDebug),
		},
		ClientPort: DefaultClientPort,
	}, nil
}

// NewPeer instantiates a Peer
func NewPeer(hub *Hub, username string, rg *RefGraph) *Peer {
	return &Peer{
		Hub:      hub,
		Username: username,
		RefGraph: rg,
	}
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
		peer := NewPeer(hub, "", nil)
		sc.Clients[addr] = peer

		go func() {
			defer func() {
				conn.Close()
				delete(sc.Clients, addr)
			}()
			err := handler.HandleRequest(peer)
			if err != nil {
				sc.LogDebug("error on handle connection: %v\n", err)
				handler.HandleError(err)
			}
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
	cc.Peer = NewPeer(hub, "", nil)
	go func() {
		defer func() {
			conn.Close()
		}()
		err := handler.HandleRequest(cc.Peer)
		if err != nil {
			cc.LogDebug("error on handle connection: %v\n", err)
			handler.HandleError(err)
		}
	}()
	return nil
}

// HandleRequest boilerplates connection handling
func HandleRequest(peer *Peer, handler ConnectionHandler) error {
	for {
		req, err := peer.Hub.ReceiveRequest()
		peer.Username = req.Username
		if err != nil {
			if err == ErrorEmptyContent || err == io.EOF {
				// peer socket is closed
				return nil
			}
			handler.LogError("error on receiving message: %v\n", err)
			continue
		}
		handler.LogDebug("request data type: %v\n", req.DataType)
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
