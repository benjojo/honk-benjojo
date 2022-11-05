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

package main

import (
	"flag"
	"io/ioutil"
	"log"
	"log/syslog"
	"os"
)

// log.Default() not added until go 1.16
func logdefault() *log.Logger {
	return log.New(os.Stderr, "", log.LstdFlags)
}

var elog = logdefault()
var ilog = logdefault()
var dlog = logdefault()

var elogname, ilogname, dlogname, alllogname string

func init() {
	flag.StringVar(&elogname, "errorlog", "stderr", "error log file (or stderr, null, syslog)")
	flag.StringVar(&ilogname, "infolog", "stderr", "info log file (or stderr, null, syslog)")
	flag.StringVar(&dlogname, "debuglog", "stderr", "debug log file (or stderr, null, syslog)")
	flag.StringVar(&alllogname, "log", "stderr", "combined log file (or stderr, null, syslog)")

}

func loggingArgs() []string {
	return []string{"-errorlog", elogname, "-infolog", ilogname, "-debuglog", dlogname}
}

func initLogging(elogname, ilogname, dlogname string) {
	elog = openlog(elogname, syslog.LOG_ERR)
	ilog = openlog(ilogname, syslog.LOG_INFO)
	dlog = openlog(dlogname, syslog.LOG_DEBUG)
}

func openlog(name string, prio syslog.Priority) *log.Logger {
	if name == "stderr" {
		return log.New(os.Stderr, "", log.LstdFlags)
	}
	if name == "stdout" {
		return log.New(os.Stdout, "", log.LstdFlags)
	}
	if name == "null" {
		return log.New(ioutil.Discard, "", log.LstdFlags)
	}
	if name == "syslog" {
		w, err := syslog.New(syslog.LOG_UUCP|prio, "honk")
		if err != nil {
			elog.Printf("can't create syslog: %s", err)
			return logdefault()
		}
		return log.New(w, "", log.LstdFlags)
	}
	fd, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		elog.Printf("can't open log file %s: %s", name, err)
		return logdefault()
	}
	logger := log.New(fd, "", log.LstdFlags)
	logger.Printf("new log started")
	return logger
}
