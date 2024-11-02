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

	"humungus.tedunangst.com/r/webs/gencache"
	"humungus.tedunangst.com/r/webs/htfilter"
	"humungus.tedunangst.com/r/webs/httpsig"
	"humungus.tedunangst.com/r/webs/login"
	"humungus.tedunangst.com/r/webs/mz"
)

var honkwindow time.Duration = 90

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
		user.URL = serverURL("/%s/%s", userSep, user.Name)
		err = decodeJson(options, &user.Options)
		if err != nil {
			elog.Printf("error processing user options: %s", err)
		}
		user.ChatPubKey.key, _ = b64tokey(user.Options.ChatPubKey)
		if user.Options.ChatSecKey != "" {
			user.ChatSecKey.key, _ = b64tokey(user.Options.ChatSecKey)
		}
	} else {
		user.URL = serverURL("/%s", user.Name)
	}
	if user.Options.Reaction == "" {
		user.Options.Reaction = "none"
	}

	return user, nil
}

var somenamedusers = gencache.New(gencache.Options[string, *WhatAbout]{Fill: func(name string) (*WhatAbout, bool) {
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

var somenumberedusers = gencache.New(gencache.Options[UserID, *WhatAbout]{Fill: func(userid UserID) (*WhatAbout, bool) {
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
	user, ok := somenumberedusers.Get(serverUID)
	if !ok {
		elog.Panicf("lost server user")
	}
	return user
}

func gethonker(userid UserID, xid string) (int64, error) {
	row := opendatabase().
		QueryRow("select honkerid from honkers where xid = ? and userid = ? and flavor in ('sub')", xid, userid)
	var honkerid int64

	err := row.Scan(&honkerid)
	return honkerid, err
}

func getUserBio(name string) (*WhatAbout, error) {
	user, ok := somenamedusers.Get(name)
	if !ok {
		return nil, fmt.Errorf("no user: %s", name)
	}
	return user, nil
}

var honkerinvalidator gencache.Invalidator[UserID]

func gethonkers(userid UserID) []*Honker {
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
			err = decodeJson(meta, &h.Meta)
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

func getdubs(userid UserID) []*Honker {
	rows, err := stmtDubbers.Query(userid)
	return dubsfromrows(rows, err)
}

func getnameddubs(userid UserID, name string) []*Honker {
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

func getActivityPubActivity(userid UserID, xid string) *ActivityPubActivity {
	if xid == "" {
		return nil
	}
	row := stmtOneXonk.QueryRow(userid, xid, xid)
	return scanhonk(row)
}

func getbonk(userid UserID, xid string) *ActivityPubActivity {
	row := stmtOneBonk.QueryRow(userid, xid)
	return scanhonk(row)
}

func getpublichonks() []*ActivityPubActivity {
	dt := time.Now().Add(-honkwindow).UTC().Format(dbtimeformat)
	rows, err := stmtPublicHonks.Query(dt, 100)
	return getsomehonks(rows, err)
}
func geteventhonks(userid UserID) []*ActivityPubActivity {
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

func gethonksbyuser(name string, includeprivate bool, wanted int64) []*ActivityPubActivity {
	dt := time.Now().Add(-honkwindow).UTC().Format(dbtimeformat)
	limit := 50
	whofore := 2
	if includeprivate {
		whofore = 3
	}
	rows, err := stmtUserHonks.Query(wanted, whofore, name, dt, limit)
	return getsomehonks(rows, err)
}
func gethonksforuser(userid UserID, wanted int64) []*ActivityPubActivity {
	dt := time.Now().Add(-honkwindow).UTC().Format(dbtimeformat)
	rows, err := stmtHonksForUser.Query(wanted, userid, dt, userid, userid)
	return getsomehonks(rows, err)
}
func gethonksforuserfirstclass(userid UserID, wanted int64) []*ActivityPubActivity {
	dt := time.Now().Add(-honkwindow).UTC().Format(dbtimeformat)
	rows, err := stmtHonksForUserFirstClass.Query(wanted, userid, dt, userid, userid)
	return getsomehonks(rows, err)
}

func gethonksforme(userid UserID, wanted int64) []*ActivityPubActivity {
	dt := time.Now().Add(-honkwindow).UTC().Format(dbtimeformat)
	rows, err := stmtHonksForMe.Query(wanted, userid, dt, userid, 250)
	return getsomehonks(rows, err)
}
func gethonksfromlongago(userid UserID, wanted int64) []*ActivityPubActivity {
	var params []interface{}
	var wheres []string
	params = append(params, wanted)
	params = append(params, userid)
	now := time.Now()
	for i := 1; i <= 5; i++ {
		dt := time.Date(now.Year()-i, now.Month(), now.Day(), now.Hour(), now.Minute(),
			now.Second(), 0, now.Location())
		dt1 := dt.Add(-36 * time.Hour).UTC().Format(dbtimeformat)
		dt2 := dt.Add(12 * time.Hour).UTC().Format(dbtimeformat)
		wheres = append(wheres, "(dt > ? and dt < ?)")
		params = append(params, dt1, dt2)
	}
	params = append(params, userid)
	sql := strings.ReplaceAll(sqlHonksFromLongAgo, "WHERECLAUSE", strings.Join(wheres, " or "))
	db := opendatabase()
	rows, err := db.Query(sql, params...)
	return getsomehonks(rows, err)
}
func getsavedhonks(userid UserID, wanted int64) []*ActivityPubActivity {
	rows, err := stmtHonksISaved.Query(wanted, userid)
	return getsomehonks(rows, err)
}
func gethonksbyhonker(userid UserID, honker string, wanted int64) []*ActivityPubActivity {
	rows, err := stmtHonksByHonker.Query(wanted, userid, honker, userid)
	return getsomehonks(rows, err)
}
func gethonksbyxonker(userid UserID, xonker string, wanted int64) []*ActivityPubActivity {
	rows, err := stmtHonksByXonker.Query(wanted, userid, xonker, xonker, userid)
	return getsomehonks(rows, err)
}
func gethonksbycombo(userid UserID, combo string, wanted int64) []*ActivityPubActivity {
	combo = "% " + combo + " %"
	rows, err := stmtHonksByCombo.Query(wanted, userid, userid, combo, userid, wanted, userid, combo, userid)
	return getsomehonks(rows, err)
}
func gethonksbyconvoy(userid UserID, convoy string, wanted int64) []*ActivityPubActivity {
	rows, err := stmtHonksByConvoy.Query(convoy, wanted, userid, 1000)
	return getsomehonks(rows, err)
}
func gethonksbysearch(userid UserID, q string, wanted int64) []*ActivityPubActivity {
	var queries []string
	var params []interface{}
	queries = append(queries, "honks.honkid > ?")
	params = append(params, wanted)
	queries = append(queries, "honks.userid = ?")
	params = append(params, userid)

	terms := strings.Split(q, " ")
	for _, t := range terms {
		if strings.HasPrefix(t, "alt:") {
			return gethonksbyaltsearch(userid, t[4:], wanted) // hehe bye!
		}

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
		if t == "@me" {
			queries = append(queries, negate+"whofore = 1")
			continue
		}
		if t == "@self" {
			queries = append(queries, negate+"(whofore = 2 or whofore = 3)")
			continue
		}
		if strings.HasPrefix(t, "before:") {
			before := t[7:]
			queries = append(queries, "dt < ?")
			params = append(params, before)
			continue
		}
		if strings.HasPrefix(t, "after:") {
			after := t[6:]
			queries = append(queries, "dt > ?")
			params = append(params, after)
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
		queries = append(queries, negate+"(plain like ?)")
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

func gethonksbyaltsearch(userid UserID, q string, wanted int64) []*ActivityPubActivity {
	query := `select honks.honkid, honks.userid, username, what, honker, oonker, honks.xid, honks.rid, honks.dt, honks.url, honks.audience, honks.noise, honks.precis, honks.format, honks.convoy, whofore, flags
		from honks, filemeta, donks
		join users on honks.userid = users.userid
		where filemeta.description LIKE ? AND donks.honkid = honks.honkid AND donks.fileid = filemeta.fileid order by honks.honkid desc limit 250`

	p := "%" + q + "%"
	rows, err := opendatabase().Query(query, p)
	honks := getsomehonks(rows, err)

	return honks
}

func gethonksbyontology(userid int64, name string, wanted int64) []*ActivityPubActivity {
	rows, err := stmtHonksByOntology.Query(wanted, name, userid, userid)
	honks := getsomehonks(rows, err)
	return honks
}

func reversehonks(honks []*ActivityPubActivity) {
	for i, j := 0, len(honks)-1; i < j; i, j = i+1, j-1 {
		honks[i], honks[j] = honks[j], honks[i]
	}
}

func getsomehonks(rows *sql.Rows, err error) []*ActivityPubActivity {
	if err != nil {
		elog.Printf("error querying honks: %s", err)
		return nil
	}
	defer rows.Close()
	honks := make([]*ActivityPubActivity, 0, 64)
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

func scanhonk(row RowLike) *ActivityPubActivity {
	h := new(ActivityPubActivity)
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

func donksforhonks(honks []*ActivityPubActivity) {
	db := opendatabase()
	ids := make([]string, len(honks))
	hmap := make(map[int64]*ActivityPubActivity, len(honks))
	for i, h := range honks {
		ids[i] = fmt.Sprintf("%d", h.ID)
		hmap[h.ID] = h
	}
	idset := strings.Join(ids, ",")
	// grab donks
	q := fmt.Sprintf("select honkid, donks.fileid, xid, name, description, url, media, local, meta from donks join filemeta on donks.fileid = filemeta.fileid where honkid in (%s)", idset)
	rows, err := db.Query(q)
	if err != nil {
		elog.Printf("error querying donks: %s", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var hid int64
		var j string
		d := new(Donk)
		err = rows.Scan(&hid, &d.FileID, &d.XID, &d.Name, &d.Desc, &d.URL, &d.Media, &d.Local, &j)
		if err != nil {
			elog.Printf("error scanning donk: %s", err)
			continue
		}
		decodeJson(j, &d.Meta)
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
			err = decodeJson(j, p)
			if err != nil {
				elog.Printf("error parsing place: %s", err)
				continue
			}
			h.Place = p
		case "time":
			t := new(Time)
			err = decodeJson(j, t)
			if err != nil {
				elog.Printf("error parsing time: %s", err)
				continue
			}
			h.Time = t
		case "mentions":
			err = decodeJson(j, &h.Mentions)
			if err != nil {
				elog.Printf("error parsing mentions: %s", err)
				continue
			}
		case "badonks":
			err = decodeJson(j, &h.Badonks)
			if err != nil {
				elog.Printf("error parsing badonks: %s", err)
				continue
			}
		case "seealso":
			h.SeeAlso = j
		case "onties":
			h.Onties = j
		case "link":
			h.Link = j
		case "legalname":
			h.LegalName = j
		case "oldrev":
		default:
			elog.Printf("unknown meta genus: %s", genus)
		}
	}
	rows.Close()
}

func donksforchonks(chonks []*Chonk) {
	db := opendatabase()
	ids := make([]string, len(chonks))
	chmap := make(map[int64]*Chonk, len(chonks))
	for i, ch := range chonks {
		ids[i] = fmt.Sprintf("%d", ch.ID)
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

func savechonk(ch *Chonk) error {
	dt := ch.Date.UTC().Format(dbtimeformat)
	db := opendatabase()
	tx, err := db.Begin()
	if err != nil {
		elog.Printf("can't begin tx: %s", err)
		return err
	}
	defer tx.Rollback()

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
	}
	return err
}

func chatplusone(tx *sql.Tx, userid UserID) {
	user, ok := somenumberedusers.Get(userid)
	if !ok {
		return
	}
	options := user.Options
	options.ChatCount += 1
	j, err := encodeJson(options)
	if err == nil {
		_, err = tx.Exec("update users set options = ? where username = ?", j, user.Name)
	}
	if err != nil {
		elog.Printf("error plussing chat: %s", err)
	}
	somenamedusers.Clear(user.Name)
	somenumberedusers.Clear(user.ID)
}

func chatnewnone(userid UserID) {
	user, ok := somenumberedusers.Get(userid)
	if !ok || user.Options.ChatCount == 0 {
		return
	}
	options := user.Options
	options.ChatCount = 0
	j, err := encodeJson(options)
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

func meplusone(tx *sql.Tx, userid UserID) {
	user, ok := somenumberedusers.Get(userid)
	if !ok {
		return
	}
	options := user.Options
	options.MeCount += 1
	j, err := encodeJson(options)
	if err == nil {
		_, err = tx.Exec("update users set options = ? where username = ?", j, user.Name)
	}
	if err != nil {
		elog.Printf("error plussing me: %s", err)
	}
	somenamedusers.Clear(user.Name)
	somenumberedusers.Clear(user.ID)
}

func menewnone(userid UserID) {
	user, ok := somenumberedusers.Get(userid)
	if !ok || user.Options.MeCount == 0 {
		return
	}
	options := user.Options
	options.MeCount = 0
	j, err := encodeJson(options)
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

func loadchatter(userid UserID, wanted int64) []*Chatter {
	duedt := time.Now().Add(-3 * 24 * time.Hour).UTC().Format(dbtimeformat)
	rows, err := stmtLoadChonks.Query(userid, duedt, wanted)
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
	rows.Close()
	donksforchonks(allchonks)
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

func (honk *ActivityPubActivity) Plain() string {
	return honktoplain(honk, false)
}

func (honk *ActivityPubActivity) VeryPlain() string {
	return honktoplain(honk, true)
}

func honktoplain(honk *ActivityPubActivity, very bool) string {
	var plain []string
	var filt htfilter.Filter
	if !very {
		filt.WithLinks = true
	}
	if honk.Precis != "" {
		t, _ := filt.TextOnly(honk.Precis)
		plain = append(plain, t)
	}
	if honk.Format == "html" {
		t, _ := filt.TextOnly(honk.Noise)
		plain = append(plain, t)
	} else {
		plain = append(plain, honk.Noise)
	}
	for _, d := range honk.Donks {
		plain = append(plain, d.Name)
		plain = append(plain, d.Desc)
	}
	for _, o := range honk.Onts {
		plain = append(plain, o)
	}
	return strings.Join(plain, " ")
}

func savehonk(h *ActivityPubActivity) error {
	dt := h.Date.UTC().Format(dbtimeformat)
	aud := strings.Join(h.Audience, " ")

	db := opendatabase()
	tx, err := db.Begin()
	if err != nil {
		elog.Printf("can't begin tx: %s", err)
		return err
	}
	defer tx.Rollback()
	plain := h.Plain()

	res, err := tx.Stmt(stmtSaveHonk).Exec(h.UserID, h.What, h.Honker, h.XID, h.RID, dt, h.URL,
		aud, h.Noise, h.Convoy, h.Whofore, h.Format, h.Precis,
		h.Oonker, h.Flags, plain)
	if err == nil {
		h.ID, _ = res.LastInsertId()
		err = saveextras(tx, h)
	}
	if err == nil {
		if h.Whofore == WhoAtme {
			dlog.Printf("another one for me: %s", h.XID)
			meplusone(tx, h.UserID)
		}
		err = tx.Commit()
	}
	if err != nil {
		elog.Printf("error saving honk: %s", err)
	}
	honkhonkline()
	return err
}

func updatehonk(h *ActivityPubActivity) error {
	old := getActivityPubActivity(h.UserID, h.XID)
	oldrev := OldRevision{Precis: old.Precis, Noise: old.Noise}
	dt := h.Date.UTC().Format(dbtimeformat)

	db := opendatabase()
	tx, err := db.Begin()
	if err != nil {
		elog.Printf("can't begin tx: %s", err)
		return err
	}
	defer tx.Rollback()
	plain := h.Plain()

	err = deleteextras(tx, h.ID, false)
	if err == nil {
		_, err = tx.Stmt(stmtUpdateHonk).Exec(h.Precis, h.Noise, h.Format, h.Whofore, dt, plain, h.ID)
	}
	if err == nil {
		err = saveextras(tx, h)
	}
	if err == nil {
		var j string
		j, err = encodeJson(&oldrev)
		if err == nil {
			_, err = tx.Stmt(stmtSaveMeta).Exec(old.ID, "oldrev", j)
		}
		if err != nil {
			elog.Printf("error saving oldrev: %s", err)
		}
	}
	if err == nil {
		err = tx.Commit()
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
	defer tx.Rollback()

	err = deleteextras(tx, honkid, true)
	if err == nil {
		_, err = tx.Stmt(stmtDeleteHonk).Exec(honkid)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		elog.Printf("error deleting honk %d: %s", honkid, err)
	}
	return err
}

func saveextras(tx *sql.Tx, h *ActivityPubActivity) error {
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
		j, err := encodeJson(p)
		if err == nil {
			_, err = tx.Stmt(stmtSaveMeta).Exec(h.ID, "place", j)
		}
		if err != nil {
			elog.Printf("error saving place: %s", err)
			return err
		}
	}
	if t := h.Time; t != nil {
		j, err := encodeJson(t)
		if err == nil {
			_, err = tx.Stmt(stmtSaveMeta).Exec(h.ID, "time", j)
		}
		if err != nil {
			elog.Printf("error saving time: %s", err)
			return err
		}
	}
	if m := h.Mentions; len(m) > 0 {
		j, err := encodeJson(m)
		if err == nil {
			_, err = tx.Stmt(stmtSaveMeta).Exec(h.ID, "mentions", j)
		}
		if err != nil {
			elog.Printf("error saving mentions: %s", err)
			return err
		}
	}
	if onties := h.Onties; onties != "" {
		_, err := tx.Stmt(stmtSaveMeta).Exec(h.ID, "onties", onties)
		if err != nil {
			elog.Printf("error saving onties: %s", err)
			return err
		}
	}
	if legalname := h.LegalName; legalname != "" {
		_, err := tx.Stmt(stmtSaveMeta).Exec(h.ID, "legalname", legalname)
		if err != nil {
			elog.Printf("error saving legalname: %s", err)
			return err
		}
	}
	if seealso := h.SeeAlso; seealso != "" {
		_, err := tx.Stmt(stmtSaveMeta).Exec(h.ID, "seealso", seealso)
		if err != nil {
			elog.Printf("error saving seealso: %s", err)
			return err
		}
	}
	if link := h.Link; link != "" {
		_, err := tx.Stmt(stmtSaveMeta).Exec(h.ID, "link", link)
		if err != nil {
			elog.Printf("error saving link: %s", err)
			return err
		}
	}
	return nil
}

var baxonker sync.Mutex

func addreaction(user *WhatAbout, xid string, who, react string) {
	baxonker.Lock()
	defer baxonker.Unlock()
	h := getActivityPubActivity(user.ID, xid)
	if h == nil {
		return
	}
	h.Badonks = append(h.Badonks, Badonk{Who: who, What: react})
	j, _ := encodeJson(h.Badonks)
	db := opendatabase()
	tx, err := db.Begin()
	if err != nil {
		return
	}
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

func encodeJson(what interface{}) (string, error) {
	var buf bytes.Buffer
	e := json.NewEncoder(&buf)
	e.SetEscapeHTML(false)
	e.SetIndent("", "")
	err := e.Encode(what)
	return buf.String(), err
}

func decodeJson(s string, dest interface{}) error {
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

func savexonker(what, value, flav string) {
	when := time.Now().UTC().Format(dbtimeformat)
	_, err := stmtSaveXonker.Exec(what, value, flav, when)
	if err != nil {
		elog.Printf("error saving xonker: %s", err)
	}
}

func savehonker(user *WhatAbout, url, name, flavor, combos, mj string) (int64, string, error) {
	var owner string
	if url[0] == '#' {
		flavor = "peep"
		if name == "" {
			name = url[1:]
		}
		owner = url
	} else if strings.HasSuffix(url, ".rss") {
		flavor = "peep"
		if name == "" {
			name = url[strings.LastIndexByte(url, '/')+1:]
		}
		owner = url

	} else {
		info, _, err := investigate(url)
		if err != nil {
			ilog.Printf("failed to investigate honker: %s", err)
			return 0, "", err
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
		return 0, "", err
	}

	res, err := stmtSaveHonker.Exec(user.ID, name, url, flavor, combos, owner, mj)
	if err != nil {
		elog.Print(err)
		return 0, "", err
	}
	honkerid, _ := res.LastInsertId()
	if strings.HasSuffix(url, ".rss") {
		go syndicate(user, url)
	}
	return honkerid, flavor, nil
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
	sqlMustQuery(db, "delete from honks where flags & 4 = 0 and whofore = 0 and "+where, sqlargs...)
	sqlMustQuery(db, "delete from donks where honkid > 0 and honkid not in (select honkid from honks)")
	sqlMustQuery(db, "delete from onts where honkid not in (select honkid from honks)")
	sqlMustQuery(db, "delete from honkmeta where honkid not in (select honkid from honks)")

	sqlMustQuery(db, "delete from filemeta where fileid not in (select fileid from donks)")
	for _, u := range allusers() {
		sqlMustQuery(db, "delete from zonkers where userid = ? and wherefore = 'zonvoy' and zonkerid < (select zonkerid from zonkers where userid = ? and wherefore = 'zonvoy' order by zonkerid desc limit 1 offset 200)", u.UserID, u.UserID)
	}

	cleanupfiles()
}

var stmtHonkers, stmtDubbers, stmtNamedDubbers, stmtSaveHonker, stmtUpdateFlavor, stmtUpdateHonker *sql.Stmt
var stmtDeleteHonker *sql.Stmt
var stmtAnyXonk, stmtOneXonk, stmtPublicHonks, stmtUserHonks, stmtHonksByCombo, stmtHonksByConvoy *sql.Stmt
var stmtHonksByOntology, stmtHonksForUser, stmtHonksForMe, stmtSaveDub, stmtHonksByXonker *sql.Stmt
var sqlHonksFromLongAgo string
var stmtHonksByHonker, stmtSaveHonk, stmtUserByName, stmtUserByNumber *sql.Stmt
var stmtEventHonks, stmtOneBonk, stmtFindZonk, stmtFindXonk, stmtSaveDonk *sql.Stmt
var stmtGetFileInfo, stmtFindFile, stmtFindFileId, stmtSaveFile *sql.Stmt
var stmtGetFileMedia, stmtSaveFileHash, stmtCheckFileHash *sql.Stmt
var stmtAddDoover, stmtGetDoovers, stmtLoadDoover, stmtZapDoover, stmtOneHonker *sql.Stmt
var stmtUntagged, stmtDeleteHonk, stmtDeleteDonks, stmtDeleteOnts, stmtSaveZonker *sql.Stmt
var stmtGetZonkers, stmtRecentHonkers, stmtGetXonker, stmtSaveXonker, stmtDeleteXonker, stmtDeleteOldXonkers *sql.Stmt
var stmtAllOnts, stmtSaveOnt, stmtUpdateFlags, stmtClearFlags *sql.Stmt
var stmtHonksForUserFirstClass *sql.Stmt
var stmtSaveMeta, stmtDeleteAllMeta, stmtDeleteOneMeta, stmtDeleteSomeMeta, stmtUpdateHonk *sql.Stmt
var stmtHonksISaved, stmtGetFilters, stmtSaveFilter, stmtDeleteFilter *sql.Stmt
var stmtGetTracks *sql.Stmt
var stmtSaveChonk, stmtLoadChonks, stmtGetChatters *sql.Stmt
var stmtGetTopDubbed *sql.Stmt
var stmtDeliquentCheck, stmtDeliquentUpdate *sql.Stmt
var stmtGetBlobData, stmtSaveBlobData *sql.Stmt

func sqlMustPrepare(db *sql.DB, s string) *sql.Stmt {
	stmt, err := db.Prepare(s)
	if err != nil {
		elog.Fatalf("error %s: %s", err, s)
	}
	return stmt
}

var g_blobdb *sql.DB

func closedatabases() {
	err := alreadyopendb.Close()
	if err != nil {
		elog.Printf("error closing database: %s", err)
	}
	if g_blobdb != nil {
		err = g_blobdb.Close()
		if err != nil {
			elog.Printf("error closing database: %s", err)
		}
	}
}

func prepareStatements(db *sql.DB) {
	stmtHonkers = sqlMustPrepare(db, "select honkerid, userid, name, xid, flavor, combos, meta from honkers where userid = ? and (flavor = 'presub' or flavor = 'sub' or flavor = 'peep' or flavor = 'unsub') order by name")
	stmtSaveHonker = sqlMustPrepare(db, "insert into honkers (userid, name, xid, flavor, combos, owner, meta, folxid) values (?, ?, ?, ?, ?, ?, ?, '')")
	stmtUpdateFlavor = sqlMustPrepare(db, "update honkers set flavor = ?, folxid = ? where userid = ? and name = ? and xid = ? and flavor = ?")
	stmtUpdateHonker = sqlMustPrepare(db, "update honkers set name = ?, combos = ?, meta = ? where honkerid = ? and userid = ?")
	stmtDeleteHonker = sqlMustPrepare(db, "delete from honkers where honkerid = ?")
	stmtOneHonker = sqlMustPrepare(db, "select xid from honkers where name = ? and userid = ?")
	stmtDubbers = sqlMustPrepare(db, "select honkerid, userid, name, xid, flavor from honkers where userid = ? and flavor = 'dub'")
	stmtNamedDubbers = sqlMustPrepare(db, "select honkerid, userid, name, xid, flavor from honkers where userid = ? and name = ? and flavor = 'dub'")

	selecthonks := "select honks.honkid, honks.userid, username, what, honker, oonker, honks.xid, rid, dt, url, audience, noise, precis, format, convoy, whofore, flags from honks join users on honks.userid = users.userid "
	limit := " order by honks.honkid desc limit 250"
	smalllimit := " order by honks.honkid desc limit ?"
	butnotthose := " and convoy not in (select name from zonkers where userid = ? and wherefore = 'zonvoy' order by zonkerid desc limit 100)"
	stmtOneXonk = sqlMustPrepare(db, selecthonks+"where honks.userid = ? and (xid = ? or url = ?)")
	stmtAnyXonk = sqlMustPrepare(db, selecthonks+"where xid = ? and what <> 'bonk' order by honks.honkid asc")
	stmtOneBonk = sqlMustPrepare(db, selecthonks+"where honks.userid = ? and xid = ? and what = 'bonk' and whofore = 2")
	stmtPublicHonks = sqlMustPrepare(db, selecthonks+"where whofore = 2 and dt > ?"+smalllimit)
	stmtEventHonks = sqlMustPrepare(db, selecthonks+"where (whofore = 2 or honks.userid = ?) and what = 'event'"+smalllimit)
	stmtUserHonks = sqlMustPrepare(db, selecthonks+"where honks.honkid > ? and (whofore = 2 or whofore = ?) and username = ? and dt > ?"+smalllimit)
	myhonkers := " and honker in (select xid from honkers where userid = ? and (flavor = 'sub' or flavor = 'peep' or flavor = 'presub') and combos not like '% - %')"
	stmtHonksForUser = sqlMustPrepare(db, selecthonks+"where honks.honkid > ? and honks.userid = ? and dt > ?"+myhonkers+butnotthose+limit)
	stmtHonksForUserFirstClass = sqlMustPrepare(db, selecthonks+"where honks.honkid > ? and honks.userid = ? and dt > ? and (rid = '' or what = 'bonk')"+myhonkers+butnotthose+limit)
	stmtHonksForMe = sqlMustPrepare(db, selecthonks+"where honks.honkid > ? and honks.userid = ? and dt > ? and whofore = 1"+butnotthose+smalllimit)
	sqlHonksFromLongAgo = selecthonks + "where honks.honkid > ? and honks.userid = ? and (WHERECLAUSE) and (whofore = 2 or flags & 4)" + butnotthose + limit
	stmtHonksISaved = sqlMustPrepare(db, selecthonks+"where honks.honkid > ? and honks.userid = ? and flags & 4 order by honks.honkid desc")
	stmtHonksByHonker = sqlMustPrepare(db, selecthonks+"join honkers on (honkers.xid = honks.honker or honkers.xid = honks.oonker) where honks.honkid > ? and honks.userid = ? and honkers.name = ?"+butnotthose+limit)
	stmtHonksByXonker = sqlMustPrepare(db, selecthonks+" where honks.honkid > ? and honks.userid = ? and (honker = ? or oonker = ?)"+butnotthose+limit)
	stmtHonksByCombo = sqlMustPrepare(db, selecthonks+" where honks.honkid > ? and honks.userid = ? and honks.honker in (select xid from honkers where honkers.userid = ? and honkers.combos like ?) "+butnotthose+" union "+selecthonks+"join onts on honks.honkid = onts.honkid where honks.honkid > ? and honks.userid = ? and onts.ontology in (select xid from honkers where combos like ?)"+butnotthose+limit)
	stmtHonksByConvoy = sqlMustPrepare(db, `with recursive getthread(x, c) as (
		values('', ?)
		union
		select xid, convoy from honks, getthread where honks.convoy = getthread.c
		union
		select xid, convoy from honks, getthread where honks.rid <> '' and honks.rid = getthread.x
		union
		select rid, convoy from honks, getthread where honks.xid = getthread.x and rid <> ''
	) `+selecthonks+"where honks.honkid > ? and honks.userid = ? and xid in (select x from getthread)"+smalllimit)
	stmtHonksByOntology = sqlMustPrepare(db, selecthonks+"join onts on honks.honkid = onts.honkid where honks.honkid > ? and onts.ontology = ? and (honks.userid = ? or (? = -1 and honks.whofore = 2))"+limit)

	stmtSaveMeta = sqlMustPrepare(db, "insert into honkmeta (honkid, genus, json) values (?, ?, ?)")
	stmtDeleteAllMeta = sqlMustPrepare(db, "delete from honkmeta where honkid = ?")
	stmtDeleteSomeMeta = sqlMustPrepare(db, "delete from honkmeta where honkid = ? and genus not in ('oldrev')")
	stmtDeleteOneMeta = sqlMustPrepare(db, "delete from honkmeta where honkid = ? and genus = ?")
	stmtSaveHonk = sqlMustPrepare(db, "insert into honks (userid, what, honker, xid, rid, dt, url, audience, noise, convoy, whofore, format, precis, oonker, flags, plain) values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)")
	stmtDeleteHonk = sqlMustPrepare(db, "delete from honks where honkid = ?")
	stmtUpdateHonk = sqlMustPrepare(db, "update honks set precis = ?, noise = ?, format = ?, whofore = ?, dt = ?, plain = ? where honkid = ?")
	stmtSaveOnt = sqlMustPrepare(db, "insert into onts (ontology, honkid) values (?, ?)")
	stmtDeleteOnts = sqlMustPrepare(db, "delete from onts where honkid = ?")
	stmtSaveDonk = sqlMustPrepare(db, "insert into donks (honkid, chonkid, fileid) values (?, ?, ?)")
	stmtDeleteDonks = sqlMustPrepare(db, "delete from donks where honkid = ?")
	stmtSaveFile = sqlMustPrepare(db, "insert into filemeta (xid, name, description, url, media, local, meta) values (?, ?, ?, ?, ?, ?, ?)")
	stmtSaveFileHash = sqlMustPrepare(db, "insert into filehashes (xid, hash, media) values (?, ?, ?)")
	stmtCheckFileHash = sqlMustPrepare(db, "select xid from filehashes where hash = ?")
	stmtGetFileMedia = sqlMustPrepare(db, "select media from filehashes where xid = ?")
	stmtFindXonk = sqlMustPrepare(db, "select honkid from honks where userid = ? and xid = ?")
	stmtGetFileInfo = sqlMustPrepare(db, "select url from filemeta where xid = ?")
	stmtFindFile = sqlMustPrepare(db, "select fileid, xid from filemeta where url = ? and local = 1")
	stmtFindFileId = sqlMustPrepare(db, "select xid, local, description from filemeta where fileid = ? and url = ? and local = 1")
	stmtUserByName = sqlMustPrepare(db, "select userid, username, displayname, about, pubkey, seckey, options from users where username = ? and userid > 0")
	stmtUserByNumber = sqlMustPrepare(db, "select userid, username, displayname, about, pubkey, seckey, options from users where userid = ?")
	stmtSaveDub = sqlMustPrepare(db, "insert into honkers (userid, name, xid, flavor, combos, owner, meta, folxid) values (?, ?, ?, ?, '', '', '', ?)")
	stmtAddDoover = sqlMustPrepare(db, "insert into doovers (dt, tries, userid, rcpt, msg) values (?, ?, ?, ?, ?)")
	stmtGetDoovers = sqlMustPrepare(db, "select dooverid, dt from doovers")
	stmtLoadDoover = sqlMustPrepare(db, "select tries, userid, rcpt, msg from doovers where dooverid = ?")
	stmtZapDoover = sqlMustPrepare(db, "delete from doovers where dooverid = ?")
	stmtUntagged = sqlMustPrepare(db, "select xid, rid, flags from (select honkid, xid, rid, flags from honks where userid = ? order by honkid desc limit 10000) order by honkid asc")
	stmtFindZonk = sqlMustPrepare(db, "select zonkerid from zonkers where userid = ? and name = ? and wherefore = 'zonk'")
	stmtGetZonkers = sqlMustPrepare(db, "select zonkerid, name, wherefore from zonkers where userid = ? and wherefore <> 'zonk'")
	stmtSaveZonker = sqlMustPrepare(db, "insert into zonkers (userid, name, wherefore) values (?, ?, ?)")
	stmtGetXonker = sqlMustPrepare(db, "select info from xonkers where name = ? and flavor = ?")
	stmtSaveXonker = sqlMustPrepare(db, "insert into xonkers (name, info, flavor, dt) values (?, ?, ?, ?)")
	stmtDeleteXonker = sqlMustPrepare(db, "delete from xonkers where name = ? and flavor = ? and dt < ?")
	stmtDeleteOldXonkers = sqlMustPrepare(db, "delete from xonkers where dt < ? and flavor <> 'handle'")
	stmtRecentHonkers = sqlMustPrepare(db, "select distinct(honker) from honks where userid = ? and honker not in (select xid from honkers where userid = ? and flavor = 'sub') order by honkid desc limit 100")
	stmtUpdateFlags = sqlMustPrepare(db, "update honks set flags = flags | ? where honkid = ?")
	stmtClearFlags = sqlMustPrepare(db, "update honks set flags = flags & ~ ? where honkid = ?")
	stmtAllOnts = sqlMustPrepare(db, "select ontology, count(ontology) from onts join honks on onts.honkid = honks.honkid where (honks.userid = ? or honks.whofore = 2) group by ontology")
	stmtGetFilters = sqlMustPrepare(db, "select hfcsid, json from hfcs where userid = ?")
	stmtSaveFilter = sqlMustPrepare(db, "insert into hfcs (userid, json) values (?, ?)")
	stmtDeleteFilter = sqlMustPrepare(db, "delete from hfcs where userid = ? and hfcsid = ?")
	stmtGetTracks = sqlMustPrepare(db, "select fetches from tracks where xid = ?")
	stmtSaveChonk = sqlMustPrepare(db, "insert into chonks (userid, xid, who, target, dt, noise, format) values (?, ?, ?, ?, ?, ?, ?)")
	stmtLoadChonks = sqlMustPrepare(db, "select chonkid, userid, xid, who, target, dt, noise, format from chonks where userid = ? and dt > ? and chonkid > ? order by chonkid asc")
	stmtGetChatters = sqlMustPrepare(db, "select distinct(target) from chonks where userid = ?")
	stmtGetTopDubbed = sqlMustPrepare(db, `SELECT COUNT(*) as c,userid FROM honkers WHERE flavor = "dub" GROUP BY userid`)
	stmtDeliquentCheck = sqlMustPrepare(db, "select dooverid, msg from doovers where userid = ? and rcpt = ?")
	stmtDeliquentUpdate = sqlMustPrepare(db, "update doovers set msg = ? where dooverid = ?")
	g_blobdb = openblobdb()
	if g_blobdb != nil {
		stmtSaveBlobData = sqlMustPrepare(g_blobdb, "insert into filedata (xid, content) values (?, ?)")
		stmtGetBlobData = sqlMustPrepare(g_blobdb, "select content from filedata where xid = ?")
	} else if !storeTheFilesInTheFileSystem {
		elog.Fatal("the blob.db has disappeared")
	}
}
