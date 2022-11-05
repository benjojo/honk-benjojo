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
	"net/http"
	"strings"

	"humungus.tedunangst.com/r/webs/junk"
)

func servewonkles(w http.ResponseWriter, r *http.Request) {
	url := r.FormValue("w")
	dlog.Printf("getting wordlist: %s", url)
	wonkles := getxonker(url, "wonkles")
	if wonkles == "" {
		wonkles = savewonkles(url)
		if wonkles == "" {
			http.NotFound(w, r)
			return
		}
	}
	var words []string
	for _, l := range strings.Split(wonkles, "\n") {
		words = append(words, l)
	}
	if !develMode {
		w.Header().Set("Cache-Control", "max-age=7776000")
	}

	j := junk.New()
	j["wordlist"] = words
	j.Write(w)
}

func savewonkles(url string) string {
	w := getxonker(url, "wonkles")
	if w != "" {
		return w
	}
	ilog.Printf("fetching wonkles: %s", url)
	res, err := fetchsome(url)
	if err != nil {
		ilog.Printf("error fetching wonkles: %s", err)
		return ""
	}
	w = getxonker(url, "wonkles")
	if w != "" {
		return w
	}
	w = string(res)
	savexonker(url, w, "wonkles", "")
	return w
}
