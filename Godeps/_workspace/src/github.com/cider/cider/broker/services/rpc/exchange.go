// Copyright (c) 2013 The cider AUTHORS
//
// Use of this source code is governed by The MIT License
// that can be found in the LICENSE file.

package rpc

// Exchange takes care of RPC requests routing and dispatching. Its sole purpose
// is to receive requests, decide what application should be in change and
// forward it to the given applicaton.
type Exchange interface {
	// RegisterMethod tells the handler that the app can start processing
	// requests to the method specified.
	RegisterMethod(app string, endpoint Endpoint, method string) error

	// UnregisterMethod is the opposite of RegisterMethod.
	UnregisterMethod(app string, method string)

	// UnregisterApp unregisters the app. This should be called by the endpoint
	// one the application has disconnected.
	UnregisterApp(app string)

	// UnregisterEndpoint is invoked when the relevant endpoint is closed so
	// that the handler can perform some cleanup if necessary.
	UnregisterEndpoint(endpoint Endpoint)

	HandleRequest(msg Request, srcEndpoint Endpoint)
	HandleInterrupt(msg Interrupt)
	HandleProgress(msg Progress)
	HandleStreamFrame(msg StreamFrame)
	HandleReply(msg Reply)
}
