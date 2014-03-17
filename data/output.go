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
	"io"
	"strconv"
	"strings"
	"time"
)

type BuildResult struct {
	PullDuration  time.Duration `codec:"pullDuration"`
	BuildDuration time.Duration `codec:"buildDuration"`
	Error         string        `codec:"error"`
}

func (result BuildResult) WriteSummary(w io.Writer) {
	var (
		total = result.PullDuration + result.BuildDuration

		totalStr = total.String()
		pullStr  = result.PullDuration.String()
		buildStr = result.BuildDuration.String()

		all = [...]*string{&pullStr, &buildStr, &totalStr}

		maxDotIndex    int
		maxFragmentLen int
	)

	for _, s := range all {
		dot := strings.Index(*s, ".")
		if dot == -1 {
			dot = len(*s) - 1
		}
		if dot > maxDotIndex {
			maxDotIndex = dot
		}
		fragment := len(*s) - dot
		if fragment > maxFragmentLen {
			maxFragmentLen = fragment
		}
	}

	format := "%" + strconv.Itoa(maxDotIndex) + "v"
	formatFrag := "%" + strconv.Itoa(maxDotIndex) + "v.%-" + strconv.Itoa(maxFragmentLen) + "v"
	for _, s := range all {
		parts := strings.Split(*s, ".")
		if len(parts) == 1 {
			*s = fmt.Sprintf(format, parts[0])
		} else {
			*s = fmt.Sprintf(formatFrag, parts[0], parts[1])
		}
	}

	fmt.Fprintf(w, "Pull  duration: %v\n", *all[0])
	fmt.Fprintf(w, "Build duration: %v\n", *all[1])
	fmt.Fprintf(w, "Total duration: %v\n", *all[2])
}
