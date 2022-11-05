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
	"database/sql"
	"fmt"
	"log"
	"os"
)

func doordie(db *sql.DB, s string, args ...interface{}) {
	_, err := db.Exec(s, args...)
	if err != nil {
		log.Fatal(err)
	}
}

func upgradedb() {
	db := opendatabase()
	dbversion := 0
	getconfig("dbversion", &dbversion)
	getconfig("servername", &serverName)

	switch dbversion {
	case 0:
		doordie(db, "insert into config (key, value) values ('dbversion', 1)")
		fallthrough
	case 1:
		doordie(db, "create table doovers(dooverid integer primary key, dt text, tries integer, username text, rcpt text, msg blob)")
		doordie(db, "update config set value = 2 where key = 'dbversion'")
		fallthrough
	case 2:
		doordie(db, "alter table honks add column convoy text")
		doordie(db, "update honks set convoy = ''")
		doordie(db, "create index idx_honksconvoy on honks(convoy)")
		doordie(db, "create table xonkers (xonkerid integer primary key, xid text, ibox text, obox text, sbox text, pubkey text)")
		doordie(db, "insert into xonkers (xid, ibox, obox, sbox, pubkey) select xid, '', '', '', pubkey from honkers where flavor = 'key'")
		doordie(db, "delete from honkers where flavor = 'key'")
		doordie(db, "create index idx_xonkerxid on xonkers(xid)")
		doordie(db, "create table zonkers (zonkerid integer primary key, userid integer, name text, wherefore text)")
		doordie(db, "create index idx_zonkersname on zonkers(name)")
		doordie(db, "update config set value = 3 where key = 'dbversion'")
		fallthrough
	case 3:
		doordie(db, "alter table honks add column whofore integer")
		doordie(db, "update honks set whofore = 0")
		doordie(db, "update honks set whofore = 1 where honkid in (select honkid from honks join users on honks.userid = users.userid where instr(audience, username) > 0)")
		doordie(db, "update config set value = 4 where key = 'dbversion'")
		fallthrough
	case 4:
		doordie(db, "alter table honkers add column combos text")
		doordie(db, "update honkers set combos = ''")
		doordie(db, "update config set value = 5 where key = 'dbversion'")
		fallthrough
	case 5:
		doordie(db, "delete from donks where honkid in (select honkid from honks where what = 'zonk')")
		doordie(db, "delete from honks where what = 'zonk'")
		doordie(db, "update config set value = 6 where key = 'dbversion'")
		fallthrough
	case 6:
		doordie(db, "alter table honks add column format")
		doordie(db, "update honks set format = 'html'")
		doordie(db, "alter table honks add column precis")
		doordie(db, "update honks set precis = ''")
		doordie(db, "alter table honks add column oonker")
		doordie(db, "update honks set oonker = ''")
		doordie(db, "update config set value = 7 where key = 'dbversion'")
		fallthrough
	case 7:
		users := allusers()
		for _, u := range users {
			h := fmt.Sprintf("https://%s/u/%s", serverName, u.Username)
			doordie(db, fmt.Sprintf("update honks set xid = '%s/h/' || xid, honker = ?, whofore = 2 where userid = ? and honker = '' and (what = 'honk' or what = 'tonk')", h), h, u.UserID)
			doordie(db, "update honks set honker = ?, whofore = 2 where userid = ? and honker = '' and what = 'bonk'", h, u.UserID)
		}
		doordie(db, "update config set value = 8 where key = 'dbversion'")
		fallthrough
	case 8:
		doordie(db, "alter table files add column local integer")
		doordie(db, "update files set local = 1")
		doordie(db, "update config set value = 9 where key = 'dbversion'")
		fallthrough
	case 9:
		doordie(db, "drop table xonkers")
		doordie(db, "create table xonkers (xonkerid integer primary key, name text, info text, flavor text)")
		doordie(db, "create index idx_xonkername on xonkers(name)")
		doordie(db, "update config set value = 10 where key = 'dbversion'")
		fallthrough
	case 10:
		doordie(db, "update zonkers set wherefore = 'zomain' where wherefore = 'zurl'")
		doordie(db, "update zonkers set wherefore = 'zord' where wherefore = 'zword'")
		doordie(db, "update config set value = 11 where key = 'dbversion'")
		fallthrough
	case 11:
		doordie(db, "alter table users add column options text")
		doordie(db, "update users set options = ''")
		doordie(db, "update config set value = 12 where key = 'dbversion'")
		fallthrough
	case 12:
		doordie(db, "create index idx_honksoonker on honks(oonker)")
		doordie(db, "update config set value = 13 where key = 'dbversion'")
		fallthrough
	case 13:
	default:
		log.Fatalf("can't upgrade unknown version %d", dbversion)
	}
	cleanupdb("30")
	os.Exit(0)
}
