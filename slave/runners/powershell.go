// Copyright (c) 2014 The cider AUTHORS
//
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

package runners

import "os/exec"

func powerShellFactory() *Runner {
	if exec.Command("PowerShell.exe", "-Command", "& {Get-Date}").Run() != nil {
		return nil
	}

	return &Runner{
		Name: "powershell",
		NewCommand: func(script string) *exec.Cmd {
			return exec.Command("PowerShell.exe", "-NoLogo", "-NonInteractive", script)
		},
	}
}
