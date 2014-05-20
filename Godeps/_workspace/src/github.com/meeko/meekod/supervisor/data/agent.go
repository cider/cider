// Copyright (c) 2013 The meeko AUTHORS
//
// Use of this source code is governed by The MIT License
// that can be found in the LICENSE file.

package data

import (
	"errors"
	"fmt"
	"labix.org/v2/mgo/bson"
	"strconv"
	"time"
	"unicode"
)

type Agent struct {
	Id          bson.ObjectId `bson:"_id"            codec:"-"`
	Alias       string        `bson:"alias"          codec:"alias"`
	Name        string        `bson:"name"           codec:"name,omitempty"`
	Version     string        `bson:"version"        codec:"version,omitempty"`
	Description string        `bson:"description"    codec:"description,omitempty"`
	Repository  string        `bson:"repository"     codec:"repository,omitempty"`
	Vars        Variables     `bson:"vars,omitempty" codec:"vars,omitempty"        json:"variables"`
	Enabled     bool          `bson:"enabled"        codec:"enabled,omitempty"`
	Status      string        `bson:"-"              codec:"status,omitempty"`
}

func (agent *Agent) FillAndValidate() error {
	switch {
	case agent.Name == "":
		return fieldMissing("name")
	case agent.Version == "":
		return fieldMissing("version")
	case agent.Description == "":
		return fieldMissing("description")
	}

	for k, v := range agent.Vars {
		if err := v.FillAndValidate(k); err != nil {
			return err
		}
	}

	return nil
}

type Variable struct {
	Usage    string `bson:"usage"              codec:"usage"`
	Type     string `bson:"type"               codec:"type"`
	Secret   bool   `bson:"secret,omitempty"   codec:"secret,omitempty"`
	Optional bool   `bson:"optional,omitempty" codec:"optional,omitempty"`
	Value    string `bson:"value"              codec:"value"`
}

func (v *Variable) FillAndValidate(name string) error {
	for _, r := range name {
		if !unicode.IsUpper(r) && r != '_' {
			return errors.New("only upper case letters and _ allowed in var name")
		}
	}

	if v.Type == "" {
		v.Type = "string"
	}

	switch {
	case v.Usage == "":
		return fieldMissing("vars." + name + ".usage")
	case v.Type != "string" && v.Type != "integer" && v.Type != "float64" && v.Type != "boolean" && v.Type != "duration":
		return fmt.Errorf("vars.%s.type value is invalid", name)
	}

	return nil
}

func (v *Variable) Set(value string) error {
	switch v.Type {
	case "string":
		v.Value = value

	case "integer":
		if _, err := strconv.Atoi(value); err != nil {
			return fmt.Errorf("failed to parse value as integer: %v", err)
		}
		v.Value = value

	case "float64":
		if _, err := strconv.ParseFloat(value, 64); err != nil {
			return fmt.Errorf("failed to parse value as float64: %v", err)
		}
		v.Value = value

	case "boolean":
		if _, err := strconv.ParseBool(value); err != nil {
			return fmt.Errorf("failed to parse value as boolean: %v", err)
		}
		v.Value = value

	case "duration":
		if _, err := time.ParseDuration(value); err != nil {
			return fmt.Errorf("failed to parse value as duration: %v", err)
		}
		v.Value = value

	default:
		panic(errors.New("variable type not set"))
	}

	return nil
}

type Variables map[string]*Variable

func (vs Variables) Filled() error {
	for k, v := range vs {
		if !v.Optional && v.Value == "" {
			return fmt.Errorf("variable %s is not set", k)
		}
	}

	return nil
}

func fieldMissing(field string) error {
	return fmt.Errorf("%s field is empty", field)
}
