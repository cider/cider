// Copyright (c) 2014 The cider AUTHORS
//
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

package runners

import "os/exec"

type Runner struct {
	Name       string
	NewCommand func(script string) *exec.Cmd
}

var factories = [...]func() *Runner{
	bashFactory,
	cmdFactory,
	powerShellFactory,
	nodeFactory,
}

var Available = make([]*Runner, 0)

func init() {
	ch := make(chan *Runner, len(factories))
	for i := range factories {
		go func(index int) {
			ch <- factories[index]()
		}(i)
	}
	for _ = range factories {
		if runner := <-ch; runner != nil {
			Available = append(Available, runner)
		}
	}
}
