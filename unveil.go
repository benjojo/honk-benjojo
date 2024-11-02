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
	"humungus.tedunangst.com/r/gonix"
)

func securitizeweb() {
	err := gonix.Unveil("/etc/ssl", "r")
	if err != nil {
		elog.Fatalf("unveil(%s, %s) failure (%d)", "/etc/ssl", "r", err)
	}
	if viewDir != dataDir {
		err = gonix.Unveil(viewDir, "r")
		if err != nil {
			elog.Fatalf("unveil(%s, %s) failure (%d)", viewDir, "r", err)
		}
	}
	err = gonix.Unveil(dataDir, "rwc")
	if err != nil {
		elog.Fatalf("unveil(%s, %s) failure (%d)", dataDir, "rwc", err)
	}
	gonix.UnveilEnd()
	promises := "stdio rpath wpath cpath flock dns inet unix"
	err = gonix.Pledge(promises)
	if err != nil {
		elog.Fatalf("pledge(%s) failure (%d)", promises, err)
	}
}

func securitizebackend() {
	gonix.UnveilEnd()
	promises := "stdio unix"
	err := gonix.Pledge(promises)
	if err != nil {
		elog.Fatalf("pledge(%s) failure (%d)", promises, err)
	}
}
