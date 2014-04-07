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
	"os"
	"strings"

	// Paprika
	"github.com/paprikaci/paprika/slave/runners"

	// Cider
	"github.com/cider/go-cider/cider/services/rpc"
	ws "github.com/cider/go-cider/cider/transports/websocket/rpc"

	// Others
	"bitbucket.org/kardianos/service"
	"code.google.com/p/go.net/websocket"
	"github.com/cihub/seelog"
	"github.com/tchap/gocli"
)

var logger service.Logger

func RunSlaveService(cmd *gocli.Command, args []string) {
	// Make sure there are no other args specified.
	if len(args) != 0 {
		cmd.Usage()
		os.Exit(2)
	}

	// Disable regular logging.
	seelog.ReplaceLogger(seelog.Disabled)

	// Prepare a Windows service instance.
	srv, err := service.NewService("Paprika slave", "Paprika slave", "Paprika slave")
	if err != nil {
		os.Exit(1)
	}

	// Use the service logger for logging.
	logger = srv

	// Run the service.
	if err := srv.Run(serviceOnStart, serviceOnStop); err != nil {
		os.Exit(1)
	}
}

var (
	termCh    = make(chan struct{})
	termAckCh = make(chan error, 1)
)

func serviceOnStart() error {
	// Connect to the master node using the WebSocket transport.
	// The specified token is used to authenticated the build slave.
	logger.Info("---> Connecting to %v\n", master)
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

	go loop(srv)
	return nil
}

func serviceOnStop() error {
	close(termCh)
	return <-termAckCh
}

func loop(srv *rpc.Service) {
	var err error
	defer func() {
		termAckCh <- err
	}()

	// Number of concurrent builds is limited by creating a channel of the
	// specified length. Every time a build is requested, the request handler
	// sends some data to the channel, and when it is finished, it reads data
	// from the same channel.
	execQueue := make(chan bool, executors)
	logger.Info("---> Initiating %v build executor(s)\n", executors)

	// Export all available labels and runners.
	logger.Info("---> Available runners:\n")
	for _, runner := range runners.Available {
		logger.Info("       %v\n", runner.Name)
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
			err = srv.RegisterMethod(methodName, builder.Build)
			if err != nil {
				logger.Error("Error: %v\n", err)
				goto Close
			}
		}
	}

	logger.Info("---> Waiting for incoming requests\n")

	// Block until either there is a fatal error or a signal is received.
	select {
	case <-srv.Closed():
		goto Wait
	case <-termCh:
		logger.Info("---> Signal received, exiting...\n")
	}

Close:
	if ex := srv.Close(); ex != nil {
		logger.Error("Error: %v\n", ex)
		if err == nil {
			err = ex
		}
		return
	}
Wait:
	if ex := srv.Wait(); ex != nil {
		logger.Error("Error: %v\n", ex)
		if err == nil {
			err = ex
		}
		return
	}
}
