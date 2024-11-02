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
	"strings"

	"humungus.tedunangst.com/r/termvc"
	"humungus.tedunangst.com/r/webs/log"
)

func adminscreen() {
	log.Init(log.Options{Progname: "honk", Alllogname: "null"})

	var avatarColors string
	getConfigValue("avatarcolors", &avatarColors)
	loadLingo()

	type adminfield struct {
		name  string
		label string
		text  string
		ptr   *string
	}

	messages := []*adminfield{
		{
			name:  "servermsg",
			label: "server banner",
			text:  string(serverMsg),
		},
		{
			name:  "aboutmsg",
			label: "about page message",
			text:  string(aboutMsg),
		},
		{
			name:  "loginmsg",
			label: "login banner",
			text:  string(loginMsg),
		},
		{
			name:  "avatarcolors",
			label: "avatar colors (4 RGBA hex numbers)",
			text:  string(avatarColors),
		},
	}

	app := termvc.NewApp()
	scr := termvc.NewScreen()
	scr.DefaultColor(35)
	var tabs []termvc.Element
	insns := termvc.NewStringWrapper("honk admin")
	tabs = append(tabs, insns)

	for _, m := range messages {
		input := termvc.NewTextArea()
		input.Label = m.label
		input.Set(m.text)
		m.ptr = &input.Value
		tabs = append(tabs, input)
	}
	{
		var inputs []termvc.Element
		var offset int
		for _, l := range []string{"honked", "bonked", "honked back", "qonked", "evented"} {
			field := termvc.NewTextInput(l, &offset)
			field.Set(relingo[l])
			inputs = append(inputs, field)
			messages = append(messages, &adminfield{
				name: "lingo-" + strings.ReplaceAll(l, " ", ""),
				ptr:  &field.Value,
			})
		}
		form := termvc.NewForm(inputs...)
		tabs = append(tabs, form)
	}
	btn := termvc.NewButton("save")
	btn.Submit = func() {
		app.Quit()
		termvc.Restore()
		for _, m := range messages {
			setconfig(m.name, *m.ptr)
		}
	}
	tabs = append(tabs, btn)
	group := termvc.NewTabGroup(tabs...)
	group.SetSkip(0, true)
	group.SetHeight(0, 1)
	group.SetFocus(1)
	group.SetHeight(len(tabs)-1, 1)
	group.SetHeight(len(tabs)-2, 6)

	app.Element = group
	app.Screen = scr

	termvc.Start()
	defer termvc.Restore()
	go termvc.Catch(nil)
	app.Loop()
}
