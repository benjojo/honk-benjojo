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

	"humungus.tedunangst.com/r/webs/htfilter"
)

var myVersion = 53 // index filehashes.xid

type dbexecer interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
}

func sqlMustQuery(db dbexecer, s string, args ...interface{}) {
	_, err := db.Exec(s, args...)
	if err != nil {
		elog.Fatalf("can't run %s: %s", s, err)
	}
}

func upgradedb() {
	db := opendatabase()
	dbversion := 0
	getConfigValue("dbversion", &dbversion)
	getConfigValue("servername", &serverName)

	if dbversion < 40 {
		elog.Fatal("database is too old to upgrade")
	}
	var err error
	var tx *sql.Tx
	try := func(s string, args ...interface{}) *sql.Rows {
		var rows *sql.Rows
		if strings.HasPrefix(s, "select") {
			if tx != nil {
				rows, err = tx.Query(s, args...)
			} else {
				rows, err = db.Query(s, args...)
			}
		} else {
			if tx != nil {
				_, err = tx.Exec(s, args...)
			} else {
				_, err = db.Exec(s, args...)
			}
		}
		if err != nil {
			elog.Fatalf("can't run %s: %s", s, err)
		}
		return rows
	}
	setV := func(ver int64) {
		try("update config set value = ? where key = 'dbversion'", ver)
	}

	switch dbversion {
	case 41:
		tx, err := db.Begin()
		checkErr(err)
		rows, err := tx.Query("select honkid, noise from honks where format = 'markdown' and precis <> ''")
		checkErr(err)
		m := make(map[int64]string)
		var dummy ActivityPubActivity
		for rows.Next() {
			err = rows.Scan(&dummy.ID, &dummy.Noise)
			checkErr(err)
			precipitate(&dummy)
			m[dummy.ID] = dummy.Noise
		}
		rows.Close()
		for id, noise := range m {
			_, err = tx.Exec("update honks set noise = ? where honkid = ?", noise, id)
			checkErr(err)
		}
		err = tx.Commit()
		checkErr(err)
		sqlMustQuery(db, "update config set value = 42 where key = 'dbversion'")
		fallthrough
	case 42:
		sqlMustQuery(db, "update honks set what = 'honk', flags = flags & ~ 32 where what = 'tonk' or what = 'wonk'")
		sqlMustQuery(db, "delete from honkmeta where genus = 'wonkles' or genus = 'guesses'")
		sqlMustQuery(db, "update config set value = 43 where key = 'dbversion'")
		fallthrough
	case 43:
		try("alter table honks add column plain text")
		try("update honks set plain = ''")
		setV(44)
		fallthrough
	case 44:
		makeplain := func(noise, precis, format string) []string {
			var plain []string
			var filt htfilter.Filter
			filt.WithLinks = true
			if precis != "" {
				t, _ := filt.TextOnly(precis)
				plain = append(plain, t)
			}
			if format == "html" {
				t, _ := filt.TextOnly(noise)
				plain = append(plain, t)
			} else {
				plain = append(plain, noise)
			}
			return plain
		}
		tx, err = db.Begin()
		checkErr(err)
		plainmap := make(map[int64][]string)
		rows, err := tx.Query("select honkid, noise, precis, format from honks")
		checkErr(err)
		for rows.Next() {
			var honkid int64
			var noise, precis, format string
			err = rows.Scan(&honkid, &noise, &precis, &format)
			checkErr(err)
			plainmap[honkid] = makeplain(noise, precis, format)
		}
		rows.Close()
		rows, err = tx.Query("select honkid, name, description from donks join filemeta on donks.fileid = filemeta.fileid")
		checkErr(err)
		for rows.Next() {
			var honkid int64
			var name, desc string
			err = rows.Scan(&honkid, &name, &desc)
			checkErr(err)
			plainmap[honkid] = append(plainmap[honkid], name)
			plainmap[honkid] = append(plainmap[honkid], desc)
		}
		rows.Close()
		for honkid, plain := range plainmap {
			try("update honks set plain = ? where honkid = ?", strings.Join(plain, " "), honkid)
		}
		setV(45)
		err = tx.Commit()
		checkErr(err)
		tx = nil
		fallthrough
	case 45:
		try("create index idx_honkswhotwo on honks(whofore) where whofore = 2")
		setV(46)
		fallthrough
	case 46:
		try("create index idx_honksforme on honks(whofore) where whofore = 1")
		setV(47)
		fallthrough
	case 47:
		rows := try("select userid, options from users where userid > 0")
		var users []*WhatAbout
		for rows.Next() {
			var user WhatAbout
			var jopt string
			err = rows.Scan(&user.ID, &jopt)
			checkErr(err)
			err = decodeJson(jopt, &user.Options)
			checkErr(err)
			users = append(users, &user)
		}
		rows.Close()
		for _, user := range users {
			chatpubkey, chatseckey := newChatKeys()
			user.Options.ChatPubKey = tob64(chatpubkey.key[:])
			user.Options.ChatSecKey = tob64(chatseckey.key[:])
			jopt, _ := encodeJson(user.Options)
			try("update users set options = ? where userid = ?", jopt, user.ID)
		}
		setV(48)
		fallthrough
	case 48:
		try("create index idx_honksurl on honks(url)")
		setV(49)
		fallthrough
	case 49:
		try("create index idx_honksrid on honks(rid) where rid <> ''")
		setV(50)
		fallthrough
	case 50:
		try("alter table filemeta add column meta text")
		try("update filemeta set meta = '{}'")
		setV(51)
		fallthrough
	case 51:
		hashes := make(map[string]string)
		blobdb := openblobdb()
		rows, err := blobdb.Query("select xid, hash, media from filedata")
		checkErr(err)
		for rows.Next() {
			var xid, hash, media string
			err = rows.Scan(&xid, &hash, &media)
			checkErr(err)
			hashes[xid] = hash + " " + media
		}
		rows.Close()
		tx, err = db.Begin()
		checkErr(err)
		try("create table filehashes (xid text, hash text, media text)")
		try("create index idx_filehashes on filehashes(hash)")
		for xid, data := range hashes {
			parts := strings.Split(data, " ")
			try("insert into filehashes (xid, hash, media) values (?, ?, ?)", xid, parts[0], parts[1])
		}
		setV(52)
		err = tx.Commit()
		checkErr(err)
		tx = nil
		fallthrough
	case 52:
		try("create index idx_filehashesxid on filehashes(xid)")
		setV(53)
		fallthrough
	case 53:
		setcsrfkey()
		try("analyze")
		closedatabases()

	default:
		elog.Fatalf("can't upgrade unknown version %d", dbversion)
	}
	os.Exit(0)
}
