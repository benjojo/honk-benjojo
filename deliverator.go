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
	"fmt"
	notrand "math/rand"
	"time"

	"humungus.tedunangst.com/r/webs/gate"
)

type Doover struct {
	ID   int64
	When time.Time
}

func sayitagain(goarounds int64, userid int64, rcpt string, msg []byte) {
	var drift time.Duration
	switch goarounds {
	case 1:
		drift = 5 * time.Minute
	case 2:
		drift = 1 * time.Hour
	case 3:
		drift = 4 * time.Hour
	case 4:
		drift = 12 * time.Hour
	case 5:
		drift = 24 * time.Hour
	default:
		ilog.Printf("he's dead jim: %s", rcpt)
		clearoutbound(rcpt)
		return
	}
	drift += time.Duration(notrand.Int63n(int64(drift / 10)))
	when := time.Now().Add(drift)
	_, err := stmtAddDoover.Exec(when.UTC().Format(dbtimeformat), goarounds, userid, rcpt, msg)
	if err != nil {
		elog.Printf("error saving doover: %s", err)
	}
	select {
	case pokechan <- 0:
	default:
	}
}

func clearoutbound(rcpt string) {
	hostname := originate(rcpt)
	if hostname == "" {
		return
	}
	xid := fmt.Sprintf("%%https://%s/%%", hostname)
	ilog.Printf("clearing outbound for %s", xid)
	db := opendatabase()
	db.Exec("delete from doovers where rcpt like ?", xid)
}

var garage = gate.NewLimiter(40)

func deliverate(goarounds int64, userid int64, rcpt string, msg []byte, prio bool) {
	garage.Start()
	defer garage.Finish()

	var ki *KeyInfo
	ok := ziggies.Get(userid, &ki)
	if !ok {
		elog.Printf("lost key for delivery")
		return
	}
	var inbox string
	// already did the box indirection
	if rcpt[0] == '%' {
		inbox = rcpt[1:]
	} else {
		var box *Box
		ok := boxofboxes.Get(rcpt, &box)
		if !ok {
			ilog.Printf("failed getting inbox for %s", rcpt)
			sayitagain(goarounds+1, userid, rcpt, msg)
			return
		}
		inbox = box.In
	}
	err := PostMsg(ki.keyname, ki.seckey, inbox, msg)
	if err != nil {
		ilog.Printf("failed to post json to %s: %s", inbox, err)
		if prio {
			sayitagain(goarounds+1, userid, rcpt, msg)
		}
		return
	}
}

var pokechan = make(chan int, 1)

func getdoovers() []Doover {
	rows, err := stmtGetDoovers.Query()
	if err != nil {
		elog.Printf("wat?")
		time.Sleep(1 * time.Minute)
		return nil
	}
	defer rows.Close()
	var doovers []Doover
	for rows.Next() {
		var d Doover
		var dt string
		err := rows.Scan(&d.ID, &dt)
		if err != nil {
			elog.Printf("error scanning dooverid: %s", err)
			continue
		}
		d.When, _ = time.Parse(dbtimeformat, dt)
		doovers = append(doovers, d)
	}
	return doovers
}

func redeliverator() {
	sleeper := time.NewTimer(5 * time.Second)
	for {
		select {
		case <-pokechan:
			if !sleeper.Stop() {
				<-sleeper.C
			}
			time.Sleep(5 * time.Second)
		case <-sleeper.C:
		}

		doovers := getdoovers()

		now := time.Now()
		nexttime := now.Add(24 * time.Hour)
		for _, d := range doovers {
			if d.When.Before(now) {
				var goarounds, userid int64
				var rcpt string
				var msg []byte
				row := stmtLoadDoover.QueryRow(d.ID)
				err := row.Scan(&goarounds, &userid, &rcpt, &msg)
				if err != nil {
					elog.Printf("error scanning doover: %s", err)
					continue
				}
				_, err = stmtZapDoover.Exec(d.ID)
				if err != nil {
					elog.Printf("error deleting doover: %s", err)
					continue
				}
				ilog.Printf("redeliverating %s try %d", rcpt, goarounds)
				deliverate(goarounds, userid, rcpt, msg, true)
			} else if d.When.Before(nexttime) {
				nexttime = d.When
			}
		}
		now = time.Now()
		dur := 5 * time.Second
		if now.Before(nexttime) {
			dur += nexttime.Sub(now).Round(time.Second)
		}
		sleeper.Reset(dur)
	}
}
