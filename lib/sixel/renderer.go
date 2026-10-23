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
	"bytes"
	"fmt"
	"os"
	"slices"

	"go.mau.fi/mauview"
	"go.mau.fi/tcell"

	"maunium.net/go/gomuks/debug"
)

// CellSize is the size of a single terminal cell in pixels.
type CellSize struct {
	Width, Height int
}

// DefaultCellSize is used if the terminal doesn't report its size in pixels.
var DefaultCellSize = CellSize{Width: 10, Height: 20}

// markerStyle is used for the blank cells underneath images. It's never used by anything else,
// so if a cell has a different style after drawing, something (e.g. a modal) was drawn over the image.
var markerStyle = tcell.StyleDefault.UrlId("gomuks-sixel-image")

type placement struct {
	image *Image
	// Position of the visible part of the image on the screen
	x, y int
	// Range of cell rows of the image that are visible
	topRow, rows int
}

// Renderer writes sixel images to the terminal after tcell has drawn the rest of the screen.
//
// tcell doesn't know anything about the images, so the cells underneath them are filled with blanks,
// and the screen is fully redrawn whenever an image moves or disappears to get rid of the old pixels.
type Renderer struct {
	tty *os.File

	placements []placement
	prev       []placement
}

// Default is the renderer used by the UI. It's nil if sixel output is unavailable.
var Default *Renderer

// Init opens the terminal for writing sixel images and sets Default.
func Init() {
	tty, err := openTTY()
	if err != nil {
		debug.Print("Sixel images unavailable:", err)
		return
	}
	Default = &Renderer{tty: tty}
}

// CellSize returns the current size of a terminal cell in pixels.
func (r *Renderer) CellSize() CellSize {
	if size, ok := queryCellSize(r.tty); ok {
		return size
	}
	return DefaultCellSize
}

// Place reserves space for an image at the top-left corner of the given screen.
// The image itself is written to the terminal in Show.
func (r *Renderer) Place(screen mauview.Screen, img *Image) {
	for y := 0; y < img.Rows; y++ {
		for x := 0; x < img.Cols; x++ {
			screen.SetContent(x, y, ' ', nil, markerStyle)
		}
	}

	// Find the absolute position and the visible part of the image by walking up the proxy screens.
	absX, absY := 0, 0
	top, bottom := 0, img.Rows
	left, right := 0, img.Cols
	var root mauview.Screen = screen
	for {
		proxy, ok := root.(*mauview.ProxyScreen)
		if !ok {
			break
		}
		top, left = max(top, -absY), max(left, -absX)
		if proxy.Height >= 0 {
			bottom = min(bottom, proxy.Height-absY)
		}
		if proxy.Width >= 0 {
			right = min(right, proxy.Width-absX)
		}
		absX += proxy.OffsetX
		absY += proxy.OffsetY
		root = proxy.Parent
	}
	screenWidth, screenHeight := root.Size()
	top, left = max(top, -absY), max(left, -absX)
	right = min(right, screenWidth-absX)
	// Never draw on the last line: the cursor moving below the image would scroll the terminal.
	bottom = min(bottom, screenHeight-1-absY)

	if left != 0 || right != img.Cols || bottom <= top {
		// Cropping horizontally isn't supported, and there's nothing to draw if all rows are hidden.
		return
	}
	r.placements = append(r.placements, placement{
		image:  img,
		x:      absX,
		y:      absY + top,
		topRow: top,
		rows:   bottom - top,
	})
}

func (r *Renderer) isUncovered(screen tcell.Screen, p placement) bool {
	for y := p.y; y < p.y+p.rows; y++ {
		for x := p.x; x < p.x+p.image.Cols; x++ {
			if _, _, style, _ := screen.GetContent(x, y); style != markerStyle {
				return false
			}
		}
	}
	return true
}

// Show flushes the screen and writes the images placed since the last call.
// It's meant to be used as the mauview show handler.
func (r *Renderer) Show(screen tcell.Screen, clear bool) {
	visible := r.placements[:0]
	for _, p := range r.placements {
		// Images that are partially covered by something else can't be drawn without covering it.
		if r.isUncovered(screen, p) {
			visible = append(visible, p)
		}
	}
	r.placements = nil

	// If any previously drawn image moved or disappeared, its pixels have to be cleared,
	// which tcell will only do with a full redraw (that also clears all other images).
	fullRedraw := clear && len(visible) > 0
	for _, p := range r.prev {
		if !slices.Contains(visible, p) {
			fullRedraw = true
			break
		}
	}
	var toDraw []placement
	if fullRedraw {
		screen.Sync()
		toDraw = visible
	} else {
		screen.Show()
		for _, p := range visible {
			if !slices.Contains(r.prev, p) {
				toDraw = append(toDraw, p)
			}
		}
	}
	r.prev = visible

	if len(toDraw) == 0 {
		return
	}
	var buf bytes.Buffer
	// Save the cursor position so that tcell's cursor (e.g. in the input area) stays where it was.
	buf.WriteString("\x1b7")
	for _, p := range toDraw {
		fmt.Fprintf(&buf, "\x1b[%d;%dH", p.y+1, p.x+1)
		buf.Write(p.image.Encoded(p.topRow, p.rows))
	}
	buf.WriteString("\x1b8")
	if _, err := r.tty.Write(buf.Bytes()); err != nil {
		debug.Print("Failed to write sixel images:", err)
	}
}
