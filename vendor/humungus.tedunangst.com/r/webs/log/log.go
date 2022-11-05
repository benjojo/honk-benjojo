//
// Copyright (c) 2022 Ted Unangst <tedu@tedunangst.com>
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

package log

import (
	"flag"
	"io"
	"io/ioutil"
	"log"
	"log/syslog"
	"os"
)

func logdefault() *log.Logger {
	return log.New(unctrl(os.Stderr), "", log.LstdFlags)
}

var E = logdefault()
var I = logdefault()
var D = logdefault()

var elogname, ilogname, dlogname, alllogname string

func init() {
	flag.StringVar(&elogname, "errorlog", "", "error log file (or stderr, null, syslog)")
	flag.StringVar(&ilogname, "infolog", "", "info log file (or stderr, null, syslog)")
	flag.StringVar(&dlogname, "debuglog", "", "debug log file (or stderr, null, syslog)")
	flag.StringVar(&alllogname, "log", "", "combined log file (or stderr, null, syslog)")
}

func Args() []string {
	var args []string
	if elogname != alllogname {
		args = append(args, []string{"-errorlog", elogname}...)
	}
	if ilogname != alllogname {
		args = append(args, []string{"-infolog", ilogname}...)
	}
	if dlogname != alllogname {
		args = append(args, []string{"-debuglog", dlogname}...)
	}
	if alllogname != "" {
		args = append(args, []string{"-log", alllogname}...)
	}
	return args
}

type Options struct {
	Progname   string
	Facility   syslog.Priority
	Alllogname string
	Elogname   string
	Ilogname   string
	Dlogname   string
	NoFilter   bool
}

func Init(options Options) {
	facility := options.Facility
	progname := options.Progname
	filter := !options.NoFilter
	if facility == 0 {
		facility = syslog.LOG_LOCAL0
	}
	if options.Alllogname != "" {
		alllogname = options.Alllogname
	}
	if options.Elogname != "" {
		elogname = options.Elogname
	}
	if options.Ilogname != "" {
		ilogname = options.Ilogname
	}
	if options.Dlogname != "" {
		dlogname = options.Dlogname
	}
	if alllogname != "" {
		if elogname == "" {
			elogname = alllogname
		}
		if ilogname == "" {
			ilogname = alllogname
		}
		if dlogname == "" {
			dlogname = alllogname
		}
	}
	if elogname != "" {
		E = openlog(progname, elogname, facility|syslog.LOG_ERR, filter)
	}
	if ilogname != "" {
		I = openlog(progname, ilogname, facility|syslog.LOG_INFO, filter)
	}
	if dlogname != "" {
		D = openlog(progname, dlogname, facility|syslog.LOG_DEBUG, filter)
	}
}

func openlog(progname, name string, prio syslog.Priority, filter bool) *log.Logger {
	var w io.Writer
	if name == "stderr" {
		w = os.Stderr
	} else if name == "stdout" {
		w = os.Stdout
	} else if name == "null" {
		w = ioutil.Discard
	} else if name == "syslog" {
		fd, err := syslog.New(prio, progname)
		if err != nil {
			E.Printf("can't create syslog: %s", err)
			w = os.Stderr
		} else {
			w = fd
		}
	} else {
		fd, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
		if err != nil {
			E.Printf("can't open log file %s: %s", name, err)
			w = os.Stderr
		} else {
			w = fd
		}
	}
	if filter {
		w = unctrl(w)
	}
	logger := log.New(w, "", log.LstdFlags)
	return logger
}
