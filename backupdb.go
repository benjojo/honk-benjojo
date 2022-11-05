package main

import (
	"database/sql"
	"fmt"
	"os"
	"time"

	"strings"
)

func qordie(db *sql.DB, s string, args ...interface{}) *sql.Rows {
	rows, err := db.Query(s, args...)
	if err != nil {
		elog.Fatalf("can't query %s: %s", s, err)
	}
	return rows
}

func scanordie(rows *sql.Rows, args ...interface{}) {
	err := rows.Scan(args...)
	if err != nil {
		elog.Fatalf("can't scan: %s", err)
	}
}

func svalbard(dirname string) {
	err := os.Mkdir(dirname, 0700)
	if err != nil && !os.IsExist(err) {
		elog.Fatalf("can't create directory: %s", dirname)
	}
	now := time.Now().Unix()
	backupdbname := fmt.Sprintf("%s/honk-%d.db", dirname, now)
	backup, err := sql.Open("sqlite3", backupdbname)
	if err != nil {
		elog.Fatalf("can't open backup database")
	}
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
	rows := qordie(orig, "select userid, username, hash, displayname, about, pubkey, seckey, options from users")
	for rows.Next() {
		var userid int64
		var username, hash, displayname, about, pubkey, seckey, options string
		scanordie(rows, &userid, &username, &hash, &displayname, &about, &pubkey, &seckey, &options)
		doordie(tx, "insert into users (userid, username, hash, displayname, about, pubkey, seckey, options) values (?, ?, ?, ?, ?, ?, ?, ?)", userid, username, hash, displayname, about, pubkey, seckey, options)
	}
	rows.Close()

	rows = qordie(orig, "select honkerid, userid, name, xid, flavor, combos, owner, meta, folxid from honkers")
	for rows.Next() {
		var honkerid, userid int64
		var name, xid, flavor, combos, owner, meta, folxid string
		scanordie(rows, &honkerid, &userid, &name, &xid, &flavor, &combos, &owner, &meta, &folxid)
		doordie(tx, "insert into honkers (honkerid, userid, name, xid, flavor, combos, owner, meta, folxid) values (?, ?, ?, ?, ?, ?, ?, ?, ?)", honkerid, userid, name, xid, flavor, combos, owner, meta, folxid)
	}
	rows.Close()

	rows = qordie(orig, "select convoy from honks where flags & 4 or whofore = 2 or whofore = 3")
	convoys := make(map[string]bool)
	for rows.Next() {
		var convoy string
		scanordie(rows, &convoy)
		convoys[convoy] = true
	}
	rows.Close()

	honkids := make(map[int64]bool)
	for c := range convoys {
		rows = qordie(orig, "select honkid, userid, what, honker, xid, rid, dt, url, audience, noise, convoy, whofore, format, precis, oonker, flags from honks where convoy = ?", c)
		for rows.Next() {
			var honkid, userid int64
			var what, honker, xid, rid, dt, url, audience, noise, convoy string
			var whofore int64
			var format, precis, oonker string
			var flags int64
			scanordie(rows, &honkid, &userid, &what, &honker, &xid, &rid, &dt, &url, &audience, &noise, &convoy, &whofore, &format, &precis, &oonker, &flags)
			honkids[honkid] = true
			doordie(tx, "insert into honks (honkid, userid, what, honker, xid, rid, dt, url, audience, noise, convoy, whofore, format, precis, oonker, flags) values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)", honkid, userid, what, honker, xid, rid, dt, url, audience, noise, convoy, whofore, format, precis, oonker, flags)
		}
		rows.Close()
	}
	fileids := make(map[int64]bool)
	for h := range honkids {
		rows = qordie(orig, "select honkid, chonkid, fileid from donks where honkid = ?", h)
		for rows.Next() {
			var honkid, chonkid, fileid int64
			scanordie(rows, &honkid, &chonkid, &fileid)
			fileids[fileid] = true
			doordie(tx, "insert into donks (honkid, chonkid, fileid) values (?, ?, ?)", honkid, chonkid, fileid)
		}
		rows.Close()
		rows = qordie(orig, "select ontology, honkid from onts where honkid = ?", h)
		for rows.Next() {
			var ontology string
			var honkid int64
			scanordie(rows, &ontology, &honkid)
			doordie(tx, "insert into onts (ontology, honkid) values (?, ?)", ontology, honkid)
		}
		rows.Close()
		rows = qordie(orig, "select honkid, genus, json from honkmeta where honkid = ?", h)
		for rows.Next() {
			var honkid int64
			var genus, json string
			scanordie(rows, &honkid, &genus, &json)
			doordie(tx, "insert into honkmeta (honkid, genus, json) values (?, ?, ?)", honkid, genus, json)
		}
		rows.Close()
	}
	chonkids := make(map[int64]bool)
	rows = qordie(orig, "select chonkid, userid, xid, who, target, dt, noise, format from chonks")
	for rows.Next() {
		var chonkid, userid int64
		var xid, who, target, dt, noise, format string
		scanordie(rows, &chonkid, &userid, &xid, &who, &target, &dt, &noise, &format)
		chonkids[chonkid] = true
		doordie(tx, "insert into chonks (chonkid, userid, xid, who, target, dt, noise, format) values (?, ?, ?, ?, ?, ?, ?, ?)", chonkid, userid, xid, who, target, dt, noise, format)
	}
	rows.Close()
	for c := range chonkids {
		rows = qordie(orig, "select honkid, chonkid, fileid from donks where chonkid = ?", c)
		for rows.Next() {
			var honkid, chonkid, fileid int64
			scanordie(rows, &honkid, &chonkid, &fileid)
			fileids[fileid] = true
			doordie(tx, "insert into donks (honkid, chonkid, fileid) values (?, ?, ?)", honkid, chonkid, fileid)
		}
		rows.Close()
	}
	filexids := make(map[string]bool)
	for f := range fileids {
		rows = qordie(orig, "select fileid, xid, name, description, url, media, local from filemeta where fileid = ?", f)
		for rows.Next() {
			var fileid int64
			var xid, name, description, url, media string
			var local int64
			scanordie(rows, &fileid, &xid, &name, &description, &url, &media, &local)
			filexids[xid] = true
			doordie(tx, "insert into filemeta (fileid, xid, name, description, url, media, local) values (?, ?, ?, ?, ?, ?, ?)", fileid, xid, name, description, url, media, local)
		}
		rows.Close()
	}

	rows = qordie(orig, "select key, value from config")
	for rows.Next() {
		var key string
		var value interface{}
		scanordie(rows, &key, &value)
		doordie(tx, "insert into config (key, value) values (?, ?)", key, value)
	}

	err = tx.Commit()
	if err != nil {
		elog.Fatalf("can't commit backp: %s", err)
	}
	backup.Close()

	backupblobname := fmt.Sprintf("%s/blob-%d.db", dirname, now)
	blob, err := sql.Open("sqlite3", backupblobname)
	if err != nil {
		elog.Fatalf("can't open backup blob database")
	}
	doordie(blob, "create table filedata (xid text, media text, hash text, content blob)")
	doordie(blob, "create index idx_filexid on filedata(xid)")
	doordie(blob, "create index idx_filehash on filedata(hash)")
	tx, err = blob.Begin()
	if err != nil {
		elog.Fatalf("can't start transaction: %s", err)
	}
	origblob := openblobdb()
	for x := range filexids {
		rows = qordie(origblob, "select xid, media, hash, content from filedata where xid = ?", x)
		for rows.Next() {
			var xid, media, hash string
			var content sql.RawBytes
			scanordie(rows, &xid, &media, &hash, &content)
			doordie(tx, "insert into filedata (xid, media, hash, content) values (?, ?, ?, ?)", xid, media, hash, content)
		}
		rows.Close()
	}

	err = tx.Commit()
	if err != nil {
		elog.Fatalf("can't commit blobs: %s", err)
	}
	blob.Close()
}
