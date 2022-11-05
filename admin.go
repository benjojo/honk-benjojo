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
#include <termios.h>
*/
import "C"
import (
	"bufio"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
)

func adminscreen() {
	log.SetOutput(ioutil.Discard)
	stdout := bufio.NewWriter(os.Stdout)
	esc := "\x1b"
	smcup := esc + "[?1049h"
	rmcup := esc + "[?1049l"

	messages := []*struct {
		name  string
		label string
		text  string
	}{
		{
			name:  "servermsg",
			label: "server",
			text:  string(serverMsg),
		},
		{
			name:  "aboutmsg",
			label: "about",
			text:  string(aboutMsg),
		},
		{
			name:  "loginmsg",
			label: "login",
			text:  string(loginMsg),
		},
	}
	cursel := 0

	hidecursor := func() {
		stdout.WriteString(esc + "[?25l")
	}
	showcursor := func() {
		stdout.WriteString(esc + "[?12;25h")
	}
	movecursor := func(x, y int) {
		stdout.WriteString(fmt.Sprintf(esc+"[%d;%dH", y, x))
	}
	moveleft := func() {
		stdout.WriteString(esc + "[1D")
	}
	clearscreen := func() {
		stdout.WriteString(esc + "[2J")
	}
	clearline := func() {
		stdout.WriteString(esc + "[2K")
	}
	colorfn := func(code int) func(string) string {
		return func(s string) string {
			return fmt.Sprintf(esc+"[%dm"+"%s"+esc+"[0m", code, s)
		}
	}
	reverse := colorfn(7)
	magenta := colorfn(35)
	readchar := func() byte {
		var buf [1]byte
		os.Stdin.Read(buf[:])
		c := buf[0]
		return c
	}

	savedtio := new(C.struct_termios)
	C.tcgetattr(1, savedtio)
	restore := func() {
		stdout.WriteString(rmcup)
		showcursor()
		stdout.Flush()
		C.tcsetattr(1, C.TCSAFLUSH, savedtio)
	}
	defer restore()
	go func() {
		sig := make(chan os.Signal)
		signal.Notify(sig, os.Interrupt)
		<-sig
		restore()
		os.Exit(0)
	}()

	init := func() {
		tio := new(C.struct_termios)
		C.tcgetattr(1, tio)
		tio.c_lflag = tio.c_lflag & ^C.uint(C.ECHO|C.ICANON)
		C.tcsetattr(1, C.TCSADRAIN, tio)

		hidecursor()
		stdout.WriteString(smcup)
		clearscreen()
		movecursor(1, 1)
		stdout.Flush()
	}

	msglineno := func(idx int) int {
		return 4 + idx*2
	}

	drawmessage := func(idx int) {
		line := msglineno(idx)
		movecursor(4, line)
		label := messages[idx].label
		if idx == cursel {
			label = reverse(label)
		}
		stdout.WriteString(fmt.Sprintf("%s %s", label, messages[idx].text))
	}

	drawscreen := func() {
		clearscreen()
		movecursor(4, msglineno(-1))
		stdout.WriteString(magenta(serverName + " admin panel"))
		for i := range messages {
			drawmessage(i)
		}
		movecursor(4, msglineno(len(messages)))
		stdout.WriteString(magenta("j/k to move - q to quit - enter to edit"))
		stdout.Flush()
	}

	selectnext := func() {
		if cursel < len(messages)-1 {
			cursel++
		}
		drawscreen()
	}
	selectprev := func() {
		if cursel > 0 {
			cursel--
		}
		drawscreen()
	}
	editsel := func() {
		movecursor(4, msglineno(cursel))
		clearline()
		m := messages[cursel]
		stdout.WriteString(reverse(magenta(m.label)))
		text := m.text
		stdout.WriteString(" ")
		stdout.WriteString(text)
		showcursor()
		stdout.Flush()
	loop:
		for {
			c := readchar()
			switch c {
			case '\n':
				break loop
			case 127:
				if len(text) > 0 {
					moveleft()
					stdout.WriteString(" ")
					moveleft()
					text = text[:len(text)-1]
				}
			default:
				text = text + string(c)
				stdout.WriteString(string(c))
			}
			stdout.Flush()
		}
		m.text = text
		updateconfig(m.name, m.text)
		hidecursor()
		drawscreen()
	}

	init()
	drawscreen()

	for {
		c := readchar()
		switch c {
		case 'q':
			return
		case 'j':
			selectnext()
		case 'k':
			selectprev()
		case '\n':
			editsel()
		default:

		}
	}
}
