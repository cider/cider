// Copyright (c) 2013 The go-cider AUTHORS
//
// Use of this source code is governed by The MIT License
// that can be found in the LICENSE file.

package rpc

import (
	"errors"
	"io"
	"sync/atomic"
)

// RemoteCall represents an RPC call that is to be executed.
type RemoteCall struct {
	// Private data set and used by Service.
	id        RequestID
	stdoutTag *StreamTag
	stderrTag *StreamTag

	disp *dispatcher

	dispatchedFlag  uint32
	interruptedFlag uint32

	method string
	args   interface{}

	Stdout     io.Writer
	Stderr     io.Writer
	OnProgress func()

	reply      RemoteCallReply
	resolvedCh chan struct{}
	err        error
}

func newRemoteCall(disp *dispatcher, method string, args interface{}) *RemoteCall {
	return &RemoteCall{
		disp:       disp,
		method:     method,
		args:       args,
		resolvedCh: make(chan struct{}),
	}
}

// Execute performs the RPC and blocks until the reply is received.
func (call *RemoteCall) Execute() error {
	if !atomic.CompareAndSwapUint32(&call.dispatchedFlag, 0, 1) {
		return nil
	}
	call.disp.executeRemoteCall(call)
	return call.Wait()
}

// GoExecute runs Execute asynchronously. It can block while waiting for the
// request to be processed, but it does not wait until the reply is received.
func (call *RemoteCall) GoExecute() *RemoteCall {
	if !atomic.CompareAndSwapUint32(&call.dispatchedFlag, 0, 1) {
		return call
	}
	call.disp.executeRemoteCall(call)
	return call
}

// Interrupt sends a form of SIGINT to the component taking care of the request.
// The processing component should stop executing the method as soon as possible
// and return equivalent of EINTR.
//
// Interrupt can be called even before Execute. If that happens, the request
// is not even sent, it is simply dropped silently.
func (call *RemoteCall) Interrupt() error {
	// Make it possible to call interrupt only once.
	if !atomic.CompareAndSwapUint32(&call.interruptedFlag, 0, 1) {
		return nil
	}
	// Always forward the interrupt to the service. The service might now know
	// about this call yet, in which case the interrupt is dropped. In any case,
	// interruptedFlag is set to 1, so when the call gets to the service, it
	// will be just resolved as interrupted immediately.
	return call.disp.interrupt(call)
}

// Abandon discards the call. It should be used as the last resort to free
// resorces connected to the call when Interrupt is not making the call return
// fast enough.
//
// So, Abandon basically cast an interrupt and then deallocates all the resources
// occupied by the call. The reply received after Abandon is silently dropped
// because the call does not exist in the service any more.
func (call *RemoteCall) Abandon() error {
	return call.disp.abandon(call)
}

// Returned returns a channel that is closed when the call is resolved.
func (call *RemoteCall) Resolved() <-chan struct{} {
	return call.resolvedCh
}

// Wait blocks until the reply is received. It returns an error if Execute
// fails to dispatch the request. It fits together with GoExecute.
func (call *RemoteCall) Wait() error {
	<-call.resolvedCh
	return call.err
}

// ReturnCode returns the resulting return code. The semantics are the same
// as in the operating system. Zero is treated as success, other return
// codes must be defined and documented by the method requested.
//
// ReturnCode must be called after Execute or Wait, otherwise it panics.
func (call *RemoteCall) ReturnCode() ReturnCode {
	select {
	case <-call.resolvedCh:
		return call.reply.ReturnCode()
	default:
		panic(ErrNotResolvedYet)
	}
}

// UnmarshalReturnValue decodes the return value object into dst. The object
// can and probably will vary depending on the return code.
//
// UnmarshalReturnValue must be called after Execute or Wait, otherwise it panics.
func (call *RemoteCall) UnmarshalReturnValue(dst interface{}) error {
	select {
	case <-call.resolvedCh:
		return call.reply.UnmarshalReturnValue(dst)
	default:
		panic(ErrNotResolvedYet)
	}
}

func (call *RemoteCall) interrupted() bool {
	return atomic.LoadUint32(&call.interruptedFlag) != 0
}

func (call *RemoteCall) resolve(reply RemoteCallReply, err error) {
	call.reply = reply
	call.err = err
	close(call.resolvedCh)
}

// Errors ----------------------------------------------------------------------

var (
	ErrNotResolvedYet = errors.New("call not resolved yet")
)
