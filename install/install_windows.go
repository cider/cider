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

package install

import (
	// Stdlib
	"log"
	"os"

	// Others
	"bitbucket.org/kardianos/service"
	"github.com/tchap/gocli"
)

func init() {
	Command.Action = install
}

func install(cmd *gocli.Command, args []string) {
	log.SetFlags(0)
	log.SetPrefix("Error: ")

	// Make suret here are exactly 3 arguments.
	if len(args) != 3 {
		cmd.Usage()
		os.Exit(2)
	}

	// Install Paprika as a Windows service.
	srv, err := service.NewService(args[0], args[1], args[2])
	if err != nil {
		log.Fatalln(err)
	}

	if err := srv.Install(); err != nil {
		log.Fatalln(err)
	}
}
