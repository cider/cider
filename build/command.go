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

package build

import (
	// Stdlib
	"fmt"
	"log"
	"os"
	"strings"

	// Paprika
	"github.com/paprikaci/paprika/data"
	"github.com/paprikaci/paprika/utils"

	// Others
	"github.com/cihub/seelog"
	"github.com/tchap/gocli"
)

var (
	verboseMode bool
	master      string
	token       string
	label       string
	runner      string
	repository  string
	script      string
	env         = Env(make([]string, 0))
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

var Command = &gocli.Command{
	UsageLine: `
  build [-verbose] [-master=URL] [-token=TOKEN] [-label=LABEL] [-runner=RUNNER]
        [-repository=REPO] [-script=SCRIPT] [-env KEY=VALUE ...]`,
	Short: "trigger a build",
	Long: `
  Trigger a build on the specified build slave.

  The build slave is chosen depending on LABEL and RUNNER. When suitable
  build slave is found, a job is enqueued, which is defined by a repository
  located at SOURCES, and SCRIPT, which is a relative path to a script located
  within SOURCES.

  Example:
    $ paprika build -master wss://paprika.example.com/build -token=12345
	            -label macosx -runner bash
                    -repository git+ssh://github.com/foo/bar.git#develop
                    -script scripts/build -env ENVIRONMENT=testing -env DEBUG=y

  ENVIRONMENT:
    The following environment variables can be used instead of the relevant
	command line flags. The flags have higher priority, though.
      PAPRIKA_MASTER
	  PAPRIKA_TOKEN
	  PAPRIKA_LABEL
	  PAPRIKA_RUNNER
	  PAPRIKA_REPOSITORY
	  PAPRIKA_SCRIPT
	`,
	Action: triggerBuild,
}

func init() {
	cmd := Command
	cmd.Flags.BoolVar(&verboseMode, "verbose", verboseMode, "print more verbose output")
	cmd.Flags.StringVar(&master, "master", master, "build master to connect to")
	cmd.Flags.StringVar(&token, "token", token, "build master access token")
	cmd.Flags.StringVar(&label, "label", label, "slave label")
	cmd.Flags.StringVar(&runner, "runner", runner, "script runner")
	cmd.Flags.StringVar(&repository, "repository", repository, "project repository URL")
	cmd.Flags.StringVar(&script, "script", script, "relative path to the script to run")
	cmd.Flags.Var(&env, "env", "define an environment variable for the build run")
}

func triggerBuild(cmd *gocli.Command, argv []string) {
	// Make sure there were no arguments specified.
	if len(argv) != 0 {
		cmd.Usage()
		os.Exit(2)
	}

	// Read the environment to fill in missing parameters.
	utils.GetenvOrFailNow(&master, "PAPRIKA_MASTER", cmd)
	utils.GetenvOrFailNow(&token, "PAPRIKA_TOKEN", cmd)
	utils.GetenvOrFailNow(&label, "PAPRIKA_LABEL", cmd)
	utils.GetenvOrFailNow(&runner, "PAPRIKA_RUNNER", cmd)
	utils.GetenvOrFailNow(&repository, "PAPRIKA_REPOSITORY", cmd)
	utils.GetenvOrFailNow(&script, "PAPRIKA_SCRIPT", cmd)

	// Provide some extra support for Circle CI.
	if os.Getenv("CIRCLECI") != "" {
		repository = fmt.Sprintf("git+ssh://git@github.com/%s/%s.git#%s",
			os.Getenv("CIRCLE_PROJECT_USERNAME"),
			os.Getenv("CIRCLE_PROJECT_REPONAME"),
			os.Getenv("CIRCLE_BRANCH"))
	}

	// Disable all the log prefixes and what not.
	log.SetFlags(0)

	// This must be here as long as go-cider logging is retarded as it is now.
	seelog.ReplaceLogger(seelog.Disabled)

	// Parse the RPC arguments. This performs some early arguments validation.
	method, args, err := data.ParseArgs(label, runner, repository, script, env)
	if err != nil {
		log.Fatalf("\nError: %v\n", err)
	}

	// Since we managed to parse and verify the arguments, we can happily start
	// building the project and streaming the output.
	var result data.BuildResult
	if err := call(method, args, &result); err != nil {
		log.Fatalf("\nError: %v\n", err)
	}

	// Write the build summary into the console.
	result.WriteSummary(os.Stderr)

	// Check for the build error.
	if result.Error != "" {
		log.Fatalf("\nError: %v\n", result.Error)
	}
}
