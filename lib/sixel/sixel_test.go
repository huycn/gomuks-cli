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
	"image"
	"image/color"
	"os"
	"regexp"
	"strconv"
	"testing"

	"go.mau.fi/mauview"
	"go.mau.fi/tcell"
)

// decode is a minimal sixel decoder that returns the palette index painted at each pixel (-1 if unpainted).
func decode(t *testing.T, data []byte) (width, height int, pixels [][]int) {
	t.Helper()
	if !bytes.HasPrefix(data, []byte("\x1bP0;1;0q")) || !bytes.HasSuffix(data, []byte("\x1b\\")) {
		t.Fatalf("missing DCS header or ST terminator: %q", data)
	}
	body := data[len("\x1bP0;1;0q") : len(data)-2]
	raster := regexp.MustCompile(`^"1;1;(\d+);(\d+)`).FindSubmatch(body)
	if raster == nil {
		t.Fatalf("missing raster attributes: %q", body)
	}
	width, _ = strconv.Atoi(string(raster[1]))
	height, _ = strconv.Atoi(string(raster[2]))
	body = body[len(raster[0]):]
	pixels = make([][]int, height)
	for y := range pixels {
		pixels[y] = make([]int, width)
		for x := range pixels[y] {
			pixels[y][x] = -1
		}
	}
	readNum := func(i int) (int, int) {
		start := i
		for i < len(body) && body[i] >= '0' && body[i] <= '9' {
			i++
		}
		n, _ := strconv.Atoi(string(body[start:i]))
		return n, i
	}
	color, x, bandY := 0, 0, 0
	paint := func(char byte, count int) {
		bits := char - '?'
		for ; count > 0; count-- {
			for dy := 0; dy < 6; dy++ {
				if bits&(1<<dy) != 0 {
					if bandY+dy >= height || x >= width {
						t.Fatalf("pixel %d,%d out of bounds", x, bandY+dy)
					}
					pixels[bandY+dy][x] = color
				}
			}
			x++
		}
	}
	for i := 0; i < len(body); {
		switch c := body[i]; {
		case c == '#':
			color, i = readNum(i + 1)
			// Skip color definitions
			for i < len(body) && body[i] == ';' {
				_, i = readNum(i + 1)
			}
		case c == '!':
			var count int
			count, i = readNum(i + 1)
			paint(body[i], count)
			i++
		case c == '$':
			x = 0
			i++
		case c == '-':
			x = 0
			bandY += 6
			i++
		case c >= '?' && c <= '~':
			paint(c, 1)
			i++
		default:
			t.Fatalf("unexpected byte %q at %d", c, i)
		}
	}
	return
}

func TestEncodeRoundTrip(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 9, 13))
	for y := 0; y < 13; y++ {
		for x := 0; x < 9; x++ {
			switch {
			case x == 4 && y == 7:
				img.Set(x, y, color.Transparent)
			case y < 6:
				img.Set(x, y, color.White)
			default:
				img.Set(x, y, color.Black)
			}
		}
	}
	width, height, pixels := decode(t, Encode(img))
	if width != 9 || height != 13 {
		t.Fatalf("unexpected size %dx%d", width, height)
	}
	white, black := pixels[0][0], pixels[12][8]
	if white == black || white < 0 || black < 0 {
		t.Fatalf("expected distinct painted colors, got %d and %d", white, black)
	}
	for y := 0; y < 13; y++ {
		for x := 0; x < 9; x++ {
			expected := black
			if x == 4 && y == 7 {
				expected = -1
			} else if y < 6 {
				expected = white
			}
			if pixels[y][x] != expected {
				t.Errorf("pixel %d,%d: expected %d, got %d", x, y, expected, pixels[y][x])
			}
		}
	}
}

func TestImageScaling(t *testing.T) {
	cell := CellSize{Width: 10, Height: 20}
	img := NewImage(image.NewNRGBA(image.Rect(0, 0, 1000, 500)), 50, 10, cell)
	// Limited by width: 500x250 px -> 50x13 cells, then by height: 400x200 px -> 40x10 cells
	if img.Cols != 40 || img.Rows != 10 {
		t.Errorf("unexpected size %dx%d", img.Cols, img.Rows)
	}
	small := NewImage(image.NewNRGBA(image.Rect(0, 0, 25, 25)), 50, 10, cell)
	if small.Cols != 3 || small.Rows != 2 {
		t.Errorf("small image shouldn't be scaled up, got %dx%d", small.Cols, small.Rows)
	}
	// The last row is cropped to the actual image height
	if _, height, _ := decode(t, small.Encoded(1, 1)); height != 5 {
		t.Errorf("expected cropped height 5, got %d", height)
	}
}

func newTestRenderer(t *testing.T) (*Renderer, tcell.SimulationScreen, *os.File) {
	t.Helper()
	tty, err := os.CreateTemp(t.TempDir(), "tty")
	if err != nil {
		t.Fatal(err)
	}
	screen := tcell.NewSimulationScreen("UTF-8")
	if err = screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(40, 20)
	return &Renderer{tty: tty}, screen, tty
}

func readOutput(t *testing.T, tty *os.File) string {
	t.Helper()
	data, err := os.ReadFile(tty.Name())
	if err != nil {
		t.Fatal(err)
	}
	if err = tty.Truncate(0); err != nil {
		t.Fatal(err)
	}
	_, _ = tty.Seek(0, 0)
	return string(data)
}

func TestPlaceAndShow(t *testing.T) {
	r, screen, tty := newTestRenderer(t)
	img := NewImage(image.NewNRGBA(image.Rect(0, 0, 40, 60)), 10, 10, CellSize{Width: 10, Height: 20})
	if img.Cols != 4 || img.Rows != 3 {
		t.Fatalf("unexpected size %dx%d", img.Cols, img.Rows)
	}

	view := mauview.NewProxyScreen(screen, 5, 2, 30, 10)
	r.Place(mauview.NewProxyScreen(view, 3, 4, 20, img.Rows), img)
	r.Show(screen, false)
	if out := readOutput(t, tty); !bytes.Contains([]byte(out), []byte("\x1b7\x1b[7;9H\x1bP")) {
		t.Fatalf("expected image at row 7 col 9, got %q", out)
	}

	// Same placement on the next frame: nothing is rewritten
	r.Place(mauview.NewProxyScreen(view, 3, 4, 20, img.Rows), img)
	r.Show(screen, false)
	if out := readOutput(t, tty); out != "" {
		t.Fatalf("expected no output for unchanged placement, got %q", out)
	}

	// Scrolled so that the first row is above the view: cropped to the last 2 rows
	r.Place(mauview.NewProxyScreen(view, 3, -1, 20, img.Rows), img)
	if len(r.placements) != 1 || r.placements[0].topRow != 1 || r.placements[0].rows != 2 || r.placements[0].y != 2 {
		t.Fatalf("unexpected placement %+v", r.placements)
	}
	r.Show(screen, false)
	if out := readOutput(t, tty); !bytes.Contains([]byte(out), []byte("\x1b[3;9H")) {
		t.Fatalf("expected cropped image at row 3, got %q", out)
	}

	// Covered by something drawn later (e.g. a modal): not drawn
	r.Place(mauview.NewProxyScreen(view, 3, 4, 20, img.Rows), img)
	screen.SetContent(9, 7, 'x', nil, tcell.StyleDefault)
	r.Show(screen, false)
	if out := readOutput(t, tty); out != "" {
		t.Fatalf("expected no image output when covered, got %q", out)
	}
	if len(r.prev) != 0 {
		t.Fatalf("covered image shouldn't be visible")
	}

	// Touching the last screen line gets cropped to avoid scrolling the terminal
	r.Place(mauview.NewProxyScreen(screen, 0, 18, 40, 5), img)
	if len(r.placements) != 1 || r.placements[0].rows != 1 {
		t.Fatalf("unexpected placement %+v", r.placements)
	}
}
