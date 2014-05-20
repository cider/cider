// Copyright (c) 2013 The go-meeko AUTHORS
//
// Use of this source code is governed by The MIT License
// that can be found in the LICENSE file.

package rpc

import (
	"errors"
	log "github.com/cihub/seelog"
)

type RequestHandler func(request RemoteRequest)

type executor struct {
	transport Transport

	methodHandlers map[string]RequestHandler
	taskManager    *asyncTaskManager

	registerCh   chan *registerCmd
	unregisterCh chan *unregisterCmd
	deleteCh     chan *string
	termCh       chan struct{}
	termAckCh    chan struct{}
}

func newExecutor(transport Transport) *executor {
	exec := &executor{
		transport:      transport,
		methodHandlers: make(map[string]RequestHandler),
		taskManager:    newAsyncTaskManager(),
		registerCh:     make(chan *registerCmd),
		unregisterCh:   make(chan *unregisterCmd),
		deleteCh:       make(chan *string),
		termCh:         make(chan struct{}),
		termAckCh:      make(chan struct{}),
	}

	go exec.loop()
	return exec
}

// Public API ------------------------------------------------------------------

type registerCmd struct {
	method  string
	handler RequestHandler
	errCh   chan error
}

func (cmd *registerCmd) Type() int {
	return CmdRegister
}

func (cmd *registerCmd) Method() string {
	return cmd.method
}

func (cmd *registerCmd) RequestHandler() RequestHandler {
	return cmd.handler
}

func (cmd *registerCmd) ErrorChan() chan<- error {
	return cmd.errCh
}

func (exec *executor) RegisterMethod(method string, handler RequestHandler) (err error) {
	errCh := make(chan error, 1)

	select {
	case exec.registerCh <- &registerCmd{method, handler, errCh}:
		err = <-errCh
		if err != nil {
			exec.deleteMethod(method)
		}
	case <-exec.termCh:
		return ErrTerminated
	}

	return
}

func (exec *executor) MustRegisterMethod(method string, handler RequestHandler) {
	if err := exec.RegisterMethod(method, handler); err != nil {
		panic(err)
	}
}

type unregisterCmd struct {
	method string
	errCh  chan error
}

func (cmd *unregisterCmd) Type() int {
	return CmdUnregister
}

func (cmd *unregisterCmd) Method() string {
	return cmd.method
}

func (cmd *unregisterCmd) ErrorChan() chan<- error {
	return cmd.errCh
}

func (exec *executor) UnregisterMethod(method string) (err error) {
	errCh := make(chan error, 1)

	select {
	case exec.unregisterCh <- &unregisterCmd{method, errCh}:
		err = <-errCh
		if err == nil {
			err = exec.deleteMethod(method)
		}
	case <-exec.termCh:
		err = ErrTerminated
	}

	return
}

func (exec *executor) deleteMethod(method string) (err error) {
	select {
	case exec.deleteCh <- &method:
	case <-exec.termCh:
		err = ErrTerminated
	}

	return
}

// Private API for Server ------------------------------------------------------

func (exec *executor) shutdown() {
	select {
	case <-exec.termCh:
	default:
		close(exec.termCh)
	}
}

func (exec *executor) terminated() <-chan struct{} {
	return exec.termAckCh
}

// Private methods -------------------------------------------------------------

func (exec *executor) loop() {
	for {
		select {
		// registerCh is an internal command channel that accepts requests for
		// method handlers to be registered and exported.
		case cmd := <-exec.registerCh:
			if _, ok := exec.methodHandlers[cmd.method]; ok {
				cmd.errCh <- ErrAlreadyRegistered
				continue
			}

			exec.methodHandlers[cmd.method] = cmd.handler
			exec.transport.RegisterMethod(cmd)

		// unregisterCh is an internal command channel that accepts requests for
		// method handlers to be unregistered.
		case cmd := <-exec.unregisterCh:
			if _, ok := exec.methodHandlers[cmd.method]; !ok {
				cmd.errCh <- ErrNotRegistered
				continue
			}

			exec.transport.UnregisterMethod(cmd)

		// deleteCh accepts requests for method deletion from the internal map.
		case method := <-exec.deleteCh:
			delete(exec.methodHandlers, *method)

		// RequestChan contains incoming RPC requests.
		case request := <-exec.transport.RequestChan():
			handler, ok := exec.methodHandlers[request.Method()]
			if !ok {
				// This should never ever happen since the broker should not
				// event route messages to unregistered methods here.
				continue
			}

			exec.taskManager.Go(func() {
				handler(request)
			})

		// termCh is closed when the executor is to be terminated.
		case <-exec.termCh:
			log.Debug("Executor: terminating")
			for {
				select {
				case <-exec.taskManager.Terminate():
					close(exec.termAckCh)
					log.Debug("Executor: terminated")
					return

				case request := <-exec.transport.RequestChan():
					request.Resolve(254, "terminating")
				}
			}
		}
	}
}

// Errors ----------------------------------------------------------------------

var (
	ErrAlreadyRegistered = errors.New("method already registered")
	ErrNotRegistered     = errors.New("method not registered")
)
