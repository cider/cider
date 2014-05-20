// Copyright (c) 2013 The meeko AUTHORS
//
// Use of this source code is governed by The MIT License
// that can be found in the LICENSE file.

package supervisor

import (
	"fmt"
	"github.com/meeko/go-meeko/meeko/services/rpc"
	"github.com/meeko/meekod/broker/services/logging"
	"github.com/meeko/meekod/supervisor/data"
	"github.com/wsxiaoys/terminal/color"
	"io"
	"labix.org/v2/mgo/bson"
	"reflect"
)

const (
	cmdList = iota
	cmdInstall
	cmdUpgrade
	cmdRemove
	cmdInfo
	cmdEnv
	cmdSet
	cmdUnset
	cmdStart
	cmdStop
	cmdRestart
	cmdStatus
	cmdWatch
	numCmds
)

func (sup *Supervisor) ExportManagementMethods(client *rpc.Service) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = r.(error)
		}
	}()

	client.MustRegisterMethod("Meeko.Agent.List", sup.handleList)
	client.MustRegisterMethod("Meeko.Agent.Install", sup.handleInstall)
	client.MustRegisterMethod("Meeko.Agent.Upgrade", sup.handleUpgrade)
	client.MustRegisterMethod("Meeko.Agent.Remove", sup.handleRemove)

	client.MustRegisterMethod("Meeko.Agent.Info", sup.handleInfo)
	client.MustRegisterMethod("Meeko.Agent.Env", sup.handleEnv)
	client.MustRegisterMethod("Meeko.Agent.Set", sup.handleSet)
	client.MustRegisterMethod("Meeko.Agent.Unset", sup.handleUnset)

	client.MustRegisterMethod("Meeko.Agent.Start", sup.handleStart)
	client.MustRegisterMethod("Meeko.Agent.Stop", sup.handleStop)
	client.MustRegisterMethod("Meeko.Agent.Restart", sup.handleRestart)
	client.MustRegisterMethod("Meeko.Agent.Status", sup.handleStatus)
	client.MustRegisterMethod("Meeko.Agent.Watch", sup.handleWatch)

	return
}

func (sup *Supervisor) enqueueCmdAndWait(cmd int, request rpc.RemoteRequest) {
	select {
	case sup.cmdChans[cmd] <- request:
	case <-request.Interrupted():
		request.Resolve(1, map[string]string{"error": "interrupted"})
	case <-sup.termCh:
		request.Resolve(2, map[string]string{"error": "terminated"})
	}

	<-request.Resolved()
}

func (sup *Supervisor) loop() {
	handlers := []rpc.RequestHandler{
		cmdList:    sup.safeHandleList,
		cmdInstall: sup.safeHandleInstall,
		cmdUpgrade: sup.safeHandleUpgrade,
		cmdRemove:  sup.safeHandleRemove,
		cmdInfo:    sup.safeHandleInfo,
		cmdEnv:     sup.safeHandleEnv,
		cmdSet:     sup.safeHandleSet,
		cmdUnset:   sup.safeHandleUnset,
		cmdStart:   sup.safeHandleStart,
		cmdStop:    sup.safeHandleStop,
		cmdRestart: sup.safeHandleRestart,
		cmdStatus:  sup.safeHandleStatus,
		cmdWatch:   sup.safeHandleWatch,
	}

	cases := make([]reflect.SelectCase, len(sup.cmdChans)+1)
	for i := range sup.cmdChans {
		cases[i] = reflect.SelectCase{
			Dir:  reflect.SelectRecv,
			Chan: reflect.ValueOf(sup.cmdChans[i]),
		}
	}
	termChIndex := len(cases) - 1
	cases[termChIndex] = reflect.SelectCase{
		Dir:  reflect.SelectRecv,
		Chan: reflect.ValueOf(sup.termCh),
	}

	for {
		chosen, recv, _ := reflect.Select(cases)
		switch chosen {
		case termChIndex:
			close(sup.loopTermAckCh)
			return
		default:
			handlers[chosen](recv.Interface().(rpc.RemoteRequest))
		}
	}
}

// List ------------------------------------------------------------------------

func (sup *Supervisor) handleList(request rpc.RemoteRequest) {
	sup.enqueueCmdAndWait(cmdList, request)
}

func (sup *Supervisor) safeHandleList(request rpc.RemoteRequest) {
	var args data.ListArgs
	if err := request.UnmarshalArgs(&args); err != nil {
		request.Resolve(3, data.ListReply{Error: err.Error()})
		return
	}

	if err := sup.authenticate(args.Token); err != nil {
		request.Resolve(4, data.ListReply{Error: err.Error()})
		return
	}

	var agents []data.Agent
	if err := sup.agents.Find(nil).All(&agents); err != nil {
		request.Resolve(5, data.ListReply{Error: err.Error()})
	}

	request.Resolve(0, data.ListReply{Agents: agents})
}

// Install ---------------------------------------------------------------------

func (sup *Supervisor) handleInstall(request rpc.RemoteRequest) {
	sup.enqueueCmdAndWait(cmdInstall, request)
}

func (sup *Supervisor) safeHandleInstall(request rpc.RemoteRequest) {
	var args data.InstallArgs
	if err := request.UnmarshalArgs(&args); err != nil {
		request.Resolve(3, data.InstallReply{Error: err.Error()})
		return
	}

	if err := sup.authenticate(args.Token); err != nil {
		request.Resolve(4, data.InstallReply{Error: err.Error()})
		return
	}

	n, err := sup.agents.Find(bson.M{"alias": args.Alias}).Count()
	if err != nil {
		request.Resolve(5, data.InstallReply{Error: err.Error()})
		return
	}
	if n != 0 {
		request.Resolve(6, data.InstallReply{Error: "alias already taken"})
		return
	}

	agent, err := sup.impl.Install(args.Alias, args.Repository, request)
	if err != nil {
		request.Resolve(7, data.InstallReply{Error: err.Error()})
		return
	}

	agent.Id = bson.NewObjectId()

	color.Fprintf(request.Stdout(), "@{c}>>>@{|} Inserting the agent database record ... ")
	if err := sup.agents.Insert(agent); err != nil {
		fail(request.Stdout())
		sup.impl.Remove(agent, request)
		request.Resolve(8, data.InstallReply{Error: err.Error()})
		return
	}
	ok(request.Stdout())

	request.Resolve(0, data.InstallReply{})
}

// Upgrade ---------------------------------------------------------------------

func (sup *Supervisor) handleUpgrade(request rpc.RemoteRequest) {
	sup.enqueueCmdAndWait(cmdUpgrade, request)
}

func (sup *Supervisor) safeHandleUpgrade(request rpc.RemoteRequest) {
	var args data.UpgradeArgs
	if err := request.UnmarshalArgs(&args); err != nil {
		request.Resolve(3, data.UpgradeReply{Error: err.Error()})
		return
	}

	if err := sup.authenticate(args.Token); err != nil {
		request.Resolve(4, data.UpgradeReply{Error: err.Error()})
		return
	}

	var agent data.Agent
	if err := sup.agents.Find(bson.M{"alias": args.Alias}).One(&agent); err != nil {
		request.Resolve(5, data.UpgradeReply{Error: err.Error()})
		return
	}

	if err := sup.impl.Upgrade(&agent, request); err != nil {
		request.Resolve(6, data.UpgradeReply{Error: err.Error()})
		return
	}

	request.Resolve(0, data.UpgradeReply{})
}

// Remove ----------------------------------------------------------------------

func (sup *Supervisor) handleRemove(request rpc.RemoteRequest) {
	sup.enqueueCmdAndWait(cmdRemove, request)
}

func (sup *Supervisor) safeHandleRemove(request rpc.RemoteRequest) {
	var args data.RemoveArgs
	if err := request.UnmarshalArgs(&args); err != nil {
		request.Resolve(3, data.RemoveReply{Error: err.Error()})
		return
	}

	if err := sup.authenticate(args.Token); err != nil {
		request.Resolve(4, data.RemoveReply{Error: err.Error()})
		return
	}

	var agent data.Agent
	if err := sup.agents.Find(bson.M{"alias": args.Alias}).One(&agent); err != nil {
		request.Resolve(5, data.RemoveReply{Error: err.Error()})
		return
	}

	if err := sup.impl.Remove(&agent, request); err != nil {
		request.Resolve(6, data.RemoveReply{Error: err.Error()})
		return
	}

	color.Fprint(request.Stdout(), "@{c}>>>@{|} Deleting the agent database record ... ")
	if err := sup.agents.RemoveId(agent.Id); err != nil {
		fail(request.Stdout())
		request.Resolve(7, data.RemoveReply{Error: err.Error()})
		return
	}
	ok(request.Stdout())

	request.Resolve(0, data.RemoveReply{})
}

// Info ------------------------------------------------------------------------

func (sup *Supervisor) handleInfo(request rpc.RemoteRequest) {
	sup.enqueueCmdAndWait(cmdInfo, request)
}

func (sup *Supervisor) safeHandleInfo(request rpc.RemoteRequest) {
	var args data.InfoArgs
	if err := request.UnmarshalArgs(&args); err != nil {
		request.Resolve(3, data.InfoReply{Error: err.Error()})
		return
	}

	if err := sup.authenticate(args.Token); err != nil {
		request.Resolve(4, data.InfoReply{Error: err.Error()})
		return
	}

	var agent data.Agent
	if err := sup.agents.Find(bson.M{"alias": args.Alias}).One(&agent); err != nil {
		request.Resolve(5, data.InfoReply{Error: err.Error()})
		return
	}

	for _, v := range agent.Vars {
		if v.Secret && v.Value != "" {
			v.Value = "<secret>"
		}
	}

	request.Resolve(0, data.InfoReply{Agent: agent})
}

// Env -------------------------------------------------------------------------

func (sup *Supervisor) handleEnv(request rpc.RemoteRequest) {
	sup.enqueueCmdAndWait(cmdEnv, request)
}

func (sup *Supervisor) safeHandleEnv(request rpc.RemoteRequest) {
	var args data.EnvArgs
	if err := request.UnmarshalArgs(&args); err != nil {
		request.Resolve(3, data.EnvReply{Error: err.Error()})
		return
	}

	if err := sup.authenticate(args.Token); err != nil {
		request.Resolve(4, data.EnvReply{Error: err.Error()})
		return
	}

	var agent data.Agent
	err := sup.agents.Find(bson.M{"alias": args.Alias}).Select(bson.M{"vars": 1}).One(&agent)
	if err != nil {
		request.Resolve(5, data.EnvReply{Error: err.Error()})
		return
	}

	for _, v := range agent.Vars {
		if v.Secret {
			v.Value = "<secret>"
		}
	}

	request.Resolve(0, data.EnvReply{Vars: agent.Vars})
}

// Set -------------------------------------------------------------------------

func (sup *Supervisor) handleSet(request rpc.RemoteRequest) {
	sup.enqueueCmdAndWait(cmdSet, request)
}

func (sup *Supervisor) safeHandleSet(request rpc.RemoteRequest) {
	var args data.SetArgs
	if err := request.UnmarshalArgs(&args); err != nil {
		request.Resolve(3, data.SetReply{Error: err.Error()})
		return
	}

	if err := sup.authenticate(args.Token); err != nil {
		request.Resolve(4, data.SetReply{Error: err.Error()})
		return
	}

	var agent data.Agent
	err := sup.agents.Find(bson.M{"alias": args.Alias}).Select(bson.M{"vars": 1}).One(&agent)
	if err != nil {
		request.Resolve(5, data.SetReply{Error: err.Error()})
		return
	}

	vr, ok := agent.Vars[args.Variable]
	if !ok {
		request.Resolve(6, data.SetReply{Error: "unknown variable"})
		return
	}

	if err := vr.Set(args.Value); err != nil {
		request.Resolve(7, data.SetReply{Error: err.Error()})
		return
	}

	err = sup.agents.UpdateId(agent.Id, bson.M{"$set": bson.M{"vars." + args.Variable + ".value": vr.Value}})
	if err != nil {
		request.Resolve(8, data.SetReply{Error: err.Error()})
		return
	}

	request.Resolve(0, data.SetReply{})
}

// Unset -----------------------------------------------------------------------

func (sup *Supervisor) handleUnset(request rpc.RemoteRequest) {
	sup.enqueueCmdAndWait(cmdUnset, request)
}

func (sup *Supervisor) safeHandleUnset(request rpc.RemoteRequest) {
	var args data.UnsetArgs
	if err := request.UnmarshalArgs(&args); err != nil {
		request.Resolve(3, data.UnsetReply{Error: err.Error()})
		return
	}

	if err := sup.authenticate(args.Token); err != nil {
		request.Resolve(4, data.UnsetReply{Error: err.Error()})
		return
	}

	var agent data.Agent
	err := sup.agents.Find(bson.M{"alias": args.Alias}).Select(bson.M{"vars": 1}).One(&agent)
	if err != nil {
		request.Resolve(5, data.UnsetReply{Error: err.Error()})
		return
	}

	vr, ok := agent.Vars[args.Variable]
	if !ok {
		request.Resolve(6, data.UnsetReply{Error: "unknown variable"})
		return
	}

	if vr.Value != "" {
		err = sup.agents.UpdateId(agent.Id, bson.M{"$set": bson.M{"vars." + args.Variable + ".value": ""}})
		if err != nil {
			request.Resolve(7, data.UnsetReply{Error: err.Error()})
			return
		}
	}

	request.Resolve(0, data.UnsetReply{})
}

// Start -----------------------------------------------------------------------

func (sup *Supervisor) handleStart(request rpc.RemoteRequest) {
	sup.enqueueCmdAndWait(cmdStart, request)
}

func (sup *Supervisor) safeHandleStart(request rpc.RemoteRequest) {
	var args data.StartArgs
	if err := request.UnmarshalArgs(&args); err != nil {
		request.Resolve(3, data.StartReply{Error: err.Error()})
		return
	}

	if err := sup.authenticate(args.Token); err != nil {
		request.Resolve(4, data.StartReply{Error: err.Error()})
		return
	}

	var agent data.Agent
	if err := sup.agents.Find(bson.M{"alias": args.Alias}).One(&agent); err != nil {
		request.Resolve(5, data.StartReply{Error: err.Error()})
		return
	}

	if err := agent.Vars.Filled(); err != nil {
		request.Resolve(6, data.StartReply{Error: err.Error()})
		return
	}

	err := sup.agents.UpdateId(agent.Id, bson.M{"$set": bson.M{"enabled": true}})
	if err != nil {
		request.Resolve(7, data.StartReply{Error: err.Error()})
		return
	}

	if args.Watch {
		go sup.watchAgent(args.Alias, logging.LevelUnset, request)
	}

	if err := sup.impl.Start(&agent, request); err != nil {
		request.Resolve(8, data.StartReply{Error: err.Error()})
		return
	}

	if !args.Watch {
		request.Resolve(0, data.StartReply{})
	}
}

// Stop ------------------------------------------------------------------------

func (sup *Supervisor) handleStop(request rpc.RemoteRequest) {
	sup.enqueueCmdAndWait(cmdStop, request)
}

func (sup *Supervisor) safeHandleStop(request rpc.RemoteRequest) {
	var args data.StopArgs
	if err := request.UnmarshalArgs(&args); err != nil {
		request.Resolve(3, data.StopReply{Error: err.Error()})
		return
	}

	if err := sup.authenticate(args.Token); err != nil {
		request.Resolve(4, data.StopReply{Error: err.Error()})
		return
	}

	if err := sup.impl.StopWithTimeout(args.Alias, request, args.Timeout); err != nil {
		request.Resolve(5, data.StopReply{Error: err.Error()})
		return
	}

	err := sup.agents.Update(bson.M{"alias": args.Alias}, bson.M{"$set": bson.M{"enabled": false}})
	if err != nil {
		request.Resolve(6, data.StopReply{Error: err.Error()})
		return
	}

	request.Resolve(0, data.StopReply{})
}

// Restart ---------------------------------------------------------------------

func (sup *Supervisor) handleRestart(request rpc.RemoteRequest) {
	sup.enqueueCmdAndWait(cmdRestart, request)
}

func (sup *Supervisor) safeHandleRestart(request rpc.RemoteRequest) {
	var args data.RestartArgs
	if err := request.UnmarshalArgs(&args); err != nil {
		request.Resolve(3, data.RestartReply{Error: err.Error()})
		return
	}

	if err := sup.authenticate(args.Token); err != nil {
		request.Resolve(4, data.RestartReply{Error: err.Error()})
		return
	}

	var agent data.Agent
	if err := sup.agents.Find(bson.M{"alias": args.Alias}).One(&agent); err != nil {
		request.Resolve(5, data.RestartReply{Error: err.Error()})
		return
	}

	if err := sup.impl.Restart(&agent, request); err != nil {
		request.Resolve(6, data.StartReply{Error: err.Error()})
		return
	}

	request.Resolve(0, data.RestartReply{})
}

// Status ----------------------------------------------------------------------

func (sup *Supervisor) handleStatus(request rpc.RemoteRequest) {
	sup.enqueueCmdAndWait(cmdStatus, request)
}

func (sup *Supervisor) safeHandleStatus(request rpc.RemoteRequest) {
	var args data.StatusArgs
	if err := request.UnmarshalArgs(&args); err != nil {
		request.Resolve(3, data.StatusReply{Error: err.Error()})
		return
	}

	if err := sup.authenticate(args.Token); err != nil {
		request.Resolve(4, data.StatusReply{Error: err.Error()})
		return
	}

	var reply data.StatusReply
	if args.Alias != "" {
		status, err := sup.impl.Status(args.Alias, request)
		if err != nil {
			request.Resolve(5, data.StatusReply{Error: err.Error()})
			return
		}
		reply.Status = status
	} else {
		statuses, err := sup.impl.Statuses(request)
		if err != nil {
			request.Resolve(5, data.StatusReply{Error: err.Error()})
			return
		}
		reply.Statuses = statuses
	}

	request.Resolve(0, &reply)
}

// Watch -----------------------------------------------------------------------

func (sup *Supervisor) handleWatch(request rpc.RemoteRequest) {
	sup.enqueueCmdAndWait(cmdWatch, request)
}

func (sup *Supervisor) safeHandleWatch(request rpc.RemoteRequest) {
	if sup.logs == nil {
		request.Resolve(9, data.WatchReply{Error: "log watching is disabled"})
		return
	}

	var args data.WatchArgs
	if err := request.UnmarshalArgs(&args); err != nil {
		request.Resolve(3, data.WatchReply{Error: err.Error()})
		return
	}

	if err := sup.authenticate(args.Token); err != nil {
		request.Resolve(4, data.WatchReply{Error: err.Error()})
		return
	}

	status, err := sup.impl.Status(args.Alias, request)
	if err != nil {
		request.Resolve(5, data.WatchReply{Error: err.Error()})
		return
	}

	if status != AgentStateRunning {
		request.Resolve(6, data.WatchReply{Error: ErrAgentNotRunning.Error()})
		return
	}

	go sup.watchAgent(args.Alias, logging.Level(args.Level), request)
}

// XXX: If someone stops the agent while someone else is watching,
//      the logs just stop streaming. That could be improved.
func (sup *Supervisor) watchAgent(alias string, level logging.Level, request rpc.RemoteRequest) {
	color.Fprintf(request.Stdout(), "@{c}>>>@{|} Streaming logs for agent %s\n", alias)
	handle, err := sup.logs.Subscribe(alias, level,
		func(level logging.Level, record []byte) {
			fmt.Fprintf(request.Stdout(), "[%v] %s\n", level, string(record))
		})
	if err != nil {
		request.Resolve(9, data.WatchReply{Error: err.Error()})
		return
	}

	<-request.Interrupted()

	if err := sup.logs.Unsubscribe(handle); err != nil {
		request.Resolve(10, data.WatchReply{Error: err.Error()})
		return
	}

	request.Resolve(0, data.WatchReply{})
}

// Helpers

func ok(w io.Writer) {
	color.Fprintln(w, "@{g}OK@{|}")
}

func fail(w io.Writer) {
	color.Fprintln(w, "@{r}FAIL@{|}")
}

func newlineOk(w io.Writer) {
	color.Fprintln(w, "@{c}<<<@{|} @{g}OK@{|}")
}

func newlineFail(w io.Writer) {
	color.Fprintln(w, "@{c}<<<@{|} @{r}FAIL@{|}")
}
