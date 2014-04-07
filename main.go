// Copyright (c) 2014 The AUTHORS
//
// This file is part of paprika.
//
// paprika is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// paprika is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with paprika.  If not, see <http://www.gnu.org/licenses/>.

package main

import (
	"os"

	"github.com/paprikaci/paprika/build"
	"github.com/paprikaci/paprika/install"
	"github.com/paprikaci/paprika/master"
	"github.com/paprikaci/paprika/slave"

	"github.com/tchap/gocli"
)

const version = "0.0.1"

func main() {
	paprika := gocli.NewApp("paprika")
	paprika.UsageLine = "paprika SUBCMD "
	paprika.Short = "your CI server extender"
	paprika.Version = version
	paprika.Long = `
  Paprika is what could be called a CI server extender.

  The point is not to implement a complete CI server, but to add what is missing,
  most importantly to support platforms that are omitted in all popular hosted
  CI solutions.

  Paprika does this by managing its own set of build slaves, connecting to
  these from within other CI servers when requested. Paprika then streams
  the build output from the chosen build slave to the console and the original
  CI server takes care of saving the output. In this way the original CI server
  can be used whenever possible, and Paprika can be invoked when one of the
  unsupported platforms is required to build the project.

  To understand more about how Paprika works, check the available subcommands.`

	paprika.MustRegisterSubcommand(build.Command)
	paprika.MustRegisterSubcommand(install.Command)
	paprika.MustRegisterSubcommand(master.Command)
	paprika.MustRegisterSubcommand(slave.Command)

	paprika.Run(os.Args[1:])
}
