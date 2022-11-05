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
		rows = queryDB(orig, "select honkid, userid, what, honker, xid, rid, dt, url, audience, noise, convoy, whofore, format, precis, oonker, flags from honks where convoy = ?", c)
		for rows.Next() {
			var honkid, userid int64
			var what, honker, xid, rid, dt, url, audience, noise, convoy string
			var whofore int64
			var format, precis, oonker string
			var flags int64
			scanDBRow(rows, &honkid, &userid, &what, &honker, &xid, &rid, &dt, &url, &audience, &noise, &convoy, &whofore, &format, &precis, &oonker, &flags)
			honkids[honkid] = true
			sqlMustQuery(tx, "insert into honks (honkid, userid, what, honker, xid, rid, dt, url, audience, noise, convoy, whofore, format, precis, oonker, flags) values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)", honkid, userid, what, honker, xid, rid, dt, url, audience, noise, convoy, whofore, format, precis, oonker, flags)
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
		rows = queryDB(orig, "select fileid, xid, name, description, url, media, local from filemeta where fileid = ?", f)
		for rows.Next() {
			var fileid int64
			var xid, name, description, url, media string
			var local int64
			scanDBRow(rows, &fileid, &xid, &name, &description, &url, &media, &local)
			filexids[xid] = true
			sqlMustQuery(tx, "insert into filemeta (fileid, xid, name, description, url, media, local) values (?, ?, ?, ?, ?, ?, ?)", fileid, xid, name, description, url, media, local)
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
	backup.Close()

	backupblobname := fmt.Sprintf("%s/blob-%d.db", dirname, now)
	blob, err := sql.Open("sqlite3", backupblobname)
	if err != nil {
		elog.Fatalf("can't open backup blob database")
	}
	sqlMustQuery(blob, "create table filedata (xid text, media text, hash text, content blob)")
	sqlMustQuery(blob, "create index idx_filexid on filedata(xid)")
	sqlMustQuery(blob, "create index idx_filehash on filedata(hash)")
	tx, err = blob.Begin()
	if err != nil {
		elog.Fatalf("can't start transaction: %s", err)
	}
	origblob := openblobdb()
	for x := range filexids {
		rows = queryDB(origblob, "select xid, media, hash, content from filedata where xid = ?", x)
		for rows.Next() {
			var xid, media, hash string
			var content sql.RawBytes
			scanDBRow(rows, &xid, &media, &hash, &content)
			sqlMustQuery(tx, "insert into filedata (xid, media, hash, content) values (?, ?, ?, ?)", xid, media, hash, content)
		}
		rows.Close()
	}

	err = tx.Commit()
	if err != nil {
		elog.Fatalf("can't commit blobs: %s", err)
	}
	blob.Close()
}
