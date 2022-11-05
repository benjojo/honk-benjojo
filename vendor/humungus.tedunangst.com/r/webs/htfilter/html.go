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

// A hypertext filter.
// Rewrite HTML into a safer whitelisted subset.
package htfilter

import (
	"fmt"
	"html/template"
	"net/url"
	"strings"

	"golang.org/x/net/html"
	"humungus.tedunangst.com/r/webs/templates"
)

// A filter.
// Imager is a function that is used to process <img> tags.
// It should return the HTML replacement.
// SpanClasses is a map of classes allowed for span tags.
// The zero filter is useful by itself.
// By default, images are replaced with text.
type Filter struct {
	Imager      func(node *html.Node) string
	SpanClasses map[string]bool
}

var permittedtags = map[string]bool{
	"div": true, "hr": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"table": true, "thead": true, "tbody": true, "th": true,
	"tr": true, "td": true, "colgroup": true, "col": true,
	"p": true, "br": true, "pre": true, "code": true, "blockquote": true, "q": true,
	"samp": true, "mark": true, "ins": true, "dfn": true, "cite": true,
	"abbr": true, "address": true,
	"strong": true, "em": true, "b": true, "i": true, "s": true, "u": true,
	"sub": true, "sup": true, "del": true, "tt": true, "small": true,
	"ol": true, "ul": true, "li": true, "dl": true, "dt": true, "dd": true,
}
var permittedattr = map[string]bool{"colspan": true, "rowspan": true}
var bannedtags = map[string]bool{"script": true, "style": true}

// Returns the value for a node attribute.
func GetAttr(node *html.Node, attr string) string {
	for _, a := range node.Attr {
		if a.Key == attr {
			return a.Val
		}
	}
	return ""
}

// Returns true if this node has specified class
func HasClass(node *html.Node, class string) bool {
	return strings.Contains(" "+GetAttr(node, "class")+" ", " "+class+" ")
}

type writer interface {
	Write(p []byte) (n int, err error)
	WriteString(s string) (n int, err error)
}

func writetag(w writer, node *html.Node) {
	w.WriteString("<")
	w.WriteString(node.Data)
	for _, attr := range node.Attr {
		if permittedattr[attr.Key] {
			templates.Fprintf(w, ` %s="%s"`, attr.Key, attr.Val)
		}
	}
	w.WriteString(">")
}

func getclasses(node *html.Node, allowed map[string]bool) string {
	if allowed == nil {
		return ""
	}
	nodeclass := GetAttr(node, "class")
	if len(nodeclass) == 0 {
		return ""
	}
	classes := strings.Split(nodeclass, " ")
	var toprint []string
	for _, c := range classes {
		if allowed[c] {
			toprint = append(toprint, c)
		}
	}
	if len(toprint) == 0 {
		return ""
	}
	return fmt.Sprintf(` class="%s"`, strings.Join(toprint, " "))
}

// no need to escape quotes here
func writeText(w writer, text string) {
	last := 0
	for i, c := range text {
		var html string
		switch c {
		case '\000':
			html = "\ufffd"
		case '&':
			html = "&amp;"
		case '<':
			html = "&lt;"
		case '>':
			html = "&gt;"
		default:
			continue
		}
		w.WriteString(text[last:i])
		w.WriteString(html)
		last = i + 1
	}
	w.WriteString(text[last:])
}

func (filt *Filter) render(w writer, node *html.Node) {
	closespan := false
	if node.Type == html.ElementNode {
		tag := node.Data
		switch {
		case tag == "a":
			href := GetAttr(node, "href")
			hrefurl, err := url.Parse(href)
			if err != nil {
				href = "#BROKEN-" + href
			} else {
				href = hrefurl.String()
			}
			templates.Fprintf(w, `<a href="%s" rel=noreferrer>`, href)
		case tag == "img":
			if filt.Imager != nil {
				div := filt.Imager(node)
				w.WriteString(div)
			} else {
				div := imgtotext(node)
				w.WriteString(div)
			}
		case tag == "span":
			c := getclasses(node, filt.SpanClasses)
			if c != "" {
				w.WriteString("<span")
				w.WriteString(c)
				w.WriteString(">")
				closespan = true
			}
		case tag == "iframe":
			src := GetAttr(node, "src")
			templates.Fprintf(w, `&lt;iframe src="<a href="%s">%s</a>"&gt;`, src, src)
		case permittedtags[tag]:
			writetag(w, node)
		case bannedtags[tag]:
			return
		}
	} else if node.Type == html.TextNode {
		writeText(w, node.Data)
	}

	for c := node.FirstChild; c != nil; c = c.NextSibling {
		filt.render(w, c)
	}

	if node.Type == html.ElementNode {
		tag := node.Data
		if tag == "a" || (permittedtags[tag] && tag != "br") {
			fmt.Fprintf(w, "</%s>", tag)
		}
		if closespan {
			w.WriteString("</span>")
		}
		if tag == "p" || tag == "div" {
			w.WriteString("\n")
		}
	}
}

func imgtotext(node *html.Node) string {
	src := GetAttr(node, "src")
	alt := GetAttr(node, "alt")
	//title := GetAttr(node, "title")
	if HasClass(node, "Emoji") && alt != "" {
		return alt
	}
	return html.EscapeString(fmt.Sprintf(`<img alt="%s" src="%s">`, alt, src))
}

func (filt *Filter) cleannode(node *html.Node) template.HTML {
	var buf strings.Builder
	filt.render(&buf, node)
	return template.HTML(buf.String())
}

func (filt *Filter) String(shtml string) (template.HTML, error) {
	reader := strings.NewReader(shtml)
	body, err := html.Parse(reader)
	if err != nil {
		return "", err
	}
	return filt.cleannode(body), nil
}

func (filt *Filter) TextOnly(node *html.Node) string {
	var buf strings.Builder
	filt.gathertext(&buf, node, false)
	return buf.String()
}

func (filt *Filter) gathertext(w writer, node *html.Node, withlinks bool) {
	switch node.Type {
	case html.ElementNode:
		tag := node.Data
		switch {
		case tag == "a":
			fmt.Fprintf(w, " ")
			if withlinks {
				href := GetAttr(node, "href")
				fmt.Fprintf(w, `<a href="%s">`, href)
			}
		case tag == "img":
			div := filt.Imager(node)
			w.WriteString(div)
		case tag == "span":
			if HasClass(node, "tco-ellipsis") {
				return
			}
		case bannedtags[tag]:
			return
		}
	case html.TextNode:
		w.WriteString(node.Data)
	}
	for c := node.FirstChild; c != nil; c = c.NextSibling {
		filt.gathertext(w, c, withlinks)
	}
	if node.Type == html.ElementNode {
		tag := node.Data
		if withlinks && tag == "a" {
			fmt.Fprintf(w, "</%s>", tag)
		}
		if tag == "p" || tag == "div" {
			w.WriteString("\n")
		}
	}
}
