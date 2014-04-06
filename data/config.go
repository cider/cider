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
	"fmt"
	"os"
	"strings"

	yaml "launchpad.net/goyaml"
)

type Config struct {
	Master struct {
		URL   string `yaml:"url"`
		Token string `yaml:"token"`
	} `yaml:"master"`
	Slave struct {
		Label string `yaml:"label"`
	} `yaml:"slave"`
	Repository struct {
		URL string `yaml:"url"`
	} `yaml:"repository"`
	Script struct {
		Path   string   `yaml:"path"`
		Runner string   `yaml:"runner"`
		Env    []string `yaml:"env"`
	} `yaml:"script"`
}

func NewConfig() *Config {
	return new(Config)
}

func ParseConfig(data []byte) (*Config, error) {
	config := new(Config)
	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, err
	}
	return config, nil
}

func (config *Config) UpdateFromEnv(prefix string) error {
	// Check all significant environment variables.
	if v := os.Getenv(prefix + "_MASTER_URL"); v != "" && config.Master.URL == "" {
		config.Master.URL = v
	}
	if v := os.Getenv(prefix + "_MASTER_TOKEN"); v != "" && config.Master.Token == "" {
		config.Master.Token = v
	}
	if v := os.Getenv(prefix + "_SLAVE_LABEL"); v != "" && config.Slave.Label == "" {
		config.Slave.Label = v
	}
	if v := os.Getenv(prefix + "_REPOSITORY_URL"); v != "" && config.Repository.URL == "" {
		config.Repository.URL = v
	}
	if v := os.Getenv(prefix + "_SCRIPT_PATH"); v != "" && config.Script.Path == "" {
		config.Script.Path = v
	}
	if v := os.Getenv(prefix + "_SCRIPT_RUNNER"); v != "" && config.Script.Runner == "" {
		config.Script.Runner = v
	}

	// Provide some extra support for Circle CI.
	if os.Getenv("CIRCLECI") != "" {
		config.Repository.URL = fmt.Sprintf("git+ssh://git@github.com/%v/%v.git#%v",
			os.Getenv("CIRCLE_PROJECT_USERNAME"),
			os.Getenv("CIRCLE_PROJECT_REPONAME"),
			os.Getenv("CIRCLE_BRANCH"))
	}

	pre := prefix + "_SCRIPT_ENV_"
ReadEnv:
	// Iterate over all the environment variables.
	for _, kv := range os.Environ() {
		// Pick the ones that start with the right prefix.
		if strings.HasPrefix(kv, pre) {
			parts := strings.SplitN(kv, "=", 1)
			// Just ignore the malformed key-value pairs.
			if len(parts) != 2 {
				continue ReadEnv
			}
			// Drop the prefix that is not really a part of the variable name.
			varPrefix := parts[0][len(pre):] + "="
			for _, kw := range config.Script.Env {
				// Skip the variable if it is already set in config.Script.Env.
				if strings.HasPrefix(kw, varPrefix) {
					continue ReadEnv
				}
				// Otherwise update config.Script.Env to incorporate the
				// environment variable.
				config.Script.Env = append(config.Script.Env, kv[len(pre):])
			}
		}
	}

	return nil
}
