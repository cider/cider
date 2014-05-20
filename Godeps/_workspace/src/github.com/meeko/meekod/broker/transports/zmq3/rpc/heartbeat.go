// Copyright (c) 2013 The meeko AUTHORS
//
// Use of this source code is governed by The MIT License
// that can be found in the LICENSE file.

package rpc

import (
	"sync"
	"time"

	"github.com/meeko/meekod/broker/log"
	"github.com/meeko/meekod/broker/services/rpc"

	"github.com/dmotylev/nutrition"
)

// HeartbeatConfig -------------------------------------------------------------

const (
	DefaultHeartbeatPeriod  = 3 * time.Second
	DefaultHeartbeatTimeout = 3 * DefaultHeartbeatPeriod
)

type HeartbeatConfig struct {
	Enabled bool
	Period  time.Duration
	Timeout time.Duration
}

func newHeartbeatConfig() *HeartbeatConfig {
	return &HeartbeatConfig{
		Period:  DefaultHeartbeatPeriod,
		Timeout: DefaultHeartbeatTimeout,
	}
}

func (config *HeartbeatConfig) FeedConfigFromEnv(prefix string) error {
	return nutrition.Env(prefix).Feed(config)
}

func (config *HeartbeatConfig) MustFeedConfigFromEnv(prefix string) {
	if err := config.FeedConfigFromEnv(prefix); err != nil {
		panic(err)
	}
}

func (config *HeartbeatConfig) newHeartbeat(ex rpc.Exchange, ep *Endpoint) exchange {
	if config.Enabled {
		return newEnabledHeartbeat(config, ex, ep)
	} else {
		return newDisabledHeartbeat(ex, ep)
	}
}

// disabledHeartbeat -----------------------------------------------------------

// disabledHeartbeat just provides an implementation of heartbeat that does nothing.
type disabledHeartbeat struct {
	rpc.Exchange
	*Endpoint
}

func newDisabledHeartbeat(ex rpc.Exchange, ep *Endpoint) exchange {
	return &disabledHeartbeat{
		Exchange: ex,
		Endpoint: ep,
	}
}

func (beat *disabledHeartbeat) RegisterApp(appName string) {
	return
}

func (beat *disabledHeartbeat) Pong(appName string) {
	return
}

// enabledHeartbeat ------------------------------------------------------------

type enabledHeartbeat struct {
	config *HeartbeatConfig

	rpc.Exchange
	*Endpoint

	task       *periodicTask
	timestamps map[string]time.Time
	mu         *sync.Mutex
}

func newEnabledHeartbeat(config *HeartbeatConfig, ex rpc.Exchange, ep *Endpoint) exchange {
	return &enabledHeartbeat{
		config:     config,
		Exchange:   ex,
		Endpoint:   ep,
		timestamps: make(map[string]time.Time),
		mu:         new(sync.Mutex),
	}
}

func (beat *enabledHeartbeat) RegisterMethod(appName string, ep rpc.Endpoint, method string) error {
	log.Debugf("zmq3<RPC>: starting heartbeat for application %s", appName)
	beat.mu.Lock()
	if beat.task == nil {
		beat.task = every(beat.config.Period, func(t *periodicTask) {
			beat.mu.Lock()

			now := time.Now()
			for appName, timestamp := range beat.timestamps {
				if delta := now.Sub(timestamp); delta > beat.config.Timeout {
					log.Debugf("zmq3<RPC>: heartbeat timed out for %s", appName)
					beat.Exchange.UnregisterApp(appName)
				} else {
					beat.timestamps[appName] = now
				}
			}

			if len(beat.timestamps) == 0 {
				t.Pause()
				beat.mu.Unlock()
				return
			}

			for appName := range beat.timestamps {
				log.Debugf("zmq3<RPC>: sending ping to %s", appName)
				if err := beat.Endpoint.Ping([]byte(appName)); err != nil {
					log.Criticalf("zmq3<RPC>: failed to send PING: %v", err)
				}
			}

			beat.mu.Unlock()
		})
	}
	beat.unsafePong(appName)
	beat.mu.Unlock()

	return beat.Exchange.RegisterMethod(appName, ep, method)
}

func (beat *enabledHeartbeat) UnregisterApp(appName string) {
	beat.mu.Lock()
	delete(beat.timestamps, appName)
	if len(beat.timestamps) == 0 {
		beat.task.Pause()
	}
	beat.Exchange.UnregisterApp(appName)
	beat.mu.Unlock()
}

func (beat *enabledHeartbeat) UnregisterEndpoint(ep rpc.Endpoint) {
	beat.mu.Lock()
	if beat.task != nil {
		beat.task.Stop()
	}
	beat.Exchange.UnregisterEndpoint(ep)
	beat.mu.Unlock()
}

func (beat *enabledHeartbeat) Pong(appName string) {
	beat.mu.Lock()
	beat.unsafePong(appName)
	beat.mu.Unlock()
}

func (beat *enabledHeartbeat) unsafePong(appName string) {
	beat.timestamps[appName] = time.Now()
}

// periodicTask ----------------------------------------------------------------

const (
	ptCmdPause byte = iota
	ptCmdResume
	ptCmdStop
)

type periodicTask struct {
	cmdCh     chan byte
	stoppedCh chan struct{}
}

func every(period time.Duration, task func(*periodicTask)) *periodicTask {
	pt := &periodicTask{
		cmdCh:     make(chan byte, 1),
		stoppedCh: make(chan struct{}),
	}

	go func() {
		ticker := time.NewTicker(period)
		for {
			select {
			case <-ticker.C:
				task(pt)
			case cmd := <-pt.cmdCh:
				switch cmd {
				case ptCmdPause:
					ticker.Stop()
					ticker.C = nil
				case ptCmdResume:
					if ticker.C == nil {
						ticker = time.NewTicker(period)
					}
				case ptCmdStop:
					ticker.Stop()
					close(pt.stoppedCh)
					return
				default:
					panic("Unknown command encountered")
				}
			}
		}
	}()

	return pt
}

func (pt *periodicTask) Pause() {
	pt.cmdCh <- ptCmdPause
}

func (pt *periodicTask) Resume() {
	pt.cmdCh <- ptCmdResume
}

func (pt *periodicTask) Stop() {
	pt.cmdCh <- ptCmdStop
	<-pt.stoppedCh
}
