// Copyright (c) 2013 The go-meeko AUTHORS
//
// Use of this source code is governed by The MIT License
// that can be found in the LICENSE file.

package rpc

import (
	// Stdlib
	"bytes"
	"encoding/binary"

	// Meeko client
	client "github.com/meeko/go-meeko/meeko/services/rpc"
	"github.com/meeko/go-meeko/meeko/utils/codecs"
)

// rpc.Request -----------------------------------------------------------------

type rpcRequest struct {
	t    *Transport
	cmd  client.CallCmd
	args []byte
}

func newRPCRequest(t *Transport, cmd client.CallCmd) (*rpcRequest, error) {
	var argsBuffer bytes.Buffer
	if err := codecs.MessagePack.Encode(&argsBuffer, cmd.Args()); err != nil {
		return nil, err
	}

	return &rpcRequest{
		t:    t,
		cmd:  cmd,
		args: argsBuffer.Bytes(),
	}, nil
}

func (msg *rpcRequest) Sender() []byte {
	return []byte(msg.t.identity)
}

func (msg *rpcRequest) Id() []byte {
	var idBuffer bytes.Buffer
	binary.Write(&idBuffer, binary.BigEndian, msg.cmd.RequestId())
	return idBuffer.Bytes()
}

func (msg *rpcRequest) Method() []byte {
	return []byte(msg.cmd.Method())
}

func (msg *rpcRequest) Args() []byte {
	return msg.args
}

func (msg *rpcRequest) StdoutTag() []byte {
	var tagBuffer bytes.Buffer
	if tag := msg.cmd.StdoutTag(); tag != nil {
		binary.Write(&tagBuffer, binary.BigEndian, *tag)
	}
	return tagBuffer.Bytes()
}

func (msg *rpcRequest) StderrTag() []byte {
	var tagBuffer bytes.Buffer
	if tag := msg.cmd.StderrTag(); tag != nil {
		binary.Write(&tagBuffer, binary.BigEndian, *tag)
	}
	return tagBuffer.Bytes()
}

func (msg *rpcRequest) Reject(code byte, reason string) error {
	msg.t.replyCh <- &rejectedCallReply{msg.cmd.RequestId(), code, []byte(reason)}
	return nil
}

// rpc.Interrupt ---------------------------------------------------------------

type rpcInterrupt struct {
	sender          []byte
	targetRequestId []byte
}

func newRPCInterrupt(sender string, targetRequestId client.RequestID) *rpcInterrupt {
	var idBuffer bytes.Buffer
	binary.Write(&idBuffer, binary.BigEndian, targetRequestId)

	return &rpcInterrupt{
		sender:          []byte(sender),
		targetRequestId: idBuffer.Bytes(),
	}
}

func (msg *rpcInterrupt) Sender() []byte {
	return msg.sender
}

func (msg *rpcInterrupt) TargetRequestId() []byte {
	return msg.targetRequestId
}

// rpc.Progress ----------------------------------------------------------------

type rpcProgress struct {
	sender          []byte
	receiver        []byte
	targetRequestId []byte
}

func (progress *rpcProgress) Sender() []byte {
	return progress.sender
}

func (progress *rpcProgress) Receiver() []byte {
	return progress.receiver
}

func (progress *rpcProgress) TargetRequestId() []byte {
	return progress.targetRequestId
}

// rpc.StreamFrame -------------------------------------------------------------

type rpcStreamFrame struct {
	sender          []byte
	receiver        []byte
	targetStreamTag []byte
	body            []byte
}

func (frame *rpcStreamFrame) Sender() []byte {
	return frame.sender
}

func (frame *rpcStreamFrame) Receiver() []byte {
	return frame.receiver
}

func (frame *rpcStreamFrame) TargetStreamTag() []byte {
	return frame.targetStreamTag
}

func (frame *rpcStreamFrame) Body() []byte {
	return frame.body
}

// rpc.Reply -------------------------------------------------------------------

type rpcReply struct {
	sender          []byte
	receiver        []byte
	targetRequestId []byte
	returnCode      []byte
	returnValue     []byte
}

func (rep *rpcReply) Sender() []byte {
	return rep.sender
}

func (rep *rpcReply) Receiver() []byte {
	return rep.receiver
}

func (rep *rpcReply) TargetRequestId() []byte {
	return rep.targetRequestId
}

func (rep *rpcReply) ReturnCode() []byte {
	return rep.returnCode
}

func (rep *rpcReply) ReturnValue() []byte {
	return rep.returnValue
}
