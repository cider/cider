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
	// Stdlib
	"io"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	// Paprika
	"github.com/paprikaci/paprika/utils"

	// Others
	"code.google.com/p/go.net/websocket"
	log "github.com/cihub/seelog"
	"github.com/tchap/gocli"
)

var (
	master      string
	token       string
	identity    string
	labels      string
	workspace   string
	executors   = uint(runtime.NumCPU())
	verboseMode bool
	debugMode   bool
)

var Command = &gocli.Command{
	UsageLine: `
  slave [-master=URL] [-token=TOKEN] [-identity=IDENTITY] [-labels=LABELS]
        [-workspace=WORKSPACE] [-executors=EXECUTORS] [-verbose|-debug]`,
	Short: "run a build slave",
	Long: `
    Start a build slave and connect it to the specified master node.

  ENVIRONMENT:
    PAPRIKA_MASTER_URL
    PAPRIKA_MASTER_TOKEN
    PAPRIKA_SLAVE_IDENTITY
    PAPRIKA_SLAVE_LABELS
    PAPRIKA_SLAVE_WORKSPACE
	`,
	Action: enslaveThisPoorMachine,
}

func init() {
	cmd := Command
	cmd.Flags.StringVar(&master, "master", master, "build master to connect to")
	cmd.Flags.StringVar(&token, "token", token, "build master access token")
	cmd.Flags.StringVar(&identity, "identity", identity, "build slave identity; must be unique")
	cmd.Flags.StringVar(&labels, "labels", labels, "labels to apply to this slave")
	cmd.Flags.StringVar(&workspace, "workspace", workspace, "build workspace")
	cmd.Flags.UintVar(&executors, "executors", executors, "number of jobs that can run in parallel")
	cmd.Flags.BoolVar(&verboseMode, "verbose", verboseMode, "print verbose log output to the console")
	cmd.Flags.BoolVar(&debugMode, "debug", debugMode, "print debug log output to the console")
}

func enslaveThisPoorMachine(cmd *gocli.Command, args []string) {
	// Make sure there were no arguments specified.
	if len(args) != 0 {
		cmd.Usage()
		os.Exit(2)
	}

	// Read the environment to fill in missing parameters.
	utils.GetenvOrFailNow(&master, "PAPRIKA_MASTER_URL", cmd)
	utils.GetenvOrFailNow(&token, "PAPRIKA_MASTER_TOKEN", cmd)
	utils.GetenvOrFailNow(&identity, "PAPRIKA_SLAVE_IDENTITY", cmd)
	utils.Getenv(&labels, "PAPRIKA_SLAVE_LABELS")
	utils.GetenvOrFailNow(&workspace, "PAPRIKA_SLAVE_WORKSPACE", cmd)

	// Set up logging.
	var (
		logger log.LoggerInterface
		err    error
	)
	switch {
	case verboseMode:
		logger, err = log.LoggerFromConfigAsString(`<seelog minlevel="info"></seelog>`)
	case debugMode:
		logger, err = log.LoggerFromConfigAsString(`<seelog minlevel="trace"></seelog>`)
	default:
		logger, err = log.LoggerFromConfigAsString(`<seelog minlevel="warn"></seelog>`)
	}
	if err != nil {
		panic(err)
	}
	if err := log.ReplaceLogger(logger); err != nil {
		panic(err)
	}

	// Start the slave loop. This loop takes care of reconnecting to the master
	// node once the slave is disconnected. It does exponential backoff.
	var (
		slave    *BuildSlave
		backoff  = minBackoff
		signalCh = make(chan os.Signal, 1)
	)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)
	for {
		if slave != nil {
			if err := slave.Terminate(); err != nil {
				die(err)
			}
		}
		slave = New(identity, workspace, executors)
		go func() {
			select {
			case <-slave.Terminated():
				return
			case <-signalCh:
				if err := slave.Terminate(); err != nil {
					die(err)
				}
			}
		}()

		// Run the slave.
		connectT := time.Now()
		switch err := slave.Connect(master, token); {

		// EOF means disconnect. That is fine, we will try to reconnect.
		case err == io.EOF:

		// Nil error means a clean termination, in which case we just return.
		case err == nil:
			if ex := slave.Terminate(); ex != nil {
				die(ex)
			}
			return

		default:
			// Bad status is also not treated as a fatal error.
			// The master can be being restarted, so we try to reconnect later.
			if ex, ok := err.(*websocket.DialError); ok {
				if ex.Err.Error() == "bad status" {
					log.Warn(err)
					break
				}
			}

			// Other errors are fatal.
			die(err)
		}

		// Reset the backoff in case we were connected for some time.
		if time.Now().Sub(connectT) > maxBackoff {
			backoff = minBackoff
		}

		// Do exponential backoff.
		log.Infof("Waiting for %v before reconnecting...", backoff)
		time.Sleep(backoff)
		backoff = 2 * backoff
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

func die(err error) {
	log.Critical(err)
	log.Flush()
	os.Exit(1)
}
