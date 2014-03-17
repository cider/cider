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

package data

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

func ParseArgs(slave, repository, script, runner string, env []string) (method string, args interface{}, err error) {
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
	method = fmt.Sprintf("paprika.%v.%v", slave, runner)

	// RPC arguments
	args = &BuildArgs{
		Repository: repository,
		Script:     script,
		Env:        env,
	}
	err = args.(*BuildArgs).Validate()

	return
}

type BuildArgs struct {
	Repository string   `codec:"repository"`
	Script     string   `codec:"script"`
	Env        []string `codec:"env"`
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
