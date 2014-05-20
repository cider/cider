// Copyright (c) 2014 The cider AUTHORS
//
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

package utils

import (
	"github.com/tchap/gocli"
	"log"
	"os"
)

func Getenv(value *string, key string) {
	if *value == "" {
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
