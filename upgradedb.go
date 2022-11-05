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
	"os"
	"strings"
	"time"
)

var myVersion = 41

type dbexecer interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
}

func doordie(db dbexecer, s string, args ...interface{}) {
	_, err := db.Exec(s, args...)
	if err != nil {
		elog.Fatalf("can't run %s: %s", s, err)
	}
}

func upgradedb() {
	db := opendatabase()
	dbversion := 0
	getconfig("dbversion", &dbversion)
	getconfig("servername", &serverName)

	if dbversion < 13 {
		elog.Fatal("database is too old to upgrade")
	}
	switch dbversion {
	case 25:
		doordie(db, "delete from auth")
		doordie(db, "alter table auth add column expiry text")
		doordie(db, "update config set value = 26 where key = 'dbversion'")
		fallthrough
	case 26:
		s := ""
		getconfig("servermsg", &s)
		if s == "" {
			setconfig("servermsg", "<h2>Things happen.</h2>")
		}
		s = ""
		getconfig("aboutmsg", &s)
		if s == "" {
			setconfig("aboutmsg", "<h3>What is honk?</h3><p>Honk is amazing!")
		}
		s = ""
		getconfig("loginmsg", &s)
		if s == "" {
			setconfig("loginmsg", "<h2>login</h2>")
		}
		d := -1
		getconfig("devel", &d)
		if d == -1 {
			setconfig("devel", 0)
		}
		doordie(db, "update config set value = 27 where key = 'dbversion'")
		fallthrough
	case 27:
		createserveruser(db)
		doordie(db, "update config set value = 28 where key = 'dbversion'")
		fallthrough
	case 28:
		doordie(db, "drop table doovers")
		doordie(db, "create table doovers(dooverid integer primary key, dt text, tries integer, userid integer, rcpt text, msg blob)")
		doordie(db, "update config set value = 29 where key = 'dbversion'")
		fallthrough
	case 29:
		doordie(db, "alter table honkers add column owner text")
		doordie(db, "update honkers set owner = xid")
		doordie(db, "update config set value = 30 where key = 'dbversion'")
		fallthrough
	case 30:
		tx, err := db.Begin()
		if err != nil {
			elog.Fatal(err)
		}
		rows, err := tx.Query("select userid, options from users")
		if err != nil {
			elog.Fatal(err)
		}
		m := make(map[int64]string)
		for rows.Next() {
			var userid int64
			var options string
			err = rows.Scan(&userid, &options)
			if err != nil {
				elog.Fatal(err)
			}
			var uo UserOptions
			uo.SkinnyCSS = strings.Contains(options, " skinny ")
			m[userid], err = jsonify(uo)
			if err != nil {
				elog.Fatal(err)
			}
		}
		rows.Close()
		for u, o := range m {
			_, err = tx.Exec("update users set options = ? where userid = ?", o, u)
			if err != nil {
				elog.Fatal(err)
			}
		}
		err = tx.Commit()
		if err != nil {
			elog.Fatal(err)
		}
		doordie(db, "update config set value = 31 where key = 'dbversion'")
		fallthrough
	case 31:
		doordie(db, "create table tracks (xid text, fetches text)")
		doordie(db, "create index idx_trackhonkid on tracks(xid)")
		doordie(db, "update config set value = 32 where key = 'dbversion'")
		fallthrough
	case 32:
		doordie(db, "alter table xonkers add column dt text")
		doordie(db, "update xonkers set dt = ?", time.Now().UTC().Format(dbtimeformat))
		doordie(db, "update config set value = 33 where key = 'dbversion'")
		fallthrough
	case 33:
		doordie(db, "alter table honkers add column meta text")
		doordie(db, "update honkers set meta = '{}'")
		doordie(db, "update config set value = 34 where key = 'dbversion'")
		fallthrough
	case 34:
		doordie(db, "create table chonks (chonkid integer primary key, userid integer, xid text, who txt, target text, dt text, noise text, format text)")
		doordie(db, "update config set value = 35 where key = 'dbversion'")
		fallthrough
	case 35:
		doordie(db, "alter table donks add column chonkid integer")
		doordie(db, "update donks set chonkid = -1")
		doordie(db, "create index idx_donkshonk on donks(honkid)")
		doordie(db, "create index idx_donkschonk on donks(chonkid)")
		doordie(db, "update config set value = 36 where key = 'dbversion'")
		fallthrough
	case 36:
		doordie(db, "alter table honkers add column folxid text")
		doordie(db, "update honkers set folxid = 'lostdata'")
		doordie(db, "update config set value = 37 where key = 'dbversion'")
		fallthrough
	case 37:
		doordie(db, "update honkers set combos = '' where combos is null")
		doordie(db, "update honkers set owner = '' where owner is null")
		doordie(db, "update honkers set meta = '' where meta is null")
		doordie(db, "update honkers set folxid = '' where folxid is null")
		doordie(db, "update config set value = 38 where key = 'dbversion'")
		fallthrough
	case 38:
		doordie(db, "update honkers set folxid = abs(random())")
		doordie(db, "update config set value = 39 where key = 'dbversion'")
		fallthrough
	case 39:
		blobdb := openblobdb()
		doordie(blobdb, "alter table filedata add column hash text")
		doordie(blobdb, "create index idx_filehash on filedata(hash)")
		rows, err := blobdb.Query("select xid, content from filedata")
		if err != nil {
			elog.Fatal(err)
		}
		m := make(map[string]string)
		for rows.Next() {
			var xid string
			var data sql.RawBytes
			err := rows.Scan(&xid, &data)
			if err != nil {
				elog.Fatal(err)
			}
			hash := hashfiledata(data)
			m[xid] = hash
		}
		rows.Close()
		tx, err := blobdb.Begin()
		if err != nil {
			elog.Fatal(err)
		}
		for xid, hash := range m {
			doordie(tx, "update filedata set hash = ? where xid = ?", hash, xid)
		}
		err = tx.Commit()
		if err != nil {
			elog.Fatal(err)
		}
		doordie(db, "update config set value = 40 where key = 'dbversion'")
		fallthrough
	case 40:
		doordie(db, "PRAGMA journal_mode=WAL")
		blobdb := openblobdb()
		doordie(blobdb, "PRAGMA journal_mode=WAL")
		doordie(db, "update config set value = 41 where key = 'dbversion'")
		fallthrough
	case 41:

	default:
		elog.Fatalf("can't upgrade unknown version %d", dbversion)
	}
	os.Exit(0)
}
