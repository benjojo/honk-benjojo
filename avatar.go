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

package main

import (
	"bytes"
	"crypto/sha512"
	"image"
	"image/png"
)

func avatar(name string) []byte {
	h := sha512.New()
	h.Write([]byte(name))
	s := h.Sum(nil)
	img := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	for i := 0; i < 64; i++ {
		for j := 0; j < 64; j++ {
			p := i*img.Stride + j*4
			xx := i/16*16 + j/16
			x := s[xx]
			if x < 64 {
				img.Pix[p+0] = 16
				img.Pix[p+1] = 0
				img.Pix[p+2] = 48
				img.Pix[p+3] = 255
			} else if x < 128 {
				img.Pix[p+0] = 48
				img.Pix[p+1] = 0
				img.Pix[p+2] = 96
				img.Pix[p+3] = 255
			} else if x < 192 {
				img.Pix[p+0] = 72
				img.Pix[p+1] = 0
				img.Pix[p+2] = 144
				img.Pix[p+3] = 255
			} else {
				img.Pix[p+0] = 96
				img.Pix[p+1] = 0
				img.Pix[p+2] = 192
				img.Pix[p+3] = 255
			}
		}
	}
	var buf bytes.Buffer
	png.Encode(&buf, img)
	return buf.Bytes()
}
