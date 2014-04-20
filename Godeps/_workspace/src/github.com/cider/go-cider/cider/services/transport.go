// Copyright (c) 2013 The go-cider AUTHORS
//
// Use of this source code is governed by The MIT License
// that can be found in the LICENSE file.

package services

//------------------------------------------------------------------------------
// Transport
//------------------------------------------------------------------------------

// Transport interface in this package defines what all service transports
// should implement and it is inherited by all the service transports, which,
// naturally, add much more on top of these basic methods.
type Transport interface {
	// Close shall terminate the transport, possibly in a clean way, and close
	// the channel returned by Closed().
	Close() error

	// Closed returns a channel that is closed once the transport is terminated.
	// The channel should be closed when a fatal internal transport error occurs
	// or after Close() is called and the transport object is terminated.
	//
	// Possible internal error can be checked by using Wait method once this
	// channel is closed.
	Closed() <-chan struct{}

	// Wait shall block until the transport is terminated. Wait returning a
	// non-nil error means that the transport failed in a non-recoverable way.
	Wait() error
}
