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

package mz

import (
	"fmt"
	"html"
	"regexp"
	"strings"

	"humungus.tedunangst.com/r/webs/synlight"
)

var re_bolder = regexp.MustCompile(`(^|\W)\*\*((?s:.*?))\*\*($|\W)`)
var re_italicer = regexp.MustCompile(`(^|\W)\*((?s:.*?))\*($|\W)`)
var re_bigcoder = regexp.MustCompile("```(.*)\n?((?s:.*?))\n?```\n?")
var re_coder = regexp.MustCompile("`([^`]*)`")
var re_quoter = regexp.MustCompile(`(?m:^&gt; (.*)(\n- ?(.*))?\n?)`)
var re_link = regexp.MustCompile(`.?.?https?://[^\s"]+[\w/)!]`)
var re_zerolink = regexp.MustCompile(`\[([^]]*)\]\(([^)]*\)?)\)`)
var re_imgfix = regexp.MustCompile(`<img ([^>]*)>`)
var re_lister = regexp.MustCompile(`((^|\n+)(\+|-) .*)+\n?`)
var re_tabler = regexp.MustCompile(`((^|\n+)\|.*)+\n?`)
var re_header = regexp.MustCompile(`(^|\n+)(#+) (.*)\n?`)
var re_hashes = regexp.MustCompile(`(?:^|[ \n>])#[\pL\p{So}\pN]*[\pL\p{So}][\pL\p{So}\pN_-]*`)
var re_mentions = regexp.MustCompile(`(^|[ \n])@[[:alnum:]._-]+@[[:alnum:].-]*[[:alnum:]]\b`)
var re_urltions = regexp.MustCompile(`@https://\S+`)
var re_spoiler = regexp.MustCompile(":::(.*)\n?((?s:.*?))\n?:::\n?")

var lighter = synlight.New(synlight.Options{Format: synlight.HTML})

type Marker struct {
	AllowInlineHtml bool
	AllowImages     bool
	Short           bool
	HashLinker      func(h string) string
	HashTags        []string
	AtLinker        func(h string) string
	Mentions        []string
	Extras          []func(string) string
}

func (marker *Marker) Mark(s string) string {
	// prepare the string
	s = strings.TrimSpace(s)
	s = strings.Replace(s, "\r", "", -1)

	codeword := "`elided big code`"

	// save away the code blocks so we don't mess them up further
	var bigcodes, lilcodes, images []string
	s = re_bigcoder.ReplaceAllStringFunc(s, func(code string) string {
		bigcodes = append(bigcodes, code)
		return codeword
	})
	s = re_coder.ReplaceAllStringFunc(s, func(code string) string {
		lilcodes = append(lilcodes, code)
		return "`x`"
	})
	if marker.AllowImages {
		s = re_imgfix.ReplaceAllStringFunc(s, func(img string) string {
			images = append(images, img)
			return "<img x>"
		})
	}

	// fewer side effects than html.EscapeString
	buf := make([]byte, 0, len(s)+len(s)/10)
	if !marker.Short {
		buf = append(buf, []byte("\n\n")...)
	}
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

	for _, extra := range marker.Extras {
		s = extra(s)
	}

	// mark it zero
	if strings.Contains(s, "http") {
		s = re_link.ReplaceAllStringFunc(s, linkreplacer)
	}
	s = re_zerolink.ReplaceAllStringFunc(s, func(a string) string {
		m := re_zerolink.FindStringSubmatch(a)
		url := m[2]
		addparen := false
		if strings.HasSuffix(url, ")") && strings.IndexByte(url, '(') == -1 {
			url = url[:len(url)-1]
			addparen = true
		}
		r := fmt.Sprintf(`<a href="%s">%s</a>`, url, m[1])
		if addparen {
			r += ")"
		}
		return r
	})
	if strings.Contains(s, "*") {
		s = re_bolder.ReplaceAllString(s, "$1<b>$2</b>$3")
		s = re_italicer.ReplaceAllString(s, "$1<i>$2</i>$3")
	}
	if re_quoter.MatchString(s) {
		s = re_quoter.ReplaceAllString(s, "<blockquote>$1\n</blockquote><cite>$3</cite>\n\n")
		s = strings.ReplaceAll(s, "<cite></cite>", "")
		s = strings.ReplaceAll(s, "</blockquote>\n\n<blockquote>", "")
	}
	s = strings.Replace(s, "\n---\n", "<hr><p>", -1)
	s = re_lister.ReplaceAllStringFunc(s, func(m string) string {
		m = strings.Trim(m, "\n")
		items := strings.Split(m, "\n")
		r := "\n<ul>"
		for _, item := range items {
			if item == "" {
				continue
			}
			r += "<li>" + strings.Trim(item[1:], " ")
		}
		r += "</ul>\n"
		return r
	})
	s = re_tabler.ReplaceAllStringFunc(s, func(m string) string {
		m = strings.Trim(m, "\n")
		rows := strings.Split(m, "\n")
		var r strings.Builder
		r.WriteString("<table>")
		alignments := make(map[int]string)
		for _, row := range rows {
			hastr := false
			cells := strings.Split(row, "|")
			for i, cell := range cells {
				cell = strings.TrimSpace(cell)
				if cell == "" && (i == 0 || i == len(cells)-1) {
					continue
				}
				switch cell {
				case ":---":
					alignments[i] = ` class="text-left"`
					continue
				case ":---:":
					alignments[i] = ` class="text-center"`
					continue
				case "---:":
					alignments[i] = ` class="text-right"`
					continue
				}
				if !hastr {
					r.WriteString("<tr>")
					hastr = true
				}
				fmt.Fprintf(&r, "<td%s>", alignments[i])
				r.WriteString(cell)
			}
		}
		r.WriteString("</table><p>")
		return r.String()
	})
	s = re_header.ReplaceAllStringFunc(s, func(s string) string {
		s = strings.TrimSpace(s)
		m := re_header.FindStringSubmatch(s)
		num := len(m[2])
		return fmt.Sprintf("<h%d>%s</h%d><p>", num, m[3], num)
	})
	s = re_spoiler.ReplaceAllStringFunc(s, func(s string) string {
		m := re_spoiler.FindStringSubmatch(s)
		alert := strings.TrimSpace(m[1])
		if alert == "" {
			alert = "spoiler"
		}
		danger := m[2]
		return fmt.Sprintf("<details><summary>%s</summary>%s</details>", alert, danger)
	})

	// restore images
	if marker.AllowImages {
		s = strings.Replace(s, "&lt;img x&gt;", "<img x>", -1)
		s = re_imgfix.ReplaceAllStringFunc(s, func(string) string {
			img := images[0]
			images = images[1:]
			return img
		})
	}

	if marker.HashLinker != nil {
		s = re_hashes.ReplaceAllStringFunc(s, func(o string) string {
			p := ""
			h := o
			if h[0] != '#' {
				p = h[:1]
				h = h[1:]
			}
			marker.HashTags = append(marker.HashTags, h)
			return p + marker.HashLinker(h)
		})
	}
	if marker.AtLinker != nil {
		s = re_mentions.ReplaceAllStringFunc(s, func(m string) string {
			prefix := ""
			if m[0] == ' ' || m[0] == '\n' {
				prefix = m[:1]
				m = m[1:]
			}
			tail := ""
			if last := m[len(m)-1]; last == ' ' || last == '\n' || last == '.' {
				tail = m[len(m)-1:]
				m = m[:len(m)-1]
			}
			r := marker.AtLinker(m)
			if r == "" {
				return prefix + m + tail
			}
			marker.Mentions = append(marker.Mentions, m)
			return prefix + r + tail
		})
		s = re_urltions.ReplaceAllStringFunc(s, func(m string) string {
			prefix := ""
			tail := ""
			if last := m[len(m)-1]; last == ' ' || last == '\n' || last == '.' {
				tail = m[len(m)-1:]
				m = m[:len(m)-1]
			}
			r := marker.AtLinker(m)
			if r == "" {
				return prefix + m + tail
			}
			marker.Mentions = append(marker.Mentions, m)
			return prefix + r + tail
		})
	}

	s = strings.Replace(s, "\n\n", "<p>", -1)
	s = strings.Replace(s, "\n", "<br>", -1)

	// now restore the code blocks
	s = re_coder.ReplaceAllStringFunc(s, func(string) string {
		code := lilcodes[0]
		lilcodes = lilcodes[1:]
		if code == codeword && len(bigcodes) > 0 {
			code := bigcodes[0]
			bigcodes = bigcodes[1:]
			m := re_bigcoder.FindStringSubmatch(code)
			if marker.AllowInlineHtml && m[1] == "inlinehtml" {
				return m[2]
			}
			return "<pre><code>" + lighter.HighlightString(m[2], m[1]) + "</code></pre><p>"
		}
		code = html.EscapeString(code[1 : len(code)-1])
		return "<code>" + code + "</code>"
	})

	// some final fixups
	if strings.Contains(s, "<br><") {
		s = strings.Replace(s, "<br><blockquote>", "<blockquote>", -1)
		s = strings.Replace(s, "<br></blockquote>", "</blockquote>", -1)
		s = strings.Replace(s, "<br><cite></cite>", "", -1)
		s = strings.Replace(s, "<br><pre>", "<pre>", -1)
		s = strings.Replace(s, "<br><ul>", "<ul>", -1)
	}
	s = strings.Replace(s, "<p><br>", "<p>", -1)
	return s
}

func linkreplacer(orig string) string {
	url := orig
	if url[0:2] == "](" {
		return orig
	}
	prefix := ""
	for !strings.HasPrefix(url, "http") {
		prefix += url[0:1]
		url = url[1:]
	}
	if strings.HasSuffix(prefix, "@") {
		return orig
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
