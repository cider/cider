// Copyright (c) 2013 The meeko AUTHORS
//
// Use of this source code is governed by The MIT License
// that can be found in the LICENSE file.

package supervisor

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/meeko/go-meeko/meeko/services/rpc"
	log "github.com/meeko/meekod/broker/exchanges/logging/publisher"
	"github.com/meeko/meekod/supervisor/data"
	"io"
	"io/ioutil"
	"labix.org/v2/mgo"
	"os"
	"time"
)

const TerminationTimeout = 5 * time.Second

type Supervisor struct {
	agents        *mgo.Collection
	session       *mgo.Session
	token         []byte
	impl          Implementation
	logs          *log.Publisher
	cmdChans      []chan rpc.RemoteRequest
	termCh        chan struct{}
	loopTermAckCh chan struct{}
	termAckCh     chan struct{}
}

const (
	AgentStateStopped = "stopped"
	AgentStateRunning = "running"
	AgentStateCrashed = "crashed"
	AgentStateKilled  = "killed"
)

type AgentStateChange struct {
	Alias     string
	FromState string
	ToState   string
}

type ActionContext interface {
	SignalProgress() error
	Stdout() io.Writer
	Stderr() io.Writer
	Interrupted() <-chan struct{}
}

type Implementation interface {
	Install(alias string, repo string, ctx ActionContext) (*data.Agent, error)
	Upgrade(agent *data.Agent, ctx ActionContext) error
	Remove(agent *data.Agent, ctx ActionContext) error

	Start(agent *data.Agent, ctx ActionContext) error
	Stop(alias string, ctx ActionContext) error
	StopWithTimeout(alias string, ctx ActionContext, timeout time.Duration) error
	Kill(alias string, ctx ActionContext) error
	Restart(agent *data.Agent, ctx ActionContext) error
	Status(alias string, ctx ActionContext) (status string, err error)
	Statuses(ctx ActionContext) (statuses map[string]string, err error)

	AgentStateChangeFeed() <-chan *AgentStateChange
	CloseAgentStateChangeFeed()

	Terminate(timeout time.Duration)
}

func New(impl Implementation, workspace, mgoURL, token string, logs *log.Publisher) (*Supervisor, error) {
	// Make sure the directory is there. Not sure it is writable for Meeko,
	// that is not what MkdirAll checks.
	if err := os.MkdirAll(workspace, 0750); err != nil {
		return nil, err
	}

	// Connect to the database. No extended configuration supported for now.
	session, err := mgo.Dial(mgoURL)
	if err != nil {
		return nil, fmt.Errorf("MongoDB driver: %v", err)
	}

	session.SetMode(mgo.Strong, false)
	session.SetSafe(&mgo.Safe{})

	// Ensure database indexes.
	agents := session.DB("").C("agents")
	err = agents.EnsureIndex(mgo.Index{
		Key:    []string{"alias"},
		Unique: true,
	})
	if err != nil {
		session.Close()
		return nil, err
	}

	// Initialise a new Supervisor instance.
	sup := &Supervisor{
		agents:        agents,
		session:       session,
		token:         []byte(token),
		impl:          impl,
		logs:          logs,
		cmdChans:      make([]chan rpc.RemoteRequest, numCmds),
		termCh:        make(chan struct{}),
		loopTermAckCh: make(chan struct{}),
		termAckCh:     make(chan struct{}),
	}

	for i := range sup.cmdChans {
		sup.cmdChans[i] = make(chan rpc.RemoteRequest)
	}

	// Try to start agents that are supposed to be running.
	var agent data.Agent
	ctx := NewNilActionContext()
	iter := agents.Find(nil).Iter()
	for iter.Next(&agent) {
		if agent.Enabled {
			impl.Start(&agent, ctx)
		}
	}
	if err := iter.Err(); err != nil {
		session.Close()
		return nil, err
	}

	// Return the newly created agent service.
	go sup.loop()
	return sup, nil
}

func (sup *Supervisor) authenticate(token []byte) error {
	if !bytes.Equal(token, sup.token) {
		return errors.New("invalid token")
	}
	return nil
}

func (sup *Supervisor) Terminate() {
	sup.TerminateWithin(TerminationTimeout)
}

func (sup *Supervisor) TerminateWithin(timeout time.Duration) {
	select {
	case <-sup.termCh:
	default:
		close(sup.termCh)
		<-sup.loopTermAckCh
		sup.session.Close()
		sup.impl.Terminate(timeout)
		close(sup.termAckCh)
	}
}

func (sup *Supervisor) Terminated() <-chan struct{} {
	return sup.termAckCh
}

// Helper nilActionContext -----------------------------------------------------

type nilActionContext struct{}

func NewNilActionContext() ActionContext {
	return &nilActionContext{}
}

func (ctx *nilActionContext) SignalProgress() error {
	return nil
}

func (ctx *nilActionContext) Stdout() io.Writer {
	return ioutil.Discard
}

func (ctx *nilActionContext) Stderr() io.Writer {
	return ioutil.Discard
}

func (ctx *nilActionContext) Interrupted() <-chan struct{} {
	return nil
}

// Errors ----------------------------------------------------------------------

type ErrNotDefined struct {
	What string
}

func (err *ErrNotDefined) Error() string {
	return fmt.Sprintf("%s not defined", err.What)
}
