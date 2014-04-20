// Copyright (c) 2013 The cider AUTHORS
//
// Use of this source code is governed by The MIT License
// that can be found in the LICENSE file.

package pubsub

import "github.com/cider/cider/broker"

type EventSeqNum uint32

type Event interface {
	Kind() []byte
	Seq() []byte
	SetSeq(seq EventSeqNum)

	// For the clients to be able to drop events that are actually their own.
	Publisher() []byte

	// Body returns the raw event object encoded with the configured transport
	// encoding that is the same for all activated transports.
	Body() []byte
}

type EventRecord struct {
	Kind []byte
	Seq  EventSeqNum
}

type Exchange interface {

	// RegisterEndpoint registers the given endpoint with this handler so that
	// it can start receiving events collected by other PubSub endpoints.
	RegisterEndpoint(ep Endpoint)

	// UnregisterEndpoint removes the given endpoint from the set of endpoints
	// known to this handler. It will automatically stop receiving forwarded events.
	UnregisterEndpoint(ep Endpoint)

	// Publish forwards the Event to all other PubSub transport endpoints.
	// It can of course do more, but this functionality should always be there.
	// It never fails, the errors are supposed to be handled in the endpoints
	// where they happen.
	Publish(event Event)

	// RequestSequenceTable asks the handler to send the current sequence numbers
	// for the given kind prefix to the application that requested it.
	EventSequenceTable(kindPrefix []byte) []*EventRecord
}

type Endpoint interface {
	broker.Endpoint

	// Publish publishes an Event on this particular transport endpoint.
	Publish(event Event) error
}
