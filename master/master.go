// Copyright (c) 2014 The cider AUTHORS
//
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

package master

import (
	// Stdlib
	"errors"
	"net/http"
	"time"

	// Cider
	"github.com/meeko/meekod/broker"
	"github.com/meeko/meekod/broker/exchanges/rpc/roundrobin"
	"github.com/meeko/meekod/broker/transports/websocket/rpc"

	// Others
	"code.google.com/p/go.net/websocket"
)

const TokenHeader = "X-Meeko-Token"

var ErrInvalidToken = errors.New("Invalid access token")

// BuildMaster represents a Cider CI master node that serves as the hub where
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
		config := &rpc.EndpointConfig{
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
		return rpc.NewEndpoint(config, balancer)
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
