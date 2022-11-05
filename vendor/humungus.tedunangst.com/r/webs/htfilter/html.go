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
	"io"
	"net/url"
	"regexp"
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
	BaseURL     *url.URL
	WithLinks   bool
	FilterText  func(io.Writer, string)
}

var permittedtags = map[string]bool{
	"div": true, "hr": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"table": true, "thead": true, "tbody": true, "th": true, "tfoot": true,
	"tr": true, "td": true, "colgroup": true, "col": true, "caption": true,
	"p": true, "br": true, "pre": true, "code": true, "blockquote": true, "q": true,
	"kbd": true, "time": true, "wbr": true, "aside": true,
	"ruby": true, "rtc": true, "rb": true, "rt": true,
	"samp": true, "mark": true, "ins": true, "dfn": true, "cite": true,
	"abbr": true, "address": true, "details": true, "summary": true,
	"strong": true, "em": true, "b": true, "i": true, "s": true, "u": true,
	"sub": true, "sup": true, "del": true, "tt": true, "small": true,
	"ol": true, "ul": true, "li": true, "dl": true, "dt": true, "dd": true,
}
var permittedattr = map[string]bool{"colspan": true, "rowspan": true}
var bannedtags = map[string]bool{"script": true, "style": true}

// Returns the value for a node attribute.
func GetAttr(node *html.Node, name string) string {
	for _, a := range node.Attr {
		if a.Key == name {
			return a.Val
		}
	}
	return ""
}

func SetAttr(node *html.Node, name string, value string) {
	for _, a := range node.Attr {
		if a.Key == name {
			a.Val = value
			return
		}
	}
	node.Attr = append(node.Attr, html.Attribute{Key: name, Val: value})
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
		if attr.Key == "style" {
			styles := strings.Split(attr.Val, ";")
			printedstyle := false
			for _, style := range styles {
				style = strings.Replace(style, " ", "", -1)
				switch style {
				case "text-align:left":
				case "text-align:right":
				case "text-align:center":
				default:
					continue
				}
				if !printedstyle {
					w.WriteString(` style ="`)
					printedstyle = true
				}
				w.WriteString(style)
				w.WriteString(";")
			}
			if printedstyle {
				w.WriteString(`"`)
			}
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

// A very basic escaper
func EscapeText(text string) string {
	var buf strings.Builder
	WriteText(&buf, text)
	return buf.String()
}

func WriteText(w io.Writer, text string) {
	// no need to escape quotes here
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
		io.WriteString(w, text[last:i])
		io.WriteString(w, html)
		last = i + 1
	}
	io.WriteString(w, text[last:])
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
				if filt.BaseURL != nil {
					hrefurl = filt.BaseURL.ResolveReference(hrefurl)
				}
				href = hrefurl.String()
			}
			templates.Fprintf(w, `<a href="%s" rel=noreferrer>`, href)
		case tag == "img":
			if filt.BaseURL != nil {
				src := GetAttr(node, "src")
				srcurl, err := url.Parse(src)
				if err == nil {
					srcurl = filt.BaseURL.ResolveReference(srcurl)
					SetAttr(node, "src", srcurl.String())
				}
			}
			if filt.Imager != nil {
				div := filt.Imager(node)
				w.WriteString(div)
			} else {
				div := html.EscapeString(imgtotext(node))
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
		if filt.FilterText != nil {
			filt.FilterText(w, node.Data)
		} else {
			WriteText(w, node.Data)
		}
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

func (filt *Filter) rendertext(w writer, node *html.Node) {
	switch node.Type {
	case html.ElementNode:
		tag := node.Data
		switch {
		case tag == "a":
			if filt.WithLinks {
				fmt.Fprintf(w, " ")
				href := GetAttr(node, "href")
				fmt.Fprintf(w, `<a href="%s">`, href)
			}
		case tag == "img":
			if filt.Imager != nil {
				div := filt.Imager(node)
				w.WriteString(div)
			} else {
				div := imgtotext(node)
				w.WriteString(div)
			}
		case tag == "span":
			if HasClass(node, "tco-ellipsis") {
				return
			}
		case bannedtags[tag]:
			return
		}
	case html.TextNode:
		w.WriteString(strings.Replace(node.Data, "\n", " ", -1))
	}
	for c := node.FirstChild; c != nil; c = c.NextSibling {
		filt.rendertext(w, c)
	}
	if node.Type == html.ElementNode {
		tag := node.Data
		if filt.WithLinks && tag == "a" {
			fmt.Fprintf(w, "</%s>", tag)
		}
		if tag == "p" || tag == "div" || tag == "tr" {
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
	return fmt.Sprintf(`<img alt="%s" src="%s">`, alt, src)
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

var re_whitespaceeater = regexp.MustCompile("[ \t\r]*\n[ \t\r]*")
var re_blanklineeater = regexp.MustCompile("\n\n+")
var re_tabeater = regexp.MustCompile("[ \t]+")

func (filt *Filter) TextOnly(shtml string) (string, error) {
	reader := strings.NewReader(shtml)
	body, err := html.Parse(reader)
	if err != nil {
		return "", err
	}
	return filt.NodeText(body), nil
}

func (filt *Filter) NodeText(node *html.Node) string {
	var buf strings.Builder
	filt.rendertext(&buf, node)
	str := buf.String()
	str = re_whitespaceeater.ReplaceAllLiteralString(str, "\n")
	str = re_blanklineeater.ReplaceAllLiteralString(str, "\n\n")
	str = re_tabeater.ReplaceAllLiteralString(str, " ")
	for len(str) > 0 && str[0] == '\n' {
		str = str[1:]
	}
	return str
}
