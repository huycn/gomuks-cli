// gomuks - A terminal Matrix client written in Go.
// Copyright (C) 2020 Tulir Asokan
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.
package sixel

import (
	"os"
	"strings"
)

// ProbablySupported is true if the environment suggests the terminal can display sixel images.
//
// Querying the terminal directly (DA1) isn't possible because tcell owns the input stream,
// so this is a best guess based on environment variables. Terminals that aren't detected
// can still be used by setting the image protocol to sixel manually.
var ProbablySupported bool

func init() {
	ProbablySupported = detect()
}

func detect() bool {
	if os.Getenv("TMUX") != "" || strings.HasPrefix(os.Getenv("TERM"), "screen") {
		// Multiplexers either don't support sixel or need passthrough
		return false
	}
	term := os.Getenv("TERM")
	for _, name := range []string{"foot", "mlterm", "contour", "yaft", "wezterm"} {
		if strings.Contains(term, name) {
			return true
		}
	}
	switch os.Getenv("TERM_PROGRAM") {
	case "WezTerm", "iTerm.app", "mintty", "contour":
		return true
	}
	return os.Getenv("KONSOLE_VERSION") != "" || os.Getenv("WT_SESSION") != ""
}
