// Copyright (c) 2013 The meeko AUTHORS
//
// Use of this source code is governed by The MIT License
// that can be found in the LICENSE file.

package exec

import (
	// Stdlib
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	// Meeko
	"github.com/meeko/meekod/broker/log"
	"github.com/meeko/meekod/supervisor"
	"github.com/meeko/meekod/supervisor/data"
	"github.com/meeko/meekod/supervisor/utils/executil"
	"github.com/meeko/meekod/supervisor/utils/vcsutil"

	// Others
	"github.com/wsxiaoys/terminal/color"
)

const DefaultKillTimeout = 5 * time.Second

type Supervisor struct {
	workspace string

	records   map[string]*agentRecord
	recordsMu *sync.Mutex

	stopCh chan *stopCmd
	killCh chan *stopCmd
	waitCh chan *exitEvent

	feedCh       chan *supervisor.AgentStateChange
	feedClosedCh chan struct{}

	termCh    chan struct{}
	termAckCh chan struct{}
}

type agentRecord struct {
	state   string
	process *os.Process
	termCh  chan struct{}
}

type stopCmd struct {
	alias   string
	ctx     supervisor.ActionContext
	timeout time.Duration
	errCh   chan error
}

type exitEvent struct {
	alias string
	err   error
}

func NewSupervisor(workspace string) (*Supervisor, error) {
	if workspace == "" {
		return nil, &supervisor.ErrNotDefined{"Meeko workspace"}
	}

	if err := os.MkdirAll(workspace, 0750); err != nil {
		return nil, err
	}

	sup := &Supervisor{
		workspace:    workspace,
		records:      make(map[string]*agentRecord),
		recordsMu:    new(sync.Mutex),
		stopCh:       make(chan *stopCmd),
		killCh:       make(chan *stopCmd),
		waitCh:       make(chan *exitEvent),
		feedCh:       make(chan *supervisor.AgentStateChange),
		feedClosedCh: make(chan struct{}),
		termCh:       make(chan struct{}),
		termAckCh:    make(chan struct{}),
	}
	go sup.loop()
	return sup, nil
}

func (sup *Supervisor) getOrCreateRecord(alias string) *agentRecord {
	record, ok := sup.records[alias]
	if ok {
		return record
	}

	record = &agentRecord{
		state: supervisor.AgentStateStopped,
	}

	sup.records[alias] = record
	return record
}

// supervisor.Supervisor interface ---------------------------------------------------

func (sup *Supervisor) Install(alias string, repo string, ctx supervisor.ActionContext) (*data.Agent, error) {
	repoURL, err := url.Parse(repo)
	if err != nil {
		return nil, err
	}

	vcs, err := vcsutil.GetVCS(repoURL.Scheme)
	if err != nil {
		return nil, err
	}

	var (
		agentDir   = sup.agentDir(alias)
		stagingDir = sup.agentStagingDir(alias)
	)

	color.Fprint(ctx.Stdout(), "@{c}>>>@{|} Creating the agent workspace ... ")
	if err := os.Mkdir(agentDir, 0750); err != nil {
		printFAIL(ctx.Stdout())
		return nil, err
	}
	printOK(ctx.Stdout())

	color.Fprintln(ctx.Stdout(), "@{c}>>>@{|} Cloning the agent repository ... ")
	if err := vcs.Clone(repoURL, stagingDir, ctx); err != nil {
		newlineFAIL(ctx.Stdout())
		os.Remove(agentDir)
		return nil, err
	}
	newlineOK(ctx.Stdout())

	color.Fprint(ctx.Stdout(), "@{c}>>>@{|} Reading agent.json ... ")
	content, err := ioutil.ReadFile(filepath.Join(stagingDir, ".meeko", "agent.json"))
	if err != nil {
		printFAIL(ctx.Stdout())
		os.RemoveAll(agentDir)
		return nil, err
	}

	var agent data.Agent
	if err := json.Unmarshal(content, &agent); err != nil {
		printFAIL(ctx.Stdout())
		os.RemoveAll(agentDir)
		return nil, fmt.Errorf("Failed to parse agent.json: %v", err)
	}
	printOK(ctx.Stdout())

	agent.Alias = alias
	agent.Repository = repo

	color.Fprint(ctx.Stdout(), "@{c}>>>@{|} Validating agent.json ... ")
	if err := agent.FillAndValidate(); err != nil {
		printFAIL(ctx.Stdout())
		os.RemoveAll(agentDir)
		return nil, err
	}
	printOK(ctx.Stdout())

	srcDir := sup.agentSrcDir(&agent)
	color.Fprint(ctx.Stdout(), "@{c}>>>@{|} Moving files into place ... ")
	if err := os.MkdirAll(filepath.Dir(srcDir), 0750); err != nil {
		printFAIL(ctx.Stdout())
		os.RemoveAll(agentDir)
		return nil, err
	}

	if err := os.Rename(stagingDir, srcDir); err != nil {
		printFAIL(ctx.Stdout())
		os.RemoveAll(agentDir)
		return nil, err
	}
	printOK(ctx.Stdout())

	color.Fprintln(ctx.Stdout(), "@{c}>>>@{|} Running the install hook ... ")
	agent.Alias = alias
	if err := sup.runHook(&agent, "install", ctx); err != nil {
		newlineFAIL(ctx.Stdout())
		os.RemoveAll(agentDir)
		return nil, err
	}
	newlineOK(ctx.Stdout())

	return &agent, nil
}

func (sup *Supervisor) Upgrade(agent *data.Agent, ctx supervisor.ActionContext) error {
	mustHaveAlias(agent)
	mustHaveRepository(agent)

	repoURL, err := url.Parse(agent.Repository)
	if err != nil {
		return err
	}

	vcs, err := vcsutil.GetVCS(repoURL.Scheme)
	if err != nil {
		return err
	}

	color.Fprintln(ctx.Stdout(), "@{c}>>>@{|} Pulling the agent repository ... ")
	if err := vcs.Pull(repoURL, sup.agentSrcDir(agent), ctx); err != nil {
		newlineFAIL(ctx.Stdout())
		return err
	}
	newlineOK(ctx.Stdout())

	color.Fprintln(ctx.Stdout(), "@{c}>>>@{|} Running the upgrade hook ... ")
	if err := sup.runHook(agent, "upgrade", ctx); err != nil {
		newlineFAIL(ctx.Stdout())
		return err
	}
	newlineOK(ctx.Stdout())

	color.Fprint(ctx.Stdout(), "@{c}>>>@{|} Restarting the agent if running ... \n")
	if err := sup.Stop(agent.Alias, ctx); err != nil {
		if err == supervisor.ErrUnknownAlias || err == supervisor.ErrAgentNotRunning {
			printOK(ctx.Stdout())
			return nil
		}
		printFAIL(ctx.Stdout())
		return err
	}

	if err := sup.Start(agent, ctx); err != nil {
		printFAIL(ctx.Stdout())
		return err
	}
	printOK(ctx.Stdout())
	return nil
}

func (sup *Supervisor) Remove(agent *data.Agent, ctx supervisor.ActionContext) error {
	mustHaveAlias(agent)

	sup.recordsMu.Lock()
	defer sup.recordsMu.Unlock()

	// The agent must not be running.
	record, ok := sup.records[agent.Alias]
	if ok && record.state == supervisor.AgentStateRunning {
		return supervisor.ErrAgentRunning
	}

	// Remove the agent directory.
	color.Fprintf(ctx.Stdout(), "@{c}>>>@{|} Removing the agent workspace ... ")
	if err := os.RemoveAll(sup.agentDir(agent.Alias)); err != nil {
		printFAIL(ctx.Stdout())
		return err
	}
	printOK(ctx.Stdout())

	// Delete the agent record.
	delete(sup.records, agent.Alias)
	return nil
}

func (sup *Supervisor) Start(agent *data.Agent, ctx supervisor.ActionContext) error {
	mustHaveAlias(agent)
	alias := agent.Alias

	log.Infof("Starting agent %v", alias)
	sup.recordsMu.Lock()
	defer sup.recordsMu.Unlock()

	// Make sure the agent record exists.
	record := sup.getOrCreateRecord(alias)

	// The agent is not supposed to be running.
	if record.state == supervisor.AgentStateRunning {
		return supervisor.ErrAgentRunning
	}

	// Start the agent.
	var (
		bin = sup.agentBinDir(agent)
		exe = sup.agentExecutable(agent)
		run = sup.agentRunDir(agent)
	)
	if err := os.MkdirAll(run, 0750); err != nil {
		return err
	}

	sep := make([]byte, utf8.RuneLen(os.PathListSeparator))
	utf8.EncodeRune(sep, os.PathListSeparator)
	path := strings.Join([]string{os.Getenv("PATH"), bin}, string(sep))

	cmd := exec.Command(exe)
	cmd.Env = []string{
		"PATH=" + path,
		"GOPATH=" + sup.agentDir(agent.Alias),
		"MEEKO_ALIAS=" + alias,
	}
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "MEEKO_") {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	if agent.Vars != nil {
		for k, v := range agent.Vars {
			if v.Value != "" {
				cmd.Env = append(cmd.Env, k+"="+v.Value)
			}
		}
	}
	cmd.Dir = sup.agentRunDir(agent)

	if err := cmd.Start(); err != nil {
		log.Warnf("Agent %v failed to start: %v", alias, err)
		return err
	}
	log.Infof("Agent %v started", alias)
	record.state = supervisor.AgentStateRunning
	record.process = cmd.Process
	record.termCh = make(chan struct{})
	sup.emitStateChange(alias, supervisor.AgentStateStopped, supervisor.AgentStateRunning)

	// Start a monitoring goroutine that waits for the process to exit.
	go func() {
		sup.waitCh <- &exitEvent{alias, cmd.Wait()}
	}()

	return nil
}

func (sup *Supervisor) Stop(alias string, ctx supervisor.ActionContext) error {
	return sup.StopWithTimeout(alias, ctx, -1)
}

func (sup *Supervisor) Kill(alias string, ctx supervisor.ActionContext) error {
	return sup.StopWithTimeout(alias, ctx, 0)
}

func (sup *Supervisor) StopWithTimeout(alias string, ctx supervisor.ActionContext, timeout time.Duration) error {
	errCh := make(chan error, 1)
	sup.stopCh <- &stopCmd{alias, ctx, timeout, errCh}
	return <-errCh
}

func (sup *Supervisor) Restart(agent *data.Agent, ctx supervisor.ActionContext) error {
	mustHaveAlias(agent)

	if err := sup.Stop(agent.Alias, ctx); err != nil {
		return err
	}

	return sup.Start(agent, ctx)
}

func (sup *Supervisor) Status(alias string, ctx supervisor.ActionContext) (status string, err error) {
	sup.recordsMu.Lock()
	defer sup.recordsMu.Unlock()

	record, ok := sup.records[alias]
	if !ok {
		err = supervisor.ErrUnknownAlias
		return
	}

	return record.state, nil
}

func (sup *Supervisor) Statuses(ctx supervisor.ActionContext) (statuses map[string]string, err error) {
	sup.recordsMu.Lock()
	defer sup.recordsMu.Unlock()

	statuses = make(map[string]string, len(sup.records))
	for k, v := range sup.records {
		statuses[k] = v.state
	}

	return
}

func (sup *Supervisor) AgentStateChangeFeed() <-chan *supervisor.AgentStateChange {
	return sup.feedCh
}

func (sup *Supervisor) CloseAgentStateChangeFeed() {
	select {
	case <-sup.feedClosedCh:
	default:
		close(sup.feedClosedCh)
	}
}

func (sup *Supervisor) Terminate(timeout time.Duration) {
	select {
	case <-sup.termCh:
	default:
		return
	}

	var wg sync.WaitGroup
	ctx := supervisor.NewNilActionContext()

	sup.recordsMu.Lock()
	for alias, agent := range sup.records {
		if agent.state == supervisor.AgentStateRunning {
			wg.Add(1)
			go func(name string) {
				defer wg.Done()
				sup.StopWithTimeout(name, ctx, timeout)
			}(alias)
		}
	}
	sup.recordsMu.Unlock()

	wg.Wait()
	close(sup.termCh)
	<-sup.termAckCh
	return
}

// Internal event loop ---------------------------------------------------------

func (sup *Supervisor) loop() {
	for {
		select {
		case cmd := <-sup.stopCh:
			log.Infof("Stopping agent %s", cmd.alias)
			sup.recordsMu.Lock()
			record, ok := sup.records[cmd.alias]
			if !ok {
				sup.recordsMu.Unlock()
				cmd.errCh <- supervisor.ErrUnknownAlias
				continue
			}
			if record.state != supervisor.AgentStateRunning {
				sup.recordsMu.Unlock()
				cmd.errCh <- supervisor.ErrAgentNotRunning
				continue
			}

			switch cmd.timeout {
			case 0:
				sup.recordsMu.Unlock()
				go func() {
					sup.killCh <- cmd
				}()
				continue
			case -1:
				cmd.timeout = DefaultKillTimeout
			}

			fmt.Fprintf(cmd.ctx.Stdout(), "Interrupting agent %s...\n", cmd.alias)
			if err := record.process.Signal(os.Interrupt); err != nil {
				sup.recordsMu.Unlock()
				cmd.errCh <- err
				continue
			}

			termCh := record.termCh
			go func() {
				select {
				case <-termCh:
					log.Infof("Agent %s terminated", cmd.alias)
					cmd.errCh <- nil
				case <-time.After(cmd.timeout):
					sup.killCh <- cmd
				}
			}()

			sup.recordsMu.Unlock()

		case cmd := <-sup.killCh:
			log.Infof("Killing agent %s", cmd.alias)
			sup.recordsMu.Lock()
			record := sup.records[cmd.alias]
			// Check if the agent is still running. It is possible that
			// the exit event is received on waitCh between the check on stopCh
			// and this receive on killCh.
			if record.state != supervisor.AgentStateRunning {
				sup.recordsMu.Unlock()
				cmd.errCh <- nil
				continue
			}

			fmt.Fprintf(cmd.ctx.Stdout(), "Killing agent %s...\n", cmd.alias)
			if err := record.process.Signal(os.Kill); err != nil {
				sup.recordsMu.Unlock()
				cmd.errCh <- err
				continue
			}
			record.state = supervisor.AgentStateKilled

			termCh := record.termCh
			go func() {
				<-termCh
				log.Infof("Agent %s terminated", cmd.alias)
				fmt.Fprintf(cmd.ctx.Stdout(), "Agent %s terminated\n", cmd.alias)
				cmd.errCh <- nil
			}()

			sup.recordsMu.Unlock()

		case event := <-sup.waitCh:
			sup.recordsMu.Lock()
			record, ok := sup.records[event.alias]
			if !ok {
				sup.recordsMu.Unlock()
				panic(supervisor.ErrUnknownAlias)
			}

			if record.state != supervisor.AgentStateKilled {
				if event.err == nil {
					record.state = supervisor.AgentStateStopped
				} else {
					record.state = supervisor.AgentStateCrashed
				}
			}

			record.process = nil
			close(record.termCh)
			record.termCh = nil
			sup.recordsMu.Unlock()

			sup.emitStateChange(event.alias, supervisor.AgentStateRunning, record.state)

		case <-sup.termCh:
			close(sup.termAckCh)
			return
		}
	}
}

// Private methods -------------------------------------------------------------

func (sup *Supervisor) runHook(agent *data.Agent, hook string, ctx supervisor.ActionContext) error {
	var (
		agentDir    = sup.agentDir(agent.Alias)
		agentSrcDir = sup.agentSrcDir(agent)
		agentBinDir = sup.agentBinDir(agent)
		agentRunDir = sup.agentRunDir(agent)
	)

	exe := filepath.Join(agentSrcDir, ".meeko", "hooks", hook)
	cmd := exec.Command(exe)
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"GOPATH=" + agentDir,
		"MEEKO_HOMEDIR=" + agentDir,
		"MEEKO_SRCDIR=" + filepath.Dir(agentSrcDir),
		"MEEKO_APP_SRCDIR=" + agentSrcDir,
		"MEEKO_BINDIR=" + agentBinDir,
		"MEEKO_RUNDIR=" + agentRunDir,
	}
	cmd.Dir = agentSrcDir
	cmd.Stdout = ctx.Stdout()
	cmd.Stderr = ctx.Stderr()

	return executil.Run(cmd, ctx.Interrupted())
}

func (sup *Supervisor) emitStateChange(alias, from, to string) {
	select {
	case sup.feedCh <- &supervisor.AgentStateChange{alias, from, to}:
	case <-sup.feedClosedCh:
	}
}

func (sup *Supervisor) agentDir(alias string) string {
	return filepath.Join(sup.workspace, alias)
}

func (sup *Supervisor) agentStagingDir(alias string) string {
	return filepath.Join(sup.workspace, alias, "_stage")
}

func (sup *Supervisor) agentSrcDir(agent *data.Agent) string {
	mustHaveAlias(agent)
	mustHaveName(agent)
	return filepath.Join(sup.agentDir(agent.Alias), "src", agent.Name)
}

func (sup *Supervisor) agentBinDir(agent *data.Agent) string {
	return filepath.Join(sup.agentDir(agent.Alias), "bin")
}

func (sup *Supervisor) agentRunDir(agent *data.Agent) string {
	mustHaveName(agent)
	return filepath.Join(sup.agentDir(agent.Alias), "run")
}

func (sup *Supervisor) agentExecutable(agent *data.Agent) string {
	mustHaveName(agent)
	return filepath.Join(sup.agentBinDir(agent), agent.Name)
}

func mustHaveAlias(agent *data.Agent) {
	if agent.Alias == "" {
		panic("agent alias not set")
	}
}

func mustHaveName(agent *data.Agent) {
	if agent.Name == "" {
		panic("agent name not set")
	}
}

func mustHaveRepository(agent *data.Agent) {
	if agent.Repository == "" {
		panic("agent repository not set")
	}
}

func printOK(w io.Writer) {
	color.Fprintln(w, "@{g}OK@{|}")
}

func printFAIL(w io.Writer) {
	color.Fprintln(w, "@{r}FAIL@{|}")
}

func newlineOK(w io.Writer) {
	color.Fprintln(w, "@{c}<<<@{|} @{g}OK@{|}")
}

func newlineFAIL(w io.Writer) {
	color.Fprintln(w, "@{c}<<<@{|} @{r}FAIL@{|}")
}
