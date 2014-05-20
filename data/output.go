// Copyright (c) 2014 The cider AUTHORS
//
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

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
