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
	"html/template"
	"strconv"
	"strings"
	"time"

	"humungus.tedunangst.com/r/webs/httpsig"
)

type WhatAbout struct {
	ID         UserID
	Name       string
	Display    string
	About      string
	HTAbout    template.HTML
	Onts       []string
	Key        string
	URL        string
	Options    UserOptions
	SecKey     httpsig.PrivateKey
	ChatPubKey boxPubKey
	ChatSecKey boxSecKey
}

type UserOptions struct {
	SkinnyCSS    bool   `json:",omitempty"`
	OmitImages   bool   `json:",omitempty"`
	MentionAll   bool   `json:",omitempty"`
	InlineQuotes bool   `json:",omitempty"`
	Avatar       string `json:",omitempty"`
	Banner       string `json:",omitempty"`
	MapLink      string `json:",omitempty"`
	Reaction     string `json:",omitempty"`
	MeCount      int64
	ChatCount    int64
	ChatPubKey   string
	ChatSecKey   string
	TOTP         string `json:",omitempty"`
}

type KeyInfo struct {
	keyname string
	seckey  httpsig.PrivateKey
}

type UserID int64

const serverUID UserID = -2
const firstUserUID UserID = 1

type ActivityPubActivity struct {
	ID        int64
	UserID    UserID
	Username  string
	What      string
	Honker    string
	Handle    string
	Handles   string
	Oonker    string
	Oondle    string
	XID       string
	RID       string
	Date      time.Time
	URL       string
	Noise     string
	Precis    string
	Format    string
	Convoy    string
	Audience  []string
	Public    bool
	Whofore   Whofore
	Replies   []*ActivityPubActivity
	Flags     int64
	HTPrecis  template.HTML
	HTML      template.HTML
	Style     string
	Open      string
	Donks     []*Donk
	Onts      []string
	Place     *Place
	Time      *Time
	Link      string
	Mentions  []Mention
	Badonks   []Badonk
	SeeAlso   string
	Onties    string
	LegalName string
}

type Whofore int

const WhoAtme Whofore = 1
const WhoPublic Whofore = 2
const WhoPrivate Whofore = 3

type Badonk struct {
	Who  string
	What string
}

type Chonk struct {
	ID     int64
	UserID UserID
	XID    string
	Who    string
	Target string
	Date   time.Time
	Noise  string
	Format string
	Donks  []*Donk
	Handle string
	HTML   template.HTML
}

type Chatter struct {
	Target string
	Chonks []*Chonk
}

type Mention struct {
	Who   string
	Where string
}

func (mention *Mention) IsPresent(noise string) bool {
	nick := strings.TrimLeft(mention.Who, "@")
	idx := strings.IndexByte(nick, '@')
	if idx != -1 {
		nick = nick[:idx]
	}
	return strings.Contains(noise, ">@"+nick) || strings.Contains(noise, "@<span>"+nick)
}

func OntIsPresent(ont, noise string) bool {
	ont = strings.ToLower(ont[1:])
	idx := strings.IndexByte(noise, '#')
	for idx >= 0 {
		if strings.HasPrefix(noise[idx:], "#<span>") {
			idx += 6
		}
		idx += 1
		if idx+len(ont) >= len(noise) {
			return false
		}
		test := noise[idx : idx+len(ont)]
		test = strings.ToLower(test)
		if test == ont {
			return true
		}
		newidx := strings.IndexByte(noise[idx:], '#')
		if newidx == -1 {
			return false
		}
		idx += newidx
	}
	return false
}

type OldRevision struct {
	Precis string
	Noise  string
}

const (
	flagIsAcked    = 1
	flagIsBonked   = 2
	flagIsSaved    = 4
	flagIsUntagged = 8
	flagIsReacted  = 16
	flagIsBSkyd    = 128
)

func (honk *ActivityPubActivity) IsBSkyd() bool {
	return honk.Flags&flagIsBSkyd != 0
}

func (honk *ActivityPubActivity) IsAcked() bool {
	return honk.Flags&flagIsAcked != 0
}

func (honk *ActivityPubActivity) IsBonked() bool {
	return honk.Flags&flagIsBonked != 0
}

func (honk *ActivityPubActivity) IsSaved() bool {
	return honk.Flags&flagIsSaved != 0
}

func (honk *ActivityPubActivity) IsUntagged() bool {
	return honk.Flags&flagIsUntagged != 0
}

func (honk *ActivityPubActivity) IsReacted() bool {
	return honk.Flags&flagIsReacted != 0
}

func (honk *ActivityPubActivity) ShortXID() string {
	return shortxid(honk.XID)
}

type Donk struct {
	FileID   int64
	XID      string
	Name     string
	Desc     string
	URL      string
	Media    string
	Local    bool
	External bool
	Meta     DonkMeta
}
type DonkMeta struct {
	Length int `json:",omitempty"`
	Width  int `json:",omitempty"`
	Height int `json:",omitempty"`
}

type Place struct {
	Name      string
	Latitude  float64
	Longitude float64
	Url       string
}

type Duration int64

func (d Duration) String() string {
	s := time.Duration(d).String()
	if strings.HasSuffix(s, "m0s") {
		s = s[:len(s)-2]
	}
	if strings.HasSuffix(s, "h0m") {
		s = s[:len(s)-2]
	}
	return s
}

func parseDuration(s string) time.Duration {
	didx := strings.IndexByte(s, 'd')
	if didx != -1 {
		days, _ := strconv.ParseInt(s[:didx], 10, 0)
		dur, _ := time.ParseDuration(s[didx:])
		return dur + 24*time.Hour*time.Duration(days)
	}
	dur, _ := time.ParseDuration(s)
	return dur
}

type Time struct {
	StartTime time.Time
	EndTime   time.Time
	Duration  Duration
}

type Honker struct {
	ID     int64
	UserID UserID
	Name   string
	XID    string
	Handle string
	Flavor string
	Combos []string
	Meta   HonkerMeta
}

type HonkerMeta struct {
	Notes string
}

type SomeThing struct {
	What      int
	XID       string
	Owner     string
	Name      string
	AvatarURL string
}

const (
	SomeNothing int = iota
	SomeActor
	SomeCollection
)
