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

	yaml "launchpad.net/goyaml"
)

type Config struct {
	Master struct {
		URL string `yaml:"url"`
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

func (config *Config) Validate() error {
	if _, err := url.Parse(config.Master.URL); err != nil {
		return err
	}

	_, _, err := ParseArgs(config.Slave.Label, config.Repository.URL,
		config.Script.Path, config.Script.Runner, config.Script.Env)
	if err != nil {
		return err
	}

	return nil
}

func ParseConfig(data []byte) (*Config, error) {
	config := new(Config)
	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, err
	}

	if err := config.Validate(); err != nil {
		return nil, err
	}

	return config, nil
}
