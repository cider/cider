// Copyright (c) 2013 The meeko AUTHORS
//
// Use of this source code is governed by The MIT License
// that can be found in the LICENSE file.

package executil

import "os"

var (
	sigterm = os.Kill
	sigkill = os.Kill
)
