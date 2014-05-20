// Copyright (c) 2013 The go-meeko AUTHORS
//
// Use of this source code is governed by The MIT License
// that can be found in the LICENSE file.

package rpc

import (
	// Stdlib
	"errors"
	"sync"

	// Meeko broker
	"github.com/meeko/meekod/broker/services/rpc"

	// Meeko client
	"github.com/meeko/go-meeko/meeko/services"
	client "github.com/meeko/go-meeko/meeko/services/rpc"

	// Others
	log "github.com/cihub/seelog"
)

const CommandChannelBufferSize = 1000

// Transport implements client.Transport so it can be used as the underlying
// transport for a Meeko client instance.
//
// This particular transport is a bit special. It implements inproc transport
// that uses channels. It is suitable for clients that intend to live in the
// same process as Meeko.
//
// For using Transport as a service Endpoint for the Meeko broker, check the
// AsEndpoint method.
type Transport struct {
	ex       rpc.Exchange
	identity string

	// Requests that are being passed through this transport.
	incomingRequests map[string]*remoteRequest
	requestsMu       *sync.Mutex

	// endpoint makes Transport look like rpc.Endpoint
	// It just delegates calls to Transport.
	endpoint *endpointAdapter

	// Channel for forwarding data to the broker.
	cmdCh chan client.Command

	// Channels for forwarding data to the client.
	requestCh   chan client.RemoteRequest
	progressCh  chan client.RequestID
	streamingCh chan client.StreamFrame
	replyCh     chan client.RemoteCallReply
	errorCh     chan error

	// Termination management
	closedCh chan struct{}
}

// NewTransport creates a new Transport instance.
func NewTransport(identity string, exchange rpc.Exchange) *Transport {
	t := &Transport{
		ex:               exchange,
		identity:         identity,
		incomingRequests: make(map[string]*remoteRequest),
		requestsMu:       new(sync.Mutex),
		cmdCh:            make(chan client.Command, CommandChannelBufferSize),
		requestCh:        make(chan client.RemoteRequest),
		progressCh:       make(chan client.RequestID),
		streamingCh:      make(chan client.StreamFrame),
		replyCh:          make(chan client.RemoteCallReply),
		errorCh:          make(chan error),
		closedCh:         make(chan struct{}),
	}

	t.endpoint = &endpointAdapter{t}
	go t.loop()
	return t
}

// AsEndpoint returns an adapter that makes Transport look like rpc.Endpoint.
func (t *Transport) AsEndpoint() rpc.Endpoint {
	return t.endpoint
}

// client.Transport interface --------------------------------------------------

func (t *Transport) RegisterMethod(cmd client.RegisterCmd) {
	log.Debugf("inproc<RPC>: registering %q", cmd.Method())
	t.exec(cmd)
}

func (t *Transport) UnregisterMethod(cmd client.UnregisterCmd) {
	log.Debugf("inproc<RPC>: unregistering %q", cmd.Method())
	t.exec(cmd)
}

func (t *Transport) RequestChan() <-chan client.RemoteRequest {
	return t.requestCh
}

func (t *Transport) Call(cmd client.CallCmd) {
	log.Debugf("inproc<RPC>: calling %q", cmd.Method())
	t.exec(cmd)
}

func (t *Transport) Interrupt(cmd client.InterruptCmd) {
	log.Debugf("inproc<RPC>: interrupting %v", cmd.TargetRequestId())
	t.exec(cmd)
}

func (t *Transport) ProgressChan() <-chan client.RequestID {
	return t.progressCh
}

func (t *Transport) StreamFrameChan() <-chan client.StreamFrame {
	return t.streamingCh
}

func (t *Transport) ReplyChan() <-chan client.RemoteCallReply {
	return t.replyCh
}

func (t *Transport) ErrorChan() <-chan error {
	return t.errorCh
}

type closeCmd chan error

func (cmd closeCmd) Type() int {
	return client.CmdClose
}

func (cmd closeCmd) ErrorChan() chan<- error {
	return (chan error)(cmd)
}

func (t *Transport) Close() error {
	errCh := make(chan error, 1)
	t.exec(closeCmd(errCh))
	if err := <-errCh; err != nil {
		return err
	}

	t.Wait()
	return nil
}

func (t *Transport) Closed() <-chan struct{} {
	return t.closedCh
}

func (t *Transport) Wait() error {
	<-t.Closed()
	return nil
}

// Internal command loop -------------------------------------------------------

func (t *Transport) exec(cmd client.Command) {
	select {
	case t.cmdCh <- cmd:
	case <-t.Closed():
		cmd.ErrorChan() <- ErrTerminated
	}
}

func (t *Transport) loop() {
	for {
		cmd := <-t.cmdCh

		switch cmd.Type() {
		case client.CmdRegister:
			cd := cmd.(client.RegisterCmd)
			cmd.ErrorChan() <- t.ex.RegisterMethod(t.identity, t.endpoint, cd.Method())

		case client.CmdUnregister:
			cd := cmd.(client.UnregisterCmd)
			t.ex.UnregisterMethod(t.identity, cd.Method())
			cmd.ErrorChan() <- nil

		case client.CmdCall:
			cd := cmd.(client.CallCmd)
			req, err := newRPCRequest(t, cd)
			if err != nil {
				cmd.ErrorChan() <- err
				return
			}

			t.ex.HandleRequest(req, t.endpoint)
			cmd.ErrorChan() <- nil

		case client.CmdInterrupt:
			cd := cmd.(client.InterruptCmd)
			t.ex.HandleInterrupt(newRPCInterrupt(t.identity, cd.TargetRequestId()))
			cmd.ErrorChan() <- nil

		case client.CmdClose:
			t.ex.UnregisterEndpoint(t.endpoint)
			cmd.ErrorChan() <- nil

			close(t.errorCh)
			close(t.closedCh)
			return
		}
	}
}

// Private methods -------------------------------------------------------------

func (t *Transport) sendProgress(receiver, targetRequestId []byte) {
	t.ex.HandleProgress(&rpcProgress{
		sender:          []byte(t.identity),
		receiver:        receiver,
		targetRequestId: targetRequestId,
	})
}

func (t *Transport) sendStreamFrame(receiver, targetStreamTag, payload []byte) {
	t.ex.HandleStreamFrame(&rpcStreamFrame{
		sender:          []byte(t.identity),
		receiver:        receiver,
		targetStreamTag: targetStreamTag,
		body:            payload,
	})
}

func (t *Transport) newRemoteRequest(msg rpc.Request) (*remoteRequest, error) {
	key := string(append(msg.Sender(), msg.Id()...))
	request := newRemoteRequest(t, msg)

	t.requestsMu.Lock()
	if _, ok := t.incomingRequests[key]; ok {
		log.Warnf("inproc<RPC>: newRemoteRequest: duplicate request key: %q", key)
		t.requestsMu.Unlock()
		return nil, ErrDuplicateRequest
	}

	t.incomingRequests[key] = request
	t.requestsMu.Unlock()
	return request, nil
}

func (t *Transport) unregisterRequest(req *remoteRequest) {
	key := string(append(req.msg.Sender(), req.msg.Id()...))

	t.requestsMu.Lock()
	delete(t.incomingRequests, key)
	t.requestsMu.Unlock()
}

func (t *Transport) interruptRequest(sender []byte, targetRequestId []byte) {
	key := string(append(sender, targetRequestId...))

	t.requestsMu.Lock()
	request, ok := t.incomingRequests[key]
	if !ok {
		log.Warnf("inproc<RPC>: interruptRequest: no request found for key %q", key)
		t.requestsMu.Unlock()
		return
	}
	t.requestsMu.Unlock()

	request.interrupt()
}

func (t *Transport) resolveRequest(receiver, requestId, returnCode, returnValue []byte) error {
	key := string(append(receiver, requestId...))

	t.requestsMu.Lock()
	if _, ok := t.incomingRequests[key]; !ok {
		log.Warnf("inproc<RPC>: resolveRequest: no request found for key %q", key)
		t.requestsMu.Unlock()
		return ErrResolved
	}

	delete(t.incomingRequests, key)
	t.requestsMu.Unlock()

	log.Debugf("inproc<RPC>: resolving %q", key)
	t.ex.HandleReply(&rpcReply{
		sender:          []byte(t.identity),
		receiver:        receiver,
		targetRequestId: requestId,
		returnCode:      returnCode,
		returnValue:     returnValue,
	})
	return nil
}

// rpc.Endpoint adapter --------------------------------------------------------

type endpointAdapter struct {
	t *Transport
}

// Incoming requests

func (adapter *endpointAdapter) DispatchRequest(receiver []byte, msg rpc.Request) error {
	request, err := adapter.t.newRemoteRequest(msg)
	if err != nil {
		return err
	}

	select {
	case adapter.t.requestCh <- request:
	case <-adapter.t.closedCh:
		adapter.t.unregisterRequest(request)
		return ErrTerminated
	}

	return nil
}

func (adapter *endpointAdapter) DispatchInterrupt(receiver []byte, msg rpc.Interrupt) error {
	adapter.t.interruptRequest(msg.Sender(), msg.TargetRequestId())
	return nil
}

// Outgoing requests

func (adapter *endpointAdapter) DispatchProgress(msg rpc.Progress) error {
	select {
	case adapter.t.progressCh <- newProgress(msg):
	case <-adapter.t.closedCh:
		return ErrTerminated
	}
	return nil
}

func (adapter *endpointAdapter) DispatchStreamFrame(msg rpc.StreamFrame) error {
	select {
	case adapter.t.streamingCh <- newStreamFrame(msg):
	case <-adapter.t.closedCh:
		return ErrTerminated
	}
	return nil
}

func (adapter *endpointAdapter) DispatchReply(msg rpc.Reply) error {
	select {
	case adapter.t.replyCh <- newRemoteCallReply(msg):
	case <-adapter.t.closedCh:
		return ErrTerminated
	}
	return nil
}

// Common

func (adapter *endpointAdapter) ListenAndServe() error {
	return adapter.t.Wait()
}

func (adapter *endpointAdapter) Close() error {
	return adapter.t.Close()
}

// Errors ----------------------------------------------------------------------

var (
	ErrTerminated       = &services.ErrTerminated{"inproc RPC transport"}
	ErrDuplicateRequest = errors.New("duplicate request ID")
	ErrResolved         = errors.New("request already resolved")
)
