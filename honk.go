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
	"crypto/rsa"
	"fmt"
	"html/template"
	"log"
	notrand "math/rand"
	"os"
	"strconv"
	"strings"
	"time"
)

func init() {
	notrand.Seed(time.Now().Unix())
}

type WhatAbout struct {
	ID      int64
	Name    string
	Display string
	About   string
	Key     string
	URL     string
	Options UserOptions
	SecKey  *rsa.PrivateKey
}

type UserOptions struct {
	SkinnyCSS bool `json:",omitempty"`
}

type KeyInfo struct {
	keyname string
	seckey  *rsa.PrivateKey
}

const serverUID int64 = -2

type Honk struct {
	ID       int64
	UserID   int64
	Username string
	What     string
	Honker   string
	Handle   string
	Oonker   string
	Oondle   string
	XID      string
	RID      string
	Date     time.Time
	URL      string
	Noise    string
	Precis   string
	Format   string
	Convoy   string
	Audience []string
	Public   bool
	Whofore  int64
	Replies  []*Honk
	Flags    int64
	HTPrecis template.HTML
	HTML     template.HTML
	Style    string
	Open     string
	Donks    []*Donk
	Onts     []string
	Place    *Place
	Time     *Time
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
)

func (honk *Honk) IsAcked() bool {
	return honk.Flags&flagIsAcked != 0
}

func (honk *Honk) IsBonked() bool {
	return honk.Flags&flagIsBonked != 0
}

func (honk *Honk) IsSaved() bool {
	return honk.Flags&flagIsSaved != 0
}

func (honk *Honk) IsUntagged() bool {
	return honk.Flags&flagIsUntagged != 0
}

type Donk struct {
	FileID int64
	XID    string
	Name   string
	Desc   string
	URL    string
	Media  string
	Local  bool
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
	UserID int64
	Name   string
	XID    string
	Handle string
	Flavor string
	Combos []string
}

type SomeThing struct {
	What  int
	XID   string
	Owner string
	Name  string
}

const (
	SomeNothing int = iota
	SomeActor
	SomeCollection
)

var serverName string
var iconName = "icon.png"
var serverMsg template.HTML
var aboutMsg template.HTML
var loginMsg template.HTML

func ElaborateUnitTests() {
}

func main() {
	cmd := "run"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	switch cmd {
	case "init":
		initdb()
	case "upgrade":
		upgradedb()
	}
	db := opendatabase()
	dbversion := 0
	getconfig("dbversion", &dbversion)
	if dbversion != myVersion {
		log.Fatal("incorrect database version. run upgrade.")
	}
	getconfig("servermsg", &serverMsg)
	getconfig("aboutmsg", &aboutMsg)
	getconfig("loginmsg", &loginMsg)
	getconfig("servername", &serverName)
	getconfig("usersep", &userSep)
	getconfig("honksep", &honkSep)
	prepareStatements(db)
	switch cmd {
	case "admin":
		adminscreen()
	case "debug":
		if len(os.Args) != 3 {
			log.Fatal("need an argument: debug (on|off)")
		}
		switch os.Args[2] {
		case "on":
			updateconfig("debug", 1)
		case "off":
			updateconfig("debug", 0)
		default:
			log.Fatal("argument must be on or off")
		}
	case "adduser":
		adduser()
	case "chpass":
		chpass()
	case "cleanup":
		arg := "30"
		if len(os.Args) > 2 {
			arg = os.Args[2]
		}
		cleanupdb(arg)
	case "ping":
		if len(os.Args) < 4 {
			fmt.Printf("usage: honk ping from to\n")
			return
		}
		name := os.Args[2]
		targ := os.Args[3]
		user, err := butwhatabout(name)
		if err != nil {
			log.Printf("unknown user")
			return
		}
		ping(user, targ)
	case "run":
		serve()
	case "test":
		ElaborateUnitTests()
	default:
		log.Fatal("unknown command")
	}
}
