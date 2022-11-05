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
	"bytes"
	"crypto/sha512"
	"database/sql"
	_ "embed"
	"encoding/json"
	"fmt"
	"html/template"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"humungus.tedunangst.com/r/webs/cache"
	"humungus.tedunangst.com/r/webs/httpsig"
	"humungus.tedunangst.com/r/webs/login"
	"humungus.tedunangst.com/r/webs/mz"
)

//go:embed schema.sql
var sqlSchema string

func userfromrow(row *sql.Row) (*WhatAbout, error) {
	user := new(WhatAbout)
	var seckey, options string
	err := row.Scan(&user.ID, &user.Name, &user.Display, &user.About, &user.Key, &seckey, &options)
	if err == nil {
		user.SecKey, _, err = httpsig.DecodeKey(seckey)
	}
	if err != nil {
		return nil, err
	}
	if user.ID > 0 {
		user.URL = fmt.Sprintf("https://%s/%s/%s", serverName, userSep, user.Name)
		err = unjsonify(options, &user.Options)
		if err != nil {
			elog.Printf("error processing user options: %s", err)
		}
	} else {
		user.URL = fmt.Sprintf("https://%s/%s", serverName, user.Name)
	}
	if user.Options.Reaction == "" {
		user.Options.Reaction = "none"
	}

	return user, nil
}

var somenamedusers = cache.New(cache.Options{Filler: func(name string) (*WhatAbout, bool) {
	row := stmtUserByName.QueryRow(name)
	user, err := userfromrow(row)
	if err != nil {
		return nil, false
	}
	var marker mz.Marker
	marker.HashLinker = ontoreplacer
	marker.AtLinker = attoreplacer
	user.HTAbout = template.HTML(marker.Mark(user.About))
	user.Onts = marker.HashTags
	return user, true
}})

var somenumberedusers = cache.New(cache.Options{Filler: func(userid int64) (*WhatAbout, bool) {
	row := stmtUserByNumber.QueryRow(userid)
	user, err := userfromrow(row)
	if err != nil {
		return nil, false
	}
	// don't touch attoreplacer, which introduces a loop
	// finger -> getjunk -> keys -> users
	return user, true
}})

func getserveruser() *WhatAbout {
	var user *WhatAbout
	ok := somenumberedusers.Get(serverUID, &user)
	if !ok {
		elog.Panicf("lost server user")
	}
	return user
}

func butwhatabout(name string) (*WhatAbout, error) {
	var user *WhatAbout
	ok := somenamedusers.Get(name, &user)
	if !ok {
		return nil, fmt.Errorf("no user: %s", name)
	}
	return user, nil
}

var honkerinvalidator cache.Invalidator

func gethonkers(userid int64) []*Honker {
	rows, err := stmtHonkers.Query(userid)
	if err != nil {
		elog.Printf("error querying honkers: %s", err)
		return nil
	}
	defer rows.Close()
	var honkers []*Honker
	for rows.Next() {
		h := new(Honker)
		var combos, meta string
		err = rows.Scan(&h.ID, &h.UserID, &h.Name, &h.XID, &h.Flavor, &combos, &meta)
		if err == nil {
			err = unjsonify(meta, &h.Meta)
		}
		if err != nil {
			elog.Printf("error scanning honker: %s", err)
			continue
		}
		h.Combos = strings.Split(strings.TrimSpace(combos), " ")
		honkers = append(honkers, h)
	}
	return honkers
}

func getdubs(userid int64) []*Honker {
	rows, err := stmtDubbers.Query(userid)
	return dubsfromrows(rows, err)
}

func getnameddubs(userid int64, name string) []*Honker {
	rows, err := stmtNamedDubbers.Query(userid, name)
	return dubsfromrows(rows, err)
}

func dubsfromrows(rows *sql.Rows, err error) []*Honker {
	if err != nil {
		elog.Printf("error querying dubs: %s", err)
		return nil
	}
	defer rows.Close()
	var honkers []*Honker
	for rows.Next() {
		h := new(Honker)
		err = rows.Scan(&h.ID, &h.UserID, &h.Name, &h.XID, &h.Flavor)
		if err != nil {
			elog.Printf("error scanning honker: %s", err)
			return nil
		}
		honkers = append(honkers, h)
	}
	return honkers
}

func allusers() []login.UserInfo {
	var users []login.UserInfo
	rows, _ := opendatabase().Query("select userid, username from users where userid > 0")
	defer rows.Close()
	for rows.Next() {
		var u login.UserInfo
		rows.Scan(&u.UserID, &u.Username)
		users = append(users, u)
	}
	return users
}

func getxonk(userid int64, xid string) *Honk {
	row := stmtOneXonk.QueryRow(userid, xid)
	return scanhonk(row)
}

func getbonk(userid int64, xid string) *Honk {
	row := stmtOneBonk.QueryRow(userid, xid)
	return scanhonk(row)
}

func getpublichonks() []*Honk {
	dt := time.Now().Add(-7 * 24 * time.Hour).UTC().Format(dbtimeformat)
	rows, err := stmtPublicHonks.Query(dt, 100)
	return getsomehonks(rows, err)
}
func geteventhonks(userid int64) []*Honk {
	rows, err := stmtEventHonks.Query(userid, 25)
	honks := getsomehonks(rows, err)
	sort.Slice(honks, func(i, j int) bool {
		var t1, t2 time.Time
		if honks[i].Time == nil {
			t1 = honks[i].Date
		} else {
			t1 = honks[i].Time.StartTime
		}
		if honks[j].Time == nil {
			t2 = honks[j].Date
		} else {
			t2 = honks[j].Time.StartTime
		}
		return t1.After(t2)
	})
	now := time.Now().Add(-24 * time.Hour)
	for i, h := range honks {
		t := h.Date
		if tm := h.Time; tm != nil {
			t = tm.StartTime
		}
		if t.Before(now) {
			honks = honks[:i]
			break
		}
	}
	reversehonks(honks)
	return honks
}
func gethonksbyuser(name string, includeprivate bool, wanted int64) []*Honk {
	dt := time.Now().Add(-7 * 24 * time.Hour).UTC().Format(dbtimeformat)
	limit := 50
	whofore := 2
	if includeprivate {
		whofore = 3
	}
	rows, err := stmtUserHonks.Query(wanted, whofore, name, dt, limit)
	return getsomehonks(rows, err)
}
func gethonksforuser(userid int64, wanted int64) []*Honk {
	dt := time.Now().Add(-7 * 24 * time.Hour).UTC().Format(dbtimeformat)
	rows, err := stmtHonksForUser.Query(wanted, userid, dt, userid, userid)
	return getsomehonks(rows, err)
}
func gethonksforuserfirstclass(userid int64, wanted int64) []*Honk {
	dt := time.Now().Add(-7 * 24 * time.Hour).UTC().Format(dbtimeformat)
	rows, err := stmtHonksForUserFirstClass.Query(wanted, userid, dt, userid, userid)
	return getsomehonks(rows, err)
}

func gethonksforme(userid int64, wanted int64) []*Honk {
	dt := time.Now().Add(-7 * 24 * time.Hour).UTC().Format(dbtimeformat)
	rows, err := stmtHonksForMe.Query(wanted, userid, dt, userid)
	return getsomehonks(rows, err)
}
func gethonksfromlongago(userid int64, wanted int64) []*Honk {
	now := time.Now()
	var honks []*Honk
	for i := 1; i <= 3; i++ {
		dt := time.Date(now.Year()-i, now.Month(), now.Day(), now.Hour(), now.Minute(),
			now.Second(), 0, now.Location())
		dt1 := dt.Add(-36 * time.Hour).UTC().Format(dbtimeformat)
		dt2 := dt.Add(12 * time.Hour).UTC().Format(dbtimeformat)
		rows, err := stmtHonksFromLongAgo.Query(wanted, userid, dt1, dt2, userid)
		honks = append(honks, getsomehonks(rows, err)...)
	}
	return honks
}
func getsavedhonks(userid int64, wanted int64) []*Honk {
	rows, err := stmtHonksISaved.Query(wanted, userid)
	return getsomehonks(rows, err)
}
func gethonksbyhonker(userid int64, honker string, wanted int64) []*Honk {
	rows, err := stmtHonksByHonker.Query(wanted, userid, honker, userid)
	return getsomehonks(rows, err)
}
func gethonksbyxonker(userid int64, xonker string, wanted int64) []*Honk {
	rows, err := stmtHonksByXonker.Query(wanted, userid, xonker, xonker, userid)
	return getsomehonks(rows, err)
}
func gethonksbycombo(userid int64, combo string, wanted int64) []*Honk {
	combo = "% " + combo + " %"
	rows, err := stmtHonksByCombo.Query(wanted, userid, userid, combo, userid, wanted, userid, combo, userid)
	return getsomehonks(rows, err)
}
func gethonksbyconvoy(userid int64, convoy string, wanted int64) []*Honk {
	rows, err := stmtHonksByConvoy.Query(wanted, userid, userid, convoy)
	honks := getsomehonks(rows, err)
	return honks
}
func gethonksbysearch(userid int64, q string, wanted int64) []*Honk {
	var queries []string
	var params []interface{}
	queries = append(queries, "honks.honkid > ?")
	params = append(params, wanted)
	queries = append(queries, "honks.userid = ?")
	params = append(params, userid)

	terms := strings.Split(q, " ")
	for _, t := range terms {
		if t == "" {
			continue
		}
		negate := " "
		if t[0] == '-' {
			t = t[1:]
			negate = " not "
		}
		if t == "" {
			continue
		}
		if strings.HasPrefix(t, "site:") {
			site := t[5:]
			site = "%" + site + "%"
			queries = append(queries, "xid"+negate+"like ?")
			params = append(params, site)
			continue
		}
		if strings.HasPrefix(t, "honker:") {
			honker := t[7:]
			xid := fullname(honker, userid)
			if xid != "" {
				honker = xid
			}
			queries = append(queries, negate+"(honks.honker = ? or honks.oonker = ?)")
			params = append(params, honker)
			params = append(params, honker)
			continue
		}
		t = "%" + t + "%"
		queries = append(queries, "noise"+negate+"like ?")
		params = append(params, t)
	}

	selecthonks := "select honks.honkid, honks.userid, username, what, honker, oonker, honks.xid, rid, dt, url, audience, noise, precis, format, convoy, whofore, flags from honks join users on honks.userid = users.userid "
	where := "where " + strings.Join(queries, " and ")
	butnotthose := " and convoy not in (select name from zonkers where userid = ? and wherefore = 'zonvoy' order by zonkerid desc limit 100)"
	limit := " order by honks.honkid desc limit 250"
	params = append(params, userid)
	rows, err := opendatabase().Query(selecthonks+where+butnotthose+limit, params...)
	honks := getsomehonks(rows, err)
	return honks
}
func gethonksbyontology(userid int64, name string, wanted int64) []*Honk {
	rows, err := stmtHonksByOntology.Query(wanted, name, userid, userid)
	honks := getsomehonks(rows, err)
	return honks
}

func reversehonks(honks []*Honk) {
	for i, j := 0, len(honks)-1; i < j; i, j = i+1, j-1 {
		honks[i], honks[j] = honks[j], honks[i]
	}
}

func getsomehonks(rows *sql.Rows, err error) []*Honk {
	if err != nil {
		elog.Printf("error querying honks: %s", err)
		return nil
	}
	defer rows.Close()
	var honks []*Honk
	for rows.Next() {
		h := scanhonk(rows)
		if h != nil {
			honks = append(honks, h)
		}
	}
	rows.Close()
	donksforhonks(honks)
	return honks
}

type RowLike interface {
	Scan(dest ...interface{}) error
}

func scanhonk(row RowLike) *Honk {
	h := new(Honk)
	var dt, aud string
	err := row.Scan(&h.ID, &h.UserID, &h.Username, &h.What, &h.Honker, &h.Oonker, &h.XID, &h.RID,
		&dt, &h.URL, &aud, &h.Noise, &h.Precis, &h.Format, &h.Convoy, &h.Whofore, &h.Flags)
	if err != nil {
		if err != sql.ErrNoRows {
			elog.Printf("error scanning honk: %s", err)
		}
		return nil
	}
	h.Date, _ = time.Parse(dbtimeformat, dt)
	h.Audience = strings.Split(aud, " ")
	h.Public = loudandproud(h.Audience)
	return h
}

func donksforhonks(honks []*Honk) {
	db := opendatabase()
	var ids []string
	hmap := make(map[int64]*Honk)
	for _, h := range honks {
		ids = append(ids, fmt.Sprintf("%d", h.ID))
		hmap[h.ID] = h
	}
	idset := strings.Join(ids, ",")
	// grab donks
	q := fmt.Sprintf("select honkid, donks.fileid, xid, name, description, url, media, local from donks join filemeta on donks.fileid = filemeta.fileid where honkid in (%s)", idset)
	rows, err := db.Query(q)
	if err != nil {
		elog.Printf("error querying donks: %s", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var hid int64
		d := new(Donk)
		err = rows.Scan(&hid, &d.FileID, &d.XID, &d.Name, &d.Desc, &d.URL, &d.Media, &d.Local)
		if err != nil {
			elog.Printf("error scanning donk: %s", err)
			continue
		}
		d.External = !strings.HasPrefix(d.URL, serverPrefix)
		h := hmap[hid]
		h.Donks = append(h.Donks, d)
	}
	rows.Close()

	// grab onts
	q = fmt.Sprintf("select honkid, ontology from onts where honkid in (%s)", idset)
	rows, err = db.Query(q)
	if err != nil {
		elog.Printf("error querying onts: %s", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var hid int64
		var o string
		err = rows.Scan(&hid, &o)
		if err != nil {
			elog.Printf("error scanning donk: %s", err)
			continue
		}
		h := hmap[hid]
		h.Onts = append(h.Onts, o)
	}
	rows.Close()

	// grab meta
	q = fmt.Sprintf("select honkid, genus, json from honkmeta where honkid in (%s)", idset)
	rows, err = db.Query(q)
	if err != nil {
		elog.Printf("error querying honkmeta: %s", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var hid int64
		var genus, j string
		err = rows.Scan(&hid, &genus, &j)
		if err != nil {
			elog.Printf("error scanning honkmeta: %s", err)
			continue
		}
		h := hmap[hid]
		switch genus {
		case "place":
			p := new(Place)
			err = unjsonify(j, p)
			if err != nil {
				elog.Printf("error parsing place: %s", err)
				continue
			}
			h.Place = p
		case "time":
			t := new(Time)
			err = unjsonify(j, t)
			if err != nil {
				elog.Printf("error parsing time: %s", err)
				continue
			}
			h.Time = t
		case "mentions":
			err = unjsonify(j, &h.Mentions)
			if err != nil {
				elog.Printf("error parsing mentions: %s", err)
				continue
			}
		case "badonks":
			err = unjsonify(j, &h.Badonks)
			if err != nil {
				elog.Printf("error parsing badonks: %s", err)
				continue
			}
		case "wonkles":
			h.Wonkles = j
		case "guesses":
			h.Guesses = template.HTML(j)
		case "oldrev":
		default:
			elog.Printf("unknown meta genus: %s", genus)
		}
	}
	rows.Close()
}

func donksforchonks(chonks []*Chonk) {
	db := opendatabase()
	var ids []string
	chmap := make(map[int64]*Chonk)
	for _, ch := range chonks {
		ids = append(ids, fmt.Sprintf("%d", ch.ID))
		chmap[ch.ID] = ch
	}
	idset := strings.Join(ids, ",")
	// grab donks
	q := fmt.Sprintf("select chonkid, donks.fileid, xid, name, description, url, media, local from donks join filemeta on donks.fileid = filemeta.fileid where chonkid in (%s)", idset)
	rows, err := db.Query(q)
	if err != nil {
		elog.Printf("error querying donks: %s", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var chid int64
		d := new(Donk)
		err = rows.Scan(&chid, &d.FileID, &d.XID, &d.Name, &d.Desc, &d.URL, &d.Media, &d.Local)
		if err != nil {
			elog.Printf("error scanning donk: %s", err)
			continue
		}
		ch := chmap[chid]
		ch.Donks = append(ch.Donks, d)
	}
}

func savefile(name string, desc string, url string, media string, local bool, data []byte) (int64, error) {
	fileid, _, err := savefileandxid(name, desc, url, media, local, data)
	return fileid, err
}

func hashfiledata(data []byte) string {
	h := sha512.New512_256()
	h.Write(data)
	return fmt.Sprintf("%x", h.Sum(nil))
}

func savefileandxid(name string, desc string, url string, media string, local bool, data []byte) (int64, string, error) {
	var xid string
	if local {
		hash := hashfiledata(data)
		row := stmtCheckFileData.QueryRow(hash)
		err := row.Scan(&xid)
		if err == sql.ErrNoRows {
			xid = xfiltrate()
			switch media {
			case "image/png":
				xid += ".png"
			case "image/jpeg":
				xid += ".jpg"
			case "application/pdf":
				xid += ".pdf"
			case "text/plain":
				xid += ".txt"
			}
			_, err = stmtSaveFileData.Exec(xid, media, hash, data)
			if err != nil {
				return 0, "", err
			}
		} else if err != nil {
			elog.Printf("error checking file hash: %s", err)
			return 0, "", err
		}
		if url == "" {
			url = fmt.Sprintf("https://%s/d/%s", serverName, xid)
		}
	}

	res, err := stmtSaveFile.Exec(xid, name, desc, url, media, local)
	if err != nil {
		return 0, "", err
	}
	fileid, _ := res.LastInsertId()
	return fileid, xid, nil
}

func finddonk(url string) *Donk {
	donk := new(Donk)
	row := stmtFindFile.QueryRow(url)
	err := row.Scan(&donk.FileID, &donk.XID)
	if err == nil {
		return donk
	}
	if err != sql.ErrNoRows {
		elog.Printf("error finding file: %s", err)
	}
	return nil
}

func savechonk(ch *Chonk) error {
	dt := ch.Date.UTC().Format(dbtimeformat)
	db := opendatabase()
	tx, err := db.Begin()
	if err != nil {
		elog.Printf("can't begin tx: %s", err)
		return err
	}

	res, err := tx.Stmt(stmtSaveChonk).Exec(ch.UserID, ch.XID, ch.Who, ch.Target, dt, ch.Noise, ch.Format)
	if err == nil {
		ch.ID, _ = res.LastInsertId()
		for _, d := range ch.Donks {
			_, err := tx.Stmt(stmtSaveDonk).Exec(-1, ch.ID, d.FileID)
			if err != nil {
				elog.Printf("error saving donk: %s", err)
				break
			}
		}
		chatplusone(tx, ch.UserID)
		err = tx.Commit()
	} else {
		tx.Rollback()
	}
	return err
}

func chatplusone(tx *sql.Tx, userid int64) {
	var user *WhatAbout
	ok := somenumberedusers.Get(userid, &user)
	if !ok {
		return
	}
	options := user.Options
	options.ChatCount += 1
	j, err := jsonify(options)
	if err == nil {
		_, err = tx.Exec("update users set options = ? where username = ?", j, user.Name)
	}
	if err != nil {
		elog.Printf("error plussing chat: %s", err)
	}
	somenamedusers.Clear(user.Name)
	somenumberedusers.Clear(user.ID)
}

func chatnewnone(userid int64) {
	var user *WhatAbout
	ok := somenumberedusers.Get(userid, &user)
	if !ok || user.Options.ChatCount == 0 {
		return
	}
	options := user.Options
	options.ChatCount = 0
	j, err := jsonify(options)
	if err == nil {
		db := opendatabase()
		_, err = db.Exec("update users set options = ? where username = ?", j, user.Name)
	}
	if err != nil {
		elog.Printf("error noneing chat: %s", err)
	}
	somenamedusers.Clear(user.Name)
	somenumberedusers.Clear(user.ID)
}

func meplusone(tx *sql.Tx, userid int64) {
	var user *WhatAbout
	ok := somenumberedusers.Get(userid, &user)
	if !ok {
		return
	}
	options := user.Options
	options.MeCount += 1
	j, err := jsonify(options)
	if err == nil {
		_, err = tx.Exec("update users set options = ? where username = ?", j, user.Name)
	}
	if err != nil {
		elog.Printf("error plussing me: %s", err)
	}
	somenamedusers.Clear(user.Name)
	somenumberedusers.Clear(user.ID)
}

func menewnone(userid int64) {
	var user *WhatAbout
	ok := somenumberedusers.Get(userid, &user)
	if !ok || user.Options.MeCount == 0 {
		return
	}
	options := user.Options
	options.MeCount = 0
	j, err := jsonify(options)
	if err == nil {
		db := opendatabase()
		_, err = db.Exec("update users set options = ? where username = ?", j, user.Name)
	}
	if err != nil {
		elog.Printf("error noneing me: %s", err)
	}
	somenamedusers.Clear(user.Name)
	somenumberedusers.Clear(user.ID)
}

func loadchatter(userid int64) []*Chatter {
	duedt := time.Now().Add(-3 * 24 * time.Hour).UTC().Format(dbtimeformat)
	rows, err := stmtLoadChonks.Query(userid, duedt)
	if err != nil {
		elog.Printf("error loading chonks: %s", err)
		return nil
	}
	defer rows.Close()
	chonks := make(map[string][]*Chonk)
	var allchonks []*Chonk
	for rows.Next() {
		ch := new(Chonk)
		var dt string
		err = rows.Scan(&ch.ID, &ch.UserID, &ch.XID, &ch.Who, &ch.Target, &dt, &ch.Noise, &ch.Format)
		if err != nil {
			elog.Printf("error scanning chonk: %s", err)
			continue
		}
		ch.Date, _ = time.Parse(dbtimeformat, dt)
		chonks[ch.Target] = append(chonks[ch.Target], ch)
		allchonks = append(allchonks, ch)
	}
	donksforchonks(allchonks)
	rows.Close()
	rows, err = stmtGetChatters.Query(userid)
	if err != nil {
		elog.Printf("error getting chatters: %s", err)
		return nil
	}
	for rows.Next() {
		var target string
		err = rows.Scan(&target)
		if err != nil {
			elog.Printf("error scanning chatter: %s", target)
			continue
		}
		if _, ok := chonks[target]; !ok {
			chonks[target] = []*Chonk{}

		}
	}
	var chatter []*Chatter
	for target, chonks := range chonks {
		chatter = append(chatter, &Chatter{
			Target: target,
			Chonks: chonks,
		})
	}
	sort.Slice(chatter, func(i, j int) bool {
		a, b := chatter[i], chatter[j]
		if len(a.Chonks) == 0 || len(b.Chonks) == 0 {
			if len(a.Chonks) == len(b.Chonks) {
				return a.Target < b.Target
			}
			return len(a.Chonks) > len(b.Chonks)
		}
		return a.Chonks[len(a.Chonks)-1].Date.After(b.Chonks[len(b.Chonks)-1].Date)
	})

	return chatter
}

func savehonk(h *Honk) error {
	dt := h.Date.UTC().Format(dbtimeformat)
	aud := strings.Join(h.Audience, " ")

	db := opendatabase()
	tx, err := db.Begin()
	if err != nil {
		elog.Printf("can't begin tx: %s", err)
		return err
	}

	res, err := tx.Stmt(stmtSaveHonk).Exec(h.UserID, h.What, h.Honker, h.XID, h.RID, dt, h.URL,
		aud, h.Noise, h.Convoy, h.Whofore, h.Format, h.Precis,
		h.Oonker, h.Flags)
	if err == nil {
		h.ID, _ = res.LastInsertId()
		err = saveextras(tx, h)
	}
	if err == nil {
		if h.Whofore == 1 {
			meplusone(tx, h.UserID)
		}
		err = tx.Commit()
	} else {
		tx.Rollback()
	}
	if err != nil {
		elog.Printf("error saving honk: %s", err)
	}
	honkhonkline()
	return err
}

func updatehonk(h *Honk) error {
	old := getxonk(h.UserID, h.XID)
	oldrev := OldRevision{Precis: old.Precis, Noise: old.Noise}
	dt := h.Date.UTC().Format(dbtimeformat)

	db := opendatabase()
	tx, err := db.Begin()
	if err != nil {
		elog.Printf("can't begin tx: %s", err)
		return err
	}

	err = deleteextras(tx, h.ID, false)
	if err == nil {
		_, err = tx.Stmt(stmtUpdateHonk).Exec(h.Precis, h.Noise, h.Format, h.Whofore, dt, h.ID)
	}
	if err == nil {
		err = saveextras(tx, h)
	}
	if err == nil {
		var j string
		j, err = jsonify(&oldrev)
		if err == nil {
			_, err = tx.Stmt(stmtSaveMeta).Exec(old.ID, "oldrev", j)
		}
		if err != nil {
			elog.Printf("error saving oldrev: %s", err)
		}
	}
	if err == nil {
		err = tx.Commit()
	} else {
		tx.Rollback()
	}
	if err != nil {
		elog.Printf("error updating honk %d: %s", h.ID, err)
	}
	return err
}

func deletehonk(honkid int64) error {
	db := opendatabase()
	tx, err := db.Begin()
	if err != nil {
		elog.Printf("can't begin tx: %s", err)
		return err
	}

	err = deleteextras(tx, honkid, true)
	if err == nil {
		_, err = tx.Stmt(stmtDeleteHonk).Exec(honkid)
	}
	if err == nil {
		err = tx.Commit()
	} else {
		tx.Rollback()
	}
	if err != nil {
		elog.Printf("error deleting honk %d: %s", honkid, err)
	}
	return err
}

func saveextras(tx *sql.Tx, h *Honk) error {
	for _, d := range h.Donks {
		_, err := tx.Stmt(stmtSaveDonk).Exec(h.ID, -1, d.FileID)
		if err != nil {
			elog.Printf("error saving donk: %s", err)
			return err
		}
	}
	for _, o := range h.Onts {
		_, err := tx.Stmt(stmtSaveOnt).Exec(strings.ToLower(o), h.ID)
		if err != nil {
			elog.Printf("error saving ont: %s", err)
			return err
		}
	}
	if p := h.Place; p != nil {
		j, err := jsonify(p)
		if err == nil {
			_, err = tx.Stmt(stmtSaveMeta).Exec(h.ID, "place", j)
		}
		if err != nil {
			elog.Printf("error saving place: %s", err)
			return err
		}
	}
	if t := h.Time; t != nil {
		j, err := jsonify(t)
		if err == nil {
			_, err = tx.Stmt(stmtSaveMeta).Exec(h.ID, "time", j)
		}
		if err != nil {
			elog.Printf("error saving time: %s", err)
			return err
		}
	}
	if m := h.Mentions; len(m) > 0 {
		j, err := jsonify(m)
		if err == nil {
			_, err = tx.Stmt(stmtSaveMeta).Exec(h.ID, "mentions", j)
		}
		if err != nil {
			elog.Printf("error saving mentions: %s", err)
			return err
		}
	}
	if w := h.Wonkles; w != "" {
		_, err := tx.Stmt(stmtSaveMeta).Exec(h.ID, "wonkles", w)
		if err != nil {
			elog.Printf("error saving wonkles: %s", err)
			return err
		}
	}
	if g := h.Guesses; g != "" {
		_, err := tx.Stmt(stmtSaveMeta).Exec(h.ID, "guesses", g)
		if err != nil {
			elog.Printf("error saving guesses: %s", err)
			return err
		}
	}
	return nil
}

var baxonker sync.Mutex

func addreaction(user *WhatAbout, xid string, who, react string) {
	baxonker.Lock()
	defer baxonker.Unlock()
	h := getxonk(user.ID, xid)
	if h == nil {
		return
	}
	h.Badonks = append(h.Badonks, Badonk{Who: who, What: react})
	j, _ := jsonify(h.Badonks)
	db := opendatabase()
	tx, _ := db.Begin()
	_, _ = tx.Stmt(stmtDeleteOneMeta).Exec(h.ID, "badonks")
	_, _ = tx.Stmt(stmtSaveMeta).Exec(h.ID, "badonks", j)
	tx.Commit()
}

func deleteextras(tx *sql.Tx, honkid int64, everything bool) error {
	_, err := tx.Stmt(stmtDeleteDonks).Exec(honkid)
	if err != nil {
		return err
	}
	_, err = tx.Stmt(stmtDeleteOnts).Exec(honkid)
	if err != nil {
		return err
	}
	if everything {
		_, err = tx.Stmt(stmtDeleteAllMeta).Exec(honkid)
	} else {
		_, err = tx.Stmt(stmtDeleteSomeMeta).Exec(honkid)
	}
	if err != nil {
		return err
	}
	return nil
}

func jsonify(what interface{}) (string, error) {
	var buf bytes.Buffer
	e := json.NewEncoder(&buf)
	e.SetEscapeHTML(false)
	e.SetIndent("", "")
	err := e.Encode(what)
	return buf.String(), err
}

func unjsonify(s string, dest interface{}) error {
	d := json.NewDecoder(strings.NewReader(s))
	err := d.Decode(dest)
	return err
}

func getxonker(what, flav string) string {
	var res string
	row := stmtGetXonker.QueryRow(what, flav)
	row.Scan(&res)
	return res
}

func savexonker(what, value, flav, when string) {
	stmtSaveXonker.Exec(what, value, flav, when)
}

func savehonker(user *WhatAbout, url, name, flavor, combos, mj string) error {
	var owner string
	if url[0] == '#' {
		flavor = "peep"
		if name == "" {
			name = url[1:]
		}
		owner = url
	} else {
		info, err := investigate(url)
		if err != nil {
			ilog.Printf("failed to investigate honker: %s", err)
			return err
		}
		url = info.XID
		if name == "" {
			name = info.Name
		}
		owner = info.Owner
	}

	var x string
	db := opendatabase()
	row := db.QueryRow("select xid from honkers where xid = ? and userid = ? and flavor in ('sub', 'unsub', 'peep')", url, user.ID)
	err := row.Scan(&x)
	if err != sql.ErrNoRows {
		if err != nil {
			elog.Printf("honker scan err: %s", err)
		} else {
			err = fmt.Errorf("it seems you are already subscribed to them")
		}
		return err
	}

	res, err := stmtSaveHonker.Exec(user.ID, name, url, flavor, combos, owner, mj)
	if err != nil {
		elog.Print(err)
		return err
	}
	honkerid, _ := res.LastInsertId()
	if flavor == "presub" {
		followyou(user, honkerid)
	}
	return nil
}

func cleanupdb(arg string) {
	db := opendatabase()
	days, err := strconv.Atoi(arg)
	var sqlargs []interface{}
	var where string
	if err != nil {
		honker := arg
		expdate := time.Now().Add(-3 * 24 * time.Hour).UTC().Format(dbtimeformat)
		where = "dt < ? and honker = ?"
		sqlargs = append(sqlargs, expdate)
		sqlargs = append(sqlargs, honker)
	} else {
		expdate := time.Now().Add(-time.Duration(days) * 24 * time.Hour).UTC().Format(dbtimeformat)
		where = "dt < ? and convoy not in (select convoy from honks where flags & 4 or whofore = 2 or whofore = 3)"
		sqlargs = append(sqlargs, expdate)
	}
	doordie(db, "delete from honks where flags & 4 = 0 and whofore = 0 and "+where, sqlargs...)
	doordie(db, "delete from donks where honkid > 0 and honkid not in (select honkid from honks)")
	doordie(db, "delete from onts where honkid not in (select honkid from honks)")
	doordie(db, "delete from honkmeta where honkid not in (select honkid from honks)")

	doordie(db, "delete from filemeta where fileid not in (select fileid from donks)")
	for _, u := range allusers() {
		doordie(db, "delete from zonkers where userid = ? and wherefore = 'zonvoy' and zonkerid < (select zonkerid from zonkers where userid = ? and wherefore = 'zonvoy' order by zonkerid desc limit 1 offset 200)", u.UserID, u.UserID)
	}

	filexids := make(map[string]bool)
	blobdb := openblobdb()
	rows, err := blobdb.Query("select xid from filedata")
	if err != nil {
		elog.Fatal(err)
	}
	for rows.Next() {
		var xid string
		err = rows.Scan(&xid)
		if err != nil {
			elog.Fatal(err)
		}
		filexids[xid] = true
	}
	rows.Close()
	rows, err = db.Query("select xid from filemeta")
	for rows.Next() {
		var xid string
		err = rows.Scan(&xid)
		if err != nil {
			elog.Fatal(err)
		}
		delete(filexids, xid)
	}
	rows.Close()
	tx, err := blobdb.Begin()
	if err != nil {
		elog.Fatal(err)
	}
	for xid, _ := range filexids {
		_, err = tx.Exec("delete from filedata where xid = ?", xid)
		if err != nil {
			elog.Fatal(err)
		}
	}
	err = tx.Commit()
	if err != nil {
		elog.Fatal(err)
	}
}

var stmtHonkers, stmtDubbers, stmtNamedDubbers, stmtSaveHonker, stmtUpdateFlavor, stmtUpdateHonker *sql.Stmt
var stmtDeleteHonker *sql.Stmt
var stmtAnyXonk, stmtOneXonk, stmtPublicHonks, stmtUserHonks, stmtHonksByCombo, stmtHonksByConvoy *sql.Stmt
var stmtHonksByOntology, stmtHonksForUser, stmtHonksForMe, stmtSaveDub, stmtHonksByXonker *sql.Stmt
var stmtHonksFromLongAgo *sql.Stmt
var stmtHonksByHonker, stmtSaveHonk, stmtUserByName, stmtUserByNumber *sql.Stmt
var stmtEventHonks, stmtOneBonk, stmtFindZonk, stmtFindXonk, stmtSaveDonk *sql.Stmt
var stmtFindFile, stmtGetFileData, stmtSaveFileData, stmtSaveFile *sql.Stmt
var stmtCheckFileData *sql.Stmt
var stmtAddDoover, stmtGetDoovers, stmtLoadDoover, stmtZapDoover, stmtOneHonker *sql.Stmt
var stmtUntagged, stmtDeleteHonk, stmtDeleteDonks, stmtDeleteOnts, stmtSaveZonker *sql.Stmt
var stmtGetZonkers, stmtRecentHonkers, stmtGetXonker, stmtSaveXonker, stmtDeleteXonker, stmtDeleteOldXonkers *sql.Stmt
var stmtAllOnts, stmtSaveOnt, stmtUpdateFlags, stmtClearFlags *sql.Stmt
var stmtHonksForUserFirstClass *sql.Stmt
var stmtSaveMeta, stmtDeleteAllMeta, stmtDeleteOneMeta, stmtDeleteSomeMeta, stmtUpdateHonk *sql.Stmt
var stmtHonksISaved, stmtGetFilters, stmtSaveFilter, stmtDeleteFilter *sql.Stmt
var stmtGetTracks *sql.Stmt
var stmtSaveChonk, stmtLoadChonks, stmtGetChatters *sql.Stmt

func preparetodie(db *sql.DB, s string) *sql.Stmt {
	stmt, err := db.Prepare(s)
	if err != nil {
		elog.Fatalf("error %s: %s", err, s)
	}
	return stmt
}

func prepareStatements(db *sql.DB) {
	stmtHonkers = preparetodie(db, "select honkerid, userid, name, xid, flavor, combos, meta from honkers where userid = ? and (flavor = 'presub' or flavor = 'sub' or flavor = 'peep' or flavor = 'unsub') order by name")
	stmtSaveHonker = preparetodie(db, "insert into honkers (userid, name, xid, flavor, combos, owner, meta, folxid) values (?, ?, ?, ?, ?, ?, ?, '')")
	stmtUpdateFlavor = preparetodie(db, "update honkers set flavor = ?, folxid = ? where userid = ? and name = ? and xid = ? and flavor = ?")
	stmtUpdateHonker = preparetodie(db, "update honkers set name = ?, combos = ?, meta = ? where honkerid = ? and userid = ?")
	stmtDeleteHonker = preparetodie(db, "delete from honkers where honkerid = ?")
	stmtOneHonker = preparetodie(db, "select xid from honkers where name = ? and userid = ?")
	stmtDubbers = preparetodie(db, "select honkerid, userid, name, xid, flavor from honkers where userid = ? and flavor = 'dub'")
	stmtNamedDubbers = preparetodie(db, "select honkerid, userid, name, xid, flavor from honkers where userid = ? and name = ? and flavor = 'dub'")

	selecthonks := "select honks.honkid, honks.userid, username, what, honker, oonker, honks.xid, rid, dt, url, audience, noise, precis, format, convoy, whofore, flags from honks join users on honks.userid = users.userid "
	limit := " order by honks.honkid desc limit 250"
	smalllimit := " order by honks.honkid desc limit ?"
	butnotthose := " and convoy not in (select name from zonkers where userid = ? and wherefore = 'zonvoy' order by zonkerid desc limit 100)"
	stmtOneXonk = preparetodie(db, selecthonks+"where honks.userid = ? and xid = ?")
	stmtAnyXonk = preparetodie(db, selecthonks+"where xid = ? order by honks.honkid asc")
	stmtOneBonk = preparetodie(db, selecthonks+"where honks.userid = ? and xid = ? and what = 'bonk' and whofore = 2")
	stmtPublicHonks = preparetodie(db, selecthonks+"where whofore = 2 and dt > ?"+smalllimit)
	stmtEventHonks = preparetodie(db, selecthonks+"where (whofore = 2 or honks.userid = ?) and what = 'event'"+smalllimit)
	stmtUserHonks = preparetodie(db, selecthonks+"where honks.honkid > ? and (whofore = 2 or whofore = ?) and username = ? and dt > ?"+smalllimit)
	myhonkers := " and honker in (select xid from honkers where userid = ? and (flavor = 'sub' or flavor = 'peep' or flavor = 'presub') and combos not like '% - %')"
	stmtHonksForUser = preparetodie(db, selecthonks+"where honks.honkid > ? and honks.userid = ? and dt > ?"+myhonkers+butnotthose+limit)
	stmtHonksForUserFirstClass = preparetodie(db, selecthonks+"where honks.honkid > ? and honks.userid = ? and dt > ? and (what <> 'tonk')"+myhonkers+butnotthose+limit)
	stmtHonksForMe = preparetodie(db, selecthonks+"where honks.honkid > ? and honks.userid = ? and dt > ? and whofore = 1"+butnotthose+limit)
	stmtHonksFromLongAgo = preparetodie(db, selecthonks+"where honks.honkid > ? and honks.userid = ? and dt > ? and dt < ? and whofore = 2"+butnotthose+limit)
	stmtHonksISaved = preparetodie(db, selecthonks+"where honks.honkid > ? and honks.userid = ? and flags & 4 order by honks.honkid desc")
	stmtHonksByHonker = preparetodie(db, selecthonks+"join honkers on (honkers.xid = honks.honker or honkers.xid = honks.oonker) where honks.honkid > ? and honks.userid = ? and honkers.name = ?"+butnotthose+limit)
	stmtHonksByXonker = preparetodie(db, selecthonks+" where honks.honkid > ? and honks.userid = ? and (honker = ? or oonker = ?)"+butnotthose+limit)
	stmtHonksByCombo = preparetodie(db, selecthonks+" where honks.honkid > ? and honks.userid = ? and honks.honker in (select xid from honkers where honkers.userid = ? and honkers.combos like ?) "+butnotthose+" union "+selecthonks+"join onts on honks.honkid = onts.honkid where honks.honkid > ? and honks.userid = ? and onts.ontology in (select xid from honkers where combos like ?)"+butnotthose+limit)
	stmtHonksByConvoy = preparetodie(db, selecthonks+"where honks.honkid > ? and (honks.userid = ? or (? = -1 and whofore = 2)) and convoy = ?"+limit)
	stmtHonksByOntology = preparetodie(db, selecthonks+"join onts on honks.honkid = onts.honkid where honks.honkid > ? and onts.ontology = ? and (honks.userid = ? or (? = -1 and honks.whofore = 2))"+limit)

	stmtSaveMeta = preparetodie(db, "insert into honkmeta (honkid, genus, json) values (?, ?, ?)")
	stmtDeleteAllMeta = preparetodie(db, "delete from honkmeta where honkid = ?")
	stmtDeleteSomeMeta = preparetodie(db, "delete from honkmeta where honkid = ? and genus not in ('oldrev')")
	stmtDeleteOneMeta = preparetodie(db, "delete from honkmeta where honkid = ? and genus = ?")
	stmtSaveHonk = preparetodie(db, "insert into honks (userid, what, honker, xid, rid, dt, url, audience, noise, convoy, whofore, format, precis, oonker, flags) values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)")
	stmtDeleteHonk = preparetodie(db, "delete from honks where honkid = ?")
	stmtUpdateHonk = preparetodie(db, "update honks set precis = ?, noise = ?, format = ?, whofore = ?, dt = ? where honkid = ?")
	stmtSaveOnt = preparetodie(db, "insert into onts (ontology, honkid) values (?, ?)")
	stmtDeleteOnts = preparetodie(db, "delete from onts where honkid = ?")
	stmtSaveDonk = preparetodie(db, "insert into donks (honkid, chonkid, fileid) values (?, ?, ?)")
	stmtDeleteDonks = preparetodie(db, "delete from donks where honkid = ?")
	stmtSaveFile = preparetodie(db, "insert into filemeta (xid, name, description, url, media, local) values (?, ?, ?, ?, ?, ?)")
	blobdb := openblobdb()
	stmtSaveFileData = preparetodie(blobdb, "insert into filedata (xid, media, hash, content) values (?, ?, ?, ?)")
	stmtCheckFileData = preparetodie(blobdb, "select xid from filedata where hash = ?")
	stmtGetFileData = preparetodie(blobdb, "select media, content from filedata where xid = ?")
	stmtFindXonk = preparetodie(db, "select honkid from honks where userid = ? and xid = ?")
	stmtFindFile = preparetodie(db, "select fileid, xid from filemeta where url = ? and local = 1")
	stmtUserByName = preparetodie(db, "select userid, username, displayname, about, pubkey, seckey, options from users where username = ? and userid > 0")
	stmtUserByNumber = preparetodie(db, "select userid, username, displayname, about, pubkey, seckey, options from users where userid = ?")
	stmtSaveDub = preparetodie(db, "insert into honkers (userid, name, xid, flavor, combos, owner, meta, folxid) values (?, ?, ?, ?, '', '', '', ?)")
	stmtAddDoover = preparetodie(db, "insert into doovers (dt, tries, userid, rcpt, msg) values (?, ?, ?, ?, ?)")
	stmtGetDoovers = preparetodie(db, "select dooverid, dt from doovers")
	stmtLoadDoover = preparetodie(db, "select tries, userid, rcpt, msg from doovers where dooverid = ?")
	stmtZapDoover = preparetodie(db, "delete from doovers where dooverid = ?")
	stmtUntagged = preparetodie(db, "select xid, rid, flags from (select honkid, xid, rid, flags from honks where userid = ? order by honkid desc limit 10000) order by honkid asc")
	stmtFindZonk = preparetodie(db, "select zonkerid from zonkers where userid = ? and name = ? and wherefore = 'zonk'")
	stmtGetZonkers = preparetodie(db, "select zonkerid, name, wherefore from zonkers where userid = ? and wherefore <> 'zonk'")
	stmtSaveZonker = preparetodie(db, "insert into zonkers (userid, name, wherefore) values (?, ?, ?)")
	stmtGetXonker = preparetodie(db, "select info from xonkers where name = ? and flavor = ?")
	stmtSaveXonker = preparetodie(db, "insert into xonkers (name, info, flavor, dt) values (?, ?, ?, ?)")
	stmtDeleteXonker = preparetodie(db, "delete from xonkers where name = ? and flavor = ? and dt < ?")
	stmtDeleteOldXonkers = preparetodie(db, "delete from xonkers where flavor = ? and dt < ?")
	stmtRecentHonkers = preparetodie(db, "select distinct(honker) from honks where userid = ? and honker not in (select xid from honkers where userid = ? and flavor = 'sub') order by honkid desc limit 100")
	stmtUpdateFlags = preparetodie(db, "update honks set flags = flags | ? where honkid = ?")
	stmtClearFlags = preparetodie(db, "update honks set flags = flags & ~ ? where honkid = ?")
	stmtAllOnts = preparetodie(db, "select ontology, count(ontology) from onts join honks on onts.honkid = honks.honkid where (honks.userid = ? or honks.whofore = 2) group by ontology")
	stmtGetFilters = preparetodie(db, "select hfcsid, json from hfcs where userid = ?")
	stmtSaveFilter = preparetodie(db, "insert into hfcs (userid, json) values (?, ?)")
	stmtDeleteFilter = preparetodie(db, "delete from hfcs where userid = ? and hfcsid = ?")
	stmtGetTracks = preparetodie(db, "select fetches from tracks where xid = ?")
	stmtSaveChonk = preparetodie(db, "insert into chonks (userid, xid, who, target, dt, noise, format) values (?, ?, ?, ?, ?, ?, ?)")
	stmtLoadChonks = preparetodie(db, "select chonkid, userid, xid, who, target, dt, noise, format from chonks where userid = ? and dt > ? order by chonkid asc")
	stmtGetChatters = preparetodie(db, "select distinct(target) from chonks where userid = ?")
}
