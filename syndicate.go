//
// Copyright (c) 2024 Ted Unangst <tedu@tedunangst.com>
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
	notrand "math/rand"
	"strings"
	"time"

	"github.com/mmcdole/gofeed"
)

type Feed struct {
	user *WhatAbout
	url  string
}

func syndicate(user *WhatAbout, url string) {
	data, err := fetchsome(url)
	if err != nil {
		dlog.Printf("error fetching feed: %s", err)
		return
	}
	parser := gofeed.NewParser()
	rss, err := parser.Parse(bytes.NewReader(data))
	if err != nil {
		dlog.Printf("error parsing feed: %s", err)
		return
	}
	reverseItems(rss.Items)
	for _, item := range rss.Items {
		dlog.Printf("link: %s", item.Link)
		grabhonk(user, item.Link)
	}
}

func getfeeds() []Feed {
	var feeds []Feed
	users := allusers()
	for _, ui := range users {
		user, _ := getUserBio(ui.Username)
		honkers := gethonkers(user.ID)
		for _, h := range honkers {
			if strings.HasSuffix(h.XID, ".rss") {
				feeds = append(feeds, Feed{user: user, url: h.XID})
			}
		}
	}
	return feeds
}

func syndicator() {
	for {
		pause := 4 * time.Hour
		pause += time.Duration(notrand.Int63n(int64(pause / 4)))
		feeds := getfeeds()
		pause /= time.Duration(len(feeds) + 1)
		time.Sleep(pause)
		for _, f := range feeds {
			syndicate(f.user, f.url)
			time.Sleep(pause)
		}
	}
}

func reverseItems(items []*gofeed.Item) {
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
}
