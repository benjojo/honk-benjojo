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
	"fmt"
	"image"
	"image/png"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/gorilla/mux"
)

func bloat_showflag(writer http.ResponseWriter, req *http.Request) {
	code := mux.Vars(req)["code"]
	colors := strings.Split(code, ",")
	numcolors := len(colors)
	vert := false
	if colors[0] == "vert" {
		vert = true
		colors = colors[1:]
		numcolors--
		if numcolors == 0 {
			http.Error(writer, "bad flag", 400)
			return
		}
	}
	pixels := make([][4]byte, numcolors)
	for i := 0; i < numcolors; i++ {
		hex := colors[i]
		if len(hex) == 3 {
			hex = fmt.Sprintf("%c%c%c%c%c%c",
				hex[0], hex[0], hex[1], hex[1], hex[2], hex[2])
		}
		c, _ := strconv.ParseUint(hex, 16, 32)
		r := byte(c >> 16 & 0xff)
		g := byte(c >> 8 & 0xff)
		b := byte(c >> 0 & 0xff)
		pixels[i][0] = r
		pixels[i][1] = g
		pixels[i][2] = b
		pixels[i][3] = 255
	}

	h := 128
	w := h * 3 / 2
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	if vert {
		for j := 0; j < w; j++ {
			pix := pixels[j*numcolors/w][:]
			for i := 0; i < h; i++ {
				p := i*img.Stride + j*4
				copy(img.Pix[p:], pix)
			}
		}
	} else {
		for i := 0; i < h; i++ {
			pix := pixels[i*numcolors/h][:]
			for j := 0; j < w; j++ {
				p := i*img.Stride + j*4
				copy(img.Pix[p:], pix)
			}
		}
	}

	writer.Header().Set("Cache-Control", "max-age="+somedays())
	png.Encode(writer, img)
}

var re_flags = regexp.MustCompile("flag:[[:alnum:],]+")

func bloat_fixupflags(h *Honk) []Emu {
	var emus []Emu
	count := 0
	h.Noise = re_flags.ReplaceAllStringFunc(h.Noise, func(m string) string {
		count++
		var e Emu
		e.Name = fmt.Sprintf(":flag%d:", count)
		e.ID = fmt.Sprintf("https://%s/flag/%s", serverName, m[5:])
		emus = append(emus, e)
		return e.Name
	})
	return emus
}

func bloat_renderflags(h *Honk) {
	h.Noise = re_flags.ReplaceAllStringFunc(h.Noise, func(m string) string {
		code := m[5:]
		src := fmt.Sprintf("https://%s/flag/%s", serverName, code)
		return fmt.Sprintf(`<img class="emu" title="%s" src="%s">`, "flag", src)
	})
}
