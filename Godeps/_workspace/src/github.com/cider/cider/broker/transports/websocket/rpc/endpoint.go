// Copyright (c) 2014 The cider AUTHORS
//
// Use of this source code is governed by The MIT License
// that can be found in the LICENSE file.

package rpc

import (
	"bytes"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	ws "code.google.com/p/go.net/websocket"
	"github.com/cider/cider/broker/log"
	"github.com/cider/cider/broker/services/rpc"
	"github.com/tchap/go-websocket-frames/frames"
)

// Constants -------------------------------------------------------------------

const IdentityHeader = "X-Cider-Identity"

const Protocol = "CDR#RPC@01"

const (
	MessageTypeRegister byte = iota
	MessageTypeUnregister
	MessageTypeRequest
	MessageTypeInterrupt
	MessageTypeProgress
	MessageTypeStreamFrame
	MessageTypeReply
	MessageTypePing
	MessageTypePong
)

var (
	FrameEmpty  = []byte{}
	FrameHeader = []byte(Protocol)

	FrameRegisterMT    = []byte{MessageTypeRegister}
	FrameUnregisterMT  = []byte{MessageTypeUnregister}
	FrameRequestMT     = []byte{MessageTypeRequest}
	FrameInterruptMT   = []byte{MessageTypeInterrupt}
	FrameProgressMT    = []byte{MessageTypeProgress}
	FrameStreamFrameMT = []byte{MessageTypeStreamFrame}
	FrameReplyMT       = []byte{MessageTypeReply}
	FramePingMT        = []byte{MessageTypePing}
	FramePongMT        = []byte{MessageTypePong}
)

// Errors ----------------------------------------------------------------------

type ErrUnknownReceiver struct {
	receiver string
}

func (err *ErrUnknownReceiver) Error() string {
	return "websocket<RPC>: unknown message receiver: " + err.receiver
}

var (
	ErrTlsAlreadyConfigured    = errors.New("TLS for WebSocket is already configured")
	ErrAddrNotSet              = errors.New("WebSocket endpoint network address is not set")
	ErrNegativeHeartbeatPeriod = errors.New("WebSocket endpoint heartbeat period is negative")
	ErrTerminated              = errors.New("WebSocket endpoint already terminated")
)

//------------------------------------------------------------------------------
// EndpointConfig
//------------------------------------------------------------------------------

type EndpointConfig struct {
	// Addr is the TCP network address the new endpoint should listen on.
	// This is the minimal configuration that is required.
	Addr string

	// WSConfig is optional and can be used to specify WebSocket config in
	// more details, e.g. it can be used to configure TLS.
	WSConfig ws.Config

	// WSHandshake is an optional function in WebSocket handshake.
	// For example it can be used to authenticate or authorize the
	// connection.
	//
	// It is the same function as ws.Server.Handshake. You can check the
	// documentation there for more details.
	WSHandshake func(*ws.Config, *http.Request) error

	// Heartbeat message is sent over the connection every HeartbeatPeriod.
	// Leaving this value set to zero means that heartbeat is disabled.
	HeartbeatPeriod time.Duration
}

func NewEndpointConfig() *EndpointConfig {
	return &EndpointConfig{}
}

func (config *EndpointConfig) FeedFromEnv(prefix string) error {
	log.Debug("websocket<RPC>: Configuring endpoint from the environment...")

	// Read the network address from the environment if present.
	if v := os.Getenv(prefix + "ADDRESS"); v != "" {
		log.Debug("websocket<RPC>: Network address read from the environment")
		config.Addr = v
	}

	// Read the TLS config from the environment if present.
	var (
		certFile = os.Getenv(prefix + "TLS_CERT")
		keyFile  = os.Getenv(prefix + "TLS_KEY")
	)
	if certFile != "" && keyFile != "" {
		log.Debug("websocket<RPC>: Configuring TLS from the environment...")

		if config.WSConfig.TlsConfig != nil {
			log.Debug("websocket<RPC>: TLS configuration failed")
			return ErrTlsAlreadyConfigured
		}

		cert, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			log.Debug("websocket<RPC>: TLS configuration failed")
			return err
		}

		config.WSConfig.TlsConfig = &tls.Config{
			Certificates: []tls.Certificate{cert},
		}

		log.Debug("websocket<RPC>: TLS configured successfully")
	}

	// Read the heartbeat config from the environment.
	if p := os.Getenv(prefix + "HEARTBEAT"); p != "" {
		log.Debug("websocket<RPC>: Configuring heartbeat from the environment...")
		period, err := time.ParseDuration(p)
		if err != nil {
			return err
		}

		config.HeartbeatPeriod = period
		log.Debug("websocket<RPC>: Heartbeat set to %s", p)
	}

	return nil
}

func (config *EndpointConfig) MustFeedFromEnv(prefix string) *EndpointConfig {
	if err := config.FeedFromEnv(prefix); err != nil {
		panic(err)
	}
	return config
}

func (config *EndpointConfig) IsComplete() error {
	if config.Addr == "" {
		return ErrAddrNotSet
	}
	if config.HeartbeatPeriod < 0 {
		return ErrNegativeHeartbeatPeriod
	}
	return nil
}

func (config *EndpointConfig) MustBeComplete() *EndpointConfig {
	if err := config.IsComplete(); err != nil {
		panic(err)
	}
	return config
}

func NewEndpoint(config *EndpointConfig, exchange rpc.Exchange) (*Endpoint, error) {
	// Make sure that the config is fully configured.
	config.MustBeComplete()

	// Prepare an Endpoint instance.
	endpoint := &Endpoint{
		addr:            config.Addr,
		connections:     make(map[string]*ws.Conn),
		connMu:          new(sync.RWMutex),
		heartbeatPeriod: config.HeartbeatPeriod,
		exchange:        exchange,
		closeCh:         make(chan struct{}),
		closedCh:        make(chan struct{}),
	}
	endpoint.connCond = sync.NewCond(endpoint.connMu)

	// Initialise the WebSocket server.
	endpoint.server = &ws.Server{
		Config:    config.WSConfig,
		Handshake: config.WSHandshake,
		Handler:   endpoint.handleConnection,
	}

	return endpoint, nil
}

//------------------------------------------------------------------------------
// Endpoint
//------------------------------------------------------------------------------

type Endpoint struct {
	addr     string
	listener net.Listener
	server   *ws.Server

	connections map[string]*ws.Conn
	connMu      *sync.RWMutex
	connCond    *sync.Cond

	heartbeatPeriod time.Duration

	exchange rpc.Exchange

	closeCh  chan struct{}
	closedCh chan struct{}
}

// rpc.Endpoint interface ------------------------------------------------------

func (endpoint *Endpoint) ListenAndServe() (err error) {
	endpoint.listener, err = net.Listen("tcp", endpoint.addr)
	if err != nil {
		return
	}

	ex := http.Serve(endpoint.listener, endpoint.server)

	select {
	case <-endpoint.closeCh:
	default:
		err = ex
	}
	return
}

func (endpoint *Endpoint) Close() error {
	// Return and error in case the endpoint is already closed.
	select {
	case <-endpoint.closedCh:
		return ErrTerminated
	default:
	}

	// Make sure no new connections are established.
	select {
	case <-endpoint.closeCh:
	default:
		close(endpoint.closeCh)
	}

	// Try to close all active connections. Returns the first error encountered,
	// but calls Close on all active connections to try to do the best.
	endpoint.connMu.Lock()
	defer endpoint.connMu.Unlock()

	var err error
	for _, conn := range endpoint.connections {
		if ex := conn.Close(); ex != nil {
			log.Warnf("websocket<RPC>: failed to close connection: %v", ex)
			if err == nil {
				err = ex
			}
		}
	}
	if err != nil {
		return err
	}

	// Wait for all the connection goroutines to return.
	// The lock is already being held.
	for len(endpoint.connections) != 0 {
		endpoint.connCond.Wait()
	}

	// Close the listener.
	if err := endpoint.listener.Close(); err != nil {
		return err
	}

	// Confirm that we closed successfully and return.
	close(endpoint.closedCh)
	return nil
}

func (endpoint *Endpoint) DispatchRequest(receiver []byte, msg rpc.Request) error {
	// Get the relevant WebSocket connection.
	conn, err := endpoint.getConnFor(receiver)
	if err != nil {
		return err
	}

	// Access the internal message representation if possible.
	if req, ok := msg.(*request); ok {
		req.frames[0] = msg.Sender()

		err = frames.C.Send(conn, req.frames)

		req.frames[0] = FrameEmpty
		return err
	}

	// Construct the message manually otherwise.
	return frames.C.Send(conn, [][]byte{
		msg.Sender(),
		FrameHeader,
		FrameRequestMT,
		msg.Id(),
		msg.Method(),
		msg.Args(),
		msg.StdoutTag(),
		msg.StderrTag(),
	})
}

func (endpoint *Endpoint) DispatchInterrupt(receiver []byte, msg rpc.Interrupt) error {
	// Get the relevant WebSocket connection.
	conn, err := endpoint.getConnFor(receiver)
	if err != nil {
		return err
	}

	// Access the internal message representation if possible.
	if ipt, ok := msg.(*interrupt); ok {
		ipt.frames[0] = msg.Sender()

		err = frames.C.Send(conn, ipt.frames)

		ipt.frames[0] = FrameEmpty
		return err
	}

	// Construct the message manually otherwise.
	return frames.C.Send(conn, [][]byte{
		msg.Sender(),
		FrameHeader,
		FrameInterruptMT,
		msg.TargetRequestId(),
	})
}

func (endpoint *Endpoint) DispatchProgress(msg rpc.Progress) error {
	// Get the relevant WebSocket connection.
	conn, err := endpoint.getConnFor(msg.Receiver())
	if err != nil {
		return err
	}

	// Access the internal message representation if possible.
	if progress, ok := msg.(*progress); ok {
		raw := progress.frames
		receiver := raw[0]
		raw[0] = FrameEmpty

		err = frames.C.Send(conn, raw)

		raw[0] = receiver
		return err
	}

	// Construct the message manually otherwise.
	return frames.C.Send(conn, [][]byte{
		FrameEmpty,
		FrameHeader,
		FrameProgressMT,
		msg.TargetRequestId(),
	})
}

func (endpoint *Endpoint) DispatchStreamFrame(msg rpc.StreamFrame) error {
	// Get the relevant WebSocket connection.
	conn, err := endpoint.getConnFor(msg.Receiver())
	if err != nil {
		return err
	}

	// Access the internal message representation if possible.
	if frame, ok := msg.(*streamFrame); ok {
		raw := frame.frames
		receiver := raw[0]
		raw[0] = FrameEmpty

		err = frames.C.Send(conn, raw)

		raw[0] = receiver
		return err
	}

	// Construct the message manually otherwise.
	return frames.C.Send(conn, [][]byte{
		FrameEmpty,
		FrameHeader,
		FrameStreamFrameMT,
		msg.TargetStreamTag(),
		msg.Body(),
	})
}

func (endpoint *Endpoint) DispatchReply(msg rpc.Reply) error {
	// Get the relevant WebSocket connection.
	conn, err := endpoint.getConnFor(msg.Receiver())
	if err != nil {
		return err
	}

	// Access the internal message representation if possible.
	if repl, ok := msg.(*reply); ok {
		raw := repl.frames
		receiver := raw[0]
		raw[0] = FrameEmpty

		err := frames.C.Send(conn, raw)

		raw[0] = receiver
		return err
	}

	// Construct the message manually otherwise.
	return frames.C.Send(conn, [][]byte{
		FrameEmpty,
		FrameHeader,
		FrameReplyMT,
		msg.TargetRequestId(),
		msg.ReturnCode(),
		msg.ReturnValue(),
	})
}

// Other public methods --------------------------------------------------------

func (endpoint *Endpoint) Addr() net.Addr {
	return endpoint.listener.Addr()
}

// Private methods -------------------------------------------------------------

func (endpoint *Endpoint) getConnFor(receiver []byte) (*ws.Conn, error) {
	endpoint.connMu.RLock()
	defer endpoint.connMu.RUnlock()

	dst := string(receiver)
	conn, ok := endpoint.connections[dst]
	if !ok {
		return nil, &ErrUnknownReceiver{dst}
	}

	return conn, nil
}

func (endpoint *Endpoint) handleConnection(conn *ws.Conn) {
	// Make sure no new connections are established if we are closing.
	select {
	case <-endpoint.closeCh:
		if err := conn.Close(); err != nil {
			log.Warnf("websocket<RPC>: failed to close connection: %v", err)
		}
		return
	default:
	}

	// Get the application identity from the request header.
	req := conn.Request()
	identity := req.Header.Get(IdentityHeader)
	if identity == "" {
		log.Warnf("websocket<RPC>: connection from %v rejected: identity not set", req.RemoteAddr)
		return
	}

	// Register the connection under the given identity.
	endpoint.registerConnection(identity, conn)
	// Unregister the connection when this handler returns.
	defer endpoint.unregisterConnection(identity)

	// Incorporate heartbeat if requested.
	var pongCh chan bool
	if endpoint.heartbeatPeriod > 0 {
		var (
			pingTermCh = make(chan struct{})
			timeoutCh  = time.After(4 * endpoint.heartbeatPeriod)
		)
		pongCh = make(chan bool, 1)
		go func() {
			for {
				select {
				case <-time.After(endpoint.heartbeatPeriod):
					frames.C.Send(conn, [][]byte{
						FrameEmpty,
						FrameHeader,
						FramePingMT,
					})
				case <-pongCh:
					timeoutCh = time.After(3 * endpoint.heartbeatPeriod)
				case <-timeoutCh:
					conn.Close()
				case <-pingTermCh:
					return
				}
			}
		}()
		defer func() {
			close(pingTermCh)
		}()
	}

	// Loop until the connection is closed or an error is encountered.
	var msg [][]byte
	for {
		if err := frames.C.Receive(conn, &msg); err != nil {
			if err != io.EOF {
				log.Errorf("websocket<RPC>: receive failed: %v", err)
			}
			return
		}

		endpoint.handleMessage(identity, msg, pongCh)
	}
}

func (endpoint *Endpoint) handleMessage(appName string, msg [][]byte, pongCh chan<- bool) {
	rawName := []byte(appName)

	// Perform some sanity checks at the beginning.
	switch {
	case len(msg) < 3:
		warnInvalidMessage(appName, "", "message too short")
		return
	case !bytes.Equal(msg[1], FrameHeader):
		warnInvalidMessage(appName, "", "header mismatch")
		return
	case len(msg[2]) != 1:
		warnInvalidMessage(appName, "", "message type frame invalid")
		return
	}

	// Process the message according to its type.
	switch msg[2][0] {
	case MessageTypeRegister:
		// FRAME 0: empty
		// FRAME 1: message header
		// FRAME 2: message type
		// FRAME 3: method (string)

		switch {
		case len(msg) != 4:
			warnInvalidMessage(appName, "REGISTER", "invalid message length")
			return
		case len(msg[0]) != 0:
			warnInvalidMessage(appName, "REGISTER", "empty receiver frame expected")
			return
		case len(msg[3]) == 0:
			warnInvalidMessage(appName, "REGISTER", "method frame empty")
			return
		}

		endpoint.exchange.RegisterMethod(appName, endpoint, string(msg[3]))

	case MessageTypeUnregister:
		// FRAME 0: empty
		// FRAME 1: message header
		// FRAME 2: message type
		// FRAME 3: method

		switch {
		case len(msg) != 4:
			warnInvalidMessage(appName, "UNREGISTER", "invalid message length")
			return
		case len(msg[0]) != 0:
			warnInvalidMessage(appName, "UNREGISTER", "empty receiver frame expected")
			return
		case len(msg[3]) == 0:
			warnInvalidMessage(appName, "UNREGISTER", "method frame empty")
			return
		}

		endpoint.exchange.UnregisterMethod(appName, string(msg[3]))

	case MessageTypeRequest:
		// FRAME 0: empty
		// FRAME 1: message header
		// FRAME 2: message type
		// FRAME 3: request ID (uint16, BE)
		// FRAME 4: method
		// FRAME 5: method args object (bytes; marshalled)
		// FRAME 6: stdout stream tag (uint16, BE)
		// FRAME 7: stderr stream tag (uint16, BE)

		switch {
		case len(msg) != 8:
			warnInvalidMessage(appName, "REQUEST", "invalid message length")
			return
		case len(msg[0]) != 0:
			warnInvalidMessage(appName, "REQUEST", "empty frame expected")
			return
		case len(msg[3]) != 2:
			warnInvalidMessage(appName, "REQUEST", "request ID frame invalid")
			return
		case len(msg[4]) == 0:
			warnInvalidMessage(appName, "REQUEST", "method frame empty")
			return
		case len(msg[5]) == 0:
			warnInvalidMessage(appName, "REQUEST", "method args frame empty")
			return
		case len(msg[6]) != 0 && len(msg[6]) != 2:
			warnInvalidMessage(appName, "REQUEST", "stdout stream tag frame invalid")
			return
		case len(msg[7]) != 0 && len(msg[7]) != 2:
			warnInvalidMessage(appName, "REQUEST", "stderr stream tag frame invalid")
			return
		}

		req := &request{
			src:    rawName,
			frames: msg,
			rejectFunc: func(code byte, reason string) error {
				conn, err := endpoint.getConnFor(rawName)
				if err != nil {
					return err
				}

				return frames.C.Send(conn, [][]byte{
					FrameEmpty,
					FrameHeader,
					FrameReplyMT,
					msg[4],
					[]byte{code},
					[]byte(reason),
				})
			},
		}

		endpoint.exchange.HandleRequest(req, endpoint)

	case MessageTypeInterrupt:
		// FRAME 0: empty
		// FRAME 1: message header
		// FRAME 2: message type
		// FRAME 3: request ID

		switch {
		case len(msg) != 4:
			warnInvalidMessage(appName, "INTERRUPT", "invalid message length")
			return
		case len(msg[0]) != 0:
			warnInvalidMessage(appName, "INTERRUPT", "empty frame expected")
			return
		case len(msg[3]) != 2:
			warnInvalidMessage(appName, "INTERRUPT", "request ID frame invalid")
			return
		}

		endpoint.exchange.HandleInterrupt(&interrupt{rawName, msg})

	case MessageTypeProgress:
		// FRAME 0: receiver
		// FRAME 1: message header
		// FRAME 2: message type
		// FRAME 3: request ID

		switch {
		case len(msg) != 4:
			warnInvalidMessage(appName, "PROGRESS", "invalid message length")
			return
		case len(msg[0]) == 0:
			warnInvalidMessage(appName, "PROGRESS", "receiver frame empty")
			return
		case len(msg[3]) != 2:
			warnInvalidMessage(appName, "PROGRESS", "request ID frame invalid")
			return
		}

		endpoint.exchange.HandleProgress(&progress{rawName, msg})

	case MessageTypeStreamFrame:
		// FRAME 0: receiver
		// FRAME 1: message header
		// FRAME 2: message type
		// FRAME 3: stream tag (uint16, BE)
		// FRAME 4: frame (bytes)

		switch {
		case len(msg) != 5:
			warnInvalidMessage(appName, "STREAM_FRAME", "invalid message length")
			return
		case len(msg[0]) == 0:
			warnInvalidMessage(appName, "STREAM_FRAME", "receiver frame empty")
			return
		case len(msg[3]) != 2:
			warnInvalidMessage(appName, "STREAM_FRAME", "stream tag frame invalid")
			return
		case len(msg[4]) == 0:
			warnInvalidMessage(appName, "STREAM_FRAME", "frame body frame empty")
			return
		}

		endpoint.exchange.HandleStreamFrame(&streamFrame{rawName, msg})

	case MessageTypeReply:
		// FRAME 0: receiver
		// FRAME 1: message header
		// FRAME 2: message type
		// FRAME 3: request ID
		// FRAME 4: return code (byte)
		// FRAME 5: return value (bytes, method-specific)

		switch {
		case len(msg) != 6:
			warnInvalidMessage(appName, "REPLY", "invalid message length")
			return
		case len(msg[0]) == 0:
			warnInvalidMessage(appName, "REPLY", "receiver frame empty")
			return
		case len(msg[3]) != 2:
			warnInvalidMessage(appName, "REPLY", "request ID frame invalid")
			return
		case len(msg[4]) != 1:
			warnInvalidMessage(appName, "REPLY", "return code frame invalid")
			return
		}

		endpoint.exchange.HandleReply(&reply{rawName, msg})

	case MessageTypePong:
		// FRAME 1: empty
		// FRAME 2: message header
		// FRAME 3: message type

		switch {
		case len(msg) != 3:
			warnInvalidMessage(appName, "PONG", "invalid message length")
			return
		case len(msg[0]) != 0:
			warnInvalidMessage(appName, "PONG", "empty frame expected")
			return
		}

		if pongCh != nil {
			pongCh <- true
		}

	default:
		warnInvalidMessage(appName, "", "unknown message type")
	}
}

func (endpoint *Endpoint) registerConnection(identity string, conn *ws.Conn) bool {
	endpoint.connMu.Lock()
	defer endpoint.connMu.Unlock()

	_, ok := endpoint.connections[identity]
	if ok {
		log.Warnf("websocket<RPC>: connection from %v rejected: identity in use: %v",
			conn.Request().RemoteAddr, identity)
		return false
	}

	endpoint.connections[identity] = conn
	return true
}

func (endpoint *Endpoint) unregisterConnection(identity string) {
	endpoint.connMu.Lock()
	delete(endpoint.connections, identity)
	endpoint.connMu.Unlock()

	endpoint.exchange.UnregisterApp(identity)
	endpoint.connCond.Signal()
}

// Utilities -------------------------------------------------------------------

func warnInvalidMessage(app string, messageType string, message string) {
	log.Warnf("websocket<RPC>: Invalid %s message received from %s: %s", messageType, app, message)
}
