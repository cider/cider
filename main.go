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

package main

import (
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"

	"github.com/salsita-cider/paprika/data"

	"github.com/cihub/seelog"
	"github.com/wsxiaoys/terminal/color"
)

var (
	master = mustGetenv("PAPRIKA_MASTER")
	token  = mustGetenv("PAPRIKA_TOKEN")
)

var fverbose bool

func main() {
	// Parse the command line.
	flag.BoolVar(&fverbose, "verbose", fverbose, "print verbose output to stderr")
	slaveTag := flag.String("slave", "", "tag identifying the build slave to be used")
	repositoryURL := flag.String("repository", "", "URL identifying the sources to be built")
	flag.Parse()

	if flag.NArg() != 1 {
		usage()
	}

	scriptRelativePath := flag.Arg(0)

	// Disable Seelog logging output if verbose output is not requested.
	// This is only necessary because go-cider logging is a mess right now.
	if !fVerbose {
		seelog.ReplaceLogger(seelog.Disabled)
	}

	// Make sure the build slave tag is configured.
	if *slaveTag == "" {
		*slaveTag = os.Getenv("PAPRIKA_SLAVE")
		if *slaveTag == "" {
			color.Fprintln(os.Stderr, "\n@{r}Error: Build slave not configured")
			os.Exit(2)
		}
	}

	// Read information from the environment in case Circle CI is detected.
	if os.Getenv("CIRCLECI") != "" {
		*repo = fmt.Sprintf("git+ssh://git@github.com/%s/%s.git#%s",
			os.Getenv("CIRCLE_PROJECT_USERNAME"),
			os.Getenv("CIRCLE_PROJECT_REPONAME"),
			os.Getenv("CIRCLE_BRANCH"))
	}

	// Run the build.
	if err := build(*slaveTag, *repositoryURL, scriptRelativePath); err != nil {
		color.Fprintf(os.Stderr, "\n@{r}Error: %v\n", err)
		os.Exit(1)
	}

	color.Fprintln(os.Stderr, "\n@{g}Success")
}

func build(slave string, repository string, script string) error {
	// Parse the RPC arguments. This performs some early arguments validation.
	method, args, err := data.ParseArgs(slave, repository, script)
	if err != nil {
		return err
	}

	// Since we managed to parse and verify the arguments, we can happily start
	// building the project and streaming the output.
	var result data.BuildResult
	if err := call(method, args, &result); err != nil {
		return err
	}

	// Process the result.
	if err := result.Error(); err != nil {
		return err
	}

	result.Dump(os.Stdout)
	return nil
}

func usage() {
	io.WriteString(os.Stderr, `APPLICATION
  paprika - Paprika CI command line client

USAGE
  paprika [-verbose] [-slave=SLAVE] [-repository REPO_URL] SCRIPT

OPTIONS
`)
	flag.PrintDefaults()
	io.WriteString(os.Stderr, `
DESCRIPTION
  paprika utility connects to the Paprika CI master node and enqueues SCRIPT
  from REPO_URL to be executed. paprika blocks until the requests is scheduled
  for execution, then it streams the build output in real time to the console.
  The process exists with a non-zero exit status if the build fails.

  SCRIPT must be a relative path within the repository that denotes the build
  scripts that will be run remotely on one of the connected build slaves.

  REPO_URL must be a valid URL with one of the following schemes that define
  what source code management system is to be used to get the project sources:

    * git+https
    * git+ssh

  The URL fragment specifies what ref to build.

  Example:
    git+ssh://github.com/foobar/barfoo.git#develop would use Git over SSH to get
    the develop branch in the given repository.

  SLAVE must be a string identifying one of the build slaves connected to the
  relevant Paprika master node.

ENVIRONMENT
  PAPRIKA_MASTER - Paprika CI master node network address
  PAPRIKA_TOKEN  - Paprika CI master node access token
  PAPRIKA_SLAVE  - Paprika CI slave node to use for building the project
                   (can be overwritten by using -slave flag)

  CIRCLECI                  - use the following variables to set REPO_URL to
                              "git+ssh://github.com/$owner/$repo.git#$branch"
  CIRCLECI_PROJECT_USERNAME - used as $owner
  CIRCLECI_PROJECT_REPONAME - used as $repo
  CIRCLECI_BRANCH           - used as $branch

`)
	os.Exit(2)
}

func mustGetenv(key string) (value string) {
	value = os.Getenv(key)
	if value == "" {
		panic(fmt.Errorf("Enviroment variable %s is not set", key))
	}
	return
}
