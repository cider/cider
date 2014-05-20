// Copyright (c) 2013 The meeko AUTHORS
//
// Use of this source code is governed by The MIT License
// that can be found in the LICENSE file.

package broker

// Endpoint represents an endpoint providing certain service over certain
// transport. Every service is represented by a single Exchange where all
// transport endpoints for that particular service are interconnected.
type Endpoint interface {
	// ListenAndServe puts the endpoint into the serving state.
	//
	// This method shall block until the endpoint is terminated.
	ListenAndServe() error

	// Close signals the endpoint to shut down gracefully.
	//
	// This method shall block until the endpoint is terminated.
	Close() error
}
