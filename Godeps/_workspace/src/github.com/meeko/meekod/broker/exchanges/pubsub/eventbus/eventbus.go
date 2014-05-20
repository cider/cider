// Copyright (c) 2013 The meeko AUTHORS
//
// Use of this source code is governed by The MIT License
// that can be found in the LICENSE file.

package eventbus

import (
	"sync"

	"github.com/meeko/meekod/broker/log"
	"github.com/meeko/meekod/broker/services/pubsub"

	"github.com/tchap/go-patricia/patricia"
)

//------------------------------------------------------------------------------
// EventBus implements pubsub.Exchange
//------------------------------------------------------------------------------

type EventBus struct {
	trie        *patricia.Trie
	trieRWMutex *sync.RWMutex

	endpoints        map[pubsub.Endpoint]bool
	endpointsRWMutex *sync.RWMutex
}

func New() pubsub.Exchange {
	return &EventBus{
		trie:             patricia.NewTrie(),
		trieRWMutex:      new(sync.RWMutex),
		endpoints:        make(map[pubsub.Endpoint]bool),
		endpointsRWMutex: new(sync.RWMutex),
	}
}

func (bus *EventBus) RegisterEndpoint(ep pubsub.Endpoint) {
	bus.endpointsRWMutex.Lock()
	bus.endpoints[ep] = true
	bus.endpointsRWMutex.Unlock()
}

func (bus *EventBus) UnregisterEndpoint(ep pubsub.Endpoint) {
	bus.endpointsRWMutex.Lock()
	delete(bus.endpoints, ep)
	bus.endpointsRWMutex.Unlock()
}

func (bus *EventBus) Publish(event pubsub.Event) {
	bus.trieRWMutex.Lock()

	// Find the record for the relevant event kind.
	var record *pubsub.EventRecord
	item := bus.trie.Get(event.Kind())
	if item == nil {
		// If there is no record yet, create it (with seq number set to 0).
		log.Debugf("EventBus: Inserting new event kind record for %q", event.Kind())
		record = &pubsub.EventRecord{Kind: event.Kind()}
		bus.trie.Insert(event.Kind(), record)
		log.Debugf("%#v\n", bus.trie)
	} else {
		record = item.(*pubsub.EventRecord)
	}

	// Increment the sequence number and update the event object.
	record.Seq++
	log.Debugf("EventBus: Event sequence number for %q incremented to %v", event.Kind(), record.Seq)
	event.SetSeq(record.Seq)

	// Forward to all the registered endpoints.
	bus.endpointsRWMutex.RLock()
	for ep := range bus.endpoints {
		if err := ep.Publish(event); err != nil {
			log.Errorf("EventBus: Failed to publish event: %v", err)
		}
	}

	bus.endpointsRWMutex.RUnlock()
	bus.trieRWMutex.Unlock()
}

func (bus *EventBus) EventSequenceTable(kindPrefix []byte) []*pubsub.EventRecord {
	bus.trieRWMutex.RLock()
	records := make([]*pubsub.EventRecord, 0, 8)

	log.Debugf("Requesting sequence table for %q", kindPrefix)

	bus.trie.VisitSubtree(kindPrefix, func(prefix patricia.Prefix, item patricia.Item) error {
		records = append(records, item.(*pubsub.EventRecord))
		return nil
	})

	log.Debugf("EventBus: Sending event sequence table containing %v item(s)", len(records))
	bus.trieRWMutex.RUnlock()
	return records
}
