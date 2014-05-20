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

package slave

import (
	// Stdlib
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	// Cider
	"github.com/cider/cider/slave/runners"

	// Meeko
	"github.com/meeko/go-meeko/meeko/services/rpc"
	ws "github.com/meeko/go-meeko/meeko/transports/websocket/rpc"

	// Others
	"code.google.com/p/go.net/websocket"
	log "github.com/cihub/seelog"
)

const TokenHeader = "X-Meeko-Token"

const (
	errorCalmPeriod = 10 * time.Second
	errorThreshold  = 5

	minBackoff = time.Second
	maxBackoff = time.Minute
)

var (
	ErrConnected    = errors.New("build slave already connected")
	ErrDisconnected = errors.New("build slave has not been connected")
)

type BuildSlave struct {
	identity     string
	workspace    string
	numExecutors uint
	service      *rpc.Service
	mu           *sync.Mutex
}

func New(identity, workspace string, numExecutors uint) *BuildSlave {
	return &BuildSlave{
		identity:     identity,
		workspace:    workspace,
		numExecutors: numExecutors,
		mu:           new(sync.Mutex),
	}
}

func (slave *BuildSlave) Connect(master, token string) (err error) {
	// Connect to the master node using the WebSocket transport.
	// The specified token is used to authenticated the build slave.
	slave.mu.Lock()
	if slave.service != nil {
		return ErrConnected
	}
	log.Infof("Connecting to %v", master)
	service, err := rpc.NewService(func() (rpc.Transport, error) {
		factory := ws.NewTransportFactory()
		factory.Server = master
		factory.Origin = "http://localhost"
		factory.WSConfigFunc = func(config *websocket.Config) {
			config.Header.Set(TokenHeader, token)
		}
		return factory.NewTransport(slave.identity)
	})
	if err != nil {
		slave.mu.Unlock()
		return err
	}
	slave.service = service
	slave.mu.Unlock()

	// Number of concurrent builds is limited by creating a channel of the
	// specified length. Every time a build is requested, the request handler
	// sends some data to the channel, and when it is finished, it reads data
	// from the same channel.
	execQueue := make(chan bool, slave.numExecutors)
	log.Infof("Initiating %v build executor(s)", slave.numExecutors)

	// Export all available labels and runners.
	log.Info("Available runners:")
	for _, runner := range runners.Available {
		log.Infof("---> %v", runner.Name)
	}

	manager := newWorkspaceManager(slave.workspace)

	ls := []string{"any"}
	if labels != "" {
		ls = append(ls, strings.Split(labels, ",")...)
	}

	for _, label := range ls {
		for _, runner := range runners.Available {
			methodName := fmt.Sprintf("cider.%v.%v", label, runner.Name)
			builder := &Builder{runner, manager, execQueue}
			if ex := service.RegisterMethod(methodName, builder.Build); ex != nil {
				err = ex
				goto Close
			}
		}
	}

	log.Info("Waiting for build requests...")
	goto Wait

Close:
	if ex := service.Close(); ex != nil {
		log.Warn(err)
		return
	}

Wait:
	if ex := service.Wait(); ex != nil {
		if err == nil {
			err = ex
			return
		}
		log.Warn(err)
	}
	return
}

func (slave *BuildSlave) Terminate() error {
	slave.mu.Lock()
	defer slave.mu.Unlock()
	if slave.service == nil {
		return ErrDisconnected
	}
	return slave.service.Close()
}

func (slave *BuildSlave) Terminated() <-chan struct{} {
	slave.mu.Lock()
	defer slave.mu.Unlock()
	if slave.service == nil {
		ch := make(chan struct{})
		close(ch)
		return ch
	}
	return slave.service.Closed()
}

func (slave *BuildSlave) Wait() error {
	slave.mu.Lock()
	defer slave.mu.Unlock()
	if slave.service == nil {
		return ErrDisconnected
	}
	return slave.service.Wait()
}
