// Copyright (c) 2013 The meeko AUTHORS
//
// Use of this source code is governed by The MIT License
// that can be found in the LICENSE file.

package pubsub

import (
	"bytes"
	"encoding/binary"
	"errors"
	"unsafe"

	"github.com/meeko/meekod/broker/log"
	"github.com/meeko/meekod/broker/services/pubsub"
	"github.com/meeko/meekod/broker/transports/zmq3/loop"

	"github.com/dmotylev/nutrition"
	zmq "github.com/pebbe/zmq3"
)

// Constants -------------------------------------------------------------------

const Header = "CDR#PUBSUB@01"

const (
	MessageTypeEvent byte = iota
	MessageTypeSeqTable
)

var (
	FrameHeader = []byte(Header)

	FrameEventType    = []byte{MessageTypeEvent}
	FrameSeqTableType = []byte{MessageTypeSeqTable}
)

// Errors ----------------------------------------------------------------------

var (
	ErrConfigIncomplete = errors.New("PubSub endpoint is not fully configured")
	ErrSeqNumNotSet     = errors.New("Event sequence number is not set")
)

//------------------------------------------------------------------------------
// EndpointConfig
//------------------------------------------------------------------------------

type EndpointConfig struct {
	RouterEndpoint string
	RouterSndhwm   int
	RouterRcvhwm   int
	PubSndhwm      int
	PubEndpoint    string
}

func NewEndpointConfig() *EndpointConfig {
	return &EndpointConfig{
		RouterSndhwm: 1000,
		RouterRcvhwm: 1000,
		PubSndhwm:    1000,
	}
}

func (config *EndpointConfig) FeedFromEnv(prefix string) error {
	log.Debug("zmq3<PubSub>: Feeding config from the environment")
	return nutrition.Env(prefix).Feed(config)
}

func (config *EndpointConfig) MustFeedFromEnv(prefix string) *EndpointConfig {
	if err := config.FeedFromEnv(prefix); err != nil {
		panic(err)
	}
	return config
}

func (config *EndpointConfig) IsComplete() bool {
	return config.PubEndpoint != "" && config.RouterEndpoint != ""
}

func (config *EndpointConfig) MustBeComplete() {
	if !config.IsComplete() {
		panic(ErrConfigIncomplete)
	}
}

//------------------------------------------------------------------------------
// Endpoint
//------------------------------------------------------------------------------

type Endpoint struct {
	*loop.MessageLoop
	exchange pubsub.Exchange
}

func NewEndpoint(config *EndpointConfig, exchange pubsub.Exchange) (pubsub.Endpoint, error) {
	// Make sure the configuration is complete.
	if !config.IsComplete() {
		return nil, ErrConfigIncomplete
	}

	log.Debugf("zmq3<PubSub>: Instantiating endpoint using %#v", config)

	// Set up the ROUTER socket.
	router, err := zmq.NewSocket(zmq.ROUTER)
	if err != nil {
		return nil, err
	}

	if err := router.SetSndhwm(config.RouterSndhwm); err != nil {
		router.Close()
		return nil, err
	}

	if err := router.SetRcvhwm(config.RouterRcvhwm); err != nil {
		router.Close()
		return nil, err
	}

	if err := router.Bind(config.RouterEndpoint); err != nil {
		router.Close()
		return nil, err
	}

	// Set up the PUB socket.
	pub, err := zmq.NewSocket(zmq.PUB)
	if err != nil {
		router.Close()
		return nil, err
	}

	if err := pub.SetSndhwm(config.PubSndhwm); err != nil {
		router.Close()
		return nil, err
	}

	if err = pub.Bind(config.PubEndpoint); err != nil {
		router.Close()
		return nil, err
	}

	// Prepare a new Endpoint instance.
	ep := &Endpoint{
		exchange: exchange,
	}

	// Initialise the embedded loop. The ownership of the router socket is passed
	// onto the loop, it takes care of the cleanup on termination.
	messageLoop := loop.New(loop.PollItems{
		{pub, nil},
		{router, ep.handleMessage},
	})

	exchange.RegisterEndpoint(ep)
	ep.MessageLoop = messageLoop
	return ep, nil
}

// pubsub.Endpoint interface ---------------------------------------------------
// What is not here is implemented by the embedded loop.MessageLoop.

func (ep *Endpoint) Publish(event pubsub.Event) error {
	// Make sure the sequence number is set. This should happen in the exchange.
	if len(event.Seq()) != 4 { // XXX: Hardcoded seq length
		return ErrSeqNumNotSet
	}

	// In case the event is actually an event originating in this package,
	// use the quick way - simply reuse the bytes.
	e, ok := event.(Event)
	if ok {
		msg := [][]byte(e)

		// Put the frames into the right output order.
		tmp := msg[0]
		msg[0] = msg[1]
		msg[1] = tmp
		// Send the message. ZeroMQ copies the message into its own buffer.
		err := ep.MessageLoop.DispatchMessage(0, msg)
		// Put the frames into the original order.
		tmp = msg[0]
		msg[0] = msg[1]
		msg[1] = tmp

		return err
	}

	// Otherwise we have to use the pubsub.Event interface methods to get
	// the data and assemble the message manually.
	var seqBuf bytes.Buffer
	if err := binary.Write(&seqBuf, binary.BigEndian, event.Seq()); err != nil {
		return err
	}

	return ep.MessageLoop.DispatchMessage(0, [][]byte{
		event.Kind(),
		event.Publisher(),
		FrameHeader,
		FrameEventType,
		seqBuf.Bytes(),
		event.Body(),
	})
}

func (ep *Endpoint) Close() error {
	ep.exchange.UnregisterEndpoint(ep)
	return ep.MessageLoop.Close()
}

// Processing of incoming messages  --------------------------------------------

func (ep *Endpoint) handleMessage(msg [][]byte) {
	// This is a ROUTER socket, the first frame is always the IDENTITY.
	appName := string(msg[0])

	// Perform some sanity checks at the beginning.
	switch {
	case len(msg) < 4:
		log.Warnf("zmq3<PubSub>: Invalid message received from %s: message too short", appName)
		return
	case !bytes.Equal(msg[2], FrameHeader):
		log.Warnf("zmq3<PubSub>: Invalid message received from %s: header mismatch", appName)
		return
	case len(msg[3]) != 1:
		log.Warnf("zmq3<PubSub>: Invalid message received from %v: invalid message type", appName)
		return
	}

	// Process the message according to its type.
	switch msg[3][0] {
	case MessageTypeEvent:
		// FRAME 0: appName (string)
		// FRAME 1: kind (string)
		// FRAME 2: message header
		// FRAME 3: message type (byte)
		// FRAME 4: empty (used for seq later)
		// FRAME 5: body (bytes)
		if len(msg) != 6 || len(msg[1]) == 0 || len(msg[4]) != 0 {
			log.Warnf("zmq3<PubSub>: Invalid EVENT message received from %v", appName)
			return
		}

		log.Debugf("zmq3<PubSub>: EVENT message received from %v", appName)

		// Forward the event to the exchange.
		ep.exchange.Publish(Event(msg))

	case MessageTypeSeqTable:
		// FRAME 0: appName (string)
		// FRAME 1: kind prefix (string)
		// FRAME 2: message header
		// FRAME 3: message type (byte)
		if len(msg) != 4 || len(msg[1]) == 0 {
			log.Warnf("zmq3<PubSub>: Invalid SEQTABLE message received from %v", appName)
			return
		}

		log.Debugf("zmq3<PubSub>: SEQTABLE message received from %v", appName)

		// Get the event sequence table for the requested kind prefix.
		table := ep.exchange.EventSequenceTable(msg[1])

		// Send the table to the requester.
		ep.sendEventSequenceTable(msg[0], table)

	default:
		log.Warnf("zmq3<PubSub>: Unknown message type received from %v", appName)
	}
}

func (ep *Endpoint) sendEventSequenceTable(receiver []byte, table []*pubsub.EventRecord) error {
	// Preallocate a buffer for writing all the EventSeqNums.
	// Desired capacity = len(table) * sizeof(seq)
	var seqSize uintptr
	if len(table) != 0 {
		seqSize = unsafe.Sizeof(table[0].Seq)
	}
	buf := bytes.NewBuffer(make([]byte, 0, len(table)*int(seqSize)))

	// Preallocate the message and fill the message.
	msg := make([][]byte, 3+2*len(table))
	msg[0] = receiver
	msg[1] = FrameHeader
	msg[2] = FrameSeqTableType
	i := 3
	for _, record := range table {
		binary.Write(buf, binary.BigEndian, record.Seq)
		msg[i] = record.Kind
		msg[i+1] = buf.Bytes()
		i += 2
	}

	// Write the message.
	return ep.MessageLoop.DispatchMessage(1, msg)
}
