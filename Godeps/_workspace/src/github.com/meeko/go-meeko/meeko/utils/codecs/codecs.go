// Copyright (c) 2013 The go-meeko AUTHORS
//
// Use of this source code is governed by The MIT License
// that can be found in the LICENSE file.

package codecs

import (
	"io"

	"github.com/ugorji/go/codec"
)

type Codec interface {
	Encode(w io.Writer, src interface{}) error
	Decode(r io.Reader, dst interface{}) error
}

// MessagePack -----------------------------------------------------------------

var msgpackHandle = &codec.MsgpackHandle{
	RawToString: true,
}

type msgpackCodec struct{}

func (c *msgpackCodec) Encode(w io.Writer, src interface{}) error {
	return codec.NewEncoder(w, msgpackHandle).Encode(src)
}

func (c *msgpackCodec) Decode(r io.Reader, dst interface{}) error {
	return codec.NewDecoder(r, msgpackHandle).Decode(dst)
}

var MessagePack Codec = &msgpackCodec{}
