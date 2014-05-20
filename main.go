// Copyright (c) 2014 The AUTHORS
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

package main

import (
	"os"

	"github.com/cider/cider/build"
	"github.com/cider/cider/master"
	"github.com/cider/cider/slave"

	"github.com/tchap/gocli"
)

const version = "0.0.1"

func main() {
	cider := gocli.NewApp("cider")
	cider.UsageLine = "cider SUBCMD "
	cider.Short = "your CI server extender"
	cider.Version = version
	cider.Long = `
  Cider is what could be called a CI server extender.

  The point is not to implement a complete CI server, but to add what is missing,
  most importantly to support platforms that are omitted in all popular hosted
  CI solutions.

  Cider does this by managing its own set of build slaves, connecting to
  these from within other CI servers when requested. Cider then streams
  the build output from the chosen build slave to the console and the original
  CI server takes care of saving the output. In this way the original CI server
  can be used whenever possible, and Cider can be invoked when one of the
  unsupported platforms is required to build the project.

  To understand more about how Cider works, check the available subcommands.`

	cider.MustRegisterSubcommand(build.Command)
	cider.MustRegisterSubcommand(master.Command)
	cider.MustRegisterSubcommand(slave.Command)

	cider.Run(os.Args[1:])
}
