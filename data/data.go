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

package data

import (
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
)

func ParseArgs(slave, repository, script string) (method string, args interface{}, err error) {
	repoURL, err := url.Parse(repository)
	if err != nil {
		return
	}

	scriptExt := filepath.Ext(script)
	if scriptExt == "" {
		err = fmt.Errorf("the build script must have some file extension: %s", script)
		return
	}

	// RPC method name
	method = slave + "." + scriptExt

	// RPC arguments
	args = &Args{
		Repository: (*RepositoryURL)(repoURL),
		Script:     script,
	}

	return
}

type Args struct {
	Repository *RepositoryURL `codec:"repository"`
	Script     string         `codec:"script"`
}

func (args *Args) Validate() error {
	if args.Repository == nil {
		return errors.New("Args.Validate: repository not set")
	}
	if err := args.Repository.Validate(); err != nil {
		return fmt.Errorf("Args.Validate: %v", err)
	}
	if args.Script == "" {
		return errors.New("Args.Validate: script not set")
	}
	return nil
}

type RepositoryURL url.URL

func (u *RepositoryURL) Validate() error {
	repoURL := (*url.URL)(u)
	switch repoURL.Scheme {
	case "git+https":
	case "git+ssh":
	default:
		return fmt.Errorf("unsupported repository URL scheme: %v", repoURL.Scheme)
	}
	return nil
}
