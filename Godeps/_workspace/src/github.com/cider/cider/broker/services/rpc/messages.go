// Copyright (c) 2013 The cider AUTHORS
//
// Use of this source code is governed by The MIT License
// that can be found in the LICENSE file.

package rpc

// Request represents an RPC call from one application to another. It contains
// the method to be called and the arguments to be passed to that method when invoked.
type Request interface {

	// Sender is the application that sent the request.
	Sender() []byte

	// Id must be unique for the given sender. It always contains a uint16 (BE).
	Id() []byte

	// Method returns the remote method requested.
	Method() []byte

	// Args returns the arguments for the method call, marshalled.
	Args() []byte

	// StdoutTag returns the stream tag identifying this request's stdout stream.
	// If no stream is requested, this method returns an empty slice.
	StdoutTag() []byte

	// StderrTag is the same as StdoutTag, just for stderr instead of stdout.
	StderrTag() []byte

	// A utility function for the RPC handler to be able to reject requests
	// for methods that nobody really exported more easily.
	Reject(code byte, reason string) error
}

// Interrupt is a message for signalling that a RPC request should stop
// executing. It works much like SIGINT for OS processes.
type Interrupt interface {

	// Sender is the application that sent the interrupt.
	Sender() []byte

	// TargetRequestId returns ID of the request that is supposed to be
	// interrupted. It always contains uint16 (BE).
	TargetRequestId() []byte
}

// Progress is a message that can be sent from the app executing a method to
// the original requester to signal that something important has happened.
//
// The semantics must be defined by the method itself.
type Progress interface {
	Sender() []byte
	Receiver() []byte
	TargetRequestId() []byte
}

// StreamFrame is a message type that represents a fraction of an output stream,
// in our case it is either stderr or stdout.
//
// It contains the stream tag and the sequence number so that the receiver can
// assemble incoming stream frames into the original stream again.
type StreamFrame interface {
	Sender() []byte
	Receiver() []byte
	TargetStreamTag() []byte
	Body() []byte
}

// Reply message notifies the RPC requester about the result of the call.
//
// The semantics of the return code and value must be defined by the method.
type Reply interface {
	Sender() []byte
	Receiver() []byte
	TargetRequestId() []byte
	ReturnCode() []byte
	ReturnValue() []byte
}
