// Copyright (c) 2014 The AUTHORS
//
// This file is part of cider.
//
// cider is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// cider is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with cider.  If not, see <http://www.gnu.org/licenses/>.

package build

import (
	// Stdlib
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"os/signal"

	// Cider
	"github.com/cider/cider/data"

	// Meeko
	"github.com/meeko/go-meeko/meeko/services/rpc"
	ws "github.com/meeko/go-meeko/meeko/transports/websocket/rpc"

	// Others
	"code.google.com/p/go.net/websocket"
	"github.com/wsxiaoys/terminal/color"
)

const TokenHeader = "X-Meeko-Token"

type Session struct {
	*rpc.Service
}

func Dial(master, token string) (*Session, error) {
	service, err := rpc.NewService(func() (rpc.Transport, error) {
		factory := ws.NewTransportFactory()
		factory.Server = master
		factory.Origin = "http://localhost"
		factory.WSConfigFunc = func(config *websocket.Config) {
			config.Header.Set(TokenHeader, token)
		}
		return factory.NewTransport("cider#" + mustRandomString())
	})
	if err != nil {
		return nil, err
	}

	return &Session{service}, nil
}

func (s *Session) NewBuildRequest(method string, args *data.BuildArgs) *BuildRequest {
	return &BuildRequest{s.Service.NewRemoteCall(method, args)}
}

type BuildRequest struct {
	*rpc.RemoteCall
}

func (request *BuildRequest) Execute() (result *data.BuildResult, err error) {
	request.RemoteCall.GoExecute()
	return request.Wait()
}

func (request *BuildRequest) Wait() (result *data.BuildResult, err error) {
	err = request.RemoteCall.Wait()
	if err != nil {
		return
	}

	var res data.BuildResult
	err = request.RemoteCall.UnmarshalReturnValue(&res)
	if err != nil {
		return
	}

	result = &res
	return
}

func call(master, token, method string, args *data.BuildArgs) (*data.BuildResult, error) {
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
	}
	if unset != "" {
		panic(fmt.Errorf("call(): argument is empty: %v", unset))
	}

	// Create a Cider RPC client that uses WebSocket transport.
	fmt.Printf("---> Connecting to %v\n", master)
	session, err := Dial(master, token)
	if err != nil {
		return nil, err
	}
	defer session.Close()

	fmt.Printf("---> Sending the build request (using method %q)\n", method)

	// Start catching signals.
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt)

	// Configure the RPC call.
	call := session.NewBuildRequest(method, args)
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
		fmt.Println("---> Interrupting the build job, this can take a few seconds")
		if err := call.Interrupt(); err != nil {
			return nil, err
		}
	}
	verbose("@{c}<<<@{|} Combined output\n")
	result, err := call.Wait()
	if err != nil {
		return nil, err
	}

	// Return the results.
	verbose("@{c}>>>@{|} Return code:  ", call.ReturnCode(), "\n")
	verbose("@{c}>>>@{|} Return value: ", result, "\n")
	return result, err
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
