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
	"log"
	notrand "math/rand"
	"sync"
	"time"
)

func init() {
	notrand.Seed(time.Now().Unix())
}

type Doover struct {
	ID   int64
	When time.Time
}

func sayitagain(goarounds int, username string, rcpt string, msg []byte) {
	var drift time.Duration
	switch goarounds {
	case 1:
		drift = 5 * time.Minute
	case 2:
		drift = 1 * time.Hour
	case 3:
		drift = 12 * time.Hour
	case 4:
		drift = 24 * time.Hour
	default:
		log.Printf("he's dead jim: %s", rcpt)
		return
	}
	drift += time.Duration(notrand.Int63n(int64(drift / 10)))
	when := time.Now().UTC().Add(drift)
	stmtAddDoover.Exec(when.Format(dbtimeformat), goarounds, username, rcpt, msg)
	select {
	case pokechan <- 0:
	default:
	}
}

var trucksout = 0
var maxtrucksout = 10
var garagelock sync.Mutex
var garagebell = sync.NewCond(&garagelock)

func truckgoesout() {
	garagelock.Lock()
	for trucksout >= maxtrucksout {
		garagebell.Wait()
	}
	trucksout++
	garagelock.Unlock()
}

func truckcomesin() {
	garagelock.Lock()
	trucksout--
	garagebell.Broadcast()
	garagelock.Unlock()
}

func deliverate(goarounds int, username string, rcpt string, msg []byte) {
	truckgoesout()
	defer truckcomesin()

	keyname, key := ziggy(username)
	var inbox string
	// already did the box indirection
	if rcpt[0] == '%' {
		inbox = rcpt[1:]
	} else {
		box, err := getboxes(rcpt)
		if err != nil {
			log.Printf("error getting inbox %s: %s", rcpt, err)
			sayitagain(goarounds+1, username, rcpt, msg)
			return
		}
		inbox = box.In
	}
	err := PostMsg(keyname, key, inbox, msg)
	if err != nil {
		log.Printf("failed to post json to %s: %s", inbox, err)
		sayitagain(goarounds+1, username, rcpt, msg)
		return
	}
}

var pokechan = make(chan int)

func redeliverator() {
	sleeper := time.NewTimer(0)
	for {
		select {
		case <-pokechan:
			if !sleeper.Stop() {
				<-sleeper.C
			}
			time.Sleep(5 * time.Second)
		case <-sleeper.C:
		}

		rows, err := stmtGetDoovers.Query()
		if err != nil {
			log.Printf("wat?")
			time.Sleep(1 * time.Minute)
			continue
		}
		var doovers []Doover
		for rows.Next() {
			var d Doover
			var dt string
			rows.Scan(&d.ID, &dt)
			d.When, _ = time.Parse(dbtimeformat, dt)
			doovers = append(doovers, d)
		}
		rows.Close()
		now := time.Now().UTC()
		nexttime := now.Add(24 * time.Hour)
		for _, d := range doovers {
			if d.When.Before(now) {
				var goarounds int
				var username, rcpt string
				var msg []byte
				row := stmtLoadDoover.QueryRow(d.ID)
				row.Scan(&goarounds, &username, &rcpt, &msg)
				stmtZapDoover.Exec(d.ID)
				log.Printf("redeliverating %s try %d", rcpt, goarounds)
				deliverate(goarounds, username, rcpt, msg)
			} else if d.When.Before(nexttime) {
				nexttime = d.When
			}
		}
		dur := nexttime.Sub(now).Round(time.Second) + 5*time.Second
		sleeper.Reset(dur)
	}
}
