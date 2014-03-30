// Copyright (c) 2014 Salsita s.r.o.
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
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	// Paprika
	"github.com/paprikaci/paprika/slave/runners"

	// Cider
	"github.com/cider/go-cider/cider/services/rpc"
	ws "github.com/cider/go-cider/cider/transports/websocket/rpc"

	// Others
	"code.google.com/p/go.net/websocket"
)

const TokenHeader = "X-Paprika-Token"

func enslave() {
	var exitCode int
	log.SetFlags(0)

	// Start catching signals.
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)

	// Connect to the master node using the WebSocket transport.
	// The specified token is used to authenticated the build slave.
	srv, err := rpc.NewService(func() (rpc.Transport, error) {
		factory := ws.NewTransportFactory()
		factory.Server = master
		factory.WSConfigFunc = func(config *websocket.Config) {
			config.Header.Set(TokenHeader, token)
		}
		return factory.NewTransport(identity)
	})
	if err != nil {
		log.Fatal(err)
	}

	// Number of concurrent builds is limited by creating a channel of the
	// specified length. Every time a build is requested, the request handler
	// sends some data to the channel, and when it is finished, it reads data
	// from the same channel.
	execQueue := make(chan bool, executors)

	// Export all available runners.
	fmt.Println("Available runners:")
	for _, runner := range runners.Available {
		log.Printf("  %v\n", runner.Name)
	}

	manager := newWorkspaceManager(workspace)

	for _, label := range append(strings.Split(labels, ","), "any") {
		for _, runner := range runners.Available {
			methodName := label + "." + runner.Name
			builder := &Builder{runner, manager, execQueue}
			if err := srv.RegisterMethod(methodName, builder.Build); err != nil {
				log.Print(err)
				exitCode = 1
				goto Close
			}
		}
	}

	// Block until either there is a fatal error or a signal is received.
	select {
	case <-srv.Closed():
		goto Wait
	case <-signalCh:
		goto Close
	}

Close:
	if err := srv.Close(); err != nil {
		log.Fatal(err)
	}
Wait:
	if err := srv.Wait(); err != nil {
		log.Fatal(err)
	}

	os.Exit(exitCode)
}
