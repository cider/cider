// Copyright (c) 2014 The cider AUTHORS
//
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

package data

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

func ParseArgs(slave, repository, script, runner string, env []string) (method string, args *BuildArgs, err error) {
	// Make sure that the arguments are not empty.
	var unset string
	switch {
	case slave == "":
		slave = "any"
	case runner == "":
		unset = "runner"
	case repository == "":
		unset = "repository"
	case script == "":
		unset = "script"
	}
	if unset != "" {
		err = fmt.Errorf("argument cannot be empty: %v", unset)
		return
	}

	// RPC method name
	method = fmt.Sprintf("cider.%v.%v", slave, runner)

	// RPC arguments
	args = &BuildArgs{
		Repository: repository,
		Script:     script,
		Env:        env,
	}
	err = args.Validate()

	return
}

type BuildArgs struct {
	Repository string   `codec:"repository"`
	Script     string   `codec:"script"`
	Env        []string `codec:"env,omitempty"`
	Noop       bool     `codec:"noop,omitempty"` // For benchmarking purposes only.
}

func (args *BuildArgs) Validate() error {
	switch {
	case args.Repository == "":
		return errors.New("BuildArgs.Validate: Repository is not set")
	case args.Script == "":
		return errors.New("BuildArgs.Validate: Script is not set")
	}

	repoURL, err := url.Parse(args.Repository)
	if err != nil {
		return fmt.Errorf("BuildArgs.Validate: %v", err)
	}

	switch repoURL.Scheme {
	case "git+https":
	case "git+ssh":
	case "git+file":
	default:
		return fmt.Errorf("BuildArgs.Validate: unsupported repository URL scheme: %v",
			repoURL.Scheme)
	}

	for _, kv := range args.Env {
		if !strings.Contains(kv, "=") {
			return &ErrInvalidEnvironment{kv}
		}
	}

	return nil
}

type ErrInvalidEnvironment struct {
	kv string
}

func (err *ErrInvalidEnvironment) Error() string {
	return "invalid key-value pair: " + err.kv
}
