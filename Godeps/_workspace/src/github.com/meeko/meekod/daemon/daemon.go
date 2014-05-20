// Copyright (c) 2013-2014 The meeko AUTHORS
//
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

// This package wraps the meekod functionality into a Daemon struct that
// can be imported and started inside of any Go program.
package daemon

import (
	// Stdlib
	"errors"
	"net/http"

	// Meeko broker
	"github.com/meeko/meekod/broker"
	"github.com/meeko/meekod/broker/exchanges/logging/publisher"
	"github.com/meeko/meekod/broker/exchanges/pubsub/eventbus"
	"github.com/meeko/meekod/broker/exchanges/rpc/roundrobin"
	wsrpc "github.com/meeko/meekod/broker/transports/websocket/rpc"
	zlogging "github.com/meeko/meekod/broker/transports/zmq3/logging"
	zpubsub "github.com/meeko/meekod/broker/transports/zmq3/pubsub"
	zrpc "github.com/meeko/meekod/broker/transports/zmq3/rpc"

	// Meeko client
	rpc_client "github.com/meeko/go-meeko/meeko/services/rpc"
	rpc_inproc "github.com/meeko/go-meeko/meeko/transports/inproc/rpc"

	// Meeko apps
	"github.com/meeko/meekod/supervisor"
	"github.com/meeko/meekod/supervisor/implementations/exec"

	// Others
	ws "code.google.com/p/go.net/websocket"
	log "github.com/cihub/seelog"
	zmq "github.com/pebbe/zmq3"
)

var ErrTerminated = errors.New("daemon already terminated")

// Options can be used to define what parts of meekod will be started.
// This can be used to align meekod for a couple of scenarios.
type Options struct {

	// Do not start the agent supervisor.
	DisableSupervisor bool

	// Disable the inter-process service endpoints.
	// These are used to plug the local agents into the platform.
	// Can be useful in case only WebSocket is being used.
	DisableLocalEndpoints bool
}

type Daemon struct {
	config    *Config
	opts      *Options
	broker    *broker.Broker
	termCh    chan struct{}
	termAckCh chan struct{}
}

func NewFromConfig(config *Config, opts *Options) (*Daemon, error) {
	if config == nil {
		return nil, errors.New("nil meekod config is not allowed")
	}
	if opts == nil {
		opts = new(Options)
	}

	if !opts.DisableSupervisor {
		if err := config.ensureSupervisorConfig(); err != nil {
			return nil, err
		}
	}
	if !opts.DisableLocalEndpoints {
		if err := config.ensureZmqConfig(); err != nil {
			return nil, err
		}
	}

	return newDaemon(config, opts), nil
}

func NewFromConfigAsFile(path string, opts *Options) (*Daemon, error) {
	if path == "" {
		return nil, errors.New("meekod configuration file path is empty")
	}

	config, err := ReadConfigFile(path)
	if err != nil {
		return nil, err
	}

	if err := config.PopulateEnviron(); err != nil {
		return nil, err
	}

	return NewFromConfig(config, opts)
}

func newDaemon(config *Config, opts *Options) *Daemon {
	return &Daemon{
		config:    config,
		opts:      opts,
		broker:    broker.New(),
		termCh:    make(chan struct{}),
		termAckCh: make(chan struct{}),
	}
}

func (daemon *Daemon) Monitor(monitorCh chan<- *broker.EndpointCrashReport) {
	daemon.broker.Monitor(monitorCh)
}

func (daemon *Daemon) Serve() error {
	select {
	case <-daemon.termCh:
		return ErrTerminated
	default:
	}

	defer func() {
		close(daemon.termAckCh)
	}()

	var (
		config = daemon.config
		brookr = daemon.broker
	)

	// Prepare the service exchanges.
	var (
		logger   = publisher.New()
		pubsub   = eventbus.New()
		balancer = roundrobin.NewBalancer()
	)

	// Register a special inproc service endpoint.
	var (
		inprocClient *rpc_client.Service
		err          error
	)
	if !daemon.opts.DisableSupervisor {
		transport := rpc_inproc.NewTransport("Meeko", balancer)
		inprocClient, err = rpc_client.NewService(func() (rpc_client.Transport, error) {
			return transport, nil
		})
		if err != nil {
			return err
		}

		// Register service endpoints with the broker.
		brookr.RegisterEndpointFactory("meeko_rpc_inproc", func() (broker.Endpoint, error) {
			log.Info("Configuring Meeko management RPC inproc transport...")
			return transport.AsEndpoint(), nil
		})
	}

	if !daemon.opts.DisableLocalEndpoints {
		// Register all the ZeroMQ service endpoints.
		log.Info("Configuring ZeroMQ 3.x endpoint for RPC...")
		brookr.RegisterEndpointFactory("zmq3_rpc", func() (broker.Endpoint, error) {
			cfg := zrpc.NewEndpointConfig()
			cfg.Endpoint = config.Broker.Endpoints.RPC.ZeroMQ
			cfg.MustBeComplete()
			return zrpc.NewEndpoint(cfg, balancer)
		})

		log.Info("Configuring ZeroMQ 3.x endpoint for PubSub...")
		brookr.RegisterEndpointFactory("zmq3_pubsub", func() (broker.Endpoint, error) {
			cfg := zpubsub.NewEndpointConfig()
			cfg.RouterEndpoint = config.Broker.Endpoints.PubSub.ZeroMQ.Router
			cfg.PubEndpoint = config.Broker.Endpoints.PubSub.ZeroMQ.Pub
			cfg.MustBeComplete()
			return zpubsub.NewEndpoint(cfg, pubsub)
		})

		log.Info("Configuring ZeroMQ 3.x endpoint for Logging...")
		brookr.RegisterEndpointFactory("zmq3_logging", func() (broker.Endpoint, error) {
			cfg := zlogging.NewEndpointConfig()
			cfg.Endpoint = config.Broker.Endpoints.Logging.ZeroMQ
			cfg.MustBeComplete()
			return zlogging.NewEndpoint(cfg, logger)
		})
	}

	// Register the WebSocket RPC endpoint. This one is always enabled
	// so that the management utility can connect to Meeko.
	log.Info("Configuring WebSocket endpoint for RPC...")
	brookr.RegisterEndpointFactory("websocket_rpc", func() (broker.Endpoint, error) {
		cfg := wsrpc.NewEndpointConfig()
		cfg.Addr = config.Broker.Endpoints.RPC.WebSocket.Address
		token := config.Broker.Endpoints.RPC.WebSocket.Token
		cfg.WSHandshake = func(cfg *ws.Config, req *http.Request) error {
			if req.Header.Get("X-Meeko-Token") != token {
				return errors.New("Invalid access token")
			}
			return nil
		}
		cfg.HeartbeatPeriod = config.Broker.Endpoints.RPC.WebSocket.HeartbeatPeriod
		return wsrpc.NewEndpoint(cfg, balancer)
	})

	var sup *supervisor.Supervisor
	if !daemon.opts.DisableSupervisor {
		// Start the supervisor.
		supImpl, err := exec.NewSupervisor(config.Supervisor.Workspace)
		if err != nil {
			return err
		}
		// TODO: This is a weird method name and anyway, do we need it?
		//       There should be probably a method for enabling such a channel.
		supImpl.CloseAgentStateChangeFeed()

		sup, err = supervisor.New(
			supImpl,
			config.Supervisor.Workspace,
			config.Supervisor.MongoDbUrl,
			config.Supervisor.Token,
			logger)
		if err != nil {
			return err
		}
		// Export Meeko management calls.
		if err := sup.ExportManagementMethods(inprocClient); err != nil {
			return err
		}
	}

	// Start the service endpoints.
	brookr.ListenAndServe()
	go func() {
		select {
		case <-brookr.Terminated():
			select {
			case <-daemon.termCh:
			default:
				close(daemon.termCh)
			}
		case <-daemon.termCh:
		}
	}()

	// Wait for the termination signal, then terminate everything.
	<-daemon.termCh
	if sup != nil {
		sup.Terminate()
	}
	brookr.Terminate()
	return nil
}

// Terminate shuts the daemon down.
//
// This method blocks until the daemon is terminated.
//
// ErrTerminated is returned in case Terminate has already been called.
func (daemon *Daemon) Terminate() error {
	select {
	case <-daemon.termCh:
		return ErrTerminated
	default:
		close(daemon.termCh)
	}
	<-daemon.termAckCh
	return nil
}

// TerminateZmq calls Terminate(), then zmq.Term().
func (daemon *Daemon) TerminateZmq() error {
	if err := daemon.Terminate(); err != nil {
		return err
	}
	return zmq.Term()
}

// Terminated returns a channel that is closed when the daemon is terminated.
func (daemon *Daemon) Terminated() <-chan struct{} {
	return daemon.termAckCh
}
