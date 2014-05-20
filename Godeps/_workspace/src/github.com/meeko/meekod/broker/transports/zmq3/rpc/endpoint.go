// Copyright (c) 2013 The meeko AUTHORS
//
// Use of this source code is governed by The MIT License
// that can be found in the LICENSE file.

package rpc

import (
	// Stdlib
	"bytes"
	"errors"

	// Meeko
	"github.com/meeko/meekod/broker/log"
	"github.com/meeko/meekod/broker/services/rpc"
	"github.com/meeko/meekod/broker/transports/zmq3/loop"

	// Other
	"github.com/dmotylev/nutrition"
	zmq "github.com/pebbe/zmq3"
)

// Constants -------------------------------------------------------------------

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
	MessageTypeKthxbye
)

var (
	FrameEmpty  = []byte{}
	FrameHeader = []byte(Header)

	FrameRegisterMT    = []byte{MessageTypeRegister}
	FrameUnregisterMT  = []byte{MessageTypeUnregister}
	FrameRequestMT     = []byte{MessageTypeRequest}
	FrameInterruptMT   = []byte{MessageTypeInterrupt}
	FrameProgressMT    = []byte{MessageTypeProgress}
	FrameStreamFrameMT = []byte{MessageTypeStreamFrame}
	FrameReplyMT       = []byte{MessageTypeReply}
	FramePingMT        = []byte{MessageTypePing}
	FramePongMT        = []byte{MessageTypePong}
	FrameKthxbyeMT     = []byte{MessageTypeKthxbye}
)

// Errors ----------------------------------------------------------------------

var ErrConfigIncomplete = errors.New("RPC endpoint is not fully configured")

//------------------------------------------------------------------------------
// EndpointConfig
//------------------------------------------------------------------------------

type EndpointConfig struct {
	Endpoint  string
	Sndhwm    int
	Rcvhwm    int
	Heartbeat *HeartbeatConfig
}

func NewEndpointConfig() *EndpointConfig {
	return &EndpointConfig{
		Sndhwm:    1000,
		Rcvhwm:    1000,
		Heartbeat: newHeartbeatConfig(),
	}
}

func (config *EndpointConfig) FeedFromEnv(prefix string) error {
	log.Debug("zmq3<RPC>: Feeding config from the environment")
	if err := nutrition.Env(prefix).Feed(config); err != nil {
		return err
	}
	return nutrition.Env(prefix + "HEARTBEAT_").Feed(config.Heartbeat)
}

func (config *EndpointConfig) MustFeedFromEnv(prefix string) *EndpointConfig {
	if err := config.FeedFromEnv(prefix); err != nil {
		panic(err)
	}
	return config
}

func (config *EndpointConfig) IsComplete() bool {
	return config.Endpoint != "" && config.Heartbeat != nil
}

func (config *EndpointConfig) MustBeComplete() {
	if !config.IsComplete() {
		panic(ErrConfigIncomplete)
	}
}

//------------------------------------------------------------------------------
// Endpoint
//------------------------------------------------------------------------------

type exchange interface {
	rpc.Exchange
	Pong(appName string)
}

type Endpoint struct {
	*loop.MessageLoop
	exchange exchange
}

func NewEndpoint(config *EndpointConfig, exchange rpc.Exchange) (*Endpoint, error) {
	// Make sure the configuration is complete.
	if !config.IsComplete() {
		return nil, ErrConfigIncomplete
	}

	log.Debugf("zmq3<RPC>: Instantiating endpoint using %#v", config)
	log.Debugf("zmq3<RPC>: Instantiating endpoint using %#v", config.Heartbeat)

	// Set up the 0MQ socket of ROUTER type.
	sock, err := zmq.NewSocket(zmq.ROUTER)
	if err != nil {
		return nil, err
	}

	if config.Sndhwm != 0 {
		err = sock.SetSndhwm(config.Sndhwm)
		if err != nil {
			sock.Close()
			return nil, err
		}
	}

	err = sock.Bind(config.Endpoint)
	if err != nil {
		sock.Close()
		return nil, err
	}

	// Prepare a new Endpoint instance.
	ep := new(Endpoint)

	beat := config.Heartbeat.newHeartbeat(exchange, ep)
	ep.exchange = beat

	// Initialize the embedded MessageLoop, which claims ownership of the socket
	// passed to it and takes care of the cleanup.
	messageLoop := loop.New(loop.PollItems{
		{sock, ep.handleMessage},
	})

	ep.MessageLoop = messageLoop
	return ep, nil
}

// rpc.Endpoint interface ------------------------------------------------------

func (ep *Endpoint) Close() error {
	ep.exchange.UnregisterEndpoint(ep)
	return ep.MessageLoop.Close()
}

func (ep *Endpoint) DispatchRequest(receiver []byte, msg rpc.Request) error {
	// Access the internal message representation if possible.
	if req, ok := msg.(*Request); ok {
		raw := [][]byte(req.msg)
		raw[1] = raw[0]
		raw[0] = receiver

		err := ep.MessageLoop.DispatchMessage(0, raw)

		raw[0] = raw[1]
		raw[1] = FrameEmpty
		return err
	}

	// Construct the message manually otherwise.
	return ep.MessageLoop.DispatchMessage(0, [][]byte{
		receiver,
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

func (ep *Endpoint) DispatchInterrupt(receiver []byte, msg rpc.Interrupt) error {
	// Access the internal message representation if possible.
	if interrupt, ok := msg.(Interrupt); ok {
		raw := [][]byte(interrupt)
		raw[1] = raw[0]
		raw[0] = receiver

		err := ep.MessageLoop.DispatchMessage(0, raw)

		raw[0] = raw[1]
		raw[1] = FrameEmpty
		return err
	}

	// Construct the message manually otherwise.
	return ep.MessageLoop.DispatchMessage(0, [][]byte{
		receiver,
		msg.Sender(),
		FrameHeader,
		FrameInterruptMT,
		msg.TargetRequestId(),
	})
}

func (ep *Endpoint) DispatchProgress(msg rpc.Progress) error {
	// Access the internal message representation if possible.
	if progress, ok := msg.(Progress); ok {
		raw := [][]byte(progress)
		sender := raw[0]
		raw[0] = raw[1]
		raw[1] = FrameEmpty

		err := ep.MessageLoop.DispatchMessage(0, raw)

		raw[1] = raw[0]
		raw[0] = sender
		return err
	}

	// Construct the message manually otherwise.
	return ep.MessageLoop.DispatchMessage(0, [][]byte{
		msg.Receiver(),
		FrameEmpty,
		FrameHeader,
		FrameProgressMT,
		msg.TargetRequestId(),
	})
}

func (ep *Endpoint) DispatchStreamFrame(msg rpc.StreamFrame) error {
	// Access the internal message representation if possible.
	if frame, ok := msg.(StreamFrame); ok {
		raw := [][]byte(frame)
		sender := raw[0]
		raw[0] = raw[1]
		raw[1] = FrameEmpty

		err := ep.MessageLoop.DispatchMessage(0, raw)

		raw[1] = raw[0]
		raw[0] = sender
		return err
	}

	// Construct the message manually otherwise.
	return ep.MessageLoop.DispatchMessage(0, [][]byte{
		msg.Receiver(),
		FrameEmpty,
		FrameHeader,
		FrameStreamFrameMT,
		msg.TargetStreamTag(),
		msg.Body(),
	})
}

func (ep *Endpoint) DispatchReply(msg rpc.Reply) error {
	// Access the internal message representation if possible.
	if reply, ok := msg.(Reply); ok {
		raw := [][]byte(reply)
		sender := raw[0]
		raw[0] = raw[1]
		raw[1] = FrameEmpty

		err := ep.MessageLoop.DispatchMessage(0, raw)

		raw[1] = raw[0]
		raw[0] = sender
		return err
	}

	// Construct the message manually otherwise.
	return ep.MessageLoop.DispatchMessage(0, [][]byte{
		msg.Receiver(),
		FrameEmpty,
		FrameHeader,
		FrameReplyMT,
		msg.TargetRequestId(),
		msg.ReturnCode(),
		msg.ReturnValue(),
	})
}

func (ep *Endpoint) Ping(receiver []byte) error {
	return ep.MessageLoop.DispatchMessage(0, [][]byte{
		receiver,
		FrameEmpty,
		FrameHeader,
		FramePingMT,
	})
}

// Processing of incoming messages  --------------------------------------------

func (ep *Endpoint) handleMessage(msg [][]byte) {
	// This is a ROUTER socket, the first frame is always the identity frame.
	appName := string(msg[0])

	// Perform some sanity checks at the beginning.
	switch {
	case len(msg) < 4:
		warnInvalidMessage(appName, "", "message too short")
		return
	case !bytes.Equal(msg[2], FrameHeader):
		warnInvalidMessage(appName, "", "header mismatch")
		return
	}

	// Process the message according to its type.
	switch msg[3][0] {
	case MessageTypeRegister:
		// FRAME 0: sender
		// FRAME 1: empty
		// FRAME 2: message header
		// FRAME 3: message type
		// FRAME 4: method (string)

		switch {
		case len(msg) != 5:
			warnInvalidMessage(appName, "REGISTER", "invalid message length")
			return
		case len(msg[1]) != 0:
			warnInvalidMessage(appName, "REGISTER", "empty frame expected")
			return
		case len(msg[4]) == 0:
			warnInvalidMessage(appName, "REGISTER", "method frame empty")
			return
		}

		ep.exchange.RegisterMethod(appName, ep, string(msg[4]))

	case MessageTypeUnregister:
		// FRAME 0: sender
		// FRAME 1: empty
		// FRAME 2: message header
		// FRAME 3: message type
		// FRAME 4: method

		switch {
		case len(msg) != 5:
			warnInvalidMessage(appName, "UNREGISTER", "invalid message length")
			return
		case len(msg[1]) != 0:
			warnInvalidMessage(appName, "UNREGISTER", "empty frame expected")
			return
		case len(msg[4]) == 0:
			warnInvalidMessage(appName, "UNREGISTER", "method frame empty")
			return
		}

		ep.exchange.UnregisterMethod(appName, string(msg[4]))

	case MessageTypeRequest:
		// FRAME 0: sender
		// FRAME 1: empty
		// FRAME 2: message header
		// FRAME 3: message type
		// FRAME 4: request ID (uint16, BE)
		// FRAME 5: method
		// FRAME 6: method args object (bytes; marshalled)
		// FRAME 7: stdout stream tag (uint16, BE)
		// FRAME 8: stderr stream tag (uint16, BE)

		switch {
		case len(msg) != 9:
			warnInvalidMessage(appName, "REQUEST", "invalid message length")
			return
		case len(msg[1]) != 0:
			warnInvalidMessage(appName, "REQUEST", "empty frame expected")
			return
		case len(msg[4]) != 2:
			warnInvalidMessage(appName, "REQUEST", "request ID frame invalid")
			return
		case len(msg[5]) == 0:
			warnInvalidMessage(appName, "REQUEST", "method frame empty")
			return
		case len(msg[6]) == 0:
			warnInvalidMessage(appName, "REQUEST", "method args frame empty")
			return
		case len(msg[7]) != 0 && len(msg[7]) != 2:
			warnInvalidMessage(appName, "REQUEST", "stdout stream tag frame invalid")
			return
		case len(msg[8]) != 0 && len(msg[8]) != 2:
			warnInvalidMessage(appName, "REQUEST", "stderr stream tag frame invalid")
			return
		}

		req := &Request{
			msg: msg,
			rejectFunc: func(code byte, reason string) error {
				return ep.DispatchMessage(0, [][]byte{
					msg[0],
					FrameEmpty,
					FrameHeader,
					FrameReplyMT,
					msg[4],
					[]byte{code},
					[]byte(reason),
				})
			},
		}

		ep.exchange.HandleRequest(req, ep)

	case MessageTypeInterrupt:
		// FRAME 0: sender
		// FRAME 1: empty
		// FRAME 2: message header
		// FRAME 3: message type
		// FRAME 4: request ID

		switch {
		case len(msg) != 5:
			warnInvalidMessage(appName, "INTERRUPT", "invalid message length")
			return
		case len(msg[1]) != 0:
			warnInvalidMessage(appName, "INTERRUPT", "empty frame expected")
			return
		case len(msg[4]) != 2:
			warnInvalidMessage(appName, "INTERRUPT", "request ID frame invalid")
			return
		}

		ep.exchange.HandleInterrupt(Interrupt(msg))

	case MessageTypeProgress:
		// FRAME 0: sender
		// FRAME 1: receiver
		// FRAME 2: message header
		// FRAME 3: message type
		// FRAME 4: request ID

		switch {
		case len(msg) != 5:
			warnInvalidMessage(appName, "PROGRESS", "invalid message length")
			return
		case len(msg[1]) == 0:
			warnInvalidMessage(appName, "PROGRESS", "receiver frame empty")
			return
		case len(msg[4]) != 2:
			warnInvalidMessage(appName, "PROGRESS", "request ID frame invalid")
			return
		}

		ep.exchange.HandleProgress(Progress(msg))

	case MessageTypeStreamFrame:
		// FRAME 0: sender
		// FRAME 1: receiver
		// FRAME 2: message header
		// FRAME 3: message type
		// FRAME 4: stream tag (uint16, BE)
		// FRAME 5: frame (bytes)

		switch {
		case len(msg) != 6:
			warnInvalidMessage(appName, "STREAM_FRAME", "invalid message length")
			return
		case len(msg[1]) == 0:
			warnInvalidMessage(appName, "STREAM_FRAME", "receiver frame empty")
			return
		case len(msg[4]) != 2:
			warnInvalidMessage(appName, "STREAM_FRAME", "stream tag frame invalid")
			return
		case len(msg[5]) == 0:
			warnInvalidMessage(appName, "STREAM_FRAME", "frame body frame empty")
			return
		}

		ep.exchange.HandleStreamFrame(StreamFrame(msg))

	case MessageTypeReply:
		// FRAME 0: sender
		// FRAME 1: receiver
		// FRAME 2: message header
		// FRAME 3: message type
		// FRAME 4: request ID
		// FRAME 5: return code (byte)
		// FRAME 6: return value (bytes, method-specific)

		switch {
		case len(msg) != 7:
			warnInvalidMessage(appName, "REPLY", "invalid message length")
			return
		case len(msg[1]) == 0:
			warnInvalidMessage(appName, "REPLY", "receiver frame empty")
			return
		case len(msg[4]) != 2:
			warnInvalidMessage(appName, "REPLY", "request ID frame invalid")
			return
		case len(msg[5]) != 1:
			warnInvalidMessage(appName, "REPLY", "return code frame invalid")
			return
		}

		ep.exchange.HandleReply(Reply(msg))

	case MessageTypePong:
		// FRAME 0: sender
		// FRAME 1: empty
		// FRAME 2: message header
		// FRAME 3: message type

		switch {
		case len(msg) != 4:
			warnInvalidMessage(appName, "PONG", "invalid message length")
			return
		case len(msg[1]) != 0:
			warnInvalidMessage(appName, "PONG", "empty frame expected")
			return
		}

		ep.exchange.Pong(appName)

	case MessageTypeKthxbye:
		// FRAME 0: sender
		// FRAME 1: empty
		// FRAME 2: message header
		// FRAME 3: message type

		if len(msg[1]) != 0 {
			warnInvalidMessage(appName, "KTHXBYE", "empty frame expected")
			return
		}

		ep.exchange.UnregisterApp(appName)

	default:
		warnInvalidMessage(appName, "", "unknown message type")
		return
	}
}

// Utilities -------------------------------------------------------------------

func warnInvalidMessage(app string, messageType string, message string) {
	log.Warnf("zmq3<RPC>: Invalid %s message received from %s: %s", messageType, app, message)
}
