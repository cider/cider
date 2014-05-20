// Copyright (c) 2013 The meeko AUTHORS
//
// Use of this source code is governed by The MIT License
// that can be found in the LICENSE file.

package supervisor

import "errors"

var (
	ErrUnknownAlias    = errors.New("unknown agent alias")
	ErrAgentRunning    = errors.New("agent is running")
	ErrAgentNotRunning = errors.New("agent is not running")
	ErrInterrupted     = errors.New("operation was interrupted")
)
