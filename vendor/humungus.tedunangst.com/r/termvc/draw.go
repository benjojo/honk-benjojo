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

import ()

type Drawable interface {
	Focused() bool
	SetFocus(bool)
	WriteString(string)
	MoveTo(int, int)
	Color(byte) byte
	Inverse()
	Width() int
	Height() int
	SetCursor(int, int)
}

type Region struct {
	under   Drawable
	yoffset int
	height  int
	focus   bool
}

func (reg *Region) Focused() bool {
	return reg.focus
}
func (reg *Region) SetFocus(f bool) {
	reg.focus = f
}
func (reg *Region) WriteString(s string) {
	reg.under.WriteString(s)
}
func (reg *Region) MoveTo(x, y int) {
	if y != -1 {
		y += reg.yoffset
	}
	reg.under.MoveTo(x, y)
}
func (reg *Region) SetCursor(x, y int) {
	if y != -1 {
		y += reg.yoffset
	}
	reg.under.SetCursor(x, y)
}
func (reg *Region) Color(c byte) byte {
	return reg.under.Color(c)
}
func (reg *Region) Inverse() {
	reg.under.Inverse()
}
func (reg *Region) Width() int {
	return reg.under.Width()
}
func (reg *Region) Height() int {
	return reg.height
}

type cell struct {
	r     rune
	color byte
	attr  byte
}

type Screen struct {
	width, height int
	pos           int
	cursorpos     int
	cells         []cell
	focus         bool
	color         byte
	attr          byte
	defaultcolor  byte
}

func NewScreen() *Screen {
	scr := new(Screen)
	scr.width, scr.height = getwinsize()
	scr.cells = make([]cell, scr.height*scr.width)
	scr.focus = true
	return scr
}

func (scr *Screen) WriteString(s string) {
	pos, cells, color, attr := scr.pos, scr.cells, scr.color, scr.attr
	for _, r := range s {
		if r == '\n' {
			pos += scr.width
			pos -= pos % scr.width
			continue
		}
		if pos >= len(cells) {
			break
		}
		cells[pos] = cell{r: r, color: color, attr: attr}
		pos++
	}
	scr.pos, scr.cells = pos, cells
}
func (scr *Screen) MoveTo(x, y int) {
	scr.pos = (y-1)*scr.width + (x - 1)
}
func (scr *Screen) SetCursor(x, y int) {
	if x == -1 && y == -1 {
		scr.cursorpos = scr.pos
	} else if y == -1 {
		scr.cursorpos = scr.pos - scr.pos%scr.width + (x - 1)
	} else {
		scr.cursorpos = (y-1)*scr.width + (x - 1)
	}
}
func (scr *Screen) Focused() bool {
	return scr.focus
}
func (scr *Screen) SetFocus(f bool) {
	scr.focus = f
}
func (scr *Screen) Color(c byte) byte {
	prev := scr.color
	scr.color = c
	return prev
}
func (scr *Screen) Inverse() {
	if scr.attr == inverse {
		scr.attr = uninverse
	} else {
		scr.attr = inverse
	}
}
func (scr *Screen) Width() int {
	return scr.width
}
func (scr *Screen) Height() int {
	return scr.height
}
func (scr *Screen) DefaultColor(c byte) {
	scr.defaultcolor = c
	scr.color = c
}
func (scr *Screen) Render() {
	lastcolor := scr.defaultcolor
	var lastat byte
	stdout.WriteString(colorfn(0))
	stdout.WriteString(colorfn(lastcolor))
	for j := 0; j < scr.height; j++ {
		needmove := true
		for i := 0; i < scr.width; i++ {
			pos := j*scr.width + i
			cell := scr.cells[pos]
			if cell.r == 0 {
				needmove = true
				continue
			}
			if needmove {
				needmove = false
				moveto(i+1, j+1)
			}
			if c := cell.color; c != lastcolor {
				stdout.WriteString(colorfn(c))
				lastcolor = c
			}
			if at := cell.attr; at != lastat {
				stdout.WriteString(colorfn(at))
				lastat = at
			}
			stdout.WriteRune(cell.r)
		}
	}
	if cp := scr.cursorpos; cp > 0 {
		moveto(1+cp%scr.width, 1+cp/scr.width)
		showcursor()
		cell := scr.cells[cp]
		if cell.r == 0 {
			stdout.WriteRune(' ')
			moveleft()
		}
	}

	stdout.Flush()
	for i := range scr.cells {
		scr.cells[i] = cell{}
	}
	scr.pos = 0
	scr.cursorpos = 0
	scr.color = scr.defaultcolor
	scr.attr = 0
}
