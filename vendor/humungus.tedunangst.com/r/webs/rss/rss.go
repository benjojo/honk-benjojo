//
// Copyright (c) 2018 Ted Unangst <tedu@tedunangst.com>
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

// types for making an rss feed
package rss

import (
	"encoding/xml"
	"io"
)

type header struct {
	XMLName xml.Name `xml:"rss"`
	Version string   `xml:"version,attr"`
	Feed    *Feed
}

type Feed struct {
	XMLName        xml.Name `xml:"channel"`
	Title          string   `xml:"title"`
	Link           string   `xml:"link"`
	Description    string   `xml:"description"`
	ManagingEditor string   `xml:"managingEditor,omitempty"`
	PubDate        string   `xml:"pubDate,omitempty"`
	LastBuildDate  string   `xml:"lastBuildDate,omitempty"`
	TTL            int      `xml:"ttl,omitempty"`
	Image          *Image
	Items          []*Item
}

type Image struct {
	XMLName xml.Name `xml:"image"`
	URL     string   `xml:"url"`
	Title   string   `xml:"title"`
	Link    string   `xml:"link"`
}

type Item struct {
	XMLName     xml.Name `xml:"item"`
	Title       string   `xml:"title"`
	Description CData    `xml:"description"`
	Author      string   `xml:"author,omitempty"`
	Category    []string `xml:"category"`
	Link        string   `xml:"link"`
	PubDate     string   `xml:"pubDate"`
	Guid        *Guid
	Source      *Source
}

type Guid struct {
	XMLName     xml.Name `xml:"guid"`
	IsPermaLink bool     `xml:"isPermaLink,attr"`
	Value       string   `xml:",chardata"`
}

type Source struct {
	XMLName xml.Name `xml:"source"`
	URL     string   `xml:"url,attr"`
	Title   string   `xml:",chardata"`
}

type CData struct {
	Data string `xml:",cdata"`
}

// Write the Feed as XML.
func (fd *Feed) Write(w io.Writer) error {
	r := header{Version: "2.0", Feed: fd}
	io.WriteString(w, xml.Header)
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	err := enc.Encode(r)
	io.WriteString(w, "\n")
	return err
}
