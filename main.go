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
	"errors"
	"flag"
	"fmt"
	"html/template"
	"io/fs"
	golog "log"
	"log/syslog"
	notrand "math/rand"
	"os"
	"runtime/pprof"
	"sort"
	"strings"
	"time"

	"humungus.tedunangst.com/r/webs/log"
)

var softwareVersion = "1.4.1"

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
var collectForwards = true

func serverURL(u string, args ...interface{}) string {
	return fmt.Sprintf("https://"+serverName+u, args...)
}

func ElaborateUnitTests() {
	user, _ := getUserBio("test")
	syndicate(user, "https://mastodon.social/tags/mastoadmin.rss")
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

var cpuprofile = flag.String("cpuprofile", "", "write cpu profile to file")
var memprofile = flag.String("memprofile", "", "write memory profile to this file")
var memprofilefd *os.File

func usage() {
	flag.PrintDefaults()
	out := flag.CommandLine.Output()
	fmt.Fprintf(out, "\n  available honk commands:\n")
	var msgs []string
	for n, c := range commands {
		msgs = append(msgs, fmt.Sprintf("    %s: %s\n", n, c.help))
	}
	sort.Strings(msgs)
	fmt.Fprintf(out, "%s", strings.Join(msgs, ""))
}

func main() {
	commands["help"] = cmd{
		help: "you're looking at it",
		callback: func(args []string) {
			usage()
		},
	}
	flag.StringVar(&dataDir, "datadir", getenv("HONK_DATADIR", dataDir), "data directory")
	flag.StringVar(&viewDir, "viewdir", getenv("HONK_VIEWDIR", viewDir), "view directory")
	flag.Usage = usage

	flag.Parse()
	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			errx("can't open cpu profile: %s", err)
		}
		pprof.StartCPUProfile(f)
	}
	if *memprofile != "" {
		f, err := os.Create(*memprofile)
		if err != nil {
			errx("can't open mem profile: %s", err)
		}
		memprofilefd = f
	}

	log.Init(log.Options{Progname: "honk", Facility: syslog.LOG_UUCP})
	elog = log.E
	ilog = log.I
	dlog = log.D

	if os.Geteuid() == 0 {
		elog.Fatalf("do not run honk as root")
	}
	err := os.Mkdir(dataDir+"/attachments", 0700)
	if err != nil && !errors.Is(err, fs.ErrExist) {
		errx("can't create attachments directory: %s", err)
	}

	args := flag.Args()
	cmd := "run"
	if len(args) > 0 {
		cmd = args[0]
	}
	switch cmd {
	case "init":
		commands["init"].callback(args)
	case "upgrade":
		commands["upgrade"].callback(args)
	case "version":
		commands["version"].callback(args)
	}
	db := opendatabase()
	dbversion := 0
	getConfigValue("dbversion", &dbversion)
	if dbversion != myVersion {
		elog.Fatal("incorrect database version. run upgrade.")
	}
	getConfigValue("usefilestore", &storeTheFilesInTheFileSystem)
	getConfigValue("servermsg", &serverMsg)
	getConfigValue("aboutmsg", &aboutMsg)
	getConfigValue("loginmsg", &loginMsg)
	getConfigValue("servername", &serverName)
	getConfigValue("masqname", &masqName)
	if masqName == "" {
		masqName = serverName
	}
	serverPrefix = serverURL("/")
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
	getConfigValue("collectforwards", &collectForwards)

	prepareStatements(db)

	c, ok := commands[cmd]
	if !ok {
		errx("don't know about %q", cmd)
	}
	if c.nargs > 0 && len(args) != c.nargs {
		errx("incorrect arg count: %s", c.help2)
	}

	c.callback(args)
}
