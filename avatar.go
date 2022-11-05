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
	"bufio"
	"bytes"
	"crypto/sha512"
	"image"
	"image/png"
	"log"
	"strconv"
	"strings"
)

var avatarcolors = [4][4]byte{
	{16, 0, 48, 255},
	{48, 0, 96, 255},
	{72, 0, 144, 255},
	{96, 0, 192, 255},
}

func loadAvatarColors() {
	var colors string
	getconfig("avatarcolors", &colors)
	if colors == "" {
		return
	}
	r := bufio.NewReader(strings.NewReader(colors))
	for i := 0; i < 4; i++ {
		l, _ := r.ReadString(' ')
		for l == " " {
			l, _ = r.ReadString(' ')
		}
		l = strings.Trim(l, "# \n")
		if len(l) == 6 {
			l = l + "ff"
		}
		c, err := strconv.ParseUint(l, 16, 32)
		if err != nil {
			log.Printf("error reading avatar color %d: %s", i, err)
			continue
		}
		avatarcolors[i][0] = byte(c >> 24 & 0xff)
		avatarcolors[i][1] = byte(c >> 16 & 0xff)
		avatarcolors[i][2] = byte(c >> 8 & 0xff)
		avatarcolors[i][3] = byte(c >> 0 & 0xff)
	}
}

func genAvatar(name string) []byte {
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
				img.Pix[p+0] = avatarcolors[0][0]
				img.Pix[p+1] = avatarcolors[0][1]
				img.Pix[p+2] = avatarcolors[0][2]
				img.Pix[p+3] = avatarcolors[0][3]
			} else if x < 128 {
				img.Pix[p+0] = avatarcolors[1][0]
				img.Pix[p+1] = avatarcolors[1][1]
				img.Pix[p+2] = avatarcolors[1][2]
				img.Pix[p+3] = avatarcolors[1][3]
			} else if x < 192 {
				img.Pix[p+0] = avatarcolors[2][0]
				img.Pix[p+1] = avatarcolors[2][1]
				img.Pix[p+2] = avatarcolors[2][2]
				img.Pix[p+3] = avatarcolors[2][3]
			} else {
				img.Pix[p+0] = avatarcolors[3][0]
				img.Pix[p+1] = avatarcolors[3][1]
				img.Pix[p+2] = avatarcolors[3][2]
				img.Pix[p+3] = avatarcolors[3][3]
			}
		}
	}
	var buf bytes.Buffer
	png.Encode(&buf, img)
	return buf.Bytes()
}
