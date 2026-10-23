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
	"image"
	_ "image/gif"  // initialize decoder
	_ "image/jpeg" // initialize decoder
	_ "image/png"  // initialize decoder
	"io"

	"github.com/disintegration/imaging"
	_ "golang.org/x/image/bmp"  // initialize decoder
	_ "golang.org/x/image/tiff" // initialize decoder
	_ "golang.org/x/image/webp" // initialize decoder
)

// maxCachedCrops is the number of differently cropped encodings kept per image.
// Scrolling an image partially out of view creates a new crop for every line.
const maxCachedCrops = 16

type cropKey struct {
	top, rows int
}

// Image is an image scaled to fit in a whole number of terminal cells.
type Image struct {
	img   *image.NRGBA
	cellH int

	Cols, Rows int

	cache map[cropKey][]byte
}

// NewImageFromReader decodes an image and scales it to fit in maxCols x maxRows cells.
func NewImageFromReader(reader io.Reader, maxCols, maxRows int, cell CellSize) (*Image, error) {
	img, _, err := image.Decode(reader)
	if err != nil {
		return nil, err
	}
	return NewImage(img, maxCols, maxRows, cell), nil
}

// NewImage scales an image to fit in maxCols x maxRows cells. Images are never scaled up.
func NewImage(img image.Image, maxCols, maxRows int, cell CellSize) *Image {
	bounds := img.Bounds()
	width, height := float64(bounds.Dx()), float64(bounds.Dy())
	scale := 1.0
	if maxWidth := float64(maxCols * cell.Width); width*scale > maxWidth {
		scale = maxWidth / width
	}
	if maxHeight := float64(maxRows * cell.Height); height*scale > maxHeight {
		scale = maxHeight / height
	}
	targetWidth := max(int(width*scale), 1)
	targetHeight := max(int(height*scale), 1)

	return &Image{
		img:   imaging.Resize(img, targetWidth, targetHeight, imaging.Lanczos),
		cellH: cell.Height,
		Cols:  (targetWidth + cell.Width - 1) / cell.Width,
		Rows:  (targetHeight + cell.Height - 1) / cell.Height,
		cache: make(map[cropKey][]byte),
	}
}

// Encoded returns the sixel sequence for the given range of cell rows of the image.
func (img *Image) Encoded(topRow, rows int) []byte {
	key := cropKey{topRow, rows}
	if data, ok := img.cache[key]; ok {
		return data
	}
	bounds := img.img.Bounds()
	crop := image.Rect(bounds.Min.X, bounds.Min.Y+topRow*img.cellH, bounds.Max.X, bounds.Min.Y+(topRow+rows)*img.cellH)
	data := Encode(img.img.SubImage(crop.Intersect(bounds)))
	if len(img.cache) >= maxCachedCrops {
		clear(img.cache)
	}
	img.cache[key] = data
	return data
}
