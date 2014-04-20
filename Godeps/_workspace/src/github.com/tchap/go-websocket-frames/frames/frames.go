// Copyright (c) 2014 The go-websocket-frames AUTHORS
//
// Use of this source code is governed by The MIT License
// that can be found in the LICENSE file.

package frames

import (
	"bytes"
	"encoding/binary"
	"errors"

	ws "code.google.com/p/go.net/websocket"
)

// Frames is a WebSocket codec for handling multi-part messages.
var C = ws.Codec{marshal, unmarshal}

var (
	ErrTooManyFrames     = errors.New("too many frames")
	ErrFrameTooLarge     = errors.New("frame too large")
	ErrFrameCountMissing = errors.New("frame count missing")
	ErrMessageTooShort   = errors.New("message too short")
)

const maxUint32 = uint64(^uint32(0))

func marshal(v interface{}) (msg []byte, payloadType byte, err error) {
	frames, ok := v.([][]byte)
	if !ok {
		return nil, ws.UnknownFrame, ws.ErrNotSupported
	}

	if uint64(len(frames)) > maxUint32 {
		return nil, ws.UnknownFrame, ErrTooManyFrames
	}

	b := bytes.NewBuffer(make([]byte, 0, 4))
	if err := binary.Write(b, binary.BigEndian, uint32(len(frames))); err != nil {
		return nil, ws.UnknownFrame, err
	}
	for _, frame := range frames {
		if uint64(len(frame)) > maxUint32 {
			return nil, ws.UnknownFrame, ErrFrameTooLarge
		}
		if err := binary.Write(b, binary.BigEndian, uint32(len(frame))); err != nil {
			return nil, ws.UnknownFrame, err
		}
		if _, err := b.Write(frame); err != nil {
			return nil, ws.UnknownFrame, err
		}
	}

	return b.Bytes(), ws.BinaryFrame, nil
}

func unmarshal(msg []byte, payloadType byte, v interface{}) (err error) {
	framesPtr, ok := v.(*[][]byte)
	if !ok {
		return ws.ErrNotSupported
	}

	if len(msg) < 4 {
		return ErrFrameCountMissing
	}

	r := bytes.NewReader(msg)

	var nFrames uint32
	binary.Read(r, binary.BigEndian, &nFrames)
	*framesPtr = make([][]byte, nFrames)
	if nFrames == 0 {
		return err
	}

	var (
		frames   = *framesPtr
		frameLen uint32
	)
	for i := uint32(0); i < nFrames; i++ {
		if err := binary.Read(r, binary.BigEndian, &frameLen); err != nil {
			return err
		}
		frames[i] = make([]byte, frameLen)
		n, err := r.Read(frames[i])
		if err != nil {
			return err
		}
		if uint32(n) != frameLen {
			return ErrMessageTooShort
		}
	}

	return nil
}
