// Copyright (c) 2013 The go-meeko AUTHORS
//
// Use of this source code is governed by The MIT License
// that can be found in the LICENSE file.

package rpc

import (
	"github.com/meeko/go-meeko/meeko/services"
	"io"
)

type Command interface {
	Type() int
	ErrorChan() chan<- error
}

type RegisterCmd interface {
	Command
	Method() string
}

type UnregisterCmd interface {
	Command
	Method() string
}

type CallCmd interface {
	Command
	RequestId() RequestID
	Method() string
	Args() interface{}
	StdoutTag() *StreamTag
	StderrTag() *StreamTag
}

type InterruptCmd interface {
	Command
	TargetRequestId() RequestID
}

// Transport implements the underlying transport for Service, which
// encapsulates the transport-agnostic part of the functionality.
type Transport interface {
	services.Transport

	// Incoming requests

	RegisterMethod(RegisterCmd)

	UnregisterMethod(UnregisterCmd)

	RequestChan() <-chan RemoteRequest

	// Outgoing requests

	Call(CallCmd)

	Interrupt(InterruptCmd)

	ProgressChan() <-chan RequestID

	StreamFrameChan() <-chan StreamFrame

	ReplyChan() <-chan RemoteCallReply

	// Common

	// ErrChan returns a channel that is sending internal transport errors.
	// Any error sent to this channel is treated as unrecoverable and makes
	// the service using the transport terminate.
	ErrorChan() <-chan error
}

type (
	RequestID  uint16
	StreamTag  uint16
	ReturnCode byte
)

type RemoteRequest interface {
	Sender() string
	Id() RequestID
	Method() string
	UnmarshalArgs(dst interface{}) error
	SignalProgress() error
	Stdout() io.Writer
	Stderr() io.Writer
	Interrupted() <-chan struct{}
	Resolve(returnCode ReturnCode, returnValue interface{}) error
	Resolved() <-chan struct{}
}

type StreamFrame interface {
	TargetStreamTag() StreamTag
	Payload() []byte
}

type RemoteCallReply interface {
	TargetCallId() RequestID
	ReturnCode() ReturnCode
	UnmarshalReturnValue(dst interface{}) error
}
