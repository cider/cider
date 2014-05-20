// Copyright (c) 2013 The go-meeko AUTHORS
//
// Use of this source code is governed by The MIT License
// that can be found in the LICENSE file.

package rpc

import (
	// Stdlib
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"sync"

	// Meeko
	"github.com/meeko/go-meeko/meeko/services"
	"github.com/meeko/go-meeko/meeko/services/rpc"
	"github.com/meeko/go-meeko/meeko/utils/codecs"

	// Other
	ws "code.google.com/p/go.net/websocket"
	log "github.com/cihub/seelog"
	"github.com/tchap/go-websocket-frames/frames"
)

// HTTP header that is used to pass the application identity to the broker.
const IdentityHeader = "X-Meeko-Identity"

// TransportFactory can be used to create instances of the WebSocket RPC transport.
// All that is necessary is to set the required struct fields and call NewTransport.
type TransportFactory struct {
	// The server address where a Meeko WebSocket RPC endpoint is listening.
	//
	// This is a required field.
	Server string

	// Value of the WebSocket Origin header.
	Origin string

	// A function that can be used to modify the WebSocket connection
	// configuration before connecting to the server, for example to include
	// some custom headers and so on.
	//
	// This field is required but preset by NewTransportFactory.
	WSConfigFunc func(config *ws.Config)
}

func NewTransportFactory() *TransportFactory {
	return &TransportFactory{
		WSConfigFunc: func(config *ws.Config) {},
	}
}

func (factory *TransportFactory) IsFullyConfigured() error {
	if factory.Server == "" {
		return &services.ErrMissingConfig{"WebSocket server address", "WebSocket RPC transport"}
	}
	if factory.WSConfigFunc == nil {
		return &services.ErrMissingConfig{"WebSocket config function", "WebSocket RPC transport"}
	}
	return nil
}

func (factory *TransportFactory) MustBeFullyConfigured() *TransportFactory {
	if err := factory.IsFullyConfigured(); err != nil {
		panic(err)
	}
	return factory
}

type Transport struct {
	// WebSocket connection
	conn *ws.Conn

	// Requests that are being handled
	incomingRequests map[string]*remoteRequest
	requestsMu       *sync.Mutex

	// Output interface for the Service using this Transport
	requestCh   chan rpc.RemoteRequest
	progressCh  chan rpc.RequestID
	streamingCh chan rpc.StreamFrame
	replyCh     chan rpc.RemoteCallReply
	errorCh     chan error

	// Termination
	termCh        chan struct{}
	loopTermAckCh chan struct{}
	err           error
}

func (factory *TransportFactory) NewTransport(identity string) (rpc.Transport, error) {
	// Make sure the config is complete.
	factory.MustBeFullyConfigured()

	// Configure WebSocket to connect to the relevant endpoint.
	config, err := ws.NewConfig(factory.Server, factory.Origin)
	if err != nil {
		return nil, err
	}

	// Run the user-defined configuration function. The function is never nil
	// unless the user is really trying to commit suicide.
	factory.WSConfigFunc(config)

	// Pass the identity to the server using an HTTP header.
	config.Header.Set(IdentityHeader, identity)

	// Dial the server using the specified config.
	conn, err := ws.DialConfig(config)
	if err != nil {
		return nil, err
	}

	// Construct Transport.
	t := &Transport{
		conn:             conn,
		incomingRequests: make(map[string]*remoteRequest),
		requestsMu:       new(sync.Mutex),
		requestCh:        make(chan rpc.RemoteRequest),
		progressCh:       make(chan rpc.RequestID),
		streamingCh:      make(chan rpc.StreamFrame),
		replyCh:          make(chan rpc.RemoteCallReply),
		errorCh:          make(chan error),
		termCh:           make(chan struct{}),
		loopTermAckCh:    make(chan struct{}),
	}

	go t.loop()
	return t, nil
}

// rpc.Transport interface -----------------------------------------------------

func (t *Transport) RegisterMethod(cmd rpc.RegisterCmd) {
	log.Debugf("websocket<RPC>: registering %s", cmd.Method())

	// Construct and send the message.
	cmd.ErrorChan() <- frames.C.Send(t.conn, [][]byte{
		frameEmpty,
		frameHeader,
		frameRegisterMT,
		[]byte(cmd.Method()),
	})
}

func (t *Transport) UnregisterMethod(cmd rpc.UnregisterCmd) {
	log.Debugf("websocket<RPC>: unregistering %s", cmd.Method())

	// Construct and send the message.
	cmd.ErrorChan() <- frames.C.Send(t.conn, [][]byte{
		frameEmpty,
		frameHeader,
		frameUnregisterMT,
		[]byte(cmd.Method()),
	})
}

func (t *Transport) RequestChan() <-chan rpc.RemoteRequest {
	return t.requestCh
}

func (t *Transport) Call(cmd rpc.CallCmd) {
	log.Debugf("websocket<RPC>: calling %s", cmd.Method())

	// Marshal the request ID.
	var idBuffer bytes.Buffer
	binary.Write(&idBuffer, binary.BigEndian, cmd.RequestId())

	// Marshal the arguments.
	var argsBuffer bytes.Buffer
	if err := codecs.MessagePack.Encode(&argsBuffer, cmd.Args()); err != nil {
		cmd.ErrorChan() <- err
		return
	}

	// Marshal the stdout tag.
	var stdoutTagBuffer bytes.Buffer
	if tag := cmd.StdoutTag(); tag != nil {
		binary.Write(&stdoutTagBuffer, binary.BigEndian, *tag)
	}

	// Marshal the stderr tag.
	var stderrTagBuffer bytes.Buffer
	if tag := cmd.StderrTag(); tag != nil {
		binary.Write(&stderrTagBuffer, binary.BigEndian, *tag)
	}

	// Construct and send the message.
	cmd.ErrorChan() <- frames.C.Send(t.conn, [][]byte{
		frameEmpty,
		frameHeader,
		frameRequestMT,
		idBuffer.Bytes(),
		[]byte(cmd.Method()),
		argsBuffer.Bytes(),
		stdoutTagBuffer.Bytes(),
		stderrTagBuffer.Bytes(),
	})
}

func (t *Transport) Interrupt(cmd rpc.InterruptCmd) {
	log.Debugf("websocket<RPC>: interrupting request #%v", cmd.TargetRequestId())

	// Marshal the request ID.
	var idBuffer bytes.Buffer
	binary.Write(&idBuffer, binary.BigEndian, cmd.TargetRequestId())

	// Construct and send the message.
	cmd.ErrorChan() <- frames.C.Send(t.conn, [][]byte{
		frameEmpty,
		frameHeader,
		frameInterruptMT,
		idBuffer.Bytes(),
	})
}

func (t *Transport) ProgressChan() <-chan rpc.RequestID {
	return t.progressCh
}

func (t *Transport) StreamFrameChan() <-chan rpc.StreamFrame {
	return t.streamingCh
}

func (t *Transport) ReplyChan() <-chan rpc.RemoteCallReply {
	return t.replyCh
}

func (t *Transport) ErrorChan() <-chan error {
	return t.errorCh
}

func (t *Transport) Close() error {
	// Close the termination channel if not already closed.
	select {
	case <-t.termCh:
	default:
		close(t.termCh)
	}

	// Close the WebSocket connection.
	if err := t.conn.Close(); err != nil {
		return err
	}

	// Wait for the loop to return.
	<-t.loopTermAckCh
	return nil
}

func (t *Transport) Closed() <-chan struct{} {
	return t.loopTermAckCh
}

func (t *Transport) Wait() error {
	<-t.Closed()
	return t.err
}

// Internal command loop -------------------------------------------------------

const Header = "CDR#RPC@01"

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
	frameEmpty  = []byte{}
	frameHeader = []byte(Header)

	frameRegisterMT    = []byte{MessageTypeRegister}
	frameUnregisterMT  = []byte{MessageTypeUnregister}
	frameRequestMT     = []byte{MessageTypeRequest}
	frameInterruptMT   = []byte{MessageTypeInterrupt}
	frameProgressMT    = []byte{MessageTypeProgress}
	frameStreamFrameMT = []byte{MessageTypeStreamFrame}
	frameReplyMT       = []byte{MessageTypeReply}
	framePongMT        = []byte{MessageTypePong}
)

var pongMessage = [][]byte{
	frameEmpty,
	frameHeader,
	framePongMT,
}

func (t *Transport) loop() {
	defer func() {
		log.Debug("websocket<RPC>: closing the loop termAck channel")
		close(t.loopTermAckCh)
	}()

	var msg [][]byte
	for {
		if err := frames.C.Receive(t.conn, &msg); err != nil {
			select {
			case <-t.termCh:
				log.Debug("websocket<RPC>: loop terminating")
				// In case we are terminating, EOF is not really an error.
				// The connection is closed on this side of the connection
				// to break this loop and disconnect from the server.
				if err != io.EOF {
					t.err = err
				}
			default:
				log.Debugf("websocket<RPC>: loop terminating because of an error: %v", err)
				t.errorCh <- err
				t.err = err
			}
			return
		}

		// Check the message header frames.
		//
		// FRAME 0: empty or sender (string)
		// FRAME 1: message header (string)
		// FRAME 2: message type (byte)
		switch {
		case len(msg) < 3:
			log.Warn("websocket<RPC>: message too short")
			return
		case !bytes.Equal(msg[1], frameHeader):
			log.Warn("websocket<RPC>: invalid message header")
			return
		case len(msg[2]) != 1:
			log.Warn("websocket<RPC>: invalid message type")
			return
		}

		// Process the message depending on the message type.
		switch msg[2][0] {
		case MessageTypeRequest:
			// FRAME 0: sender
			// FRAME 3: request ID (uint16; BE)
			// FRAME 4: method (string)
			// FRAME 5: method arguments (object; encoded with MessagePack)
			// FRAME 6: stdout stream tag (empty or uint16; BE)
			// FRAME 7: stderr stream tag (empty of uint16; BE)
			switch {
			case len(msg) != 8:
				log.Warn("websocket<RPC>: REQUEST: invalid message length")
				return
			case len(msg[0]) == 0:
				log.Warn("websocket<RPC>: REQUEST: empty sender frame received")
				return
			case len(msg[3]) != 2:
				log.Warn("websocket<RPC>: REQUEST: invalid request ID frame received")
				return
			case len(msg[4]) == 0:
				log.Warn("websocket<RPC>: REQUEST: empty method frame")
				return
			case len(msg[6]) != 0 && len(msg[6]) != 2:
				log.Warn("websocket<RPC>: REQUEST: invalid stdout tag frame received")
				return
			case len(msg[7]) != 0 && len(msg[7]) != 2:
				log.Warn("websocket<RPC>: REQUEST: invalid stdout tag frame received")
				return
			}

			req, err := t.newRequest(msg)
			if err != nil {
				log.Warnf("websocket<RPC>: REQUEST: %v", err)
				return
			}
			t.requestCh <- req

		case MessageTypeInterrupt:
			// FRAME 0: sender (string)
			// FRAME 3: request ID (uint16; BE)
			switch {
			case len(msg) != 4:
				log.Warn("websocket<RPC>: INTERRUPT: invalid message length")
				return
			case len(msg[0]) == 0:
				log.Warn("websocket<RPC>: INTERRUPT: empty sender frame received")
				return
			case len(msg[3]) != 2:
				log.Warn("websocket<RPC>: INTERRUPT: invalid request ID frame received")
				return
			}

			key := string(append(msg[0], msg[3]...))
			t.requestsMu.Lock()
			request, ok := t.incomingRequests[key]
			if !ok {
				log.Warnf("websocket<RPC>: INTERRUPT: unknown request ID received: %q", key)
				t.requestsMu.Unlock()
				return
			}

			request.interrupt()
			delete(t.incomingRequests, key)
			t.requestsMu.Unlock()

		case MessageTypeProgress:
			// FRAME 0: empty
			// FRAME 3: request ID (uint16; BE)
			switch {
			case len(msg) != 4:
				log.Warn("websocket<RPC>: PROGRESS: invalid message length")
				return
			case len(msg[0]) != 0:
				log.Warn("websocket<RPC>: PROGRESS: empty sender frame expected")
				return
			case len(msg[3]) != 2:
				log.Warn("websocket<RPC>: PROGRESS: invalid request ID frame received")
				return
			}

			var id rpc.RequestID
			binary.Read(bytes.NewReader(msg[3]), binary.BigEndian, &id)
			t.progressCh <- id

		case MessageTypeStreamFrame:
			// FRAME 0: empty
			// FRAME 3: stream tag (uint16; BE)
			// FRAME 4: frame payload (bytes)
			switch {
			case len(msg) != 5:
				log.Warn("websocket<RPC>: STREAMFRAME: invalid message length")
				return
			case len(msg[0]) != 0:
				log.Warn("websocket<RPC>: STREAMFRAME: empty sender frame expected")
				return
			case len(msg[3]) != 2:
				log.Warn("websocket<RPC>: STREAMFRAME: invalid stream tag frame received")
				return
			case len(msg[4]) == 0:
				log.Warn("websocket<RPC>: STREAMFRAME: empty frame received")
				return
			}

			t.streamingCh <- newStreamFrame(msg)

		case MessageTypeReply:
			// FRAME 0: empty
			// FRAME 3: request ID (uint16; BE)
			// FRAME 4: return code (byte)
			// FRAME 5: return value (object; encoded with MessagePack)
			switch {
			case len(msg) != 6:
				log.Warn("websocket<RPC>: REPLY: invalid message length")
				return
			case len(msg[0]) != 0:
				log.Warn("websocket<RPC>: REPLY: empty sender frame expected")
				return
			case len(msg[3]) != 2:
				log.Warn("websocket<RPC>: REPLY: invalid request ID frame received")
				return
			case len(msg[4]) != 1:
				log.Warn("websocket<RPC>: REPLY: invalid return code frame received")
				return
			}

			t.replyCh <- newReply(msg)

		case MessageTypePing:
			if err := frames.C.Send(t.conn, pongMessage); err != nil {
				t.errorCh <- err
			}

		default:
			log.Warn("websocket<RPC>: Unknown message type received")
		}
	}
}

func (t *Transport) newRequest(msg [][]byte) (*remoteRequest, error) {
	// key = sender^requestId
	key := string(append(msg[0], msg[3]...))
	t.requestsMu.Lock()
	defer t.requestsMu.Unlock()
	if _, ok := t.incomingRequests[key]; ok {
		return nil, ErrDuplicateRequest
	}

	req := newRequest(t, msg)
	t.incomingRequests[key] = req
	return req, nil
}

func (t *Transport) resolveRequest(req *remoteRequest, retCode rpc.ReturnCode, retValue interface{}) error {
	var valueBuffer bytes.Buffer
	if err := codecs.MessagePack.Encode(&valueBuffer, retValue); err != nil {
		return err
	}

	key := string(append(req.msg[0], req.msg[3]...))
	t.requestsMu.Lock()
	delete(t.incomingRequests, key)
	t.requestsMu.Unlock()

	return frames.C.Send(t.conn, [][]byte{
		req.msg[0],
		frameHeader,
		frameReplyMT,
		req.msg[3],
		[]byte{byte(retCode)},
		valueBuffer.Bytes(),
	})
}

// Errors ----------------------------------------------------------------------

var (
	ErrDuplicateRequest = errors.New("duplicate request ID")
	ErrTerminated       = &services.ErrTerminated{"WebSocket RPC transport"}
)
