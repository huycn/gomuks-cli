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
	"image"
	"image/color/palette"
	"image/draw"
)

// Encode converts an image into a sixel DCS sequence.
//
// The image is quantized to the 256-color Plan 9 palette with Floyd-Steinberg dithering.
// Pixels that are mostly transparent are left unpainted.
func Encode(img image.Image) []byte {
	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width == 0 || height == 0 {
		return nil
	}

	paletted := image.NewPaletted(image.Rect(0, 0, width, height), palette.Plan9)
	draw.FloydSteinberg.Draw(paletted, paletted.Bounds(), img, bounds.Min)

	var buf bytes.Buffer
	// P2=1 means pixels that aren't set keep their current color (i.e. transparency)
	buf.WriteString("\x1bP0;1;0q")
	fmt.Fprintf(&buf, "\"1;1;%d;%d", width, height)

	var used [256]bool
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			if isOpaque(img, bounds.Min.X+x, bounds.Min.Y+y) {
				used[paletted.Pix[y*paletted.Stride+x]] = true
			}
		}
	}
	for i, isUsed := range used {
		if isUsed {
			r, g, b, _ := paletted.Palette[i].RGBA()
			fmt.Fprintf(&buf, "#%d;2;%d;%d;%d", i, r*100/0xffff, g*100/0xffff, b*100/0xffff)
		}
	}

	// Each band is 6 pixels tall. For every color in the band, the bits of a sixel character
	// mark which of the 6 pixels in that column have that color.
	bandBits := make([][]byte, 256)
	var bandColors []uint8
	for bandY := 0; bandY < height; bandY += 6 {
		bandColors = bandColors[:0]
		for dy := 0; dy < 6 && bandY+dy < height; dy++ {
			y := bandY + dy
			for x := 0; x < width; x++ {
				if !isOpaque(img, bounds.Min.X+x, bounds.Min.Y+y) {
					continue
				}
				idx := paletted.Pix[y*paletted.Stride+x]
				if bandBits[idx] == nil {
					bandBits[idx] = make([]byte, width)
					bandColors = append(bandColors, idx)
				}
				bandBits[idx][x] |= 1 << dy
			}
		}
		for i, idx := range bandColors {
			if i > 0 {
				buf.WriteByte('$')
			}
			fmt.Fprintf(&buf, "#%d", idx)
			writeRow(&buf, bandBits[idx])
			bandBits[idx] = nil
		}
		buf.WriteByte('-')
	}
	buf.WriteString("\x1b\\")
	return buf.Bytes()
}

func isOpaque(img image.Image, x, y int) bool {
	_, _, _, a := img.At(x, y).RGBA()
	return a >= 0x8000
}

// writeRow writes one color's row of sixels with run-length encoding, omitting trailing empty sixels.
func writeRow(buf *bytes.Buffer, bits []byte) {
	end := len(bits)
	for end > 0 && bits[end-1] == 0 {
		end--
	}
	for x := 0; x < end; {
		run := 1
		for x+run < end && bits[x+run] == bits[x] {
			run++
		}
		char := bits[x] + '?'
		if run > 3 {
			fmt.Fprintf(buf, "!%d%c", run, char)
		} else {
			for i := 0; i < run; i++ {
				buf.WriteByte(char)
			}
		}
		x += run
	}
}
