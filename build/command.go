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
	"io/ioutil"
	"log"
	"os"
	"strings"

	// Paprika
	"github.com/paprikaci/paprika/data"

	// Others
	"github.com/cihub/seelog"
	"github.com/tchap/gocli"
)

const ConfigFileName = ".paprika.yml"

var (
	verboseMode bool
	master      string
	token       string
	slave       string
	repository  string
	script      string
	runner      string
	env         = Env(make([]string, 0))
)

var config = data.NewConfig()

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
  build [-verbose] [-master=URL] [-token=TOKEN] [-slave=SLAVE] [-runner=RUNNER]
        [-repository=REPO] [-script=SCRIPT] [-env KEY=VALUE ...]`,
	Short: "trigger a build",
	Long: `
  Trigger a build on the specified build slave.

  The build slave is chosen depending on SLAVE and RUNNER. When suitable build
  slave is found, a new job is enqueued, which is defined by the repository
  located at REPO, and SCRIPT, which is a relative path to a script located
  within REPO. RUNNER program is used to run the script.

  Example:
    $ paprika build -master wss://paprika.example.com/build -token=12345
                    -slave macosx -runner bash
                    -repository git+ssh://github.com/foo/bar.git#develop
                    -script scripts/build -env ENVIRONMENT=testing -env DEBUG=y

  ENVIRONMENT:
    The following environment variables can be used instead of the relevant
    command line flags. The flags have higher priority, though.

      PAPRIKA_MASTER_URL
      PAPRIKA_MASTER_TOKEN
      PAPRIKA_SLAVE_LABEL
      PAPRIKA_REPOSITORY_URL
      PAPRIKA_SCRIPT_PATH
      PAPRIKA_SCRIPT_RUNNER
      PAPRIKA_SCRIPT_ENV_<KEY> - equivalent to -env KEY=...
	`,
	Action: triggerBuild,
}

func init() {
	cmd := Command
	cmd.Flags.BoolVar(&verboseMode, "verbose", verboseMode, "print more verbose output")
	cmd.Flags.StringVar(&master, "master", master, "build master to connect to")
	cmd.Flags.StringVar(&token, "token", token, "build master access token")
	cmd.Flags.StringVar(&slave, "slave", slave, "slave label")
	cmd.Flags.StringVar(&runner, "runner", runner, "script runner")
	cmd.Flags.StringVar(&repository, "repository", repository, "project repository URL")
	cmd.Flags.StringVar(&script, "script", script, "relative path to the script to run")
	cmd.Flags.Var((*Env)(&config.Script.Env), "env", "define an environment variable for the build run")
}

func triggerBuild(cmd *gocli.Command, argv []string) {
	// Make sure there were no arguments specified.
	if len(argv) != 0 {
		cmd.Usage()
		os.Exit(2)
	}

	// Disable all the log prefixes and what not.
	log.SetFlags(0)

	// This must be here as long as go-cider logging is retarded as it is now.
	seelog.ReplaceLogger(seelog.Disabled)

	// Try to read the config file.
	configContent, err := ioutil.ReadFile(ConfigFileName)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Fatalf("\nError: %v\n", err)
		}
	}

	config, err := data.ParseConfig(configContent)
	if err != nil {
		log.Fatalf("\nError: %v\n", err)
	}

	// Update the config from environment variables.
	if err := config.UpdateFromEnv("PAPRIKA"); err != nil {
		log.Fatalf("\nError: %v\n", err)
	}

	// Flags overwrite any previously set configuration.
	if master != "" {
		config.Master.URL = master
	}
	if token != "" {
		config.Master.Token = token
	}
	if slave != "" {
		config.Slave.Label = slave
	}
	if repository != "" {
		config.Repository.URL = repository
	}
	if script != "" {
		config.Script.Path = script
	}
	if runner != "" {
		config.Script.Runner = runner
	}

	// Parse the RPC arguments. This performs some early arguments validation.
	method, args, err := data.ParseArgs(config.Slave.Label, config.Repository.URL,
		config.Script.Path, config.Script.Runner, config.Script.Env)
	if err != nil {
		log.Fatalf("\nError: %v\n", err)
	}

	// Check that the build master config is complete as well.
	switch {
	case config.Master.URL == "":
		log.Fatalln("\nError: build master URL is not set")
	case config.Master.Token == "":
		log.Fatalln("\nError: build master access token is not set")
	}

	// Send the build request and stream the output to the console.
	var result data.BuildResult
	err = call(config.Master.URL, config.Master.Token, method, args, &result)
	if err != nil {
		log.Fatalf("\nError: %v\n", err)
	}

	// Write the build summary into the console.
	result.WriteSummary(os.Stderr)

	// Check for the build error.
	if result.Error != "" {
		log.Fatalf("\nError: %v\n", result.Error)
	}
}
