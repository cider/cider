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

package master

import (
	// Stdlib
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	// Paprika
	"github.com/paprikaci/paprika/utils"

	// Cider
	clog "github.com/cider/cider/broker/log"

	// Others
	"github.com/cihub/seelog"
	"github.com/tchap/gocli"
)

var (
	listen    string
	token     string
	heartbeat time.Duration
)

var Command = &gocli.Command{
	UsageLine: `master [-listen=ADDRESS] [-token=TOKEN]
	                   [-heartbeat=HEARTBEAT]`,
	Short: "start a build master node",
	Long: `
  Start a build master node and start listening on ADDRESS for build slave
  connections. Every build slave must pass TOKEN in the connection request,
  otherwise the connection is refused.

  If HEARTBEAT is specified, the build master sends a ping message to every
  build slave every HEARTBEAT and closes the connection if the slave is not
  responding.

ENVIRONMENT:
  PAPRIKA_MASTER_LISTEN - can be used instead of -listen
  PAPRIKA_MASTER_TOKEN  - can be used instead of -token
	`,
	Action: run,
}

func init() {
	Command.Flags.StringVar(&listen, "listen", listen, "network address to listen on")
	Command.Flags.StringVar(&token, "token", token, "build master access token")
	Command.Flags.DurationVar(&heartbeat, "heartbeat", heartbeat, "heartbeat period")
}

func run(cmd *gocli.Command, args []string) {
	// Set up logging.
	log.SetFlags(0)
	clog.UseLogger(seelog.Default)

	// Make sure there were no arguments specified.
	if len(args) != 0 {
		cmd.Usage()
		os.Exit(2)
	}

	// Read the environment to fill in the missing parameters.
	utils.GetenvOrFailNow(&listen, "PAPRIKA_MASTER_LISTEN", cmd)
	utils.GetenvOrFailNow(&token, "PAPRIKA_MASTER_TOKEN", cmd)

	// Start catching signals.
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)

	// Start listening for slave connections.
	m := New(listen, token).EnableHeartbeat(heartbeat).Listen()
	log.Printf("Paprika broker listening on %v\n", listen)

	select {
	// Wait for a signal, then terminate the master in a clean way.
	case <-signalCh:
		log.Println("Interrupted, exiting...")
		m.Terminate()

	// Unblock also in case the master node has crashed.
	case <-m.Terminated():
	}

	// Check the master exit status and exit the process accordingly.
	if err := m.Wait(); err != nil {
		log.Fatal(err)
	}
}