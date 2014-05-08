// Copyright (c) 2013 The cider AUTHORS
//
// Use of this source code is governed by The MIT License
// that can be found in the LICENSE file.

// +build darwin dragonfly freebsd linux netbsd openbsd

package executil

import "syscall"

var (
	sigterm = syscall.SIGTERM
	sigkill = syscall.SIGKILL
)
