// Copyright (c) 2013 The cider AUTHORS
//
// Use of this source code is governed by The MIT License
// that can be found in the LICENSE file.

package broker

import "fmt"

type ErrTerminated struct {
	What string
}

func (err *ErrTerminated) Error() string {
	return fmt.Sprintf("%v already terminated", err.What)
}
