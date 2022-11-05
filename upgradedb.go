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
	"log"
	"os"
	"regexp"
	"strings"
	"time"
)

var myVersion = 33

func doordie(db *sql.DB, s string, args ...interface{}) {
	_, err := db.Exec(s, args...)
	if err != nil {
		log.Fatalf("can't run %s: %s", s, err)
	}
}

func upgradedb() {
	db := opendatabase()
	dbversion := 0
	getconfig("dbversion", &dbversion)
	getconfig("servername", &serverName)

	if dbversion < 13 {
		log.Fatal("database is too old to upgrade")
	}
	switch dbversion {
	case 13:
		doordie(db, "alter table honks add column flags integer")
		doordie(db, "update honks set flags = 0")
		doordie(db, "update config set value = 14 where key = 'dbversion'")
		fallthrough
	case 14:
		doordie(db, "create table onts (ontology text, honkid integer)")
		doordie(db, "create index idx_ontology on onts(ontology)")
		doordie(db, "update config set value = 15 where key = 'dbversion'")
		fallthrough
	case 15:
		doordie(db, "delete from onts")
		ontmap := make(map[int64][]string)
		rows, err := db.Query("select honkid, noise from honks")
		if err != nil {
			log.Fatalf("can't query honks: %s", err)
		}
		re_more := regexp.MustCompile(`#<span>[[:alpha:]][[:alnum:]-]*`)
		for rows.Next() {
			var honkid int64
			var noise string
			err := rows.Scan(&honkid, &noise)
			if err != nil {
				log.Fatalf("can't scan honks: %s", err)
			}
			onts := ontologies(noise)
			mo := re_more.FindAllString(noise, -1)
			for _, o := range mo {
				onts = append(onts, "#"+o[7:])
			}
			if len(onts) > 0 {
				ontmap[honkid] = oneofakind(onts)
			}
		}
		rows.Close()
		tx, err := db.Begin()
		if err != nil {
			log.Fatalf("can't begin: %s", err)
		}
		stmtOnts, err := tx.Prepare("insert into onts (ontology, honkid) values (?, ?)")
		if err != nil {
			log.Fatal(err)
		}
		for honkid, onts := range ontmap {
			for _, o := range onts {
				_, err = stmtOnts.Exec(strings.ToLower(o), honkid)
				if err != nil {
					log.Fatal(err)
				}
			}
		}
		err = tx.Commit()
		if err != nil {
			log.Fatalf("can't commit: %s", err)
		}
		doordie(db, "update config set value = 16 where key = 'dbversion'")
		fallthrough
	case 16:
		doordie(db, "alter table files add column description text")
		doordie(db, "update files set description = name")
		doordie(db, "update config set value = 17 where key = 'dbversion'")
		fallthrough
	case 17:
		doordie(db, "create table forsaken (honkid integer, precis text, noise text)")
		doordie(db, "update config set value = 18 where key = 'dbversion'")
		fallthrough
	case 18:
		doordie(db, "create index idx_onthonkid on onts(honkid)")
		doordie(db, "update config set value = 19 where key = 'dbversion'")
		fallthrough
	case 19:
		doordie(db, "create table places (honkid integer, name text, latitude real, longitude real)")
		doordie(db, "create index idx_placehonkid on places(honkid)")
		fallthrough
	case 20:
		doordie(db, "alter table places add column url text")
		doordie(db, "update places set url = ''")
		doordie(db, "update config set value = 21 where key = 'dbversion'")
		fallthrough
	case 21:
		// here we go...
		initblobdb()
		blobdb := openblobdb()
		tx, err := blobdb.Begin()
		if err != nil {
			log.Fatalf("can't begin: %s", err)
		}
		doordie(db, "drop index idx_filesxid")
		doordie(db, "drop index idx_filesurl")
		doordie(db, "create table filemeta (fileid integer primary key, xid text, name text, description text, url text, media text, local integer)")
		doordie(db, "insert into filemeta select fileid, xid, name, description, url, media, local from files")
		doordie(db, "create index idx_filesxid on filemeta(xid)")
		doordie(db, "create index idx_filesurl on filemeta(url)")

		rows, err := db.Query("select xid, media, content from files where local = 1")
		if err != nil {
			log.Fatal(err)
		}
		for rows.Next() {
			var xid, media string
			var data []byte
			err = rows.Scan(&xid, &media, &data)
			if err == nil {
				_, err = tx.Exec("insert into filedata (xid, media, content) values (?, ?, ?)", xid, media, data)
			}
			if err != nil {
				log.Fatalf("can't save filedata: %s", err)
			}
		}
		rows.Close()
		err = tx.Commit()
		if err != nil {
			log.Fatalf("can't commit: %s", err)
		}
		doordie(db, "drop table files")
		doordie(db, "vacuum")
		doordie(db, "update config set value = 22 where key = 'dbversion'")
		fallthrough
	case 22:
		doordie(db, "create table honkmeta (honkid integer, genus text, json text)")
		doordie(db, "create index idx_honkmetaid on honkmeta(honkid)")
		doordie(db, "drop table forsaken") // don't bother saving this one
		rows, err := db.Query("select honkid, name, latitude, longitude, url from places")
		if err != nil {
			log.Fatal(err)
		}
		places := make(map[int64]*Place)
		for rows.Next() {
			var honkid int64
			p := new(Place)
			err = rows.Scan(&honkid, &p.Name, &p.Latitude, &p.Longitude, &p.Url)
			if err != nil {
				log.Fatal(err)
			}
			places[honkid] = p
		}
		rows.Close()
		tx, err := db.Begin()
		if err != nil {
			log.Fatalf("can't begin: %s", err)
		}
		for honkid, p := range places {
			j, err := jsonify(p)
			if err == nil {
				_, err = tx.Exec("insert into honkmeta (honkid, genus, json) values (?, ?, ?)",
					honkid, "place", j)
			}
			if err != nil {
				log.Fatal(err)
			}
		}
		err = tx.Commit()
		if err != nil {
			log.Fatalf("can't commit: %s", err)
		}
		doordie(db, "update config set value = 23 where key = 'dbversion'")
		fallthrough
	case 23:
		doordie(db, "create table hfcs (hfcsid integer primary key, userid integer, json text)")
		doordie(db, "create index idx_hfcsuser on hfcs(userid)")
		rows, err := db.Query("select userid, name, wherefore from zonkers where wherefore in ('zord', 'zilence', 'zoggle', 'zonker', 'zomain')")
		if err != nil {
			log.Fatalf("can't query zonkers: %s", err)
		}
		filtmap := make(map[int64][]*Filter)
		now := time.Now().UTC()
		for rows.Next() {
			var userid int64
			var name, wherefore string
			err = rows.Scan(&userid, &name, &wherefore)
			if err != nil {
				log.Fatalf("error scanning zonker: %s", err)
			}
			f := new(Filter)
			f.Date = now
			switch wherefore {
			case "zord":
				f.Name = "hide " + name
				f.Text = name
				f.Hide = true
			case "zilence":
				f.Name = "silence " + name
				f.Text = name
				f.Collapse = true
			case "zoggle":
				f.Name = "skip " + name
				f.Actor = name
				f.SkipMedia = true
			case "zonker":
				f.Name = "reject " + name
				f.Actor = name
				f.IncludeAudience = true
				f.Reject = true
			case "zomain":
				f.Name = "reject " + name
				f.Actor = name
				f.IncludeAudience = true
				f.Reject = true
			}
			filtmap[userid] = append(filtmap[userid], f)
		}
		rows.Close()
		tx, err := db.Begin()
		if err != nil {
			log.Fatalf("can't begin: %s", err)
		}
		for userid, filts := range filtmap {
			for _, f := range filts {
				j, err := jsonify(f)
				if err == nil {
					_, err = tx.Exec("insert into hfcs (userid, json) values (?, ?)", userid, j)
				}
				if err != nil {
					log.Fatalf("can't save filter: %s", err)
				}
			}
		}
		err = tx.Commit()
		if err != nil {
			log.Fatalf("can't commit: %s", err)
		}
		doordie(db, "delete from zonkers where wherefore in ('zord', 'zilence', 'zoggle', 'zonker', 'zomain')")
		doordie(db, "update config set value = 24 where key = 'dbversion'")
		fallthrough
	case 24:
		doordie(db, "update honks set convoy = 'missing-' || abs(random() % 987654321) where convoy = ''")
		doordie(db, "update config set value = 25 where key = 'dbversion'")
		fallthrough
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
		getconfig("debug", &d)
		if d == -1 {
			setconfig("debug", 0)
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
			log.Fatal(err)
		}
		rows, err := tx.Query("select userid, options from users")
		if err != nil {
			log.Fatal(err)
		}
		m := make(map[int64]string)
		for rows.Next() {
			var userid int64
			var options string
			err = rows.Scan(&userid, &options)
			if err != nil {
				log.Fatal(err)
			}
			var uo UserOptions
			uo.SkinnyCSS = strings.Contains(options, " skinny ")
			m[userid], err = jsonify(uo)
			if err != nil {
				log.Fatal(err)
			}
		}
		rows.Close()
		for u, o := range m {
			_, err = tx.Exec("update users set options = ? where userid = ?", o, u)
			if err != nil {
				log.Fatal(err)
			}
		}
		err = tx.Commit()
		if err != nil {
			log.Fatal(err)
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

	default:
		log.Fatalf("can't upgrade unknown version %d", dbversion)
	}
	os.Exit(0)
}
