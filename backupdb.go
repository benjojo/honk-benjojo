//
// Copyright (c) 2020 Ted Unangst <tedu@tedunangst.com>
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
	"os"
	"time"

	"strings"
)

func queryDB(db *sql.DB, s string, args ...interface{}) *sql.Rows {
	rows, err := db.Query(s, args...)
	if err != nil {
		elog.Fatalf("can't query %s: %s", s, err)
	}
	return rows
}

func scanDBRow(rows *sql.Rows, args ...interface{}) {
	err := rows.Scan(args...)
	if err != nil {
		elog.Fatalf("can't scan: %s", err)
	}
}

func backupDatabase(dirname string) {
	err := os.Mkdir(dirname, 0700)
	if err != nil && !os.IsExist(err) {
		elog.Fatalf("can't create directory: %s", dirname)
	}
	now := time.Now().Unix()
	dirname = fmt.Sprintf("%s/honk-%d", dirname, now)
	err = os.Mkdir(dirname, 0700)
	if err != nil {
		elog.Fatalf("can't create directory: %s", dirname)
	}
	backupdbname := fmt.Sprintf("%s/honk.db", dirname)
	backup, err := sql.Open("sqlite3", backupdbname)
	if err != nil {
		elog.Fatalf("can't open backup database")
	}
	_, err = backup.Exec("PRAGMA journal_mode=WAL")
	for _, line := range strings.Split(sqlSchema, ";") {
		_, err = backup.Exec(line)
		if err != nil {
			elog.Fatal(err)
			return
		}
	}
	tx, err := backup.Begin()
	if err != nil {
		elog.Fatal(err)
	}
	orig := opendatabase()
	rows := queryDB(orig, "select userid, username, hash, displayname, about, pubkey, seckey, options from users")
	for rows.Next() {
		var userid int64
		var username, hash, displayname, about, pubkey, seckey, options string
		scanDBRow(rows, &userid, &username, &hash, &displayname, &about, &pubkey, &seckey, &options)
		sqlMustQuery(tx, "insert into users (userid, username, hash, displayname, about, pubkey, seckey, options) values (?, ?, ?, ?, ?, ?, ?, ?)", userid, username, hash, displayname, about, pubkey, seckey, options)
	}
	rows.Close()

	rows = queryDB(orig, "select honkerid, userid, name, xid, flavor, combos, owner, meta, folxid from honkers")
	for rows.Next() {
		var honkerid, userid int64
		var name, xid, flavor, combos, owner, meta, folxid string
		scanDBRow(rows, &honkerid, &userid, &name, &xid, &flavor, &combos, &owner, &meta, &folxid)
		sqlMustQuery(tx, "insert into honkers (honkerid, userid, name, xid, flavor, combos, owner, meta, folxid) values (?, ?, ?, ?, ?, ?, ?, ?, ?)", honkerid, userid, name, xid, flavor, combos, owner, meta, folxid)
	}
	rows.Close()

	rows = queryDB(orig, "select convoy from honks where flags & 4 or whofore = 2 or whofore = 3")
	convoys := make(map[string]bool)
	for rows.Next() {
		var convoy string
		scanDBRow(rows, &convoy)
		convoys[convoy] = true
	}
	rows.Close()

	honkids := make(map[int64]bool)
	for c := range convoys {
		rows = queryDB(orig, "select honkid, userid, what, honker, xid, rid, dt, url, audience, noise, convoy, whofore, format, precis, oonker, flags, plain from honks where convoy = ?", c)
		for rows.Next() {
			var honkid, userid int64
			var what, honker, xid, rid, dt, url, audience, noise, convoy, plain string
			var whofore int64
			var format, precis, oonker string
			var flags int64
			scanDBRow(rows, &honkid, &userid, &what, &honker, &xid, &rid, &dt, &url, &audience, &noise, &convoy, &whofore, &format, &precis, &oonker, &flags, &plain)
			honkids[honkid] = true
			sqlMustQuery(tx, "insert into honks (honkid, userid, what, honker, xid, rid, dt, url, audience, noise, convoy, whofore, format, precis, oonker, flags, plain) values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)", honkid, userid, what, honker, xid, rid, dt, url, audience, noise, convoy, whofore, format, precis, oonker, flags, plain)
		}
		rows.Close()
	}
	fileids := make(map[int64]bool)
	for h := range honkids {
		rows = queryDB(orig, "select honkid, chonkid, fileid from donks where honkid = ?", h)
		for rows.Next() {
			var honkid, chonkid, fileid int64
			scanDBRow(rows, &honkid, &chonkid, &fileid)
			fileids[fileid] = true
			sqlMustQuery(tx, "insert into donks (honkid, chonkid, fileid) values (?, ?, ?)", honkid, chonkid, fileid)
		}
		rows.Close()
		rows = queryDB(orig, "select ontology, honkid from onts where honkid = ?", h)
		for rows.Next() {
			var ontology string
			var honkid int64
			scanDBRow(rows, &ontology, &honkid)
			sqlMustQuery(tx, "insert into onts (ontology, honkid) values (?, ?)", ontology, honkid)
		}
		rows.Close()
		rows = queryDB(orig, "select honkid, genus, json from honkmeta where honkid = ?", h)
		for rows.Next() {
			var honkid int64
			var genus, json string
			scanDBRow(rows, &honkid, &genus, &json)
			sqlMustQuery(tx, "insert into honkmeta (honkid, genus, json) values (?, ?, ?)", honkid, genus, json)
		}
		rows.Close()
	}
	chonkids := make(map[int64]bool)
	rows = queryDB(orig, "select chonkid, userid, xid, who, target, dt, noise, format from chonks")
	for rows.Next() {
		var chonkid, userid int64
		var xid, who, target, dt, noise, format string
		scanDBRow(rows, &chonkid, &userid, &xid, &who, &target, &dt, &noise, &format)
		chonkids[chonkid] = true
		sqlMustQuery(tx, "insert into chonks (chonkid, userid, xid, who, target, dt, noise, format) values (?, ?, ?, ?, ?, ?, ?, ?)", chonkid, userid, xid, who, target, dt, noise, format)
	}
	rows.Close()
	for c := range chonkids {
		rows = queryDB(orig, "select honkid, chonkid, fileid from donks where chonkid = ?", c)
		for rows.Next() {
			var honkid, chonkid, fileid int64
			scanDBRow(rows, &honkid, &chonkid, &fileid)
			fileids[fileid] = true
			sqlMustQuery(tx, "insert into donks (honkid, chonkid, fileid) values (?, ?, ?)", honkid, chonkid, fileid)
		}
		rows.Close()
	}
	filexids := make(map[string]bool)
	for f := range fileids {
		rows = queryDB(orig, "select fileid, xid, name, description, url, media, local, meta from filemeta where fileid = ?", f)
		for rows.Next() {
			var fileid int64
			var xid, name, description, url, media, meta string
			var local int64
			scanDBRow(rows, &fileid, &xid, &name, &description, &url, &media, &local, &meta)
			if xid != "" {
				filexids[xid] = true
			}
			sqlMustQuery(tx, "insert into filemeta (fileid, xid, name, description, url, media, local, meta) values (?, ?, ?, ?, ?, ?, ?, ?)", fileid, xid, name, description, url, media, local, meta)
		}
		rows.Close()
	}
	for xid := range filexids {
		rows = queryDB(orig, "select media, hash from filehashes where xid = ?", xid)
		for rows.Next() {
			var media, hash string
			scanDBRow(rows, &media, &hash)
			sqlMustQuery(tx, "insert into filehashes (xid, media, hash) values (?, ?, ?)", xid, media, hash)
		}
		rows.Close()
	}
	rows = queryDB(orig, "select key, value from config")
	for rows.Next() {
		var key string
		var value interface{}
		scanDBRow(rows, &key, &value)
		sqlMustQuery(tx, "insert into config (key, value) values (?, ?)", key, value)
	}

	err = tx.Commit()
	if err != nil {
		elog.Fatalf("can't commit backp: %s", err)
	}
	tx = nil
	backup.Close()

	var blob *sql.DB
	var filesavepath string
	if storeTheFilesInTheFileSystem {
		filesavepath = fmt.Sprintf("%s/attachments", dirname)
		os.Mkdir(filesavepath, 0700)
		filesavepath += "/"
	} else {
		backupblobname := fmt.Sprintf("%s/blob.db", dirname)
		blob, err = sql.Open("sqlite3", backupblobname)
		if err != nil {
			elog.Fatalf("can't open backup blob database")
		}
		_, err = blob.Exec("PRAGMA journal_mode=WAL")
		sqlMustQuery(blob, "create table filedata (xid text, content blob)")
		sqlMustQuery(blob, "create index idx_filexid on filedata(xid)")
		tx, err = blob.Begin()
		if err != nil {
			elog.Fatalf("can't start transaction: %s", err)
		}
		stmtSaveBlobData, err = tx.Prepare("insert into filedata (xid, content) values (?, ?)")
		checkErr(err)
	}
	for xid := range filexids {
		if storeTheFilesInTheFileSystem {
			oldname := filepath(xid)
			newname := filesavepath + oldname[14:]
			os.Mkdir(newname[:strings.LastIndexByte(newname, '/')], 0700)
			err = os.Link(oldname, newname)
			if err == nil {
				continue
			}
		}
		data, closer, err := loaddata(xid)
		if err != nil {
			elog.Printf("lost a file: %s", xid)
			continue
		}
		if storeTheFilesInTheFileSystem {
			oldname := filepath(xid)
			newname := filesavepath + oldname[14:]
			err = os.WriteFile(newname, data, 0700)
		} else {
			_, err = stmtSaveBlobData.Exec(xid, data)
		}
		if err != nil {
			elog.Printf("failed to save file %s: %s", xid, err)
		}
		closer()
	}

	if blob != nil {
		err = tx.Commit()
		if err != nil {
			elog.Fatalf("can't commit blobs: %s", err)
		}
		blob.Close()
	}
	fmt.Printf("backup saved to %s\n", dirname)
}
