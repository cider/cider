// Copyright (c) 2013 The cider AUTHORS
//
// Use of this source code is governed by The MIT License
// that can be found in the LICENSE file.

package broker

import (
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/cider/cider/broker/log"
)

//------------------------------------------------------------------------------
// Broker
//------------------------------------------------------------------------------

const Version = "0.0.1"

const (
	ErrorWindowSize             = time.Second
	NumAllowedConsecutiveErrors = 3
)

type EndpointFactory func() (Endpoint, error)

type EndpointCrashReport struct {
	FactoryId string
	Dropped   bool
	Error     error
}

// Broker functions as a supervisor for the registered Endpoints. It uses the
// registered endpoint factory methods to create instances, which are then
// supervised and restarted on crash, if that does not happen too often.
//
// It gives some guarantees to the endpoint objects, namely
//
//   * ListenAndServe is called at most once.
//   * Terminate is called at most once.
//
type Broker struct {
	// User-defined stuff.
	factories map[string]EndpointFactory
	monitorCh chan<- *EndpointCrashReport

	// State management stuff.
	state     brokerState
	termCh    chan struct{}
	termAckCh chan struct{}
	mu        *sync.Mutex
}

type brokerState int

const (
	stateInitialised brokerState = iota
	stateServing
	stateTerminated
)

var stateStrings = [...]string{
	"INITIALISED",
	"SERVING",
	"TERMINATED",
}

func (s brokerState) String() string {
	return stateStrings[int(s)]
}

// Broker constructor function.
func New() *Broker {
	return &Broker{
		factories: make(map[string]EndpointFactory),
		termCh:    make(chan struct{}),
		termAckCh: make(chan struct{}),
		mu:        new(sync.Mutex),
	}
}

// RegisterEndpointFactory adds the Endpoint factory to the set of factories
// that are invoked by the broker in the call to ListenAndServe to instantiate
// service endpoints.
//
// Endpoints are restarted when they crash, provided that it is not happening
// too often and too frequently.
//
// This methods panics if called after ListenAndServe.
func (b *Broker) RegisterEndpointFactory(identifier string, factory EndpointFactory) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.ensureState(stateInitialised)

	log.Infof("[broker] Registered endpoint factory %v", identifier)
	b.factories[identifier] = factory
}

// Monitor tells Broker to send all internal endpoint crash reports to the
// requested channel. This is the only way how to get endpoint errors out of
// Broker.
//
// If there is no interest in the internal errors. Wait can be used to block
// until all the registered endpoints crashed or Close is called.
//
// The channel passed to Monitor is closed at the same time Closed() channel
// is closed. It can be used interchangeably.
//
// This methods panics if called after ListenAndServe.
func (b *Broker) Monitor(monitorCh chan<- *EndpointCrashReport) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.ensureState(stateInitialised)

	log.Info("[broker] Monitoring enabled")
	b.monitorCh = monitorCh
}

// ListenAndServe instantiates service endpoints by invoking their respective
// factory methods that were added by RegisterEndpointFactory. Once an endpoint
// is instantiated, its ListenAndServe is called. If an error is returned from
// the endpoint-level ListenAndServe, the relevant endpoint is replaced by a new
// instance using the same factory function as before.
//
// The broker keeps restarting crashed endpoints as long as it is not happening
// to frequently.
//
// This method panics if it is called multiple times.
func (b *Broker) ListenAndServe() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.ensureState(stateInitialised)

	// Close exitStatusCh when all the endpoints are closed.
	var wg sync.WaitGroup
	wg.Add(len(b.factories))
	go func() {
		wg.Wait()
		// No need to lock, Monitor cannot be called by the time this
		// goroutine is running.
		if b.monitorCh != nil {
			close(b.monitorCh)
		}
		// Close the termination acknowledgement channel.
		close(b.termAckCh)
	}()

	// Run every endpoint supervising loop in its own goroutine.
	for identifier, factoryMethod := range b.factories {
		go func(id string, factory EndpointFactory) {
			defer func() {
				// Do not propagate panics.
				if r := recover(); r != nil {
					st := make([]byte, 4096)
					n := runtime.Stack(st, false)
					log.Errorf("[broker] Endpoint %v panicked: %v\n%v", id, r, string(st[:n]))
				}
				// Signal that this endpoint was dropped/terminated.
				log.Infof("[broker] Endpoint %v terminated", id)
				wg.Done()
			}()

			var (
				ep         Endpoint
				errCounter uint
				err        error
			)
			for {
				// Drop the endpoint in case there are too many errors.
				if errCounter == NumAllowedConsecutiveErrors {
					log.Warnf("[broker] Endpoint %v reached the error threshold, dropping ...", id)
					if b.monitorCh != nil {
						b.monitorCh <- &EndpointCrashReport{
							FactoryId: id,
							Dropped:   true,
						}
					}
					return
				}

				// Instantiate the endpoint.
				log.Infof("[broker] Instantiating endpoint %v", id)
				ep, err = factory()
				if err != nil {
					log.Errorf("[broker] Failed to instantiate %v: %v", id, err)
					errCounter++
					if b.monitorCh != nil {
						b.monitorCh <- &EndpointCrashReport{
							FactoryId: id,
							Error:     err,
						}
					}
					continue
				}

				// Make sure the endpoint is closed when the broker terminates.
				watcherReturnCh := make(chan struct{})
				go func() {
					select {
					case <-b.termCh:
						log.Infof("[broker] Terminating endpoint %v", id)
						if err := ep.Close(); err != nil {
							log.Errorf("[broker] Failed to close endpoint %v: %v", id, err)
						}
					case <-watcherReturnCh:
						return
					}
				}()

				// Block in ListenAndServe.
				listenTimestamp := time.Now()

				log.Debugf("[broker] Endpoint %v entering ListenAndServe", id)
				err = ep.ListenAndServe()
				log.Debugf("[broker] Endpoint %v leaving ListenAndServe with error=%v", id, err)

				// Make the background watcher goroutine return.
				close(watcherReturnCh)

				// Exit or increment the error counter.
				if err == nil {
					return
				}
				if err != nil {
					// Close the endpoint that has just crashed.
					if err := ep.Close(); err != nil {
						log.Debugf("[broker] Failed to close endpoint %v: %v", id, err)
					}
					// Increment the error counter.
					log.Debugf("[broker] Incrementing error counter for %v", id)
					errCounter++
				}

				// Decrement the error counter in case the error didn't happen too soon.
				if time.Now().Sub(listenTimestamp) > ErrorWindowSize {
					if errCounter > 0 {
						log.Debugf("[broker] Decrementing error counter for %v", id)
						errCounter--
					}
				}
			}
		}(identifier, factoryMethod)
	}

	b.state = stateServing
}

// Terminate terminates all the registered endpoints by invoking their
// respective Terminate methods. This way of shutting down is the clean one,
// the endpoints are supposed to perform some cleanup optionally and stop.
//
// Terminate does not block, use Wait for blocking until all the endpoints
// are terminated.
func (b *Broker) Terminate() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.ensureState(stateServing)

	log.Info("[broker] Terminating...")
	close(b.termCh)

	b.state = stateTerminated
	b.Wait()
}

// Terminated returns a channel that is closed when Terminate returns.
func (b *Broker) Terminated() <-chan struct{} {
	return b.termAckCh
}

// Wait blocks until Terminated channel is closed.
func (b *Broker) Wait() {
	<-b.Terminated()
}

func (b *Broker) ensureState(state brokerState) {
	if b.state != state {
		panic(&ErrInvalidState{state, b.state})
	}
}

// Errors ----------------------------------------------------------------------

// ErrInvalidState is returned when Broker methods are called in a wrong order.
type ErrInvalidState struct {
	expected brokerState
	current  brokerState
}

func (err ErrInvalidState) Error() string {
	return fmt.Sprintf("Invalid broker state: expected %v, got %v", err.expected, err.current)
}

//------------------------------------------------------------------------------
// Default Broker instance
//------------------------------------------------------------------------------

// Default package-level Broker instance.
var DefaultBroker = New()

// RegisterEndpointFactory calls DefaultBroker.RegisterEndpointFactory
func RegisterEndpointFactory(factoryId string, factory EndpointFactory) {
	DefaultBroker.RegisterEndpointFactory(factoryId, factory)
}

// Monitor calls DefaultBroker.Monitor
func Monitor(monitorCh chan<- *EndpointCrashReport) {
	DefaultBroker.Monitor(monitorCh)
}

// ListenAndServe calls DefaultBroker.ListenAndServe
func ListenAndServe() {
	DefaultBroker.ListenAndServe()
}

// Terminate calls DefaultBroker.Terminate
func Terminate() {
	DefaultBroker.Terminate()
}

// Terminated calls DefaultBroker.Terminated
func Terminated() <-chan struct{} {
	return DefaultBroker.Terminated()
}

// Wait calls DefaultBroker.Wait
func Wait() {
	DefaultBroker.Wait()
}
