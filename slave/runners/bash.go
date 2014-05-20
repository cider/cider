// Copyright (c) 2014 The cider AUTHORS
//
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

package runners

import "os/exec"

func bashFactory() *Runner {
	if exec.Command("bash", "--version").Run() != nil {
		return nil
	}

	return &Runner{
		Name: "bash",
		NewCommand: func(script string) *exec.Cmd {
			return exec.Command("bash", script)
		},
	}
}
