// Copyright (c) 2014 The cider AUTHORS
//
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

package runners

import (
	"os/exec"
	"path/filepath"
)

func cmdFactory() *Runner {
	if exec.Command("cmd.exe", "/c", "echo foobar").Run() != nil {
		return nil
	}

	return &Runner{
		Name: "cmd",
		NewCommand: func(script string) *exec.Cmd {
			return exec.Command("cmd.exe", "/c", filepath.FromSlash(script))
		},
	}
}
