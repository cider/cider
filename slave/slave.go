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
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	// Paprika
	"github.com/paprikaci/paprika/slave/runners"

	// Cider
	"github.com/cider/go-cider/cider/services/rpc"
	ws "github.com/cider/go-cider/cider/transports/websocket/rpc"

	// Others
	"code.google.com/p/go.net/websocket"
	log "github.com/cihub/seelog"
)

const TokenHeader = "X-Cider-Token"

const (
	errorCalmPeriod = 10 * time.Second
	errorThreshold  = 5

	maxBackoff = time.Minute
)

func enslave() {
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
	backoff := time.Second
	for {
		// Run the slave.
		switch err := runSlave(); {

		// EOF means disconnect. That is fine, we will try to reconnect.
		case err == io.EOF:

		// Nil error means a clean termination, in which case we just return.
		case err == nil:
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
			log.Critical(err)
			log.Flush()
			os.Exit(1)
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

func runSlave() (err error) {
	// Connect to the master node using the WebSocket transport.
	// The specified token is used to authenticated the build slave.
	log.Infof("Connecting to %v", master)
	srv, err := rpc.NewService(func() (rpc.Transport, error) {
		factory := ws.NewTransportFactory()
		factory.Server = master
		factory.Origin = "http://localhost"
		factory.WSConfigFunc = func(config *websocket.Config) {
			config.Header.Set(TokenHeader, token)
		}
		return factory.NewTransport(identity)
	})
	if err != nil {
		return err
	}
	// Close the service on return.
	defer func() {
		select {
		case <-srv.Closed():
			goto Wait
		default:
		}

		if ex := srv.Close(); ex != nil {
			if err == nil {
				err = ex
				return
			}
			log.Warn(err)
			return
		}

	Wait:
		if ex := srv.Wait(); ex != nil {
			if err == nil {
				err = ex
				return
			}
			log.Warn(err)
		}
	}()

	// Start catching signals.
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(signalCh)

	// Number of concurrent builds is limited by creating a channel of the
	// specified length. Every time a build is requested, the request handler
	// sends some data to the channel, and when it is finished, it reads data
	// from the same channel.
	execQueue := make(chan bool, executors)
	log.Infof("Initiating %v build executor(s)", executors)

	// Export all available labels and runners.
	log.Info("Available runners:")
	for _, runner := range runners.Available {
		log.Infof("---> %v", runner.Name)
	}

	manager := newWorkspaceManager(workspace)

	ls := []string{"any"}
	if labels != "" {
		ls = append(ls, strings.Split(labels, ",")...)
	}

	for _, label := range ls {
		for _, runner := range runners.Available {
			methodName := fmt.Sprintf("paprika.%v.%v", label, runner.Name)
			builder := &Builder{runner, manager, execQueue}
			if err := srv.RegisterMethod(methodName, builder.Build); err != nil {
				return err
			}
		}
	}

	log.Info("Waiting for incoming requests...")

	// Block until either there is a fatal error or a signal is received.
	select {
	case <-srv.Closed():
		return srv.Wait()
	case <-signalCh:
		log.Info("Signal received, exiting...")
		return
	}
}
