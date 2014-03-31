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

package build

import (
	// Stdlib
	"crypto/rand"
	"encoding/base64"
	"os"
	"os/signal"

	// Paprika
	"github.com/paprikaci/paprika/data"

	// Cider
	"github.com/cider/go-cider/cider/services/rpc"
	ws "github.com/cider/go-cider/cider/transports/websocket/rpc"

	// Others
	"code.google.com/p/go.net/websocket"
	"github.com/wsxiaoys/terminal/color"
)

const TokenHeader = "X-Paprika-Token"

const (
	OK   = "[ @{g}OK@{|} ]\n"
	FAIL = "[ @{r}FAIL@{|} ]\n"
)

func call(method string, args interface{}, result *data.BuildResult) error {
	// Create a Cider RPC client that uses WebSocket transport.
	verbose("@{c}>>>@{|} Initialising the RPC client (using WebSocket) ... ")
	client, err := rpc.NewService(func() (rpc.Transport, error) {
		factory := ws.NewTransportFactory()
		factory.Server = master
		factory.Origin = "http://localhost"
		factory.WSConfigFunc = func(config *websocket.Config) {
			config.Header.Set(TokenHeader, token)
		}
		return factory.NewTransport("paprika#" + mustRandomString())
	})
	if err != nil {
		verbose(FAIL)
		return err
	}
	verbose(OK)
	defer client.Close()

	// Start catching signals.
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt)

	// Configure the RPC call.
	call := client.NewRemoteCall(method, args)
	call.Stdout = os.Stdout
	call.Stderr = os.Stderr

	// Execute the remote call.
	verbose("@{c}>>>@{|} Calling ", method, " ... ")
	call.GoExecute()
	verbose(OK)

	// Wait for the remote call to be resolved.
	verbose("@{c}>>>@{|} Combined output\n")
	select {
	case <-call.Resolved():
	case <-signalCh:
		color.Println("@{r}---> Interrupting the build job")
		if err := call.Interrupt(); err != nil {
			return err
		}
	}
	verbose("@{c}<<<@{|} Combined output\n")
	if err := call.Wait(); err != nil {
		return err
	}

	// Return the results.
	verbose("@{c}>>>@{|} Return code:  ", call.ReturnCode(), "\n")
	err = call.UnmarshalReturnValue(&result)
	verbose("@{c}>>>@{|} Return value: ", result, "\n")
	return err
}

func mustRandomString() string {
	buf := make([]byte, 10)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}
	return base64.StdEncoding.EncodeToString(buf)
}

func verbose(v ...interface{}) {
	if verboseMode {
		color.Fprint(os.Stderr, v...)
	}
}
