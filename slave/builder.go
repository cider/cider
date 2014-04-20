// Copyright (c) 2014 The AUTHORS
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

package slave

import (
	// Stdlib
	"fmt"
	"net/url"
	"os"
	"time"

	// Paprika
	"github.com/paprikaci/paprika/data"
	"github.com/paprikaci/paprika/slave/runners"

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
	errStr := acquire("Locking the project workspace", wsQueue, request)
	if errStr != "" {
		request.Resolve(5, &data.BuildResult{Error: errStr})
		return
	}
	defer func() {
		// Release the workspace lock.
		<-wsQueue
	}()

	// Acquire a build executor.
	errStr = acquire("Waiting for a free executor", builder.execQueue, request)
	if errStr != "" {
		request.Resolve(5, &data.BuildResult{Error: errStr})
		return
	}
	defer func() {
		// Free the allocated executor.
		<-builder.execQueue
	}()

	// Start measuring the build time.
	startT := time.Now()

	// Check out the sources at the right revision.
	srcDir := builder.manager.SrcDir(workspace)
	srcDirExists, err := builder.manager.SrcDirExists(workspace)
	if err != nil {
		resolve(request, 6, startT, nil, nil, err)
		return
	}

	vcs, err := vcsutil.GetVCS(repoURL.Scheme)
	if err != nil {
		resolve(request, 7, startT, nil, nil, err)
		return
	}

	fmt.Fprintf(stdout, "\n---> Pulling the sources (using URL %q)\n", args.Repository)
	if srcDirExists {
		err = vcs.Pull(repoURL, srcDir, request)
	} else {
		err = vcs.Clone(repoURL, srcDir, request)
	}
	pullT := time.Now()
	if err != nil {
		resolve(request, 8, startT, &pullT, nil, err)
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

	fmt.Fprintf(stdout, "\n---> Running the script located at %v (using runner %q)\n",
		args.Script, builder.runner.Name)
	err = executil.Run(cmd, request.Interrupted())
	buildT := time.Now()
	if err != nil {
		resolve(request, 1, startT, &pullT, &buildT, err)
		return
	}

	// Return success, at last.
	resolve(request, 0, startT, &pullT, &buildT, nil)
}

func acquire(msg string, queue chan bool, request rpc.RemoteRequest) (err string) {
	stdout := request.Stdout()
	fmt.Fprintf(stdout, "---> %v\n", msg)
	for {
		select {
		case queue <- true:
			return
		case <-request.Interrupted():
			return "interrupted"
		case <-time.After(30 * time.Second):
			fmt.Fprintln(stdout, "---> ...")
		}
	}
}

func resolve(req rpc.RemoteRequest, code rpc.ReturnCode, startT time.Time, pullT *time.Time, buildT *time.Time, err error) {
	result := new(data.BuildResult)
	if pullT != nil {
		result.PullDuration = pullT.Sub(startT)
	}
	if buildT != nil {
		result.BuildDuration = buildT.Sub(*pullT)
	}
	if err != nil {
		result.Error = err.Error()
		fmt.Fprintln(req.Stdout(), "\n---> Build failed")
	} else {
		fmt.Fprintln(req.Stdout(), "\n---> Build succeeded")
	}
	result.WriteSummary(req.Stdout())
	req.Resolve(code, result)
}
