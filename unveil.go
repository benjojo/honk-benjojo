//go:build openbsd
// +build openbsd

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

/*
#include <stdlib.h>
#include <unistd.h>
*/
import "C"

import (
	"fmt"
	"unsafe"
)

func Unveil(path string, perms string) error {
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))
	cperms := C.CString(perms)
	defer C.free(unsafe.Pointer(cperms))

	rv, err := C.unveil(cpath, cperms)
	if rv != 0 {
		return fmt.Errorf("unveil(%s, %s) failure (%d)", path, perms, err)
	}
	return nil
}

func Pledge(promises string) error {
	cpromises := C.CString(promises)
	defer C.free(unsafe.Pointer(cpromises))

	rv, err := C.pledge(cpromises, nil)
	if rv != 0 {
		return fmt.Errorf("pledge(%s) failure (%d)", promises, err)
	}
	return nil
}

func init() {
	preservehooks = append(preservehooks, func() {
		Unveil("/etc/ssl", "r")
		if viewDir != dataDir {
			Unveil(viewDir, "r")
		}
		Unveil(dataDir, "rwc")
		C.unveil(nil, nil)
		Pledge("stdio rpath wpath cpath flock dns inet unix")
	})
	backendhooks = append(backendhooks, func() {
		C.unveil(nil, nil)
		Pledge("stdio unix")
	})
}
