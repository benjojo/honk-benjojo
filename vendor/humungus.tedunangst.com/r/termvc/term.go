//
// Copyright (c) 2024 Ted Unangst <tedu@tedunangst.com>
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

package termvc

/*
#include <sys/ioctl.h>
#include <termios.h>
#include <unistd.h>
void
clearecho(struct termios *tio)
{
	tio->c_lflag = tio->c_lflag & ~(ECHO|ICANON);
}
void
setecho(struct termios *tio)
{
	tio->c_lflag = tio->c_lflag | (ECHO|ICANON);
}
int
getwinsize(int fd, int *col, int *row)
{
    struct winsize ws;
    ioctl(fd, TIOCGWINSZ, &ws);
    *col = ws.ws_col;
 	*row = ws.ws_row;
    return 0;
}
*/
import "C"
import (
	"bufio"
	"fmt"
	"os"
	"os/signal"
	"strings"
)

const esc = "\x1b"

var stdout = bufio.NewWriter(os.Stdout)

func hidecursor() {
	stdout.WriteString(esc + "[?25l")
}
func showcursor() {
	stdout.WriteString(esc + "[?12;25h")
}
func moveto(x, y int) {
	stdout.WriteString(fmt.Sprintf(esc+"[%d;%dH", y, x))
}
func moveleft() {
	stdout.WriteString(esc + "[1D")
}
func clearscreen() {
	stdout.WriteString(esc + "[2J")
}
func clearline() {
	stdout.WriteString(esc + "[2K")
}
func colorfn(code byte) string {
	return fmt.Sprintf(esc+"[%dm", code)
}

var inverse byte = 7
var uninverse byte = 27
var magenta byte = 35
var white byte = 37

var logdata []string

func log(msg string, args ...interface{}) {
	logdata = append(logdata, fmt.Sprintf(msg, args...))
}

func Restore() {
	if savedTio == nil {
		return
	}
	stdout.WriteString(rmcup)
	showcursor()
	stdout.Flush()
	C.tcsetattr(1, C.TCSAFLUSH, savedTio)
	savedTio = nil
	if len(logdata) > 0 {
		fmt.Printf("%s\n", strings.Join(logdata, "\n"))
	}
}

func Catch(fn func()) {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	<-sig
	Restore()
	if fn != nil {
		fn()
	} else {
		os.Exit(1)
	}
}

var smcup = esc + "[?1049h"
var rmcup = esc + "[?1049l"
var savedTio *C.struct_termios

func Start() {
	tio := new(C.struct_termios)
	C.tcgetattr(1, tio)
	C.clearecho(tio)
	C.tcsetattr(1, C.TCSADRAIN, tio)

	hidecursor()
	stdout.WriteString(smcup)
	clearscreen()
	moveto(1, 1)
	stdout.Flush()
	C.setecho(tio)
	savedTio = tio
}

func getwinsize() (int, int) {
	var cols, rows C.int
	C.getwinsize(1, &cols, &rows)
	return int(cols), int(rows)
}

type Event interface {
	IsEvent()
}

type Letter string

func (r Letter) IsEvent() {}

type Key string

func (k Key) IsEvent() {}

func readkey() Event {
	var buf [16]byte
	for {
		n, _ := os.Stdin.Read(buf[:])
		s := string(buf[:n])
		switch s {
		case "\x1b":
			return Key("escape")
		case "\x7f":
			return Key("bs")
		case "\t":
			return Key("tab")
		case "\r":
			return Key("enter")
		case "\n":
			return Key("enter")
		case "\x1b[A":
			return Key("up")
		case "\x1b[B":
			return Key("down")
		case "\x1b[C":
			return Key("right")
		case "\x1b[D":
			return Key("left")
		case "\x1b[Z":
			return Key("reverse-tab")
		}
		return Letter(s)
	}
}

type App struct {
	Element Element
	Screen  *Screen
	InputFn func(Event) Event
	quit    bool
}

func NewApp() *App {
	app := new(App)
	return app
}

func (app *App) Quit() {
	app.quit = true
}

func (app *App) Loop() {
	scr := app.Screen
	for !app.quit {
		hidecursor()
		clearscreen()
		moveto(1, 1)
		app.Element.Draw(scr)
		scr.Render()
		evt := readkey()
		if app.InputFn != nil {
			evt = app.InputFn(evt)
		} else if key, ok := evt.(Key); ok && key == "escape" {
			app.quit = true
			evt = nil
		}
		if evt == nil {
			continue
		}
		app.Element.Input(evt)
	}
}

func IsTerm() bool {
	return C.isatty(0) == 1 && C.isatty(1) == 1
}
