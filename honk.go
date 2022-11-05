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
	"flag"
	"fmt"
	"html/template"
	"log"
	notrand "math/rand"
	"os"
	"strconv"
	"strings"
	"time"

	"humungus.tedunangst.com/r/webs/httpsig"
)

var softwareVersion = "0.9.5"

func init() {
	notrand.Seed(time.Now().Unix())
}

type WhatAbout struct {
	ID      int64
	Name    string
	Display string
	About   string
	HTAbout template.HTML
	Onts    []string
	Key     string
	URL     string
	Options UserOptions
	SecKey  httpsig.PrivateKey
}

type UserOptions struct {
	SkinnyCSS  bool   `json:",omitempty"`
	OmitImages bool   `json:",omitempty"`
	Avatar     string `json:",omitempty"`
	MapLink    string `json:",omitempty"`
	Reaction   string `json:",omitempty"`
	MentionAll bool
}

type KeyInfo struct {
	keyname string
	seckey  httpsig.PrivateKey
}

const serverUID int64 = -2

type Honk struct {
	ID       int64
	UserID   int64
	Username string
	What     string
	Honker   string
	Handle   string
	Handles  string
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
	Mentions []Mention
}

type Chonk struct {
	ID     int64
	UserID int64
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

func (honk *Honk) IsReacted() bool {
	return honk.Flags&flagIsReacted != 0
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
	Meta   HonkerMeta
}

type HonkerMeta struct {
	Notes string
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
var serverPrefix string
var masqName string
var dataDir = "."
var viewDir = "."
var iconName = "icon.png"
var serverMsg template.HTML
var aboutMsg template.HTML
var loginMsg template.HTML

func ElaborateUnitTests() {
}

func unplugserver(hostname string) {
	db := opendatabase()
	xid := fmt.Sprintf("%%https://%s/%%", hostname)
	db.Exec("delete from honkers where xid like ? and flavor = 'dub'", xid)
	db.Exec("delete from doovers where rcpt like ?", xid)
}

func main() {
	flag.StringVar(&dataDir, "datadir", dataDir, "data directory")
	flag.StringVar(&viewDir, "viewdir", viewDir, "view directory")
	flag.Parse()
	args := flag.Args()
	cmd := "run"
	if len(args) > 0 {
		cmd = args[0]
	}
	switch cmd {
	case "init":
		initdb()
	case "upgrade":
		upgradedb()
	case "version":
		fmt.Println(softwareVersion)
		os.Exit(0)
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
	getconfig("masqname", &masqName)
	if masqName == "" {
		masqName = serverName
	}
	serverPrefix = fmt.Sprintf("https://%s/", serverName)
	getconfig("usersep", &userSep)
	getconfig("honksep", &honkSep)
	getconfig("debug", &debugMode)
	prepareStatements(db)
	switch cmd {
	case "admin":
		adminscreen()
	case "import":
		if len(args) != 4 {
			log.Fatal("import username mastodon|twitter srcdir")
		}
		importMain(args[1], args[2], args[3])
	case "debug":
		if len(args) != 2 {
			log.Fatal("need an argument: debug (on|off)")
		}
		switch args[1] {
		case "on":
			setconfig("debug", 1)
		case "off":
			setconfig("debug", 0)
		default:
			log.Fatal("argument must be on or off")
		}
	case "adduser":
		adduser()
	case "deluser":
		if len(args) < 2 {
			fmt.Printf("usage: honk deluser username\n")
			return
		}
		deluser(args[1])
	case "chpass":
		chpass()
	case "cleanup":
		arg := "30"
		if len(args) > 1 {
			arg = args[1]
		}
		cleanupdb(arg)
	case "unplug":
		if len(args) < 2 {
			fmt.Printf("usage: honk unplug servername\n")
			return
		}
		name := args[1]
		unplugserver(name)
	case "backup":
		if len(args) < 2 {
			fmt.Printf("usage: honk backup dirname\n")
			return
		}
		name := args[1]
		svalbard(name)
	case "ping":
		if len(args) < 3 {
			fmt.Printf("usage: honk ping (from username) (to username or url)\n")
			return
		}
		name := args[1]
		targ := args[2]
		user, err := butwhatabout(name)
		if err != nil {
			log.Printf("unknown user")
			return
		}
		ping(user, targ)
	case "run":
		serve()
	case "backend":
		backendServer()
	case "test":
		ElaborateUnitTests()
	default:
		log.Fatal("unknown command")
	}
}
