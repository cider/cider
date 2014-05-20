// Copyright (c) 2014 The cider AUTHORS
//
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

package data

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v1"
)

const ConfigFileName = "cider.yml"

type Env []string

func (env *Env) Set(kv string) error {
	// Parse the key-value pair.
	parts := strings.SplitN(kv, "=", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid key-value pair: %v", kv)
	}

	slice := (*[]string)(env)

	// Delete the existing value, if present.
	for i, kw := range *slice {
		ps := strings.SplitN(kw, "=", 2)
		if ps[0] == parts[0] {
			*slice = append((*slice)[:i], (*slice)[i+i:]...)
			break
		}
	}

	// Append the new value.
	*slice = append(*slice, kv)
	return nil
}

func (env *Env) String() string {
	return fmt.Sprintf("%v", *env)
}

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
		Path   string `yaml:"path"`
		Runner string `yaml:"runner"`
		Env    Env    `yaml:"env"`
	} `yaml:"script"`
}

func NewConfig() *Config {
	var config Config
	config.Script.Env = make([]string, 0)
	return &config
}

func ParseConfig(data []byte) (*Config, error) {
	config := new(Config)
	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, err
	}
	return config, nil
}

func (config *Config) FeedFromEnv(prefix string) error {
	// Check all significant environment variables.
	if v := os.Getenv(prefix + "_MASTER_URL"); v != "" {
		config.Master.URL = v
	}
	if v := os.Getenv(prefix + "_MASTER_TOKEN"); v != "" {
		config.Master.Token = v
	}
	if v := os.Getenv(prefix + "_SLAVE_LABEL"); v != "" {
		config.Slave.Label = v
	}
	if v := os.Getenv(prefix + "_REPOSITORY_URL"); v != "" {
		config.Repository.URL = v
	}
	if v := os.Getenv(prefix + "_SCRIPT_PATH"); v != "" {
		config.Script.Path = v
	}
	if v := os.Getenv(prefix + "_SCRIPT_RUNNER"); v != "" {
		config.Script.Runner = v
	}

	// Provide some extra support for Circle CI.
	if os.Getenv("CIRCLECI") != "" {
		config.Repository.URL = fmt.Sprintf("git+ssh://git@github.com/%v/%v#%v",
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
			parts := strings.SplitN(kv, "=", 2)
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
