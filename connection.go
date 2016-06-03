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

// RequestHandler function type for server to handle requests
// this should be called as goroutine
type RequestHandler func(*Peer) error

// ErrorHandler function type for server to deal with errors when handling connections
type ErrorHandler func(error)

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

func (sc *ServerConnector) closeConn(conn *net.TCPConn) {
	sc.LogDebug("close connection of %v\n", conn.RemoteAddr())
	conn.Close()
	delete(sc.Clients, conn.RemoteAddr().(*net.TCPAddr))
}

func (cc *ClientConnector) closeConn(conn *net.TCPConn) {
	cc.LogDebug("close connection of %v\n", conn.RemoteAddr())
	conn.Close()
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

	go func() {
		err := hub.WaitInbound()
		if err != nil {
			sc.LogDebug("error on WaitInbound: %v\n", err)
			handler.HandleError(err)
		}
		sc.closeConn(conn)
	}()

	go func() {
		err := hub.WaitOutbound()
		if err != nil {
			sc.LogDebug("error on WaitOutbound: %v\n", err)
			handler.HandleError(err)
		}
		sc.closeConn(conn)
	}()

	go func() {
		err := handler.HandleRequest(peer)
		if err != nil {
			sc.LogDebug("error on handle connection: %v\n", err)
			handler.HandleError(err)
		}
		sc.closeConn(conn)
	}()
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

		go func() {
			err := hub.WaitInbound()
			if err != nil {
				sc.LogDebug("error on WaitInbound: %v\n", err)
				handler.HandleError(err)
			}
			sc.closeConn(conn)
		}()

		go func() {
			err := hub.WaitOutbound()
			if err != nil {
				sc.LogDebug("error on WaitOutbound: %v\n", err)
				handler.HandleError(err)
			}
			sc.closeConn(conn)
		}()

		go func() {
			err := handler.HandleRequest(peer)
			if err != nil {
				sc.LogDebug("error on handle connection: %v\n", err)
				handler.HandleError(err)
			}
			sc.closeConn(conn)
		}()
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
	}
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
