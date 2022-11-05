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
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"

	"github.com/andybalholm/cascadia"
	"golang.org/x/net/html"
	"humungus.tedunangst.com/r/webs/htfilter"
)

var tweetsel = cascadia.MustCompile("p.tweet-text")
var linksel = cascadia.MustCompile("a.tweet-timestamp")
var replyingto = cascadia.MustCompile(".ReplyingToContextBelowAuthor")
var imgsel = cascadia.MustCompile("div.js-adaptive-photo img")
var authorregex = regexp.MustCompile("twitter.com/([^/]+)")

var re_hoots = regexp.MustCompile(`hoot: ?https://\S+`)
var re_removepics = regexp.MustCompile(`pic\.twitter\.com/[[:alnum:]]+`)

func hootextractor(r io.Reader, url string, seen map[string]bool) string {
	root, err := html.Parse(r)
	if err != nil {
		elog.Printf("error parsing hoot: %s", err)
		return url
	}

	url = strings.Replace(url, "mobile.twitter.com", "twitter.com", -1)
	wantmatch := authorregex.FindStringSubmatch(url)
	var wanted string
	if len(wantmatch) == 2 {
		wanted = wantmatch[1]
	}

	var htf htfilter.Filter
	htf.Imager = func(node *html.Node) string {
		alt := htfilter.GetAttr(node, "alt")
		if htfilter.HasClass(node, "Emoji") && alt != "" {
			return alt
		}
		return fmt.Sprintf(" <img src='%s'>", htfilter.GetAttr(node, "src"))
	}

	var buf strings.Builder
	fmt.Fprintf(&buf, "%s\n", url)

	divs := tweetsel.MatchAll(root)
	for i, div := range divs {
		twp := div.Parent.Parent.Parent
		link := url
		alink := linksel.MatchFirst(twp)
		if alink == nil {
			if i != 0 {
				dlog.Printf("missing link")
				continue
			}
		} else {
			link = "https://twitter.com" + htfilter.GetAttr(alink, "href")
		}
		replto := replyingto.MatchFirst(twp)
		if replto != nil {
			continue
		}
		authormatch := authorregex.FindStringSubmatch(link)
		if len(authormatch) < 2 {
			dlog.Printf("no author?: %s", link)
			continue
		}
		author := authormatch[1]
		if wanted == "" {
			wanted = author
		}
		if author != wanted {
			continue
		}
		for _, img := range imgsel.MatchAll(twp) {
			img.Parent.RemoveChild(img)
			div.AppendChild(img)
		}
		text := htf.NodeText(div)
		text = strings.Replace(text, "\n", " ", -1)
		text = re_removepics.ReplaceAllString(text, "")

		if seen[text] {
			continue
		}

		fmt.Fprintf(&buf, "> @%s: %s\n", author, text)
		seen[text] = true
	}
	return buf.String()
}

func hooterize(noise string) string {
	seen := make(map[string]bool)

	hootfetcher := func(hoot string) string {
		url := hoot[5:]
		if url[0] == ' ' {
			url = url[1:]
		}
		url = strings.Replace(url, "mobile.twitter.com", "twitter.com", -1)
		dlog.Printf("hooterizing %s", url)
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			ilog.Printf("error: %s", err)
			return hoot
		}
		req.Header.Set("User-Agent", "Bot")
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
		req.Header.Set("Accept-Language", "en-US,en;q=0.9")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			ilog.Printf("error: %s", err)
			return hoot
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			ilog.Printf("error getting %s: %d", url, resp.StatusCode)
			return hoot
		}
		ld, _ := os.Create("lasthoot.html")
		r := io.TeeReader(resp.Body, ld)
		return hootextractor(r, url, seen)
	}

	return re_hoots.ReplaceAllStringFunc(noise, hootfetcher)
}
