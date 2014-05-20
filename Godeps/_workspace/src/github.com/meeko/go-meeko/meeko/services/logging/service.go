// Copyright (c) 2013 The go-meeko AUTHORS
//
// Use of this source code is governed by The MIT License
// that can be found in the LICENSE file.

package logging

import "github.com/meeko/go-meeko/meeko/services"

//------------------------------------------------------------------------------
// Service
//------------------------------------------------------------------------------

// Service represents a Logging service instance.
type Service struct {
	Transport
}

// NewService uses the factory function passed into it to construct a logging
// transport, which is then used as the underlying transport for the Service
// instance returned.
//
// Any panics in the factory function are turned into errors, so it is safe to
// panic when the transport creation process fails to save some if errs.
func NewService(factory func() (Transport, error)) (srv *Service, err error) {
	defer func() {
		if r := recover(); r != nil {
			if ex, ok := r.(error); ok {
				err = ex
			} else {
				err = services.ErrFactoryPanic
			}
		}
	}()

	transport, err := factory()
	if err != nil {
		return nil, err
	}

	return &Service{transport}, nil
}

// MustNewService does the same thing as NewService, but panics on error.
func MustNewService(factory func() (Transport, error)) *Service {
	srv, err := NewService(factory)
	if err != nil {
		panic(err)
	}

	return srv
}
