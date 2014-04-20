// Copyright (c) 2013 The cider AUTHORS
//
// Use of this source code is governed by The MIT License
// that can be found in the LICENSE file.

package logging

import (
	"bytes"
	"errors"

	"github.com/cider/cider/broker/log"
	"github.com/cider/cider/broker/services/logging"
	"github.com/cider/cider/broker/transports/zmq3/loop"

	"github.com/dmotylev/nutrition"
	zmq "github.com/pebbe/zmq3"
)

// Constants -------------------------------------------------------------------

const Header = "CDR#LOGGING@01"

var HeaderFrame = []byte(Header)

// Errors ----------------------------------------------------------------------

var ErrConfigIncomplete = errors.New("Logging endpoint is not fully configured")

//------------------------------------------------------------------------------
// EndpointConfig
//------------------------------------------------------------------------------

type EndpointConfig struct {
	Endpoint string
	Rcvhwm   int
}

func NewEndpointConfig() *EndpointConfig {
	// Keep ZeroMQ defaults.
	return &EndpointConfig{
		Rcvhwm: 1000,
	}
}

func (config *EndpointConfig) FeedFromEnv(prefix string) error {
	log.Debug("zmq3<Logging>: Feeding config from the environment")
	return nutrition.Env(prefix).Feed(config)
}

func (config *EndpointConfig) MustFeedFromEnv(prefix string) *EndpointConfig {
	if err := config.FeedFromEnv(prefix); err != nil {
		panic(err)
	}
	return config
}

func (config *EndpointConfig) IsComplete() bool {
	return config.Endpoint != ""
}

func (config *EndpointConfig) MustBeComplete() {
	if !config.IsComplete() {
		panic(ErrConfigIncomplete)
	}
}

//------------------------------------------------------------------------------
// Endpoint
//------------------------------------------------------------------------------

type Endpoint struct {
	handler logging.Exchange
}

func NewEndpoint(config *EndpointConfig, handler logging.Exchange) (logging.Endpoint, error) {
	// Make sure the configuration is complete.
	if !config.IsComplete() {
		return nil, ErrConfigIncomplete
	}

	log.Debugf("zmq3<Logging>: Instantiating endpoint using %#v", config)

	// Set up the logging socket of PULL type.
	sock, err := zmq.NewSocket(zmq.PULL)
	if err != nil {
		return nil, err
	}

	if config.Rcvhwm != 0 {
		err = sock.SetRcvhwm(config.Rcvhwm)
		if err != nil {
			sock.Close()
			return nil, err
		}
	}

	err = sock.Bind(config.Endpoint)
	if err != nil {
		sock.Close()
		return nil, err
	}

	// Prepare an internal Endpoint instance.
	ep := &Endpoint{handler}

	// Initialise the message loop. The ownership of the logging socket is passed
	// onto the loop, it takes care of the cleanup on termination.
	//
	// The loop is actually returned instead of the endpoint itself since it
	// implements logging.Endpoint as well.
	return loop.New(loop.PollItems{
		{sock, ep.handleMessage},
	}), nil
}

// Processing of incoming messages ---------------------------------------------

func (ep *Endpoint) handleMessage(msg [][]byte) {
	appName := string(msg[0])

	switch {
	case len(msg) != 4:
		log.Warnf("zmq3[LOGGING]: Invalid message received from %s: message too short", appName)
		return
	case !bytes.Equal(msg[1], HeaderFrame):
		log.Warnf("zmq3[LOGGING]: Invalid message received from %s: header mismatch", appName)
		return
	case len(msg[2]) != 1 || msg[2][0] > byte(logging.LevelCritical):
		log.Warnf("zmq3[LOGGING]: Invalid message received from %s: invalid log level", appName)
		return
	}

	ep.handler.Log(appName, logging.Level(msg[2][0]), msg[3])
}
