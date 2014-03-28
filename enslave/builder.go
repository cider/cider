// Copyright (c) 2014 Salsita s.r.o.
//
// This file is part of paprika.
//
// paprika is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// paprika is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with paprika.  If not, see <http://www.gnu.org/licenses/>.

package enslave

import (
	// Stdlib
	"fmt"
	"net/url"
	"os"
	"time"

	// Paprika
	"github.com/paprikaci/paprika/data"
	"github.com/paprikaci/paprika/enslave/runners"

	// Cider
	"github.com/cider/cider/apps/utils/executil"
	"github.com/cider/cider/apps/utils/vcsutil"
	"github.com/cider/go-cider/cider/services/rpc"
)

type Builder struct {
	runner    *runners.Runner
	manager   *WorkspaceManager
	execQueue chan bool
}

func (builder *Builder) Build(request rpc.RemoteRequest) {
	// Some shortcuts.
	stdout := request.Stdout()
	stderr := request.Stderr()

	// Unmarshal and validate the input data.
	var args data.BuildArgs
	if err := request.UnmarshalArgs(&args); err != nil {
		request.Resolve(2, &data.BuildResult{Error: err.Error()})
		return
	}
	if err := args.Validate(); err != nil {
		request.Resolve(3, &data.BuildResult{Error: err.Error()})
		return
	}

	// Generate the project workspace and make sure it exists.
	repoURL, _ := url.Parse(args.Repository)
	workspace, err := builder.manager.EnsureWorkspaceExists(repoURL)
	if err != nil {
		request.Resolve(4, &data.BuildResult{Error: err.Error()})
		return
	}

	// Acquire the workspace lock.
	wsQueue := builder.manager.GetWorkspaceQueue(workspace)
	if errStr := acquire("the workspace lock", wsQueue, request); errStr != "" {
		request.Resolve(5, &data.BuildResult{Error: errStr})
		return
	}
	defer release("the workspace lock", wsQueue, request)

	// Acquire a build executor.
	if errStr := acquire("a build executor", builder.execQueue, request); errStr != "" {
		request.Resolve(5, &data.BuildResult{Error: errStr})
		return
	}
	defer release("the build executor", builder.execQueue, request)

	// Start measuring the build time.
	startTimestamp := time.Now()

	// Check out the sources at the right revision.
	srcDir := builder.manager.SrcDir(workspace)
	srcDirExists, err := builder.manager.SrcDirExists(workspace)
	if err != nil {
		resolve(request, 6, startTimestamp, err)
		return
	}

	vcs, err := vcsutil.GetVCS(repoURL.Scheme)
	if err != nil {
		resolve(request, 7, startTimestamp, err)
		return
	}

	if srcDirExists {
		err = vcs.Pull(repoURL, srcDir, request)
	} else {
		err = vcs.Clone(repoURL, srcDir, request)
	}
	if err != nil {
		resolve(request, 8, startTimestamp, err)
		return
	}

	// Run the specified script.
	cmd := builder.runner.NewCommand(args.Script)

	env := os.Environ()
	env = append(env, args.Env...)
	env = append(env, "WORKSPACE="+workspace, "SRCDIR="+srcDir)
	cmd.Env = env

	cmd.Dir = srcDir
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	fmt.Fprintf(stdout, "+---> Running the script: %v\n", args.Script)
	err = executil.Run(cmd, request.Interrupted())
	fmt.Fprintln(stdout, "+---> Build finished")
	if err != nil {
		resolve(request, 1, startTimestamp, err)
		return
	}

	// Return success, at last.
	resolve(request, 0, startTimestamp, nil)
}

func acquire(what string, queue chan bool, request rpc.RemoteRequest) (err string) {
	stdout := request.Stdout()
	fmt.Fprintf(stdout, "+---> Trying to acquire %v\n", what)
	for {
		select {
		case queue <- true:
			fmt.Fprintln(stdout, "+---> Success")
			return
		case <-request.Interrupted():
			fmt.Fprintln(stdout, "+---> Failure - build interrupted")
			return "interrupted"
		case <-time.After(30 * time.Second):
			fmt.Fprintln(stdout, "|")
		}
	}
}

func release(what string, queue chan bool, request rpc.RemoteRequest) {
	<-queue
	fmt.Fprintf(request.Stdout(), "+---> Releasing %v\n", what)
}

func resolve(req rpc.RemoteRequest, retCode rpc.ReturnCode, start time.Time, err error) {
	retValue := &data.BuildResult{
		Duration: time.Now().Sub(start),
	}
	if err != nil {
		retValue.Error = err.Error()
	}
	req.Resolve(retCode, retValue)
}
