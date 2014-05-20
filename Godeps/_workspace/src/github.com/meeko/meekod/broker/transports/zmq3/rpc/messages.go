// Copyright (c) 2013 The meeko AUTHORS
//
// Use of this source code is governed by The MIT License
// that can be found in the LICENSE file.

package rpc

// Request ---------------------------------------------------------------------

// FRAME 0: sender
// FRAME 1: empty
// FRAME 2: message header
// FRAME 3: message type
// FRAME 4: request ID (uint16, BE)
// FRAME 5: method
// FRAME 6: method args object (bytes; marshalled)
// FRAME 7: stdout stream tag (uint16, BE)
// FRAME 8: stderr stream tag (uint16, BE)

type Request struct {
	msg        [][]byte
	rejectFunc func(code byte, reason string) error
}

func (req Request) Sender() []byte {
	return req.msg[0]
}

func (req Request) Id() []byte {
	return req.msg[4]
}

func (req Request) Method() []byte {
	return req.msg[5]
}

func (req Request) Args() []byte {
	return req.msg[6]
}

func (req Request) StdoutTag() []byte {
	return req.msg[7]
}

func (req Request) StderrTag() []byte {
	return req.msg[8]
}

func (req Request) Reject(code byte, reason string) error {
	return req.rejectFunc(code, reason)
}

// Interrupt -------------------------------------------------------------------

// FRAME 0: sender
// FRAME 1: empty
// FRAME 2: message header
// FRAME 3: message type
// FRAME 4: request ID

type Interrupt [][]byte

func (msg Interrupt) Sender() []byte {
	raw := [][]byte(msg)
	return raw[0]
}

func (msg Interrupt) TargetRequestId() []byte {
	raw := [][]byte(msg)
	return raw[4]
}

// Progress --------------------------------------------------------------------

// FRAME 0: sender
// FRAME 1: receiver
// FRAME 2: message header
// FRAME 3: message type
// FRAME 4: request ID

type Progress [][]byte

func (msg Progress) Sender() []byte {
	raw := [][]byte(msg)
	return raw[0]
}

func (msg Progress) Receiver() []byte {
	raw := [][]byte(msg)
	return raw[1]
}

func (msg Progress) TargetRequestId() []byte {
	raw := [][]byte(msg)
	return raw[4]
}

// StreamFrame -----------------------------------------------------------------

// FRAME 0: sender
// FRAME 1: receiver
// FRAME 2: message header
// FRAME 3: message type
// FRAME 4: stream tag (uint16, BE)
// FRAME 5: frame (bytes)

type StreamFrame [][]byte

func (msg StreamFrame) Sender() []byte {
	raw := [][]byte(msg)
	return raw[0]
}

func (msg StreamFrame) Receiver() []byte {
	raw := [][]byte(msg)
	return raw[1]
}

func (msg StreamFrame) TargetStreamTag() []byte {
	raw := [][]byte(msg)
	return raw[4]
}

func (msg StreamFrame) Body() []byte {
	raw := [][]byte(msg)
	return raw[6]
}

// Reply -----------------------------------------------------------------------

// FRAME 0: sender
// FRAME 1: receiver
// FRAME 2: message header
// FRAME 3: message type
// FRAME 4: request ID
// FRAME 5: return code (byte)
// FRAME 6: return value (bytes, method-specific)

type Reply [][]byte

func (msg Reply) Sender() []byte {
	raw := [][]byte(msg)
	return raw[0]
}

func (msg Reply) Receiver() []byte {
	raw := [][]byte(msg)
	return raw[1]
}

func (msg Reply) TargetRequestId() []byte {
	raw := [][]byte(msg)
	return raw[4]
}

func (msg Reply) ReturnCode() []byte {
	raw := [][]byte(msg)
	return raw[5]
}

func (msg Reply) ReturnValue() []byte {
	raw := [][]byte(msg)
	return raw[6]
}
