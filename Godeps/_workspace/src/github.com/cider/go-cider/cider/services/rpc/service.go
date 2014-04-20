// Copyright (c) 2013 The go-cider AUTHORS
//
// Use of this source code is governed by The MIT License
// that can be found in the LICENSE file.

package rpc

import (
	"errors"
	"github.com/cider/go-cider/cider/services"
	log "github.com/cihub/seelog"
)

const (
	CmdRegister int = iota
	CmdUnregister
	CmdCall
	CmdInterrupt
	CmdSignalProgress
	CmdSendStreamFrame
	CmdReply
	CmdClose
)

type Service struct {
	transport Transport
	*executor
	*dispatcher
	closedCh chan struct{}
}

type ServiceFactory func() (Transport, error)

// NewService uses factory to construct a Transport instance that is then used
// for creating the Service instance itself.
//
// factory can panic without any worries, it just makes NewService return an
// error, so some if-errs can be saved.
func NewService(factory ServiceFactory) (srv *Service, err error) {
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
		transport:  transport,
		executor:   newExecutor(transport),
		dispatcher: newDispatcher(transport),
		closedCh:   make(chan struct{}),
	}

	go func() {
		<-srv.executor.terminated()
		<-srv.dispatcher.terminated()
		close(srv.closedCh)
	}()

	go func() {
		select {
		case err := <-srv.transport.ErrorChan():
			srv.err = err
			srv.Close()
		case <-srv.Closed():
			return
		}
	}()

	return srv, nil
}

func (srv *Service) Close() error {
	log.Debug("Service: terminating")
	defer log.Debug("Service: terminated")

	srv.executor.shutdown()
	srv.dispatcher.shutdown()

	<-srv.executor.terminated()
	<-srv.dispatcher.terminated()

	return srv.transport.Close()
}

func (srv *Service) Closed() <-chan struct{} {
	return srv.closedCh
}

func (srv *Service) Wait() error {
	<-srv.Closed()
	return srv.err
}

// Errors ----------------------------------------------------------------------

var (
	ErrInterrupted = errors.New("interrupted")
	ErrTerminated  = &services.ErrTerminated{"RPC service"}
)
