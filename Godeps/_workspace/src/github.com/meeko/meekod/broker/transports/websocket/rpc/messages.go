// Copyright (c) 2013 The meeko AUTHORS
//
// Use of this source code is governed by The MIT License
// that can be found in the LICENSE file.

package rpc

// request implements rpc.Request ----------------------------------------------

// FRAME 0: empty
// FRAME 1: message header
// FRAME 2: message type
// FRAME 3: request ID (uint16, BE)
// FRAME 4: method
// FRAME 5: method args object (bytes; marshalled)
// FRAME 6: stdout stream tag (uint16, BE)
// FRAME 7: stderr stream tag (uint16, BE)

type request struct {
	src        []byte
	frames     [][]byte
	rejectFunc func(code byte, reason string) error
}

func (req *request) Sender() []byte {
	return req.src
}

func (req *request) Id() []byte {
	return req.frames[3]
}

func (req *request) Method() []byte {
	return req.frames[4]
}

func (req *request) Args() []byte {
	return req.frames[5]
}

func (req *request) StdoutTag() []byte {
	return req.frames[6]
}

func (req *request) StderrTag() []byte {
	return req.frames[7]
}

func (req *request) Reject(code byte, reason string) error {
	return req.rejectFunc(code, reason)
}

// interrupt implements rpc.Interrupt ------------------------------------------

// FRAME 0: empty
// FRAME 1: message header
// FRAME 2: message type
// FRAME 3: request ID

type interrupt struct {
	src    []byte
	frames [][]byte
}

func (msg *interrupt) Sender() []byte {
	return msg.src
}

func (msg *interrupt) TargetRequestId() []byte {
	return msg.frames[3]
}

// progress implements rpc.Progress --------------------------------------------

// FRAME 0: receiver
// FRAME 1: message header
// FRAME 2: message type
// FRAME 3: request ID

type progress struct {
	src    []byte
	frames [][]byte
}

func (msg *progress) Sender() []byte {
	return msg.src
}

func (msg *progress) Receiver() []byte {
	return msg.frames[0]
}

func (msg *progress) TargetRequestId() []byte {
	return msg.frames[3]
}

// streamFrame implements rpc.StreamFrame --------------------------------------

// FRAME 0: receiver
// FRAME 1: message header
// FRAME 2: message type
// FRAME 3: stream tag (uint16, BE)
// FRAME 4: frame (bytes)

type streamFrame struct {
	src    []byte
	frames [][]byte
}

func (msg *streamFrame) Sender() []byte {
	return msg.src
}

func (msg *streamFrame) Receiver() []byte {
	return msg.frames[0]
}

func (msg *streamFrame) TargetStreamTag() []byte {
	return msg.frames[3]
}

func (msg *streamFrame) Body() []byte {
	return msg.frames[4]
}

// reply implements rpc.Reply --------------------------------------------------

// FRAME 0: receiver
// FRAME 1: message header
// FRAME 2: message type
// FRAME 3: request ID
// FRAME 4: return code (byte)
// FRAME 5: return value (bytes, method-specific)

type reply struct {
	src    []byte
	frames [][]byte
}

func (msg *reply) Sender() []byte {
	return msg.src
}

func (msg *reply) Receiver() []byte {
	return msg.frames[0]
}

func (msg *reply) TargetRequestId() []byte {
	return msg.frames[3]
}

func (msg *reply) ReturnCode() []byte {
	return msg.frames[4]
}

func (msg *reply) ReturnValue() []byte {
	return msg.frames[5]
}
