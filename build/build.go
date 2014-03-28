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

package build

import (
	"log"
	"os"

	"github.com/paprikaci/paprika/data"

	"github.com/cihub/seelog"
)

func build() {
	// Disable all the log prefixes and what not.
	log.SetFlags(0)

	// Disable Seelog logging output. This must be here as long as go-cider
	// logging is a mess.
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
