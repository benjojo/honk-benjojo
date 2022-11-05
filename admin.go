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
void
clearecho(struct termios *tio)
{
	tio->c_lflag = tio->c_lflag & ~(ECHO|ICANON);
}
*/
import "C"
import (
	"bufio"
	"fmt"
	"os"
	"os/signal"
	"strings"

	"humungus.tedunangst.com/r/webs/log"
)

func adminscreen() {
	log.Init(log.Options{Progname: "honk", Alllogname: "null"})
	stdout := bufio.NewWriter(os.Stdout)
	esc := "\x1b"
	smcup := esc + "[?1049h"
	rmcup := esc + "[?1049l"

	var avatarColors string
	getconfig("avatarcolors", &avatarColors)
	loadLingo()

	type adminfield struct {
		name    string
		label   string
		text    string
		oneline bool
	}

	messages := []*adminfield{
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
		{
			name:    "avatarcolors",
			label:   "avatar colors (4 RGBA hex numbers)",
			text:    string(avatarColors),
			oneline: true,
		},
	}
	for _, l := range []string{"honked", "bonked", "honked back", "qonked", "evented"} {
		messages = append(messages, &adminfield{
			name:    "lingo-" + strings.ReplaceAll(l, " ", ""),
			label:   "lingo for " + l,
			text:    relingo[l],
			oneline: true,
		})
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
	//clearline := func() { stdout.WriteString(esc + "[2K") }
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
		C.clearecho(tio)
		C.tcsetattr(1, C.TCSADRAIN, tio)

		hidecursor()
		stdout.WriteString(smcup)
		clearscreen()
		movecursor(1, 1)
		stdout.Flush()
	}

	editing := false

	linecount := func(s string) int {
		lines := 1
		for i := range s {
			if s[i] == '\n' {
				lines++
			}
		}
		return lines
	}

	msglineno := func(idx int) int {
		off := 1
		if idx == -1 {
			return off
		}
		for i, m := range messages {
			off += 1
			if i == idx {
				return off
			}
			if !m.oneline {
				off += 1
				off += linecount(m.text)
			}
		}
		off += 2
		return off
	}

	forscreen := func(s string) string {
		return strings.Replace(s, "\n", "\n   ", -1)
	}

	drawmessage := func(idx int) {
		line := msglineno(idx)
		movecursor(4, line)
		label := messages[idx].label
		if idx == cursel {
			label = reverse(label)
		}
		label = magenta(label)
		text := forscreen(messages[idx].text)
		if messages[idx].oneline {
			stdout.WriteString(fmt.Sprintf("%s\t   %s", label, text))
		} else {
			stdout.WriteString(fmt.Sprintf("%s\n   %s", label, text))
		}
	}

	drawscreen := func() {
		clearscreen()
		movecursor(4, msglineno(-1))
		stdout.WriteString(magenta(serverName + " admin panel"))
		for i := range messages {
			if !editing || i != cursel {
				drawmessage(i)
			}
		}
		movecursor(4, msglineno(len(messages)))
		dir := "j/k to move - q to quit - enter to edit"
		if editing {
			dir = "esc to end"
		}
		stdout.WriteString(magenta(dir))
		if editing {
			drawmessage(cursel)
		}
		stdout.Flush()
	}

	selectnext := func() {
		if cursel < len(messages)-1 {
			movecursor(4, msglineno(cursel))
			stdout.WriteString(magenta(messages[cursel].label))
			cursel++
			movecursor(4, msglineno(cursel))
			stdout.WriteString(reverse(magenta(messages[cursel].label)))
			stdout.Flush()
		}
	}
	selectprev := func() {
		if cursel > 0 {
			movecursor(4, msglineno(cursel))
			stdout.WriteString(magenta(messages[cursel].label))
			cursel--
			movecursor(4, msglineno(cursel))
			stdout.WriteString(reverse(magenta(messages[cursel].label)))
			stdout.Flush()
		}
	}
	editsel := func() {
		editing = true
		showcursor()
		drawscreen()
		m := messages[cursel]
	loop:
		for {
			c := readchar()
			switch c {
			case '\x1b':
				break loop
			case '\n':
				if m.oneline {
					break loop
				}
				m.text += "\n"
				drawscreen()
			case 127:
				if len(m.text) > 0 {
					last := m.text[len(m.text)-1]
					m.text = m.text[:len(m.text)-1]
					if last == '\n' {
						drawscreen()
					} else {
						moveleft()
						stdout.WriteString(" ")
						moveleft()
					}
				}
			default:
				m.text += string(c)
				stdout.WriteString(string(c))
			}
			stdout.Flush()
		}
		editing = false
		setconfig(m.name, m.text)
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
