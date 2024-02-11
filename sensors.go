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
	"runtime/debug"
	"syscall"
	"time"
)

func init() {
	if softwareVersion != "develop" {
		return
	}
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}
	var vcs, rev, mod string
	for _, bs := range bi.Settings {
		if bs.Key == "vcs" {
			vcs = "/" + bs.Value
		}
		if bs.Key == "vcs.revision" {
			rev = bs.Value
			if len(rev) > 12 {
				rev = rev[:12]
			}
			rev = "-" + rev
		}
		if bs.Key == "vcs.modified" && bs.Value == "true" {
			mod = "+"
		}
	}
	softwareVersion += vcs + rev + mod
}

type Sensors struct {
	Memory float64
	Uptime float64
	CPU    float64
}

var boottime = time.Now()

func getSensors() Sensors {
	var usage syscall.Rusage
	syscall.Getrusage(syscall.RUSAGE_SELF, &usage)

	now := time.Now()

	var sensors Sensors
	sensors.Memory = float64(usage.Maxrss) / 1024.0
	sensors.Uptime = now.Sub(boottime).Seconds()
	sensors.CPU = time.Duration(usage.Utime.Nano()).Seconds()

	return sensors
}

func setLimits() error {
	var limit syscall.Rlimit
	limit.Cur = 2 * 1024 * 1024 * 1024
	limit.Max = 2 * 1024 * 1024 * 1024
	return syscall.Setrlimit(syscall.RLIMIT_DATA, &limit)
}
