// Copyright (c) 2013 The go-meeko AUTHORS
//
// Use of this source code is governed by The MIT License
// that can be found in the LICENSE file.

package pubsub

import "github.com/meeko/go-meeko/meeko/services"

//------------------------------------------------------------------------------
// Transport
//------------------------------------------------------------------------------

type Transport interface {
	// Inherit some basic functionality.
	services.Transport

	// Publish does exactly what the name says - it publishes the given event
	// object under eventKind.
	//
	// eventObject must be marshallable by github.com/ugorji/go/codec.
	Publish(eventKind string, eventObject interface{}) error

	// Subscribe sets this transport's event filter to receive all events
	// having their kind starting with eventKindPrefix.
	Subscribe(eventKindPrefix string) error

	// Unsubscribe does exactly the opposite to Subscribe. It cannot be,
	// however, called with any random event kind prefix. It must be one of the
	// prefixes that were used in a previous call to Subscribe.
	Unsubscribe(eventKindPrefix string) error

	// EventChan returns a channel that can be used for receiving events from
	// this transport, events that this transport is subscribed for.
	EventChan() <-chan Event

	// EventSeqTableChan returns a channel that can be used for receiving sync
	// messages from the broker. There messages are basically tables containing
	// the current event sequence numbers for specific events at the time that
	// the event table request reached the broker.
	EventSeqTableChan() <-chan EventSeqTable

	// ErrChan returns a channel for emitting internal transport errors.
	// Any error sent to this channel is treated as unrecoverable and makes
	// the service using the transport terminate.
	ErrorChan() <-chan error
}

// Event represents an event received on a transport.
type Event interface {
	// Kind returns the event kind this event was published as.
	Kind() string

	// Seq returns this event's sequence number.
	Seq() EventSeqNum

	// Unmarshal unmarshalls the received event into dst, which must support
	// decoding using github.com/ugorji/go/codec.
	Unmarshal(dst interface{}) error
}

type (
	EventSeqNum   uint32
	EventSeqTable map[string]EventSeqNum
)
