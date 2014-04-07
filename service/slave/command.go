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

package slave

import (
	"log"
	"os"
	"runtime"

	"github.com/paprikaci/paprika/slave"

	"bitbucket.org/kardianos/service"
	"github.com/tchap/gocli"
)

var Command = &gocli.Command{
	UsageLine: "slave SUBCMD",
	Short:     "Paprika slave Windows service management",
	Long: `
  Install and run Paprika slave as a Windows service.
	`,
}

var installCommand = &gocli.Command{
	UsageLine: "install NAME DISPLAY_NAME DESCRIPTION",
	Short:     "install Paprika slave Windows service",
	Long: `
  Install Paprika slave as a Windows service.
	`,
	Action: func(cmd *gocli.Command, args []string) {
		if runtime.GOOS != "windows" {
			panic("Platform not supported")
		}

		if len(args) != 3 {
			cmd.Usage()
			os.Exit(2)
		}

		srv, err := service.NewService(args[0], args[1], args[2])
		if err != nil {
			log.Fatalf("\nError: %v\n", err)
		}

		if err := srv.Install(); err != nil {
			log.Fatalf("\nError: %v\n", err)
		}

		log.Println("Success")
	},
}

var runCommand = &gocli.Command{
	UsageLine: "run [OPTION ...] SERVICE_NAME",
	Short:     "run Paprika slave Windows service",
	Long: `
  Run Paprika slave as a Windows service.

  This is the subcommand that must be invoked to run Paprika slave as a service.
	`,
	Action: slave.RunSlaveService,
}

func init() {
	Command.MustRegisterSubcommand(installCommand)
	Command.MustRegisterSubcommand(runCommand)
}
