// Copyright (c) 2013 The cider AUTHORS
//
// Use of this source code is governed by The MIT License
// that can be found in the LICENSE file.

package executil

import "os"

var (
	termSignal = os.Interrupt
	killSignal = os.Kill
)
