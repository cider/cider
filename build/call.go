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

package build

import (
	// Stdlib
	"crypto/rand"
	"encoding/base64"
	"fmt"
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

const TokenHeader = "X-Cider-Token"

func call(master, token, method string, args interface{}, result *data.BuildResult) error {
	// Make sure all arguments are set.
	var unset string
	switch {
	case master == "":
		unset = "master"
	case token == "":
		unset = "token"
	case method == "":
		unset = "method"
	case args == nil:
		unset = "args"
	case result == nil:
		unset = "result"
	}
	if unset != "" {
		panic(fmt.Errorf("call(): argument is empty: %v", unset))
	}

	// Create a Cider RPC client that uses WebSocket transport.
	fmt.Printf("---> Connecting to %v\n", master)
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
		return err
	}
	defer client.Close()

	fmt.Printf("---> Sending the build request (using method %q)\n", method)

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
