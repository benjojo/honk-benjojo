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

import (
	"fmt"
)

type Element interface {
	Draw(Drawable)
	Input(Event) Event
}

type VStack struct {
	children []Element
	heights  []int
	skip     []bool
	fixed    bool
	index    int
	wraps    bool
	form     bool
}

func NewTabGroup(children ...Element) *VStack {
	elem := NewVStack(children...)
	elem.fixed = false
	return elem
}
func NewForm(children ...Element) *VStack {
	elem := NewTabGroup(children...)
	for i := range children {
		elem.SetHeight(i, 1)
	}
	elem.form = true
	return elem
}
func NewVStack(children ...Element) *VStack {
	elem := new(VStack)
	elem.children = children
	elem.heights = make([]int, len(children))
	elem.skip = make([]bool, len(children))
	elem.fixed = true
	return elem
}
func (elem *VStack) SetFocus(index int) {
	elem.index = index
}
func (elem *VStack) SetHeight(index int, height int) {
	elem.heights[index] = height
}
func (elem *VStack) SetSkip(index int, skip bool) {
	elem.skip[index] = true
}

func (elem *VStack) next(evt Event) Event {
	if elem.fixed {
		return evt
	}
	end := len(elem.children)
	idx := elem.index
	for {
		idx++
		if idx == elem.index {
			return evt
		}
		if idx == end {
			if elem.wraps {
				idx = 0
			} else {
				return evt
			}
		}
		if elem.skip[idx] {
			continue
		}
		elem.index = idx
		return nil
	}
}
func (elem *VStack) prev(evt Event) Event {
	if elem.fixed {
		return evt
	}
	last := len(elem.children) - 1
	idx := elem.index
	for {
		idx--
		if idx == elem.index {
			return evt
		}
		if idx == -1 {
			if elem.wraps {
				idx = last
			} else {
				return evt
			}
		}
		if elem.skip[idx] {
			continue
		}
		elem.index = idx
		return nil
	}
}

func (elem *VStack) Input(evt Event) Event {
	evt = elem.children[elem.index].Input(evt)
	if evt == nil {
		return nil
	}
	if key, ok := evt.(Key); ok {
		switch key {
		case "reverse-tab":
			return elem.prev(evt)
		case "tab":
			return elem.next(evt)
		}
		if elem.form {
			switch key {
			case "up":
				elem.prev(evt)
				return nil
			case "down":
				elem.next(evt)
				return nil
			case "enter":
				elem.next(evt)
				return nil
			}
		}
	}
	return evt
}
func (elem *VStack) Draw(draw Drawable) {
	space := draw.Height()
	flex := 0
	for _, h := range elem.heights {
		if h == 0 {
			flex++
		} else {
			space -= h
		}
	}
	var gap int
	if flex > 0 {
		gap = space / flex
	}
	line := 0
	for i, tab := range elem.children {
		h := elem.heights[i]
		if h == 0 {
			h = gap
		}
		a := Region{under: draw, yoffset: line, height: h}
		a.MoveTo(1, 1)
		a.SetFocus(draw.Focused() && i == elem.index)
		tab.Draw(&a)
		line += h
	}
}

type List struct {
	children []Element
	toggled  []bool
	index    int
	SelectFn func(int)
}

func toList(opt any) Element {
	if s, ok := opt.(string); ok {
		return NewStringWrapper(s)
	} else if e, ok := opt.(Element); ok {
		return e
	} else if f, ok := opt.(fmt.Stringer); ok {
		return NewStringField(f)
	} else {
		panic("do not want")
	}
}
func NewList(options ...any) *List {
	children := make([]Element, len(options))
	for i, opt := range options {
		children[i] = toList(opt)
	}
	elem := new(List)
	elem.children = children
	elem.toggled = make([]bool, len(options))
	return elem
}
func (elem *List) Add(index int, options ...any) {
	for _, opt := range options {
		elem.children = append(elem.children, toList(opt))
		elem.toggled = append(elem.toggled, false)
	}
}
func sliceDelete[S ~[]E, E any](s S, i, j int) S {
	_ = s[i:j] // bounds check

	return append(s[:i], s[j:]...)
}
func (elem *List) Delete(index int) {
	elem.children = sliceDelete(elem.children, index, index+1)
	elem.toggled = sliceDelete(elem.toggled, index, index+1)
	if elem.index >= len(elem.children) {
		elem.index--
	}
}
func (elem *List) Clear() {
	elem.children = nil
	elem.toggled = nil
	elem.index = 0
}
func (elem *List) GetIndex() int {
	return elem.index
}
func (elem *List) Input(evt Event) Event {
	if key, ok := evt.(Key); ok {
		switch key {
		case "up":
			if elem.index > 0 {
				elem.index--
			}
			return nil
		case "down":
			if elem.index < len(elem.children)-1 {
				elem.index++
			}
			return nil
		case "enter":
			if elem.SelectFn != nil {
				elem.SelectFn(elem.index)
			}
			return nil
		}
	}
	if let, ok := evt.(Letter); ok {
		switch let {
		case "j":
			if elem.index < len(elem.children)-1 {
				elem.index++
			}
			return nil
		case "k":
			if elem.index > 0 {
				elem.index--
			}
			return nil
		case " ":
			if elem.index < len(elem.toggled) {
				elem.toggled[elem.index] = !elem.toggled[elem.index]
			}
			return nil
		}
	}
	if len(elem.children) == 0 {
		return evt
	}
	return elem.children[elem.index].Input(evt)
}
func (elem *List) Draw(draw Drawable) {
	focus := draw.Focused()
	space := draw.Height()
	var skip int
	if elem.index >= space-1 {
		skip = elem.index - space + 2
	}
	for i, child := range elem.children {
		if skip > 0 {
			skip--
			continue
		}
		a := draw
		if elem.toggled[i] {
			a.Color(magenta)
		}
		a.SetFocus(focus && i == elem.index)
		child.Draw(a)
		if elem.toggled[i] {
			a.Color(white)
		}
		draw.WriteString("\n")
	}
	draw.SetFocus(focus)
}

type String string

func (s *String) String() string {
	return string(*s)
}

type StringField struct {
	stringer fmt.Stringer
}

func NewStringField(s fmt.Stringer) *StringField {
	elem := new(StringField)
	elem.stringer = s
	return elem
}
func NewStringWrapper(s string) *StringField {
	return NewStringField((*String)(&s))
}

func (elem *StringField) Input(evt Event) Event {
	return evt
}
func (elem *StringField) Draw(draw Drawable) {
	if draw.Focused() {
		draw.Inverse()
	}
	draw.WriteString(elem.stringer.String())
	if draw.Focused() {
		draw.Inverse()
	}
}

type TextInput struct {
	Label  string
	Value  string
	offset *int
	echo   bool
	pos    int
}

func NewTextInput(label string, offset *int) *TextInput {
	elem := new(TextInput)
	elem.Label = label
	elem.offset = offset
	if offset != nil && *offset < len(label) {
		*offset = len(label)
	}
	elem.echo = true
	return elem
}
func NewPasswordInput(label string, offset *int) *TextInput {
	elem := NewTextInput(label, offset)
	elem.echo = false
	return elem
}
func (elem *TextInput) Set(s string) {
	elem.Value = s
	elem.pos = len(elem.Value)
}

func (elem *TextInput) Input(evt Event) Event {
	if key, ok := evt.(Key); ok {
		switch key {
		case "bs":
			if elem.pos > 0 {
				val := elem.Value
				elem.Value = val[0 : elem.pos-1]
				if elem.pos < len(val) {
					elem.Value += val[elem.pos:]
				}
				elem.pos--
			}
			return nil
		case "left":
			if elem.pos > 0 {
				elem.pos--
			}
			return nil
		case "right":
			if elem.pos < len(elem.Value) {
				elem.pos++
			}
			return nil
		}
	}
	if let, ok := evt.(Letter); ok {
		if let != "" {
			elem.Value = elem.Value[:elem.pos] + string(let) + elem.Value[elem.pos:]
			elem.pos++
			return nil
		}
	}
	return evt
}

func (elem *TextInput) Draw(draw Drawable) {
	w := len(elem.Label)
	if elem.offset != nil {
		w = *elem.offset
	}
	if draw.Focused() {
		draw.Inverse()
	}
	draw.WriteString(fmt.Sprintf("%*s", w+1, elem.Label+":"))
	if draw.Focused() {
		draw.Inverse()
	}
	draw.WriteString(" ")
	if elem.echo {
		draw.WriteString(elem.Value)
	} else {
		for range elem.Value {
			draw.WriteString("*")
		}
	}
	if draw.Focused() {
		draw.SetCursor(w+2+elem.pos+1, -1)
	}
}

type HPad struct {
	left, right *int
	child       Element
}

func NewHPad(left *int, child Element, right *int) *HPad {
	elem := new(HPad)
	elem.left, elem.right = left, right
	elem.child = child
	return elem
}

func (elem *HPad) Input(evt Event) Event {
	return elem.child.Input(evt)
}

func (elem *HPad) Draw(draw Drawable) {
	if elem.left != nil {
		draw.WriteString(fmt.Sprintf("%*s", *elem.left, " "))
	}
	elem.child.Draw(draw)
	if elem.right != nil {
		draw.WriteString(fmt.Sprintf("%*s", *elem.right, " "))
	}
}

type Button struct {
	Label  string
	Submit func()
}

func NewButton(label string) *Button {
	elem := new(Button)
	elem.Label = label
	return elem
}

func (elem *Button) Input(evt Event) Event {
	if key, ok := evt.(Key); ok && key == "enter" {
		if elem.Submit != nil {
			elem.Submit()
		}
		return nil
	}
	if let, ok := evt.(Letter); ok && let == " " {
		if elem.Submit != nil {
			elem.Submit()
		}
		return nil
	}
	return evt
}

func (elem *Button) Draw(draw Drawable) {
	if draw.Focused() {
		draw.Inverse()
		draw.WriteString("< ")
	} else {
		draw.WriteString("[ ")
	}
	draw.WriteString(elem.Label)
	if draw.Focused() {
		draw.WriteString(" >")
		draw.Inverse()
	} else {
		draw.WriteString(" ]")
	}
}

type TextArea struct {
	Label string
	Value string
	pos   int
}

func NewTextArea() *TextArea {
	elem := new(TextArea)
	return elem
}
func (elem *TextArea) Set(s string) {
	elem.Value = s
	elem.pos = len(elem.Value)
}

func (elem *TextArea) Input(evt Event) Event {
	if key, ok := evt.(Key); ok {
		switch key {
		case "bs":
			if elem.pos > 0 {
				val := elem.Value
				elem.Value = val[0 : elem.pos-1]
				if elem.pos < len(val) {
					elem.Value += val[elem.pos:]
				}
				elem.pos--
			}
			return nil
		case "left":
			if elem.pos > 0 {
				elem.pos--
			}
			return nil
		case "right":
			if elem.pos < len(elem.Value) {
				elem.pos++
			}
			return nil
		case "enter":
			evt = Letter("\n")
		}
	}
	if let, ok := evt.(Letter); ok {
		elem.Value = elem.Value[:elem.pos] + string(let) + elem.Value[elem.pos:]
		elem.pos++
		return nil
	}
	return evt
}

func (elem *TextArea) Draw(draw Drawable) {
	if elem.Label != "" {
		if draw.Focused() {
			draw.Inverse()
		}
		draw.WriteString(elem.Label)
		if draw.Focused() {
			draw.Inverse()
		}
		draw.MoveTo(1, 2)
	}
	draw.WriteString(elem.Value)
	if draw.Focused() {
		x, y := textpos(elem.Value[:elem.pos], draw.Width())
		if elem.Label != "" {
			y++
		}
		draw.SetCursor(1+x, 1+y)
	}
}

func textpos(s string, w int) (int, int) {
	var x, y int
	for _, c := range s {
		switch c {
		case '\t':
			x++
			for x&7 != 0 {
				x++
			}
		case '\n':
			x = 0
			y++
		default:
			x++
		}
	}
	return x, y
}
