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

package utils

import (
	"github.com/tchap/gocli"
	"log"
	"os"
)

func Getenv(value *string, key string) {
	if *value != "" {
		*value = os.Getenv(key)
	}
}

func GetenvOrFailNow(value *string, key string, cmd *gocli.Command) {
	// In case the flag was used, we do not read the environment.
	if *value != "" {
		return
	}

	// Read the value from the environment or exit.
	v := os.Getenv(key)
	if v == "" {
		log.Printf("Error: %v is not set and neither is the associated flag\n\n", key)
		cmd.Usage()
		os.Exit(2)
	}

	*value = v
}
