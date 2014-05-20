// Copyright (c) 2014 The AUTHORS
//
// This file is part of cider.
//
// cider is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// cider is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with cider.  If not, see <http://www.gnu.org/licenses/>.

package build

import (
	// Stdlib
	"io/ioutil"
	"log"
	"os"

	// Cider
	"github.com/cider/cider/data"

	// Others
	"github.com/cihub/seelog"
	"github.com/tchap/gocli"
)

var (
	verboseMode bool
	master      string
	token       string
	slave       string
	repository  string
	script      string
	runner      string
	env         = data.Env(make([]string, 0))
)

var config = data.NewConfig()

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
    $ cider build -master wss://cider.example.com:443/build -token=12345
                  -slave macosx -runner bash
                  -repository git+ssh://github.com/foo/bar.git#develop
                  -script scripts/build -env ENVIRONMENT=testing -env DEBUG=y

  ENVIRONMENT:
    The following environment variables can be used instead of the relevant
    command line flags. The flags have higher priority, though.

      CIDER_MASTER_URL
      CIDER_MASTER_TOKEN
      CIDER_SLAVE_LABEL
      CIDER_REPOSITORY_URL
      CIDER_SCRIPT_PATH
      CIDER_SCRIPT_RUNNER
      CIDER_SCRIPT_ENV_<KEY> - equivalent to -env KEY=...
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
	cmd.Flags.Var(&env, "env", "define an environment variable for the build run")
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
	configContent, err := ioutil.ReadFile(data.ConfigFileName)
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
	if err := config.FeedFromEnv("CIDER"); err != nil {
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

	for _, kv := range []string(env) {
		config.Script.Env.Set(kv)
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
	result, err := call(config.Master.URL, config.Master.Token, method, args)
	if err != nil {
		log.Fatalf("\nError: %v\n", err)
	}

	// Check for the build error.
	if result.Error != "" {
		log.Fatalf("\nError: %v\n", result.Error)
	}
}
