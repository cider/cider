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

package data

import (
	"fmt"
	"io"
	"time"
)

type BuildResult struct {
	Duration time.Duration
	Error    error
}

func (result BuildResult) WriteSummary(w io.Writer) {
	fmt.Fprintf(w, "=== BUILD SUMMARY ==========================================\n")
	fmt.Fprintf(w, "Duration: %v\n", result.Duration)
	fmt.Fprintf(w, "Error:    %v\n", result.Error)
	fmt.Fprintf(w, "============================================================\n")
}
