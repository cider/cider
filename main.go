// Copyright (c) 2013 The cider AUTHORS
//
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

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
