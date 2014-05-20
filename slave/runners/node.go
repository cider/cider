// Copyright (c) 2014 The cider AUTHORS
//
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

package runners

import "os/exec"

func nodeFactory() *Runner {
	if exec.Command("node", "--version").Run() != nil {
		return nil
	}

	return &Runner{
		Name: "node",
		NewCommand: func(script string) *exec.Cmd {
			return exec.Command("node", "-e", script)
		},
	}
}
