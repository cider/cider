// Copyright (c) 2013 The go-meeko AUTHORS
//
// Use of this source code is governed by The MIT License
// that can be found in the LICENSE file.

package logging

import "github.com/meeko/go-meeko/meeko/services"

//------------------------------------------------------------------------------
// Transport
//------------------------------------------------------------------------------

// Transport interface for logging defines a set of common methods any other
// logger would implement. The task of structs implementing this interface is
// to stream the inserted log records to the Meeko broker, where they are
// processed centrally.
type Transport interface {
	services.Transport

	Tracef(format string, params ...interface{})
	Debugf(format string, params ...interface{})
	Infof(format string, params ...interface{})
	Warnf(format string, params ...interface{}) error
	Errorf(format string, params ...interface{}) error
	Criticalf(format string, params ...interface{}) error

	Trace(v ...interface{})
	Debug(v ...interface{})
	Info(v ...interface{})
	Warn(v ...interface{}) error
	Error(v ...interface{}) error
	Critical(v ...interface{}) error

	// Flush is there just in case the transport decides to implement some kind
	// of buffering. Flush should force the transport to send the logs immediately.
	Flush()
}
