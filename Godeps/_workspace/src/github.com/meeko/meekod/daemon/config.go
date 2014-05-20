// Copyright (c) 2013-2014 The meeko AUTHORS
//
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

package daemon

import (
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"gopkg.in/yaml.v1"
)

type Config struct {
	Verbose bool
	Broker  *struct {
		Endpoints *struct {
			Logging *struct {
				ZeroMQ string `yaml:"zeromq"`
			} `yaml:"logging"`
			PubSub *struct {
				ZeroMQ *struct {
					Router string
					Pub    string
				} `yaml:"zeromq"`
			} `yaml:"pubsub"`
			RPC *struct {
				ZeroMQ    string `yaml:"zeromq"`
				WebSocket *struct {
					Address         string
					Token           string
					HeartbeatPeriod time.Duration `yaml:"heartbeat_period"`
				} `yaml:"websocket"`
			} `yaml:"rpc"`
		}
	}
	Supervisor *struct {
		Workspace  string
		MongoDbUrl string `yaml:"mongodb_url"`
		Token      string
	}
}

func (cfg *Config) ensureCommonConfig() error {
	var missing string

	switch {
	case cfg.Broker == nil:
		missing = "broker"
	case cfg.Broker.Endpoints == nil:
		missing = "broker.endpoints"
	case cfg.Broker.Endpoints.RPC == nil:
		missing = "broker.endpoints.rpc"
	case cfg.Broker.Endpoints.RPC.WebSocket == nil:
		missing = "broker.endpoints.rpc.websocket"
	case cfg.Broker.Endpoints.RPC.WebSocket.Address == "":
		missing = "broker.endpoints.rpc.websocket.address"
	case cfg.Broker.Endpoints.RPC.WebSocket.Token == "":
		missing = "broker.endpoints.rpc.websocket.token"
	}

	if missing != "" {
		return fmt.Errorf("config key missing: %v", missing)
	}
	return nil
}

func (cfg *Config) ensureSupervisorConfig() error {
	var missing string

	switch {
	case cfg.Supervisor == nil:
		missing = "supervisor"
	case cfg.Supervisor.Workspace == "":
		missing = "supervisor.workspace"
	case cfg.Supervisor.MongoDbUrl == "":
		missing = "supervisor.mongodb_url"
	case cfg.Supervisor.Token == "":
		missing = "supervisor.token"
	}

	if missing != "" {
		return fmt.Errorf("config key missing: %v", missing)
	}
	return nil
}

func (cfg *Config) ensureZmqConfig() error {
	var missing string

	switch {
	case cfg.Broker == nil:
		missing = "broker"
	case cfg.Broker.Endpoints == nil:
		missing = "broker.endpoints"
	case cfg.Broker.Endpoints.Logging == nil:
		missing = "broker.endpoints.logging"
	case cfg.Broker.Endpoints.Logging.ZeroMQ == "":
		missing = "broker.endpoints.logging.zeromq"
	case cfg.Broker.Endpoints.PubSub == nil:
		missing = "broker.endpoints.pubsub"
	case cfg.Broker.Endpoints.PubSub.ZeroMQ == nil:
		missing = "broker.endpoints.pubsub.zeromq"
	case cfg.Broker.Endpoints.PubSub.ZeroMQ.Router == "":
		missing = "broker.endpoints.pubsub.zeromq.router"
	case cfg.Broker.Endpoints.PubSub.ZeroMQ.Pub == "":
		missing = "broker.endpoints.pubsub.zeromq.pub"
	case cfg.Broker.Endpoints.RPC == nil:
		missing = "broker.endpoints.rpc"
	case cfg.Broker.Endpoints.RPC.ZeroMQ == "":
		missing = "broker.endpoints.rpc.zeromq"
	}

	if missing != "" {
		return fmt.Errorf("config key missing: %v", missing)
	}
	return nil
}

// PopulateEnviron exports the values from the config file as environment
// variables so that the values are reachable from the agents.
//
// This is not super clean solution, there is space for rethinking this.
func (cfg *Config) PopulateEnviron() error {
	// TODO: This whole thing is pretty ugly, we should come up with a general
	//       mechanism how to pass config around...
	if err := cfg.ensureCommonConfig(); err != nil {
		return err
	}

	err := os.Setenv("MEEKO_WEBSOCKET_RPC_ADDRESS", cfg.Broker.Endpoints.RPC.WebSocket.Address)
	if err != nil {
		return err
	}
	err = os.Setenv("MEEKO_WEBSOCKET_RPC_TOKEN", cfg.Broker.Endpoints.RPC.WebSocket.Token)
	if err != nil {
		return err
	}

	if err := cfg.ensureZmqConfig(); err != nil {
		return nil
	}

	err = os.Setenv("MEEKO_ZMQ3_LOGGING_ENDPOINT", cfg.Broker.Endpoints.Logging.ZeroMQ)
	if err != nil {
		return err
	}
	err = os.Setenv("MEEKO_ZMQ3_PUBSUB_ROUTERENDPOINT", cfg.Broker.Endpoints.PubSub.ZeroMQ.Router)
	if err != nil {
		return err
	}
	err = os.Setenv("MEEKO_ZMQ3_PUBSUB_PUBENDPOINT", cfg.Broker.Endpoints.PubSub.ZeroMQ.Pub)
	if err != nil {
		return err
	}
	return os.Setenv("MEEKO_ZMQ3_RPC_ENDPOINT", cfg.Broker.Endpoints.RPC.ZeroMQ)
}

func ReadConfigFile(path string) (*Config, error) {
	content, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	config := new(Config)
	if err := yaml.Unmarshal(content, config); err != nil {
		return nil, err
	}

	if err := config.ensureCommonConfig(); err != nil {
		return nil, err
	}
	return config, nil
}
