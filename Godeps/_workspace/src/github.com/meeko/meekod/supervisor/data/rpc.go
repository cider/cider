// Copyright (c) 2013 The meeko AUTHORS
//
// Use of this source code is governed by The MIT License
// that can be found in the LICENSE file.

package data

import "time"

// List ------------------------------------------------------------------------

type ListArgs struct {
	Token []byte `codec:"token"`
}

type ListReply struct {
	Agents []Agent `codec:"agent,omitempty"`
	Error  string  `codec:"error,omitempty"`
}

// Install ---------------------------------------------------------------------

type InstallArgs struct {
	Token      []byte `codec:"token"`
	Alias      string `codec:"alias"`
	Repository string `codec:"repository"`
}

type InstallReply struct {
	Error string `codec:"error"`
}

// Upgrade ---------------------------------------------------------------------

type UpgradeArgs struct {
	Token []byte `codec:"token"`
	Alias string `codec:"alias"`
}

type UpgradeReply struct {
	Error string `codec:"error"`
}

// Remove ----------------------------------------------------------------------

type RemoveArgs struct {
	Token []byte `codec:"token"`
	Alias string `codec:"alias"`
}

type RemoveReply struct {
	Error string `codec:"error"`
}

// Info ------------------------------------------------------------------------

type InfoArgs struct {
	Token []byte `codec:"token"`
	Alias string `codec:"alias"`
}

type InfoReply struct {
	Agent Agent  `codec:"agent,omitempty"`
	Error string `codec:"error,omitempty"`
}

// Env -------------------------------------------------------------------------

type EnvArgs struct {
	Token []byte `codec:"token"`
	Alias string `codec:"alias"`
}

type EnvReply struct {
	Vars  Variables `codec:"vars,omitempty"`
	Error string    `codec:"error,omitempty"`
}

// Set -------------------------------------------------------------------------

type SetArgs struct {
	Token     []byte `codec:"token"`
	Alias     string `codec:"alias"`
	Variable  string `codec:"variable"`
	Value     string `codec:"value"`
	CopyFrom  string `codec:"from"`
	MergeMode string `codec:"mergeMode"`
}

type SetReply struct {
	Error string `codec:"error"`
}

// Unset -----------------------------------------------------------------------

type UnsetArgs struct {
	Token    []byte `codec:"token"`
	Alias    string `codec:"alias"`
	Variable string `codec:"variable"`
}

type UnsetReply struct {
	Error string `codec:"error"`
}

// Start -----------------------------------------------------------------------

type StartArgs struct {
	Token []byte `codec:"token"`
	Alias string `codec:"alias"`
	Watch bool   `codec:"watch,omitempty"`
}

type StartReply struct {
	Error string `codec:"error"`
}

// Restart ---------------------------------------------------------------------

type RestartArgs struct {
	Token []byte `codec:"token"`
	Alias string `codec:"alias"`
}

type RestartReply struct {
	Error string `codec:"error"`
}

// Stop ------------------------------------------------------------------------

type StopArgs struct {
	Token   []byte        `codec:"token"`
	Alias   string        `codec:"alias"`
	Timeout time.Duration `codec:"timeout"`
}

type StopReply struct {
	Error string `codec:"error"`
}

// Status ----------------------------------------------------------------------

type StatusArgs struct {
	Token []byte `codec:"token"`
	Alias string `codec:"alias,omitempty"`
}

type StatusReply struct {
	Status   string            `codec:"status,omitempty"`
	Statuses map[string]string `codec:"statuses,omitempty"`
	Error    string            `codec:"error,omitempty"`
}

// Watch -----------------------------------------------------------------------

type WatchArgs struct {
	Token []byte `codec:"token"`
	Alias string `codec:"alias"`
	Level uint32 `codec:"level"`
}

type WatchReply struct {
	Error string `codec:"error"`
}
