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
	"regexp"
	"strings"

	"golang.org/x/net/html"
	"humungus.tedunangst.com/r/webs/synlight"
)

var re_bolder = regexp.MustCompile(`(^|\W)\*\*((?s:.*?))\*\*($|\W)`)
var re_italicer = regexp.MustCompile(`(^|\W)\*((?s:.*?))\*($|\W)`)
var re_bigcoder = regexp.MustCompile("```(.*)\n?((?s:.*?))\n?```\n?")
var re_coder = regexp.MustCompile("`([^`]*)`")
var re_quoter = regexp.MustCompile(`(?m:^&gt; (.*)(\n- ?(.*))?\n?)`)
var re_reciter = regexp.MustCompile(`(<cite><a href=".*?">)https://twitter.com/([^/]+)/.*?(</a></cite>)`)
var re_link = regexp.MustCompile(`.?.?https?://[^\s"]+[\w/)!]`)
var re_zerolink = regexp.MustCompile(`\[([^]]*)\]\(([^)]*\)?)\)`)
var re_imgfix = regexp.MustCompile(`<img ([^>]*)>`)
var re_lister = regexp.MustCompile(`((^|\n)(\+|-).*)+\n?`)

var lighter = synlight.New(synlight.Options{Format: synlight.HTML})

func markitzero(s string) string {
	// prepare the string
	s = strings.TrimSpace(s)
	s = strings.Replace(s, "\r", "", -1)

	// save away the code blocks so we don't mess them up further
	var bigcodes, lilcodes, images []string
	s = re_bigcoder.ReplaceAllStringFunc(s, func(code string) string {
		bigcodes = append(bigcodes, code)
		return "``````"
	})
	s = re_coder.ReplaceAllStringFunc(s, func(code string) string {
		lilcodes = append(lilcodes, code)
		return "`x`"
	})
	s = re_imgfix.ReplaceAllStringFunc(s, func(img string) string {
		images = append(images, img)
		return "<img x>"
	})

	// fewer side effects than html.EscapeString
	buf := make([]byte, 0, len(s))
	for _, c := range []byte(s) {
		switch c {
		case '&':
			buf = append(buf, []byte("&amp;")...)
		case '<':
			buf = append(buf, []byte("&lt;")...)
		case '>':
			buf = append(buf, []byte("&gt;")...)
		default:
			buf = append(buf, c)
		}
	}
	s = string(buf)

	// mark it zero
	s = re_link.ReplaceAllStringFunc(s, linkreplacer)
	s = re_zerolink.ReplaceAllString(s, `<a href="$2">$1</a>`)
	s = re_bolder.ReplaceAllString(s, "$1<b>$2</b>$3")
	s = re_italicer.ReplaceAllString(s, "$1<i>$2</i>$3")
	s = re_quoter.ReplaceAllString(s, "<blockquote>$1<br><cite>$3</cite></blockquote><p>")
	s = re_reciter.ReplaceAllString(s, "$1$2$3")
	s = strings.Replace(s, "\n---\n", "<hr><p>", -1)

	s = re_lister.ReplaceAllStringFunc(s, func(m string) string {
		m = strings.Trim(m, "\n")
		items := strings.Split(m, "\n")
		r := "<ul>"
		for _, item := range items {
			r += "<li>" + strings.Trim(item[1:], " ")
		}
		r += "</ul><p>"
		return r
	})

	// restore images
	s = strings.Replace(s, "&lt;img x&gt;", "<img x>", -1)
	s = re_imgfix.ReplaceAllStringFunc(s, func(string) string {
		img := images[0]
		images = images[1:]
		return img
	})

	// now restore the code blocks
	s = re_coder.ReplaceAllStringFunc(s, func(string) string {
		code := lilcodes[0]
		lilcodes = lilcodes[1:]
		code = html.EscapeString(code)
		return code
	})
	s = re_bigcoder.ReplaceAllStringFunc(s, func(string) string {
		code := bigcodes[0]
		bigcodes = bigcodes[1:]
		m := re_bigcoder.FindStringSubmatch(code)
		return "<pre><code>" + lighter.HighlightString(m[2], m[1]) + "</code></pre><p>"
	})
	s = re_coder.ReplaceAllString(s, "<code>$1</code>")

	// some final fixups
	s = strings.Replace(s, "\n", "<br>", -1)
	s = strings.Replace(s, "<br><blockquote>", "<blockquote>", -1)
	s = strings.Replace(s, "<br><cite></cite>", "", -1)
	s = strings.Replace(s, "<br><pre>", "<pre>", -1)
	s = strings.Replace(s, "<br><ul>", "<ul>", -1)
	s = strings.Replace(s, "<p><br>", "<p>", -1)
	return s
}

func linkreplacer(url string) string {
	if url[0:2] == "](" {
		return url
	}
	prefix := ""
	for !strings.HasPrefix(url, "http") {
		prefix += url[0:1]
		url = url[1:]
	}
	addparen := false
	adddot := false
	if strings.HasSuffix(url, ")") && strings.IndexByte(url, '(') == -1 {
		url = url[:len(url)-1]
		addparen = true
	}
	if strings.HasSuffix(url, ".") {
		url = url[:len(url)-1]
		adddot = true
	}
	url = fmt.Sprintf(`<a href="%s">%s</a>`, url, url)
	if adddot {
		url += "."
	}
	if addparen {
		url += ")"
	}
	return prefix + url
}
