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
	"os"
	"strings"

	"github.com/salsita-cider/paprika/data"

	"github.com/cihub/seelog"
	"github.com/wsxiaoys/terminal/color"
)

var (
	master string
	token  string
)

var (
	fverbose bool
	fenv     = Env(make([]string, 0))
)

type Env []string

func (env *Env) Set(kv string) error {
	parts := strings.SplitN(kv, "=", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid key-value pair: %v", kv)
	}

	slice := (*[]string)(env)
	*slice = append(*slice, kv)
	return nil
}

func (env *Env) String() string {
	return fmt.Sprintf("%v", *env)
}

func main() {
	// Parse the command line.
	flag.BoolVar(&fverbose, "verbose", fverbose, "print verbose output to stderr")
	flag.Var(&fenv, "env", "set build environment variable")
	slaveTag := flag.String("slave", "", "tag identifying the build slave to be used")
	repositoryURL := flag.String("repository", "", "URL identifying the sources to be built")
	flag.Parse()

	if flag.NArg() != 1 {
		usage()
	}

	scriptRelativePath := flag.Arg(0)

	// Parse the Paprika-specific environment variables.
	master = mustGetenv("PAPRIKA_MASTER")
	token = mustGetenv("PAPRIKA_TOKEN")

	if *slaveTag == "" {
		*slaveTag = mustGetenv("PAPRIKA_SLAVE")
	}

	// Read information from the environment in case Circle CI is detected.
	if os.Getenv("CIRCLECI") != "" {
		*repositoryURL = fmt.Sprintf("git+ssh://git@github.com/%s/%s.git#%s",
			os.Getenv("CIRCLE_PROJECT_USERNAME"),
			os.Getenv("CIRCLE_PROJECT_REPONAME"),
			os.Getenv("CIRCLE_BRANCH"))
	}

	// Disable Seelog logging output if verbose output is not requested.
	// This is only necessary because go-cider logging is a mess right now.
	if !fverbose {
		seelog.ReplaceLogger(seelog.Disabled)
	}

	// Run the build.
	err := build(*slaveTag, *repositoryURL, scriptRelativePath, []string(fenv))
	if err != nil {
		color.Fprintf(os.Stderr, "\n@{r}Error: %v\n", err)
		os.Exit(1)
	}

	color.Fprintln(os.Stderr, "\n@{g}Success")
}

func build(slave, repository, script string, env []string) error {
	// Parse the RPC arguments. This performs some early arguments validation.
	method, args, err := data.ParseArgs(slave, repository, script, env)
	if err != nil {
		return err
	}

	// Since we managed to parse and verify the arguments, we can happily start
	// building the project and streaming the output.
	var result data.BuildResult
	if err := call(method, args, &result); err != nil {
		return err
	}

	// Write the build summary into the console.
	result.WriteSummary(os.Stderr)

	// Return the build error, if any.
	return result.Error
}

func usage() {
	io.WriteString(os.Stderr, `APPLICATION
  paprika - Paprika CI command line client

USAGE
  paprika [-verbose]
          [-slave SLAVE]
          [-repository REPO_URL]
          [-env KEY1=VALUE1 -env KEY2=VALUE2 ... ]
          SCRIPT

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
  script that will be run remotely on one of the connected build slaves.
  The env flag can be used to specify additional environment variables that
  are exported for the build script during the build.

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
		color.Fprintf(os.Stderr, "\n@{r}Error: %v is not set\n", key)
		os.Exit(2)
	}
	return
}
