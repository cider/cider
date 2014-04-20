// Copyright (c) 2013 The go-cider AUTHORS
//
// Use of this source code is governed by The MIT License
// that can be found in the LICENSE file.

package pubsub

import (
	// Stdlib
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	// Cider
	"github.com/cider/go-cider/cider/services"

	// Others
	log "github.com/cihub/seelog"
	"github.com/tchap/go-patricia/patricia"
)

//------------------------------------------------------------------------------
// Service
//------------------------------------------------------------------------------

type (
	EventHandler  func(event Event)
	EventListener uint16
)

// Service represents a PubSub service instance.
//
// All methods are thread-safe if not stated otherwise.
type Service struct {
	// The underlying transport being used by this Service instance.
	transport Transport

	// For processing of incoming events.
	trie                *patricia.Trie
	trieItemsByListener map[EventListener]*trieItem
	nextListener        EventListener
	seqNumsByEventKind  map[string]EventSeqNum

	// monitorCh is defined by the user.
	monitorCh chan<- error

	// For clean termination process.
	numRunningHandlers int32
	handlerReturnedCh  chan bool
	abortCh            chan error
	err                error
	closedCh           chan struct{}

	// For synchronizatin where user method calls could clash with internal
	// asynchronous operations.
	mu *sync.Mutex
}

// NewService uses factory to construct a Transport instance that is then used
// for creating a Service instance.
//
// factory can panic without any worries, it just makes NewService return an
// error, so some if errs can be saved.
func NewService(factory func() (Transport, error)) (srv *Service, err error) {
	defer func() {
		if r := recover(); r != nil {
			if ex, ok := r.(error); ok {
				err = ex
			} else {
				err = services.ErrFactoryPanic
			}
		}
	}()

	transport, err := factory()
	if err != nil {
		return nil, err
	}

	srv = &Service{
		transport:           transport,
		trie:                patricia.NewTrie(),
		trieItemsByListener: make(map[EventListener]*trieItem),
		seqNumsByEventKind:  make(map[string]EventSeqNum),
		handlerReturnedCh:   make(chan bool),
		abortCh:             make(chan error),
		closedCh:            make(chan struct{}),
		mu:                  new(sync.Mutex),
	}

	go srv.loop()
	return
}

// Publish sends eventObject to Cider and thus publishes it for other apps.
//
// eventObject must be marshallable by github.com/ugorji/go/codec.
func (srv *Service) Publish(eventKind string, eventObject interface{}) error {
	if err := srv.transport.Publish(eventKind, eventObject); err != nil {
		return srv.abort(err)
	}
	return nil
}

// Subscribe registers a handler for events starting with eventKindPrefix.
// There can be multiple handlers registered for given event kind prefix, but
// Subscribe does not and cannot check for handler duplicates, so do not try
// to subscribe the same handler for the same event prefix multiple times
// because it will work.
//
// EventListener returned by this method can be later used to remove the handler
// by calling RemoveListener method.
func (srv *Service) Subscribe(eventKindPrefix string, handler EventHandler) (EventListener, error) {
	srv.mu.Lock()
	defer srv.mu.Unlock()

	listener, err := srv.registerHandler(eventKindPrefix, handler)
	if err != nil {
		return 0, err
	}

	if err := srv.transport.Subscribe(eventKindPrefix); err != nil {
		return 0, srv.abort(err)
	}

	return listener, nil
}

// Unsubscribe cancels all subscriptions with the given event kind prefix and
// removes all assigned event handlers.
func (srv *Service) Unsubscribe(eventKindPrefix string) error {
	srv.mu.Lock()
	defer srv.mu.Unlock()

	srv.unregisterPrefix(eventKindPrefix)
	if err := srv.transport.Unsubscribe(eventKindPrefix); err != nil {
		return srv.abort(err)
	}

	return nil
}

// RemoveListener unregisters listener and possibly unsubscribes from the kind
// prefix that listener was listening for. That may fail and return an error.
func (srv *Service) RemoveListener(listener EventListener) error {
	srv.mu.Lock()
	defer srv.mu.Unlock()

	if prefix, ok := srv.unregisterListener(listener); ok {
		if err := srv.transport.Unsubscribe(prefix); err != nil {
			return srv.abort(err)
		}
	}

	return nil
}

// Monitor registers errChan for receiving service errors not really connected
// to the transport itself, but rather to some bad service conditions.
//
// Possible error types that can be received on this channel:
//   - *ErrEventSequenceGap - some events were missed due to transport overload
func (srv *Service) Monitor(errChan chan<- error) {
	srv.mu.Lock()
	srv.monitorCh = errChan
	srv.mu.Unlock()
}

// Close terminated the service as well as the underlying transport.
func (srv *Service) Close() error {
	err := srv.transport.Close()
	srv.abort(err)
	return err
}

// Closed return a channel that is closed once the service is terminated.
func (srv *Service) Closed() <-chan struct{} {
	return srv.closedCh
}

// Wait blocks until the service is terminated and return the last
// unrecoverable error encountered.
func (srv *Service) Wait() error {
	<-srv.Closed()
	return srv.err
}

func (srv *Service) loop() {
	for {
		select {
		// For receiving of incoming event messages.
		case event := <-srv.transport.EventChan():
			srv.invokeHandlers(event)

		// For receiving of incoming event sequence synchronization messages.
		case seqTable := <-srv.transport.EventSeqTableChan():
			srv.updateEventSeqNums(seqTable)

		// For receiving of internal transport errors.
		case err := <-srv.transport.ErrorChan():
			srv.abort(err)

		// For receiving notifications about returning event handlers.
		case <-srv.handlerReturnedCh:
			atomic.AddInt32(&srv.numRunningHandlers, -1)

		// Receive on abortCh means that we want to terminate, so just wait
		// for all the handlers to terminate and close closedCh.
		case err := <-srv.abortCh:
			if srv.err == nil {
				srv.err = err
			}
			for {
				running := atomic.LoadInt32(&srv.numRunningHandlers)
				if running == 0 {
					close(srv.closedCh)
					return
				}
				<-srv.handlerReturnedCh
			}
		}
	}
}

func (srv *Service) abort(err error) error {
	go func() {
		select {
		case srv.abortCh <- err:
		case <-srv.closedCh:
		}
	}()
	return err
}

// Event handlers management ---------------------------------------------------

type trieItem struct {
	kindPrefix string
	records    []*handlerRecord
}

type handlerRecord struct {
	listener EventListener
	handler  EventHandler
}

// registerHandler inserts handler into the handlers tree and returns
// EventListener identifying the handler.
func (srv *Service) registerHandler(kindPrefix string, handler EventHandler) (EventListener, error) {
	listener, err := srv.nextListenerHandle()
	if err != nil {
		return 0, err
	}

	// Try to insert a new node.
	key := patricia.Prefix(kindPrefix)
	record := &handlerRecord{listener, handler}
	if item := srv.trie.Get(key); item == nil {
		// If kindPrefix is not registered yet, update the item index.
		itm := &trieItem{
			kindPrefix: kindPrefix,
			records:    []*handlerRecord{record},
		}
		srv.trie.Insert(key, itm)
		srv.trieItemsByListener[listener] = itm
		return listener, nil
	} else {
		// If the relevant kind prefix node exists, append the record.
		itm := item.(*trieItem)
		itm.records = append(itm.records, record)
		return listener, nil
	}
}

// unregisterListener removes listener and returns a string identifying the
// subscription that is to be canceled. Empty string means no cancelation.
func (srv *Service) unregisterListener(listener EventListener) (prefix string, ok bool) {
	item, ok := srv.trieItemsByListener[listener]
	if !ok {
		return "", false
	}

	if len(item.records) == 1 {
		if ok := srv.trie.Delete(patricia.Prefix(item.kindPrefix)); !ok {
			panic("Kind prefix unexpectedly not found in the kinds trie")
		}
		return item.kindPrefix, true
	} else {
		for i, record := range item.records {
			if record.listener == listener {
				item.records = append(item.records[:i], item.records[i+1:]...)
				return item.kindPrefix, true
			}
		}
	}

	panic("Listener not found in the kinds trie")
}

// unregisterPrefix removes all the listeners matching kindPrefix.
func (srv *Service) unregisterPrefix(kindPrefix string) {
	// Free trieItemsByListener.
	srv.trie.VisitSubtree(
		patricia.Prefix(kindPrefix),
		func(prefix patricia.Prefix, item patricia.Item) error {
			for _, record := range item.(*trieItem).records {
				delete(srv.trieItemsByListener, record.listener)
			}
			return nil
		})

	// Drop part of the kinds trie.
	srv.trie.DeleteSubtree(patricia.Prefix(kindPrefix))
}

func (srv *Service) nextListenerHandle() (EventListener, error) {
	begin := srv.nextListener - 1
	for next := srv.nextListener; next != begin; next++ {
		if _, ok := srv.trieItemsByListener[next]; !ok {
			srv.nextListener = next + 1
			return next, nil
		}
	}
	return 0, ErrListenerHandlesDepleted
}

func (srv *Service) updateEventSeqNums(seqTable EventSeqTable) {
	srv.mu.Lock()
	for k, v := range seqTable {
		log.Debugf("zmq3<PubSub>: Setting event sequence number for %v to %v", k, v)
		srv.updateSeqNum(k, v)
	}
	srv.mu.Unlock()
}

func (srv *Service) updateSeqNum(eventKind string, seqNum EventSeqNum) {
	current, ok := srv.seqNumsByEventKind[eventKind]

	// Update the current sequence number.
	srv.seqNumsByEventKind[eventKind] = seqNum

	// Return if there was no sequence number before. That means that an event
	// was received before the reply to the relevant event sequence table request.
	if !ok {
		return
	}

	// Sequence number matches, return.
	if seqNum == current+1 {
		return
	}

	// Otherwise there were some events lost.
	//
	// No need to srv.mu.Lock() here, the lock is already being held by
	// the calling function (it's updateEventSeqNums or invokeHandlers).
	if srv.monitorCh != nil {
		srv.monitorCh <- &ErrEventSequenceGap{
			EventKind:   eventKind,
			ExpectedSeq: current + 1,
			ReceivedSeq: seqNum,
		}
	}
}

// Event handlers invocation ---------------------------------------------------

func (srv *Service) invokeHandlers(event Event) {
	srv.mu.Lock()
	srv.updateSeqNum(event.Kind(), event.Seq())
	srv.trie.VisitPrefixes(
		patricia.Prefix(event.Kind()),
		func(prefix patricia.Prefix, item patricia.Item) error {
			for _, record := range item.(*trieItem).records {
				srv.runHandler(record.handler, event)
			}
			return nil
		})
	srv.mu.Unlock()
}

func (srv *Service) runHandler(handler EventHandler, event Event) {
	atomic.AddInt32(&srv.numRunningHandlers, 1)
	go func() {
		defer func() {
			recover()
			srv.handlerReturnedCh <- true
		}()
		handler(event)
	}()
}

// Errors ----------------------------------------------------------------------

type ErrEventSequenceGap struct {
	EventKind   string
	ExpectedSeq EventSeqNum
	ReceivedSeq EventSeqNum
}

func (err *ErrEventSequenceGap) Error() string {
	return fmt.Sprintf("Event sequence gap detected for %v: expected=%v, got %v",
		err.EventKind, err.ExpectedSeq, err.ReceivedSeq)
}

var (
	ErrListenerHandlesDepleted = errors.New("EventListener handles depleted")
)
