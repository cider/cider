// Copyright (c) 2013 The go-meeko AUTHORS
//
// Use of this source code is governed by The MIT License
// that can be found in the LICENSE file.

package rpc

import (
	// Stdlib
	"bytes"
	"encoding/binary"
	"io"
	"io/ioutil"

	// Meeko broker
	"github.com/meeko/meekod/broker/services/rpc"

	// Meeko client
	client "github.com/meeko/go-meeko/meeko/services/rpc"
	"github.com/meeko/go-meeko/meeko/utils/codecs"
)

// client.RemoteRequest --------------------------------------------------------

type remoteRequest struct {
	t   *Transport
	msg rpc.Request

	stdout io.Writer
	stderr io.Writer

	interrupted chan struct{}
	resolved    chan struct{}
}

func newRemoteRequest(t *Transport, msg rpc.Request) *remoteRequest {
	// Set up stdout streaming.
	var stdoutWriter io.Writer
	if tag := msg.StdoutTag(); len(tag) == 2 {
		stdoutWriter = &streamWriter{
			t:        t,
			receiver: msg.Sender(),
			tag:      tag,
		}
	} else {
		stdoutWriter = ioutil.Discard
	}

	// Set up stderr streaming.
	var stderrWriter io.Writer
	if tag := msg.StderrTag(); len(tag) == 2 {
		stderrWriter = &streamWriter{
			t:        t,
			receiver: msg.Sender(),
			tag:      tag,
		}
	} else {
		stderrWriter = ioutil.Discard
	}

	// Create a new remoteRequest instance.
	return &remoteRequest{
		t:           t,
		msg:         msg,
		stdout:      stdoutWriter,
		stderr:      stderrWriter,
		interrupted: make(chan struct{}),
		resolved:    make(chan struct{}),
	}
}

func (req *remoteRequest) Sender() string {
	return string(req.msg.Sender())
}

func (req *remoteRequest) Id() client.RequestID {
	var id client.RequestID
	err := binary.Read(bytes.NewReader(req.msg.Id()), binary.BigEndian, &id)
	if err != nil {
		panic(err)
	}
	return id
}

func (req *remoteRequest) Method() string {
	return string(req.msg.Method())
}

func (req *remoteRequest) UnmarshalArgs(dst interface{}) error {
	return codecs.MessagePack.Decode(bytes.NewReader(req.msg.Args()), dst)
}

func (req *remoteRequest) SignalProgress() error {
	req.t.sendProgress(req.msg.Sender(), req.msg.Id())
	return nil
}

func (req *remoteRequest) Stdout() io.Writer {
	return req.stdout
}

func (req *remoteRequest) Stderr() io.Writer {
	return req.stderr
}

func (req *remoteRequest) Interrupted() <-chan struct{} {
	return req.interrupted
}

func (req *remoteRequest) Resolve(returnCode client.ReturnCode, returnValue interface{}) error {
	var valueBuffer bytes.Buffer
	if err := codecs.MessagePack.Encode(&valueBuffer, returnValue); err != nil {
		return err
	}

	err := req.t.resolveRequest(req.msg.Sender(), req.msg.Id(),
		[]byte{byte(returnCode)}, valueBuffer.Bytes())
	if err != nil {
		return err
	}

	close(req.resolved)
	return nil
}

func (req *remoteRequest) Resolved() <-chan struct{} {
	return req.resolved
}

func (req *remoteRequest) interrupt() {
	select {
	case <-req.interrupted:
	default:
		close(req.interrupted)
	}
}

type streamWriter struct {
	t        *Transport
	receiver []byte
	tag      []byte
}

func (w *streamWriter) Write(p []byte) (n int, err error) {
	w.t.sendStreamFrame(w.receiver, w.tag, p)
	return len(p), nil
}

// client.Progress -------------------------------------------------------------

func newProgress(msg rpc.Progress) client.RequestID {
	var id client.RequestID
	err := binary.Read(bytes.NewReader(msg.TargetRequestId()), binary.BigEndian, &id)
	if err != nil {
		panic(err)
	}
	return id
}

// client.StreamFrame ----------------------------------------------------------

type streamFrame struct {
	msg rpc.StreamFrame
}

func newStreamFrame(msg rpc.StreamFrame) client.StreamFrame {
	return &streamFrame{msg}
}

func (frame *streamFrame) TargetStreamTag() client.StreamTag {
	var tag client.StreamTag
	err := binary.Read(bytes.NewReader(frame.msg.TargetStreamTag()), binary.BigEndian, &tag)
	if err != nil {
		panic(err)
	}
	return tag
}

func (frame *streamFrame) Payload() []byte {
	return frame.msg.Body()
}

// client.RemoteCallReply ------------------------------------------------------

type remoteCallReply struct {
	msg rpc.Reply
}

func newRemoteCallReply(msg rpc.Reply) client.RemoteCallReply {
	return &remoteCallReply{msg}
}

func (reply *remoteCallReply) TargetCallId() client.RequestID {
	var id client.RequestID
	err := binary.Read(bytes.NewReader(reply.msg.TargetRequestId()), binary.BigEndian, &id)
	if err != nil {
		panic(err)
	}
	return id
}

func (reply *remoteCallReply) ReturnCode() client.ReturnCode {
	return client.ReturnCode(reply.msg.ReturnCode()[0])
}

func (reply *remoteCallReply) UnmarshalReturnValue(dst interface{}) error {
	return codecs.MessagePack.Decode(bytes.NewReader(reply.msg.ReturnValue()), dst)
}

type rejectedCallReply struct {
	requestId   client.RequestID
	returnCode  byte
	returnValue []byte
}

func (reply *rejectedCallReply) TargetCallId() client.RequestID {
	return reply.requestId
}

func (reply *rejectedCallReply) ReturnCode() client.ReturnCode {
	return client.ReturnCode(reply.returnCode)
}

func (reply *rejectedCallReply) UnmarshalReturnValue(dst interface{}) error {
	return codecs.MessagePack.Decode(bytes.NewReader(reply.returnValue), dst)
}
