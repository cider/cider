// Copyright 2014 The AUTHORS
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
	"errors"
	"net/http"
	"time"

	// Cider
	"github.com/cider/cider/broker"
	"github.com/cider/cider/broker/exchanges/rpc/roundrobin"
	"github.com/cider/cider/broker/transports/websocket/rpc"

	// Others
	"code.google.com/p/go.net/websocket"
)

const TokenHeader = "X-Paprika-Token"

var ErrInvalidToken = errors.New("Invalid access token")

// BuildMaster represents a Paprika CI master node that serves as the hub where
// all the build slaves and build clients exchange data between each other.
type BuildMaster struct {
	address   string
	token     string
	heartbeat time.Duration
	broker    *broker.Broker
	termCh    chan struct{}
	err       error
}

// NewBuildMaster is the BuildMaster constructor function, surprisingly.
func New(address, token string) *BuildMaster {
	return &BuildMaster{
		address: address,
		token:   token,
		broker:  broker.New(),
		termCh:  make(chan struct{}),
	}
}

// EnableHeartbeat enables heartbeat using the specified period.
func (m *BuildMaster) EnableHeartbeat(period time.Duration) *BuildMaster {
	m.heartbeat = period
	return m
}

// Listen makes the build master listen and start accepting connections
// on the specified network address. It uses WebSocket as the transport
// protocol, so it actually starts a WebSocket server in the server root.
func (m *BuildMaster) Listen() *BuildMaster {
	// Prepare the RPC service exchange.
	balancer := roundrobin.NewBalancer()

	// Start monitoring the broker.
	m.broker = broker.New()
	monitorCh := make(chan *broker.EndpointCrashReport)
	m.broker.Monitor(monitorCh)

	// Set up the RPC service endpoint using WebSocket as the transport.
	m.broker.RegisterEndpointFactory("websocket_rpc", func() (broker.Endpoint, error) {
		factory := rpc.EndpointFactory{
			Addr: m.address,
			WSHandshake: func(cfg *websocket.Config, req *http.Request) error {
				// Make sure that the a valid access token is present in the request.
				if req.Header.Get(TokenHeader) != m.token {
					return ErrInvalidToken
				}
				return nil
			},
			HeartbeatPeriod: m.heartbeat,
		}

		return factory.NewEndpoint(balancer), nil
	})

	// Start the registered endpoint.
	m.broker.ListenAndServe()

	// Start checking for endpoint errors and broker termination.
	// broker.Terminate() closes monitorCh, which in turn closes termCh,
	// which marks the whole BuildMaster as terminated.
	go func() {
		for {
			select {
			case report, ok := <-monitorCh:
				if !ok {
					close(m.termCh)
					return
				}
				m.err = report.Error
			}
		}
	}()

	return m
}

// Terminate the build master.
func (m *BuildMaster) Terminate() {
	m.broker.Terminate()
}

// Terminated returns a channel that is closed once the master is terminated.
func (m *BuildMaster) Terminated() <-chan struct{} {
	return m.termCh
}

// Wait blocks until Terminate is called or the master crashes, then it returns
// any internal master node error that was encountered.
func (m *BuildMaster) Wait() error {
	<-m.Terminated()
	return m.err
}
