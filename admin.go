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
	"io/ioutil"
	"log"

	"github.com/gdamore/tcell"
	"github.com/rivo/tview"
)

func adminscreen() {
	log.SetOutput(ioutil.Discard)

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

	app := tview.NewApplication()
	var maindriver func(event *tcell.EventKey) *tcell.EventKey

	table := tview.NewTable().SetFixed(1, 0).SetSelectable(true, false).
		SetSelectedStyle(tcell.ColorBlack, tcell.ColorPurple, 0)

	mainframe := tview.NewFrame(table)
	mainframe.AddText(tview.Escape("honk admin - [q] quit"),
		true, 0, tcell.ColorPurple)
	mainframe.SetBorders(1, 0, 1, 0, 4, 0)

	dupecell := func(base *tview.TableCell) *tview.TableCell {
		rv := new(tview.TableCell)
		*rv = *base
		return rv
	}

	showtable := func() {
		table.Clear()

		row := 0
		{
			col := 0
			headcell := tview.TableCell{
				Color:         tcell.ColorWhite,
				NotSelectable: true,
			}
			cell := dupecell(&headcell)
			cell.Text = "which       "
			table.SetCell(row, col, cell)
			col++
			cell = dupecell(&headcell)
			cell.Text = "message"
			table.SetCell(row, col, cell)

			row++
		}
		for i := 0; i < 3; i++ {
			col := 0
			msg := messages[i]
			headcell := tview.TableCell{
				Color: tcell.ColorWhite,
			}
			cell := dupecell(&headcell)
			cell.Text = msg.label
			table.SetCell(row, col, cell)
			col++
			cell = dupecell(&headcell)
			cell.Text = tview.Escape(msg.text)
			table.SetCell(row, col, cell)

			row++
		}

		app.SetInputCapture(maindriver)
		app.SetRoot(mainframe, true)
	}

	arrowadapter := func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyDown:
			return tcell.NewEventKey(tcell.KeyTab, '\t', tcell.ModNone)
		case tcell.KeyUp:
			return tcell.NewEventKey(tcell.KeyBacktab, '\t', tcell.ModNone)
		}
		return event
	}

	editform := tview.NewForm()
	descbox := tview.NewInputField().SetLabel("msg: ").SetFieldWidth(60)
	editform.AddButton("save", nil)
	editform.AddButton("cancel", nil)
	savebutton := editform.GetButton(0)
	editform.SetFieldTextColor(tcell.ColorBlack)
	editform.SetFieldBackgroundColor(tcell.ColorPurple)
	editform.SetLabelColor(tcell.ColorWhite)
	editform.SetButtonTextColor(tcell.ColorPurple)
	editform.SetButtonBackgroundColor(tcell.ColorBlack)
	editform.GetButton(1).SetSelectedFunc(showtable)
	editform.SetCancelFunc(showtable)

	editframe := tview.NewFrame(editform)
	editframe.SetBorders(1, 0, 1, 0, 4, 0)

	showform := func() {
		editform.Clear(false)
		editform.AddFormItem(descbox)
		app.SetInputCapture(arrowadapter)
		app.SetRoot(editframe, true)
	}

	editmsg := func(which int) {
		msg := messages[which]
		editframe.Clear()
		editframe.AddText(tview.Escape("edit "+msg.label+" message"),
			true, 0, tcell.ColorPurple)
		descbox.SetText(msg.text)
		savebutton.SetSelectedFunc(func() {
			msg.text = descbox.GetText()
			updateconfig(msg.name, msg.text)
			showtable()
		})
		showform()
	}

	table.SetSelectedFunc(func(row, col int) {
		editmsg(row - 1)
	})

	maindriver = func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Rune() {
		case 'e':
			r, _ := table.GetSelection()
			r--
			editmsg(r)
		case 'q':
			app.Stop()
			return nil
		}
		return event
	}

	showtable()
	app.Run()
}
