// Copyright (c) 2014 Salsita s.r.o.
//
// This file is part of cider.
//
// cider is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// cider is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with cider.  If not, see <http://www.gnu.org/licenses/>.

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
