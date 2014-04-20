// Copyright (c) 2013 The cider AUTHORS
//
// Use of this source code is governed by The MIT License
// that can be found in the LICENSE file.

package pubsub

import (
	"bytes"
	"encoding/binary"

	"github.com/cider/cider/broker/services/pubsub"
)

// FRAME 0: publisher (string)
// FRAME 1: event kind (string)
// FRAME 2: message header (string)
// FRAME 3: message type (byte)
// FRAME 4: event sequence number (uint32, BE)
// FRAME 5: event object, marshalled (bytes)
type Event [][]byte

func (e Event) Publisher() []byte {
	msg := [][]byte(e)
	return msg[0]
}

func (e Event) Kind() []byte {
	msg := [][]byte(e)
	return msg[1]
}

func (e Event) Seq() []byte {
	msg := [][]byte(e)
	return msg[4]
}

func (e Event) SetSeq(seq pubsub.EventSeqNum) {
	buf := bytes.NewBuffer(make([]byte, 0, 4)) // XXX: Hardcoded seq length
	if err := binary.Write(buf, binary.BigEndian, seq); err != nil {
		panic(err)
	}

	msg := [][]byte(e)
	msg[4] = buf.Bytes()
}

func (e Event) Body() []byte {
	msg := [][]byte(e)
	return msg[5]
}
