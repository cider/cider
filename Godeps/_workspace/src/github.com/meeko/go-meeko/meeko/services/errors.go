// Copyright (c) 2013 The go-meeko AUTHORS
//
// Use of this source code is governed by The MIT License
// that can be found in the LICENSE file.

package services

import (
	"errors"
	"fmt"
)

var (
	ErrFactoryPanic = errors.New("Transport factory panicked")
)

type ErrMissingConfig struct {
	Where string
	What  string
}

func (err *ErrMissingConfig) Error() string {
	return fmt.Sprintf("%v config missing for %v", err.What, err.Where)
}

type ErrTerminated struct {
	What string
}

func (err *ErrTerminated) Error() string {
	return fmt.Sprintf("%v already terminated", err.What)
}
