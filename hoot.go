package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"

	"github.com/andybalholm/cascadia"
	"golang.org/x/net/html"
	"humungus.tedunangst.com/r/webs/htfilter"
)

var tweetsel = cascadia.MustCompile("p.tweet-text")
var linksel = cascadia.MustCompile(".time a.tweet-timestamp")
var authorregex = regexp.MustCompile("twitter.com/([^/]+)")

func hootfetcher(hoot string) string {
	url := hoot[5:]
	if url[0] == ' ' {
		url = url[1:]
	}
	url = strings.Replace(url, "mobile.twitter.com", "twitter.com", -1)
	log.Printf("hooterizing %s", url)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Printf("error: %s", err)
		return hoot
	}
	req.Header.Set("User-Agent", "OpenBSD ftp")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("error: %s", err)
		return hoot
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		log.Printf("error getting %s: %d", url, resp.StatusCode)
		return hoot
	}
	ld, _ := os.Create("lasthoot.html")
	r := io.TeeReader(resp.Body, ld)
	return hootfixer(r, url)
}

func hootfixer(r io.Reader, url string) string {
	root, _ := html.Parse(r)
	divs := tweetsel.MatchAll(root)

	wanted := ""
	var buf strings.Builder

	fmt.Fprintf(&buf, "hoot: %s\n", url)
	for _, div := range divs {
		twp := div.Parent.Parent.Parent
		alink := linksel.MatchFirst(twp)
		if alink == nil {
			log.Printf("missing link")
			continue
		}
		link := "https://twitter.com" + htfilter.GetAttr(alink, "href")
		authormatch := authorregex.FindStringSubmatch(link)
		if len(authormatch) < 2 {
			log.Printf("no author?")
			continue
		}
		author := authormatch[1]
		if wanted == "" {
			wanted = author
		}
		if author != wanted {
			continue
		}
		text := htfilter.TextOnly(div)
		text = strings.Replace(text, "\n", " ", -1)
		text = strings.Replace(text, "pic.twitter.com", "https://pic.twitter.com", -1)

		fmt.Fprintf(&buf, "> @%s: %s\n", author, text)
	}
	return buf.String()
}

var re_hoots = regexp.MustCompile(`hoot: ?https://\S+`)

func hooterize(noise string) string {
	return re_hoots.ReplaceAllStringFunc(noise, hootfetcher)
}
