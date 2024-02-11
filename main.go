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
	golog "log"
	"log/syslog"
	notrand "math/rand"
	"os"
	"strconv"
	"time"

	"humungus.tedunangst.com/r/webs/log"
)

var softwareVersion = "1.2.1"

func init() {
	notrand.Seed(time.Now().Unix())
}

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
	xid := fmt.Sprintf("https://%s", hostname)
	db.Exec("delete from honkers where xid = ? and flavor = 'dub'", xid)
	db.Exec("delete from doovers where rcpt = ?", xid)
	xid += "/%"
	db.Exec("delete from honkers where xid like ? and flavor = 'dub'", xid)
	db.Exec("delete from doovers where rcpt like ?", xid)
}

func reexecArgs(cmd string) []string {
	args := []string{"-datadir", dataDir}
	args = append(args, log.Args()...)
	args = append(args, cmd)
	return args
}

var elog, ilog, dlog *golog.Logger

func errx(msg string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, msg+"\n", args...)
	os.Exit(1)
}

func main() {
	flag.StringVar(&dataDir, "datadir", dataDir, "data directory")
	flag.StringVar(&viewDir, "viewdir", viewDir, "view directory")
	flag.Parse()

	log.Init(log.Options{Progname: "honk", Facility: syslog.LOG_UUCP})
	elog = log.E
	ilog = log.I
	dlog = log.D

	if os.Geteuid() == 0 {
		elog.Fatalf("do not run honk as root")
	}

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
	getConfigValue("dbversion", &dbversion)
	if dbversion != myVersion {
		elog.Fatal("incorrect database version. run upgrade.")
	}
	getConfigValue("servermsg", &serverMsg)
	getConfigValue("aboutmsg", &aboutMsg)
	getConfigValue("loginmsg", &loginMsg)
	getConfigValue("servername", &serverName)
	getConfigValue("masqname", &masqName)
	if masqName == "" {
		masqName = serverName
	}
	serverPrefix = fmt.Sprintf("https://%s/", serverName)
	getConfigValue("usersep", &userSep)
	getConfigValue("honksep", &honkSep)
	getConfigValue("devel", &develMode)
	if develMode {
		disableTLSValidation()
	}
	getConfigValue("fasttimeout", &fastTimeout)
	getConfigValue("slowtimeout", &slowTimeout)
	getConfigValue("honkwindow", &honkwindow)
	honkwindow *= 24 * time.Hour

	prepareStatements(db)

	switch cmd {
	case "admin":
		adminscreen()
	case "import":
		if len(args) != 4 {
			errx("import username honk|mastodon|twitter srcdir")
		}
		importMain(args[1], args[2], args[3])
	case "export":
		if len(args) != 3 {
			errx("export username destdir")
		}
		export(args[1], args[2])
	case "devel":
		if len(args) != 2 {
			errx("need an argument: devel (on|off)")
		}
		switch args[1] {
		case "on":
			setConfigValue("devel", 1)
		case "off":
			setConfigValue("devel", 0)
		default:
			errx("argument must be on or off")
		}
	case "setconfig":
		if len(args) != 3 {
			errx("need an argument: setconfig key val")
		}
		var val interface{}
		var err error
		if val, err = strconv.Atoi(args[2]); err != nil {
			val = args[2]
		}
		setConfigValue(args[1], val)
	case "adduser":
		adduser()
	case "deluser":
		if len(args) < 2 {
			errx("usage: honk deluser username")
		}
		deluser(args[1])
	case "chpass":
		if len(args) < 2 {
			errx("usage: honk chpass username")
		}
		chpass(args[1])
	case "follow":
		if len(args) < 3 {
			errx("usage: honk follow username url")
		}
		user, err := getUserBio(args[1])
		if err != nil {
			errx("user %s not found", args[1])
		}
		var meta HonkerMeta
		mj, _ := encodeJson(&meta)
		honkerid, err := savehonker(user, args[2], "", "presub", "", mj)
		if err != nil {
			errx("had some trouble with that: %s", err)
		}
		followyou(user, honkerid, true)
	case "unfollow":
		if len(args) < 3 {
			errx("usage: honk unfollow username url")
		}
		user, err := getUserBio(args[1])
		if err != nil {
			errx("user not found")
		}
		row := db.QueryRow("select honkerid from honkers where xid = ? and userid = ? and flavor in ('sub')", args[2], user.ID)
		var honkerid int64
		err = row.Scan(&honkerid)
		if err != nil {
			errx("sorry couldn't find them")
		}
		unfollowyou(user, honkerid, true)
	case "sendmsg":
		if len(args) < 4 {
			errx("usage: honk send username filename rcpt")
		}
		user, err := getUserBio(args[1])
		if err != nil {
			errx("user %s not found", args[1])
		}
		data, err := os.ReadFile(args[2])
		if err != nil {
			errx("can't read file: %s", err)
		}
		deliverate(user.ID, args[3], data)
	case "cleanup":
		arg := "30"
		if len(args) > 1 {
			arg = args[1]
		}
		cleanupdb(arg)
	case "unplug":
		if len(args) < 2 {
			errx("usage: honk unplug servername")
		}
		name := args[1]
		unplugserver(name)
	case "backup":
		if len(args) < 2 {
			errx("usage: honk backup dirname")
		}
		name := args[1]
		backupDatabase(name)
	case "ping":
		if len(args) < 3 {
			errx("usage: honk ping (from username) (to username or url)")
		}
		name := args[1]
		targ := args[2]
		user, err := getUserBio(name)
		if err != nil {
			errx("unknown user %s", name)
		}
		ping(user, targ)
	case "run":
		serve()
	case "backend":
		backendServer()
	case "test":
		ElaborateUnitTests()
	default:
		errx("unknown command")
	}
}
