//
// Copyright (c) 2019 Ted Unangst <tedu@tedunangst.com>
//
// Permission to use, copy, modify, and distribute this software for any
// purpose with or without fee is hereby granted, provided that the above
// copyright notice and this permission notice appear in all copies.
//
// THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES
// WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF
// MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR
// ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES
// WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN AN
// ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT OF
// OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.

// basic image manipulation (resizing)
package image

import (
	"bytes"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"math"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

// A returned image in compressed format
type Image struct {
	Data   []byte
	Format string
	Width  int
	Height int
}

// Argument for the Vacuum function
type Params struct {
	LimitSize int // max input dimension in pixels
	MaxWidth  int
	MaxHeight int
	MaxSize   int // max output file size in bytes
	Quality   int // for jpeg output
}

const dirLeft = 1
const dirRight = 2

func fixrotation(img image.Image, dir int) image.Image {
	w, h := img.Bounds().Max.X, img.Bounds().Max.Y
	newimg := image.NewRGBA(image.Rectangle{Max: image.Point{X: h, Y: w}})
	for j := 0; j < h; j++ {
		for i := 0; i < w; i++ {
			c := img.At(i, j)
			if dir == dirLeft {
				newimg.Set(j, w-i-1, c)
			} else {
				newimg.Set(h-j-1, i, c)
			}
		}
	}
	return newimg
}

var rotateLeftSigs = [][]byte{
	{0x01, 0x12, 0x00, 0x03, 0x00, 0x00, 0x00, 0x01, 0x00, 0x08},
	{0x12, 0x01, 0x03, 0x00, 0x01, 0x00, 0x00, 0x00, 0x08, 0x00},
}
var rotateRightSigs = [][]byte{
	{0x12, 0x01, 0x03, 0x00, 0x01, 0x00, 0x00, 0x00, 0x06, 0x00},
	{0x01, 0x12, 0x00, 0x03, 0x00, 0x00, 0x00, 0x01, 0x00, 0x06},
}

// Read an image and shrink it down to web scale
func Vacuum(reader io.Reader, params Params) (*Image, error) {
	var tmpbuf bytes.Buffer
	tee := io.TeeReader(reader, &tmpbuf)
	conf, _, err := image.DecodeConfig(tee)
	if err != nil {
		return nil, err
	}
	limitSize := 16000
	if conf.Width > limitSize || conf.Height > limitSize ||
		(params.LimitSize > 0 && conf.Width*conf.Height > params.LimitSize) {
		return nil, fmt.Errorf("image is too large: x: %d y: %d", conf.Width, conf.Height)
	}
	peek := tmpbuf.Bytes()
	img, format, err := image.Decode(io.MultiReader(bytes.NewReader(peek), reader))
	if err != nil {
		return nil, err
	}

	maxh := params.MaxHeight
	maxw := params.MaxWidth
	if maxw == 0 {
		maxw = 16000
	}
	if maxh == 0 {
		maxh = 16000
	}
	if params.MaxSize == 0 {
		params.MaxSize = 512 * 1024
	}

	if format == "jpeg" {
		for _, sig := range rotateLeftSigs {
			if bytes.Contains(peek, sig) {
				img = fixrotation(img, dirLeft)
				break
			}
		}
		for _, sig := range rotateRightSigs {
			if bytes.Contains(peek, sig) {
				img = fixrotation(img, dirRight)
				break
			}
		}
	}

	bounds := img.Bounds()
	for bounds.Max.X > maxw || bounds.Max.Y > maxh {
		if bounds.Max.X > maxw*2 || bounds.Max.Y > maxh*2 {
			bounds.Max.X = bounds.Max.X / 2
			bounds.Max.Y = bounds.Max.Y / 2
		} else {
			if bounds.Max.X > maxw {
				r := float64(maxw) / float64(bounds.Max.X)
				bounds.Max.X = maxw
				bounds.Max.Y = int(float64(bounds.Max.Y) * r)
			}
			if bounds.Max.Y > maxh {
				r := float64(maxh) / float64(bounds.Max.Y)
				bounds.Max.Y = maxh
				bounds.Max.X = int(float64(bounds.Max.X) * r)
			}
		}
		dst := image.NewRGBA(bounds)
		draw.BiLinear.Scale(dst, dst.Bounds(), img, img.Bounds(), draw.Over, nil)
		img = dst
		bounds = img.Bounds()
	}

	quality := params.Quality
	if quality == 0 {
		quality = 80
	}
	var buf bytes.Buffer
	for {
		switch format {
		case "gif":
			format = "png"
			png.Encode(&buf, img)
		case "png":
			png.Encode(&buf, img)
		case "webp":
			format = "jpeg"
			fallthrough
		case "jpeg":
			jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality})
		default:
			return nil, fmt.Errorf("can't encode format: %s", format)
		}
		if buf.Len() > params.MaxSize && quality > 30 {
			switch format {
			case "png":
				format = "jpeg"
			case "jpeg":
				quality -= 10
			}
			buf.Reset()
			continue
		}
		break
	}
	rv := &Image{
		Data:   buf.Bytes(),
		Format: format,
		Width:  img.Bounds().Max.X,
		Height: img.Bounds().Max.Y,
	}
	return rv, nil
}

func lineate(s uint8) float32 {
	x := float64(s)
	x /= 255.0
	if x < 0.04045 {
		x /= 12.92
	} else {
		x += 0.055
		x /= 1.055
		x = math.Pow(x, 2.4)
	}
	return float32(x)
}

func delineate(x float64) uint8 {
	if x > 0.0031308 {
		x = math.Pow(x, 1/2.4)
		x *= 1.055
		x -= 0.055
	} else {
		x *= 12.92
	}
	x *= 255.0
	return uint8(x)
}

func gammaConvert(g []float32, d []byte, w int, h int, pixsize int, chanoffset int, stride int) []float32 {
	for j := 0; j < h; j++ {
		for i := 0; i < w; i++ {
			g[j*w+i] = lineate(d[j*stride+i*pixsize+chanoffset])
		}
	}
	return g
}

func gammaRevert(d []byte, g []float32, w int, h int, pixsize int, chanoffset int, stride int) {
	for j := 0; j < h; j++ {
		for i := 0; i < w; i++ {
			d[j*stride+i*pixsize+chanoffset] = delineate(float64(g[j*w+i]))
		}
	}
}

func blend(g []float32, s1, s2, s3, s4 int) float32 {
	l1 := g[s1]
	l2 := g[s2]
	l3 := g[s3]
	l4 := g[s4]
	return ((l1 + l2 + l3 + l4) / 4.0)
}

func oldblend(d []byte, s1, s2, s3, s4 int) byte {
	l1 := lineate(d[s1])
	l2 := lineate(d[s2])
	l3 := lineate(d[s3])
	l4 := lineate(d[s4])
	return delineate(float64(l1+l2+l3+l4) / 4.0)
}

func squish(d []byte, s1, s2, s3, s4 int) byte {
	return uint8((uint32(d[s1]) + uint32(d[s2]) + uint32(d[s3]) + uint32(d[s4])) / 4)
}
