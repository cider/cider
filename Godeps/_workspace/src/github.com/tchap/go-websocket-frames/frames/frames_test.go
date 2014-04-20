// Copyright (c) 2014 The go-websocket-frames AUTHORS
//
// Use of this source code is governed by The MIT License
// that can be found in the LICENSE file.

package frames

import (
	"bytes"
	"testing"

	ws "code.google.com/p/go.net/websocket"
)

func Test_Codec(t *testing.T) {
	data := [][]byte{
		[]byte("first"),
		[]byte("second"),
		[]byte("third"),
		[]byte("fourth"),
		[]byte("fifth"),
	}

	msg, _, err := C.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}

	var decoded [][]byte
	if err := C.Unmarshal(msg, ws.BinaryFrame, &decoded); err != nil {
		t.Fatal(err)
	}

	if len(data) != len(decoded) {
		t.Fatalf("frame counts do not match: expected %d, got %d", len(data), len(decoded))
	}

	for i := 0; i < len(data); i++ {
		if !bytes.Equal(data[i], decoded[i]) {
			t.Fatalf("frames number %d do not match: expected %q, got %q", i, data, decoded)
		}
	}
}
