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
	"context"
	"crypto/tls"
	"database/sql"
	"fmt"
	"html"
	"io"
	"log"
	notrand "math/rand"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"humungus.tedunangst.com/r/webs/gate"
	"humungus.tedunangst.com/r/webs/gencache"
	"humungus.tedunangst.com/r/webs/httpsig"
	"humungus.tedunangst.com/r/webs/junk"
	"humungus.tedunangst.com/r/webs/templates"
)

const ldjsonContentType = `application/ld+json; profile="https://www.w3.org/ns/activitystreams"`
const activityJsonContentType = ldjsonContentType + `,application/activity+json`

var falsenames = []string{
	`application/ld+json`,
	`application/activity+json`,
}

const itiswhatitis = "https://www.w3.org/ns/activitystreams"
const w3idSecurityString = "https://w3id.org/security/v1"
const atContextString = "https://www.w3.org/ns/activitystreams#Public"
const tinyworld = "as:Public"
const chatKeyProp = "chatKeyV0"

var fastTimeout time.Duration = 5
var slowTimeout time.Duration = 30

func friendorfoe(ct string) bool {
	ct = strings.ToLower(ct)
	for _, at := range falsenames {
		if strings.HasPrefix(ct, at) {
			return true
		}
	}
	return false
}

var honkTransport = http.Transport{
	MaxIdleConns:    120,
	MaxConnsPerHost: 4,
}

var honkClient = http.Client{
	Transport: &honkTransport,
}

func disableTLSValidation() {
	honkTransport.TLSClientConfig = &tls.Config{
		InsecureSkipVerify: true,
	}
}

func PostJunk(keyname string, key httpsig.PrivateKey, url string, j junk.Junk) error {
	return PostMsg(keyname, key, url, j.ToBytes())
}

func PostMsg(keyname string, key httpsig.PrivateKey, url string, msg []byte) error {
	req, err := http.NewRequest("POST", url, bytes.NewReader(msg))
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "honksnonk/5.0; "+serverName)
	req.Header.Set("Content-Type", ldjsonContentType)
	httpsig.SignRequest(keyname, key, req, msg)
	ctx, cancel := context.WithTimeout(context.Background(), 2*slowTimeout*time.Second)
	defer cancel()
	req = req.WithContext(ctx)
	resp, err := honkClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case 200:
	case 201:
	case 202:
	default:
		var buf [240]byte
		n, _ := resp.Body.Read(buf[:])
		dlog.Printf("post failure message: %s", buf[:n])
		return fmt.Errorf("http post status: %d", resp.StatusCode)
	}
	ilog.Printf("successful post: %s %d", url, resp.StatusCode)
	return nil
}

func GetJunk(userid UserID, url string) (junk.Junk, error) {
	return GetJunkTimeout(userid, url, slowTimeout*time.Second, nil)
}

func GetJunkFast(userid UserID, url string) (junk.Junk, error) {
	return GetJunkTimeout(userid, url, fastTimeout*time.Second, nil)
}

func GetJunkHardMode(userid UserID, url string) (junk.Junk, error) {
	j, err := GetJunk(userid, url)
	if err != nil {
		emsg := err.Error()
		if emsg == "http get status: 429" || emsg == "http get status: 502" ||
			strings.Contains(emsg, "timeout") {
			ilog.Printf("trying %s again after error: %s", url, emsg)
			time.Sleep(time.Duration(60+notrand.Int63n(60)) * time.Second)
			j, err = GetJunk(userid, url)
			if err != nil {
				ilog.Printf("still couldn't get %s", url)
			} else {
				ilog.Printf("retry success!")
			}
		}
	}
	return j, err
}

var flightdeck = gate.NewSerializer()

func GetJunkTimeout(userid UserID, url string, timeout time.Duration, final *string) (junk.Junk, error) {
	if rejectorigin(userid, url, false) {
		return nil, fmt.Errorf("rejected origin: %s", url)
	}
	if final != nil {
		*final = url
	}
	sign := func(req *http.Request) error {
		ki := getPrivateKey(userid)
		if ki != nil {
			httpsig.SignRequest(ki.keyname, ki.seckey, req, nil)
		}
		return nil
	}
	if develMode {
		sign = nil
	}
	client := honkClient
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return fmt.Errorf("stopped after 5 redirects")
		}
		if final != nil {
			*final = req.URL.String()
		}
		if sign != nil {
			sign(req)
		}
		return nil
	}
	fn := func() (interface{}, error) {
		at := activityJsonContentType
		if strings.Contains(url, ".well-known/webfinger?resource") {
			at = "application/jrd+json"
		}
		j, err := getsomejunk(url, junk.GetArgs{
			Accept:  at,
			Agent:   "honksnonk/5.0; " + serverName,
			Timeout: timeout,
			Client:  &client,
			Fixup:   sign,
			Limit:   1 * 1024 * 1024,
		})
		return j, err
	}

	ji, err := flightdeck.Call(url, fn)
	if err != nil {
		return nil, err
	}
	j := ji.(junk.Junk)
	return j, nil
}

func getsomejunk(url string, args junk.GetArgs) (junk.Junk, error) {
	client := http.DefaultClient
	if args.Client != nil {
		client = args.Client
	}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	if args.Accept != "" {
		req.Header.Set("Accept", args.Accept)
	}
	if args.Agent != "" {
		req.Header.Set("User-Agent", args.Agent)
	}
	if args.Fixup != nil {
		err = args.Fixup(req)
		if err != nil {
			return nil, err
		}
	}
	if args.Timeout != 0 {
		ctx, cancel := context.WithTimeout(context.Background(), args.Timeout)
		defer cancel()
		req = req.WithContext(ctx)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case 200:
	case 201:
	case 202:
	default:
		return nil, fmt.Errorf("http get status: %d", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if args.Accept != "application/jrd+json" && !friendorfoe(ct) {
		return nil, fmt.Errorf("incompatible content type %s", ct)
	}
	var r io.Reader = resp.Body
	if args.Limit > 0 {
		r = io.LimitReader(r, args.Limit)
	}
	return junk.Read(r)
}

func fetchsome(url string) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		ilog.Printf("error fetching %s: %s", url, err)
		return nil, err
	}
	req.Header.Set("User-Agent", "honksnonk/5.0; "+serverName)
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()
	req = req.WithContext(ctx)
	resp, err := honkClient.Do(req)
	if err != nil {
		ilog.Printf("error fetching %s: %s", url, err)
		return nil, err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case 200:
	case 201:
	case 202:
	default:
		return nil, fmt.Errorf("http get not 200: %d %s", resp.StatusCode, url)
	}
	var buf bytes.Buffer
	limiter := io.LimitReader(resp.Body, maxFetchSize)
	io.Copy(&buf, limiter)
	return buf.Bytes(), nil
}

const maxFetchSize = 14 * 1024 * 1024

func savedonk(url string, name, desc, media string, localize bool) *Donk {
	if url == "" {
		return nil
	}
	if donk := finddonk(url); donk != nil {
		return donk
	}
	ilog.Printf("saving donk: %s", url)
	data := []byte{}
	var meta DonkMeta
	if localize {
		fn := func() (interface{}, error) {
			return fetchsome(url)
		}
		ii, err := flightdeck.Call(url, fn)
		if err != nil {
			ilog.Printf("error fetching donk: %s", err)
			localize = false
			goto saveit
		}
		data = ii.([]byte)

		if len(data) == maxFetchSize {
			ilog.Printf("truncation likely")
		}
		if strings.HasPrefix(media, "image") {
			img, err := shrinkit(data)
			if err != nil {
				ilog.Printf("unable to decode image: %s", err)
				localize = false
				data = []byte{}
				goto saveit
			}
			data = img.Data
			meta.Width = img.Width
			meta.Height = img.Height
			media = "image/" + img.Format
		} else if media == "application/pdf" {
			if len(data) > 1000000 {
				ilog.Printf("not saving large pdf")
				localize = false
				data = []byte{}
			}
		} else if media == "video/mp4" {
			if len(data) > 10000000 {
				ilog.Printf("not saving large mp4")
				localize = false
				data = []byte{}
			}
		} else if len(data) > 100000 {
			ilog.Printf("not saving large attachment")
			localize = false
			data = []byte{}
		}
		meta.Length = len(data)
	}
saveit:
	fileid, err := savefile(name, desc, url, media, localize, data, &meta)
	if err != nil {
		elog.Printf("error saving file %s: %s", url, err)
		return nil
	}
	donk := new(Donk)
	donk.FileID = fileid
	return donk
}

func iszonked(userid UserID, xid string) bool {
	var id int64
	row := stmtFindZonk.QueryRow(userid, xid)
	err := row.Scan(&id)
	if err == nil {
		return true
	}
	if err != sql.ErrNoRows {
		ilog.Printf("error querying zonk: %s", err)
	}
	return false
}

func needActivityPubActivity(user *WhatAbout, x *ActivityPubActivity) bool {
	if rejectxonk(x) {
		return false
	}
	return needActivityPubActivityID(user, x.XID)
}
func needbonkid(user *WhatAbout, xid string) bool {
	return needxonkidX(user, xid, true)
}
func needActivityPubActivityID(user *WhatAbout, xid string) bool {
	return needxonkidX(user, xid, false)
}
func needxonkidX(user *WhatAbout, xid string, isannounce bool) bool {
	if !strings.HasPrefix(xid, "https://") {
		return false
	}
	if strings.HasPrefix(xid, user.URL+"/") {
		return false
	}
	if rejectorigin(user.ID, xid, isannounce) {
		ilog.Printf("rejecting origin: %s", xid)
		return false
	}
	if iszonked(user.ID, xid) {
		ilog.Printf("already zonked: %s", xid)
		return false
	}
	var id int64
	row := stmtFindXonk.QueryRow(user.ID, xid)
	err := row.Scan(&id)
	if err == nil {
		return false
	}
	if err != sql.ErrNoRows {
		ilog.Printf("error querying xonk: %s", err)
	}
	return true
}

func deleteActivityPubActivity(userid UserID, xid string) {
	xonk := getActivityPubActivity(userid, xid)
	if xonk != nil {
		deletehonk(xonk.ID)
		_, err := stmtSaveZonker.Exec(userid, xid, "zonk")
		if err != nil {
			elog.Printf("error eradicating: %s", err)
		}
	}
}

func saveActivityPubActivity(x *ActivityPubActivity) {
	ilog.Printf("saving xonk: %s", x.XID)
	go handles(x.Honker)
	go handles(x.Oonker)
	savehonk(x)
}

type Box struct {
	In     string
	Out    string
	Shared string
}

var boxofboxes = gencache.New(gencache.Options[string, *Box]{Fill: func(ident string) (*Box, bool) {
	var info string
	row := stmtGetXonker.QueryRow(ident, "boxes")
	err := row.Scan(&info)
	if err != nil {
		dlog.Printf("need to get boxes for %s", ident)
		var j junk.Junk
		j, err = GetJunk(firstUserUID, ident)
		if err != nil {
			dlog.Printf("error getting boxes for %s: %s", ident, err)
			str := err.Error()
			if strings.Contains(str, "http get status: 410") ||
				strings.Contains(str, "http get status: 404") {
				savexonker(ident, "dead", "boxes")
			}
			return nil, true
		}
		allinjest(originate(ident), j)
		row = stmtGetXonker.QueryRow(ident, "boxes")
		err = row.Scan(&info)
	}
	if err == nil {
		if info == "dead" {
			return nil, true
		}
		m := strings.Split(info, " ")
		b := &Box{In: m[0], Out: m[1], Shared: m[2]}
		return b, true
	}
	return nil, false
}, Invalidator: &xonkInvalidator})

var gettergate = gate.NewLimiter(1)

func getsomemore(user *WhatAbout, page string) {
	time.Sleep(5 * time.Second)
	gettergate.Start()
	defer gettergate.Finish()
	gimmexonks(user, page)
}

func gimmexonks(user *WhatAbout, outbox string) {
	dlog.Printf("getting outbox: %s", outbox)
	j, err := GetJunk(user.ID, outbox)
	if err != nil {
		ilog.Printf("error getting outbox: %s", err)
		return
	}
	t, _ := j.GetString("type")
	origin := originate(outbox)
	if t == "OrderedCollection" || t == "CollectionPage" {
		items, _ := j.GetArray("orderedItems")
		if items == nil {
			items, _ = j.GetArray("items")
		}
		if items == nil {
			obj, ok := j.GetMap("first")
			if ok {
				items, _ = obj.GetArray("orderedItems")
			} else {
				page1, ok := j.GetString("first")
				if ok {
					j, err = GetJunk(user.ID, page1)
					if err != nil {
						ilog.Printf("error getting page1: %s", err)
						return
					}
					items, _ = j.GetArray("orderedItems")
				}
			}
		}
		if len(items) > 20 {
			items = items[0:20]
		}
		for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
			items[i], items[j] = items[j], items[i]
		}
		for _, item := range items {
			obj, ok := item.(junk.Junk)
			if ok {
				xonksaver(user, obj, origin)
				continue
			}
			xid, ok := item.(string)
			if ok {
				if !needActivityPubActivityID(user, xid) {
					continue
				}
				obj, err = GetJunk(user.ID, xid)
				if err != nil {
					ilog.Printf("error getting item: %s", err)
					continue
				}
				xonksaver(user, obj, originate(xid))
			}
		}
	}
}

func newphone(a []string, obj junk.Junk) []string {
	for _, addr := range []string{"to", "cc", "attributedTo"} {
		who, _ := obj.GetString(addr)
		if who != "" {
			a = append(a, who)
		}
		whos, _ := obj.GetArray(addr)
		for _, w := range whos {
			who, _ := w.(string)
			if who != "" {
				a = append(a, who)
			}
		}
	}
	return a
}

func extractattrto(obj junk.Junk) string {
	arr := handleManyJunkTypes(obj, "attributedTo")
	for _, a := range arr {
		s, ok := a.(string)
		if ok {
			return s
		}
		o, ok := a.(junk.Junk)
		if ok {
			t, _ := o.GetString("type")
			id, _ := o.GetString("id")
			if t == "Person" || t == "" {
				return id
			}
		}
	}
	return ""
}

func handleManyJunkTypes(obj junk.Junk, key string) []interface{} {
	if val, ok := obj.GetMap(key); ok {
		return []interface{}{val}
	}
	if str, ok := obj.GetString(key); ok {
		return []interface{}{str}
	}
	arr, _ := obj.GetArray(key)
	return arr
}

func firstofmany(obj junk.Junk, key string) string {
	if val, _ := obj.GetString(key); val != "" {
		return val
	}
	if arr, _ := obj.GetArray(key); len(arr) > 0 {
		val, ok := arr[0].(string)
		if ok {
			return val
		}
	}
	return ""
}

func grabhonk(user *WhatAbout, xid string) {
	if x := getActivityPubActivity(user.ID, xid); x != nil {
		dlog.Printf("already have it: %s", xid)
		return
	}
	var final string
	j, err := GetJunkTimeout(user.ID, xid, fastTimeout*time.Second, &final)
	if err != nil {
		dlog.Printf("unable to fetch xid: %s", err)
		return
	}
	xonksaver(user, j, originate(final))
}

var re_mast0link = regexp.MustCompile(`https://[[:alnum:].]+/users/[[:alnum:]]+/statuses/[[:digit:]]+`)
var re_masto1ink = regexp.MustCompile(`https://([[:alnum:].]+)/@([[:alnum:]]+)(@[[:alnum:].]+)?/([[:digit:]]+)`)
var re_misslink = regexp.MustCompile(`https://[[:alnum:].]+/notes/[[:alnum:]]+`)
var re_honklink = regexp.MustCompile(`https://[[:alnum:].]+/u/[[:alnum:]]+/h/[[:alnum:]]+`)
var re_r0malink = regexp.MustCompile(`https://[[:alnum:].]+/objects/[[:alnum:]-]+`)
var re_roma1ink = regexp.MustCompile(`https://[[:alnum:].]+/notice/[[:alnum:]]+`)
var re_qtlinks = regexp.MustCompile(`>https://[^\s<]+<`)

func xonksaver(user *WhatAbout, item junk.Junk, origin string) *ActivityPubActivity {
	return xonksaver2(user, item, origin, false)
}
func xonksaver2(user *WhatAbout, item junk.Junk, origin string, myown bool) *ActivityPubActivity {
	depth := 0
	maxdepth := 10
	currenttid := ""
	goingup := 0
	var xonkxonkfn2 func(junk.Junk, string, bool, string, bool) *ActivityPubActivity
	xonkxonkfn := func(item junk.Junk, origin string, isUpdate bool, bonker string) *ActivityPubActivity {
		return xonkxonkfn2(item, origin, isUpdate, bonker, false)
	}

	qutify := func(user *WhatAbout, qurl, content string) string {
		if depth >= maxdepth {
			ilog.Printf("in too deep")
			return content
		}
		// well this is gross
		malcontent := strings.ReplaceAll(content, `</span><span class="ellipsis">`, "")
		malcontent = strings.ReplaceAll(malcontent, `</span><span class="invisible">`, "")
		mlinks := re_qtlinks.FindAllString(malcontent, -1)
		if qurl != "" {
			mlinks = append(mlinks, ">"+qurl+"<")
		}
		mlinks = stringArrayTrimUntilDupe(mlinks)
		for _, m := range mlinks {
			tryit := false
			m = m[1 : len(m)-1]
			if re_mast0link.MatchString(m) || re_misslink.MatchString(m) ||
				re_honklink.MatchString(m) || re_r0malink.MatchString(m) ||
				re_roma1ink.MatchString(m) {
				tryit = true
			} else if re_masto1ink.MatchString(m) {
				tryit = true
			}
			if tryit {
				dlog.Printf("trying to get a quote from %s", m)
				var prefix string
				if m == qurl {
					prefix += fmt.Sprintf("<p><a href=\"%s\">%s</a>", m, m)
				}
				var final string
				if x := getActivityPubActivity(user.ID, m); x != nil {
					dlog.Printf("already had it")
					content = fmt.Sprintf("%s%s<blockquote>%s</blockquote>", content, prefix, x.Noise)
				} else {
					j, err := GetJunkTimeout(user.ID, m, fastTimeout*time.Second, &final)
					if err != nil {
						dlog.Printf("unable to fetch quote: %s", err)
						continue
					}
					q, ok := j.GetString("content")
					if ok {
						content = fmt.Sprintf("%s%s<blockquote>%s</blockquote>", content, prefix, q)
					} else {
						dlog.Printf("apparently no content")
					}
					prevdepth := depth
					depth = maxdepth
					xonkxonkfn(j, originate(final), false, "")
					depth = prevdepth
				}
			}
		}
		return content
	}

	saveonemore := func(xid string) {
		dlog.Printf("getting onemore: %s", xid)
		if depth >= maxdepth {
			ilog.Printf("in too deep")
			return
		}
		obj, err := GetJunkHardMode(user.ID, xid)
		if err != nil {
			ilog.Printf("error getting onemore: %s: %s", xid, err)
			return
		}
		xonkxonkfn(obj, originate(xid), false, "")
	}

	xonkxonkfn2 = func(item junk.Junk, origin string, isUpdate bool, bonker string, myown bool) *ActivityPubActivity {
		id, _ := item.GetString("id")
		typ := firstofmany(item, "type")
		what := typ
		dt, ok := item.GetString("published")
		if !ok {
			dt = time.Now().Format(time.RFC3339)
		}
		if depth >= maxdepth+5 {
			ilog.Printf("went too deep in xonkxonk")
			return nil
		}
		depth++
		defer func() { depth-- }()

		var err error
		var xid, rid, url, convoy string
		var replies []string
		var obj junk.Junk
		waspage := false
		preferorig := false
		switch what {
		case "Delete":
			obj, ok = item.GetMap("object")
			if ok {
				xid, _ = obj.GetString("id")
			} else {
				xid, _ = item.GetString("object")
			}
			if xid == "" {
				return nil
			}
			if originate(xid) != origin {
				ilog.Printf("forged delete: %s", xid)
				return nil
			}
			ilog.Printf("eradicating %s", xid)
			deleteActivityPubActivity(user.ID, xid)
			return nil
		case "Remove":
			xid, _ = item.GetString("object")
			targ, _ := item.GetString("target")
			ilog.Printf("remove %s from %s", xid, targ)
			return nil
		case "Tombstone":
			xid, _ = item.GetString("id")
			if xid == "" {
				return nil
			}
			if originate(xid) != origin {
				ilog.Printf("forged delete: %s", xid)
				return nil
			}
			ilog.Printf("eradicating %s", xid)
			deleteActivityPubActivity(user.ID, xid)
			return nil
		case "Announce":
			obj, ok = item.GetMap("object")
			if ok {
				// peek ahead some
				what := firstofmany(obj, "type")
				if what == "Create" || what == "Update" {
					if what == "Update" {
						isUpdate = true
					}
					inner, ok := obj.GetMap("object")
					if ok {
						obj = inner
					} else {
						xid, _ = obj.GetString("object")
					}
				}
				if xid == "" {
					xid, _ = obj.GetString("id")
				}
			} else {
				xid, _ = item.GetString("object")
			}
			if !isUpdate && !needbonkid(user, xid) {
				return nil
			}
			bonker, _ = item.GetString("actor")
			if originate(bonker) != origin {
				ilog.Printf("out of bounds actor in bonk: %s not from %s", bonker, origin)
				item.Write(ilog.Writer())
				return nil
			}
			origin = originate(xid)
			if ok && originate(id) == origin {
				dlog.Printf("using object in announce for %s", xid)
			} else {
				dlog.Printf("getting bonk: %s", xid)
				obj, err = GetJunkHardMode(user.ID, xid)
				if err != nil {
					ilog.Printf("error getting bonk: %s: %s", xid, err)
					return nil
				}
			}
			return xonkxonkfn(obj, origin, isUpdate, bonker)
		case "Update":
			isUpdate = true
			fallthrough
		case "Create":
			obj, ok = item.GetMap("object")
			if !ok {
				xid, _ = item.GetString("object")
				dlog.Printf("getting created honk: %s", xid)
				if originate(xid) != origin {
					ilog.Printf("out of bounds object in create: %s not from %s", xid, origin)
					return nil
				}
				obj, err = GetJunkHardMode(user.ID, xid)
				if err != nil {
					ilog.Printf("error getting creation: %s", err)
				}
			}
			if obj == nil {
				ilog.Printf("no object for creation %s", id)
				return nil
			}
			return xonkxonkfn2(obj, origin, isUpdate, bonker, myown)
		case "Read":
			xid, ok = item.GetString("object")
			if ok {
				if !needActivityPubActivityID(user, xid) {
					dlog.Printf("don't need read obj: %s", xid)
					return nil
				}
				obj, err = GetJunkHardMode(user.ID, xid)
				if err != nil {
					ilog.Printf("error getting read: %s", err)
					return nil
				}
				return xonkxonkfn(obj, originate(xid), false, "")
			}
			return nil
		case "Add":
			xid, ok = item.GetString("object")
			if ok {
				// check target...
				if !needActivityPubActivityID(user, xid) {
					dlog.Printf("don't need added obj: %s", xid)
					return nil
				}
				obj, err = GetJunkHardMode(user.ID, xid)
				if err != nil {
					ilog.Printf("error getting add: %s", err)
					return nil
				}
				return xonkxonkfn(obj, originate(xid), false, "")
			}
			return nil
		case "Move":
			obj = item
			what = "move"
		case "Page":
			waspage = true
			fallthrough
		case "Audio":
			fallthrough
		case "Video":
			fallthrough
		case "Image":
			if what == "Image" || what == "Video" {
				preferorig = true
			}
			fallthrough
		case "Question":
			fallthrough
		case "Commit":
			fallthrough
		case "Article":
			fallthrough
		case "Note":
			obj = item
			what = "honk"
		case "Event":
			obj = item
			what = "event"
		case "ChatMessage":
			bonker = ""
			obj = item
			what = "chonk"
		case "Dislike":
			return nil
		default:
			ilog.Printf("unknown activity: %s", what)
			dumpactivity(item)
			return nil
		}
		if bonker != "" {
			what = "bonk"
		}

		if obj != nil {
			xid, _ = obj.GetString("id")
		}

		if xid == "" {
			ilog.Printf("don't know what xid is")
			item.Write(ilog.Writer())
			return nil
		}
		if originate(xid) != origin {
			if !develMode && origin != "" {
				ilog.Printf("original sin: %s not from %s", xid, origin)
				item.Write(ilog.Writer())
				return nil
			}
		}

		var xonk ActivityPubActivity
		// early init
		xonk.XID = xid
		xonk.UserID = user.ID
		xonk.Honker, _ = item.GetString("actor")
		if xonk.Honker == "" {
			xonk.Honker = extractattrto(item)
		}
		if myown && xonk.Honker != user.URL {
			ilog.Printf("not allowing local impersonation: %s <> %s", xonk.Honker, user.URL)
			item.Write(ilog.Writer())
			return nil
		}
		if originate(xonk.Honker) != origin {
			ilog.Printf("out of bounds honker %s from %s", xonk.Honker, origin)
			item.Write(ilog.Writer())
			return nil
		}
		if obj != nil {
			if xonk.Honker == "" {
				xonk.Honker = extractattrto(obj)
			}
			if bonker != "" {
				xonk.Honker, xonk.Oonker = bonker, xonk.Honker
			}
			if xonk.Oonker == xonk.Honker {
				xonk.Oonker = ""
			}
			xonk.Audience = newphone(nil, obj)
		}
		xonk.Audience = append(xonk.Audience, xonk.Honker)
		xonk.Audience = stringArrayTrimUntilDupe(xonk.Audience)
		for i, a := range xonk.Audience {
			if a == tinyworld {
				xonk.Audience[i] = atContextString
			}
		}
		xonk.Public = loudandproud(xonk.Audience)

		var mentions []Mention
		if obj != nil {
			ot := firstofmany(obj, "type")
			url, _ = obj.GetString("url")
			if dt2, ok := obj.GetString("published"); ok {
				dt = dt2
			}
			content, _ := obj.GetString("content")
			if mt, _ := obj.GetString("mediaType"); mt == "text/plain" {
				if typ == "Commit" {
					content = highlight(content, "diff")
				} else {
					content = html.EscapeString(content)
				}
			}
			if !strings.HasPrefix(content, "<p>") {
				content = "<p>" + content
			}
			if desc, _ := obj.GetMap("description"); desc != nil {
				content2, _ := desc.GetString("content")
				if mt, _ := desc.GetString("mediaType"); mt == "text/plain" {
					content2 = html.EscapeString(content2)
				}
				content = content2 + content
			}
			if !strings.HasPrefix(content, "<p>") {
				content = "<p>" + content
			}
			precis, _ := obj.GetString("summary")
			if name, ok := obj.GetString("name"); ok {
				if precis != "" {
					content = precis + "<p>" + content
				}
				precis = html.EscapeString(name)
			}
			if sens, _ := obj["sensitive"].(bool); sens && precis == "" {
				precis = "unspecified horror"
			}
			if waspage && url != "" {
				content += fmt.Sprintf(`<p><a href="%s">%s</a>`, url, url)
				url = xid
			}
			if user.Options.InlineQuotes {
				qurl, _ := obj.GetString("quoteUrl")
				content = qutify(user, qurl, content)
			}
			rid, ok = obj.GetString("inReplyTo")
			if !ok {
				if robj, ok := obj.GetMap("inReplyTo"); ok {
					rid, _ = robj.GetString("id")
				}
			}
			convoy, _ = obj.GetString("context")
			if convoy == "" {
				convoy, _ = obj.GetString("conversation")
			}
			if ot == "Question" {
				if what == "honk" {
					what = "qonk"
				}
				content += "<ul>"
				ans, _ := obj.GetArray("oneOf")
				for _, ai := range ans {
					a, ok := ai.(junk.Junk)
					if !ok {
						continue
					}
					as, _ := a.GetString("name")
					content += "<li>" + as
				}
				ans, _ = obj.GetArray("anyOf")
				for _, ai := range ans {
					a, ok := ai.(junk.Junk)
					if !ok {
						continue
					}
					as, _ := a.GetString("name")
					content += "<li>" + as
				}
				content += "</ul>"
			}
			if ot == "Move" {
				targ, _ := obj.GetString("target")
				content += string(templates.Sprintf(`<p>Moved to <a href="%s">%s</a>`, targ, targ))
			}
			if len(content) > 90001 {
				ilog.Printf("content too long. truncating")
				content = content[:90001]
			}

			xonk.Noise = content
			xonk.Precis = precis
			if rejectxonk(&xonk) {
				dlog.Printf("fast reject: %s", xid)
				return nil
			}

			numatts := 0
			procatt := func(att junk.Junk) {
				at, _ := att.GetString("type")
				log.Printf("Donk debug %#v", att)
				mt, _ := att.GetString("mediaType")
				if mt == "" {
					mt = "image"
				}
				u, ok := att.GetString("url")
				if !ok {
					u, ok = att.GetString("href")
				}
				if !ok {
					if ua, ok := att.GetArray("url"); ok && len(ua) > 0 {
						u, ok = ua[0].(string)
						if !ok {
							if uu, ok := ua[0].(junk.Junk); ok {
								u, _ = uu.GetString("href")
								if mt == "" {
									mt, _ = uu.GetString("mediaType")
								}
							}
						}
					} else if uu, ok := att.GetMap("url"); ok {
						u, _ = uu.GetString("href")
						if mt == "" {
							mt, _ = uu.GetString("mediaType")
						}
					}
				}
				name, _ := att.GetString("name")
				desc, _ := att.GetString("summary")
				desc = html.UnescapeString(desc)
				if desc == "" {
					desc = name
				}
				localize := false
				if at == "Document" || at == "Image" || at == "Video" {
					mt = strings.ToLower(mt)
					dlog.Printf("attachment: %s %s", mt, u)
					if mt == "text/plain" || mt == "application/pdf" ||
						strings.HasPrefix(mt, "image") || mt == "video/mp4" {
						if numatts > 4 {
							ilog.Printf("excessive attachment: %s", at)
						} else {
							localize = true
						}
					}
				} else if at == "Link" {
					if waspage {
						xonk.Noise += fmt.Sprintf(`<p><a href="%s">%s</a>`, u, u)
						return
					}
					if u == id {
						return
					}
					if name == "" {
						name = u
					}
				} else {
					ilog.Printf("unknown attachment: %s", at)
				}
				if skipMedia(&xonk) {
					localize = false
				}
				donk := savedonk(u, name, desc, mt, localize)
				if donk != nil {
					xonk.Donks = append(xonk.Donks, donk)
				}
				numatts++
			}
			if img, ok := obj.GetMap("image"); ok {
				procatt(img)
			}
			if preferorig {
				atts, _ := obj.GetArray("url")
				for _, atti := range atts {
					att, ok := atti.(junk.Junk)
					if !ok {
						ilog.Printf("attachment that wasn't map?")
						continue
					}
					procatt(att)
				}
				if numatts == 0 {
					preferorig = false
				}
			}
			if !preferorig {
				atts := handleManyJunkTypes(obj, "attachment")
				for _, atti := range atts {
					att, ok := atti.(junk.Junk)
					if !ok {
						ilog.Printf("attachment that wasn't map?")
						continue
					}
					procatt(att)
				}
			}
			proctag := func(tag junk.Junk) {
				tt, _ := tag.GetString("type")
				name, _ := tag.GetString("name")
				desc, _ := tag.GetString("summary")
				desc = html.UnescapeString(desc)
				if desc == "" {
					desc = name
				}
				if tt == "Emoji" {
					icon, _ := tag.GetMap("icon")
					mt, _ := icon.GetString("mediaType")
					if mt == "" {
						mt = "image/png"
					}
					u, _ := icon.GetString("url")
					donk := savedonk(u, name, desc, mt, true)
					if donk != nil {
						xonk.Donks = append(xonk.Donks, donk)
					}
				}
				if tt == "Hashtag" {
					if name == "" || name == "#" {
						// skip it
					} else {
						if name[0] != '#' {
							name = "#" + name
						}
						xonk.Onts = append(xonk.Onts, name)
					}
				}
				if tt == "Place" {
					p := new(Place)
					p.Name = name
					p.Latitude, _ = tag.GetNumber("latitude")
					p.Longitude, _ = tag.GetNumber("longitude")
					p.Url, _ = tag.GetString("url")
					xonk.Place = p
				}
				if tt == "Mention" {
					var m Mention
					m.Who, _ = tag.GetString("name")
					m.Where, _ = tag.GetString("href")
					if m.Who == "" {
						m.Who = m.Where
					}
					if m.Where != "" {
						mentions = append(mentions, m)
					}
				}
			}
			tags := handleManyJunkTypes(obj, "tag")
			for _, tagi := range tags {
				tag, ok := tagi.(junk.Junk)
				if !ok {
					continue
				}
				proctag(tag)
			}
			if starttime, ok := obj.GetString("startTime"); ok {
				if start, err := time.Parse(time.RFC3339, starttime); err == nil {
					t := new(Time)
					t.StartTime = start
					endtime, _ := obj.GetString("endTime")
					t.EndTime, _ = time.Parse(time.RFC3339, endtime)
					dura, _ := obj.GetString("duration")
					if strings.HasPrefix(dura, "PT") {
						dura = strings.ToLower(dura[2:])
						d, _ := time.ParseDuration(dura)
						t.Duration = Duration(d)
					}
					xonk.Time = t
				}
			}
			if loca, ok := obj.GetMap("location"); ok {
				if tt, _ := loca.GetString("type"); tt == "Place" {
					p := new(Place)
					p.Name, _ = loca.GetString("name")
					p.Latitude, _ = loca.GetNumber("latitude")
					p.Longitude, _ = loca.GetNumber("longitude")
					p.Url, _ = loca.GetString("url")
					xonk.Place = p
				}
			}

			xonk.Onts = stringArrayTrimUntilDupe(xonk.Onts)
			replyobj, ok := obj.GetMap("replies")
			if ok {
				items, ok := replyobj.GetArray("items")
				if !ok {
					first, ok := replyobj.GetMap("first")
					if ok {
						items, _ = first.GetArray("items")
					}
				}
				for _, repl := range items {
					s, ok := repl.(string)
					if ok {
						replies = append(replies, s)
					}
				}
			}

		}

		if currenttid == "" {
			currenttid = convoy
		}

		// init xonk
		xonk.What = what
		xonk.RID = rid
		xonk.Date, _ = time.Parse(time.RFC3339, dt)
		if originate(url) == originate(xonk.XID) {
			xonk.URL = url
		}
		xonk.Format = "html"
		xonk.Convoy = convoy
		xonk.Mentions = mentions
		if myown {
			if xonk.Public {
				xonk.Whofore = WhoPublic
			} else {
				xonk.Whofore = WhoPrivate
			}
		} else {
			for _, m := range mentions {
				if m.Where == user.URL {
					xonk.Whofore = WhoAtme
				}
			}
		}
		imaginate(&xonk)

		if what == "chonk" {
			// undo damage above
			xonk.Noise = strings.TrimPrefix(xonk.Noise, "<p>")
			target := firstofmany(obj, "to")
			if target == user.URL {
				target = xonk.Honker
			}
			enc, _ := obj.GetString(chatKeyProp)
			if enc != "" {
				if pubkey, ok := getchatkey(xonk.Honker); ok {
					dec, err := decryptString(xonk.Noise, user.ChatSecKey, pubkey)
					if err != nil {
						ilog.Printf("failed to decrypt chonk")
					} else {
						dlog.Printf("successful decrypt from %s", xonk.Honker)
						xonk.Noise = dec
					}
				}
			}
			ch := Chonk{
				UserID: xonk.UserID,
				XID:    xid,
				Who:    xonk.Honker,
				Target: target,
				Date:   xonk.Date,
				Noise:  xonk.Noise,
				Format: xonk.Format,
				Donks:  xonk.Donks,
			}
			savechonk(&ch)
			return nil
		}

		if isUpdate {
			dlog.Printf("something has changed! %s", xonk.XID)
			prev := getActivityPubActivity(user.ID, xonk.XID)
			if prev == nil {
				ilog.Printf("didn't find old version for update: %s", xonk.XID)
				isUpdate = false
			} else {
				xonk.ID = prev.ID
				updatehonk(&xonk)
			}
		}
		if !isUpdate && (myown || needActivityPubActivity(user, &xonk)) {
			if rid != "" && xonk.Public {
				if needActivityPubActivityID(user, rid) {
					goingup++
					saveonemore(rid)
					goingup--
				}
				if convoy == "" {
					xx := getActivityPubActivity(user.ID, rid)
					if xx != nil {
						convoy = xx.Convoy
					}
				}
			}
			if convoy == "" {
				convoy = currenttid
			}
			if convoy == "" {
				convoy = xonk.XID
				currenttid = convoy
			}
			xonk.Convoy = convoy
			saveActivityPubActivity(&xonk)
		}
		if goingup == 0 {
			for _, replid := range replies {
				if needActivityPubActivityID(user, replid) {
					dlog.Printf("missing a reply: %s", replid)
					saveonemore(replid)
				}
			}
		}
		return &xonk
	}

	return xonkxonkfn2(item, origin, false, "", myown)
}

func dumpactivity(item junk.Junk) {
	fd, err := os.OpenFile("savedinbox.json", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		elog.Printf("error opening inbox! %s", err)
		return
	}
	defer fd.Close()
	item.Write(fd)
	io.WriteString(fd, "\n")
}

func rubadubdub(user *WhatAbout, req junk.Junk) {
	actor, _ := req.GetString("actor")
	j := junk.New()
	j["@context"] = itiswhatitis
	j["id"] = user.URL + "/dub/" + make18CharRandomString()
	j["type"] = "Accept"
	j["actor"] = user.URL
	j["to"] = actor
	j["published"] = time.Now().UTC().Format(time.RFC3339)
	j["object"] = req

	deliverate(user.ID, actor, j.ToBytes())
}

func itakeitallback(user *WhatAbout, xid string, owner string, folxid string) {
	j := junk.New()
	j["@context"] = itiswhatitis
	j["id"] = user.URL + "/unsub/" + folxid
	j["type"] = "Undo"
	j["actor"] = user.URL
	j["to"] = owner
	f := junk.New()
	f["id"] = user.URL + "/sub/" + folxid
	f["type"] = "Follow"
	f["actor"] = user.URL
	f["to"] = owner
	f["object"] = xid
	j["object"] = f
	j["published"] = time.Now().UTC().Format(time.RFC3339)

	deliverate(user.ID, owner, j.ToBytes())
}

func subsub(user *WhatAbout, xid string, owner string, folxid string) {
	if xid == "" {
		ilog.Printf("can't subscribe to empty")
		return
	}
	j := junk.New()
	j["@context"] = itiswhatitis
	j["id"] = user.URL + "/sub/" + folxid
	j["type"] = "Follow"
	j["actor"] = user.URL
	j["to"] = owner
	j["object"] = xid
	j["published"] = time.Now().UTC().Format(time.RFC3339)

	deliverate(user.ID, owner, j.ToBytes())
}

func activatedonks(donks []*Donk) []junk.Junk {
	var atts []junk.Junk
	for _, d := range donks {
		if re_emus.MatchString(d.Name) {
			continue
		}
		jd := junk.New()
		jd["mediaType"] = d.Media
		jd["name"] = d.Name
		jd["summary"] = html.EscapeString(d.Desc)
		jd["type"] = "Document"
		jd["url"] = d.URL
		atts = append(atts, jd)
	}
	return atts
}

// returns activity, object
func jonkjonk(user *WhatAbout, h *ActivityPubActivity) (junk.Junk, junk.Junk) {
	dt := h.Date.Format(time.RFC3339)
	var jo junk.Junk
	j := junk.New()
	j["id"] = user.URL + "/" + h.What + "/" + shortxid(h.XID)
	who := h.Honker
	j["actor"] = who
	j["published"] = dt
	if h.Public && h.Honker == user.URL {
		h.Audience = append(h.Audience, user.URL+"/followers")
	}
	j["to"] = h.Audience[0]
	if len(h.Audience) > 1 {
		j["cc"] = h.Audience[1:]
	}

	switch h.What {
	case "update":
		fallthrough
	case "event":
		fallthrough
	case "honk":
		j["type"] = "Create"
		jo = junk.New()
		jo["id"] = h.XID
		jo["type"] = "Note"
		if h.What == "update" {
			j["type"] = "Update"
			jo["updated"] = dt
		}
		jo["published"] = dt
		jo["url"] = h.XID
		jo["attributedTo"] = who
		if h.RID != "" {
			jo["inReplyTo"] = h.RID
		}
		if h.Convoy != "" {
			jo["context"] = h.Convoy
			jo["conversation"] = h.Convoy
		}
		jo["to"] = h.Audience[0]
		if len(h.Audience) > 1 {
			jo["cc"] = h.Audience[1:]
		}
		if !h.Public {
			jo["directMessage"] = true
		}
		translate(h)
		redoimages(h)
		if h.Precis != "" {
			jo["sensitive"] = true
		}

		var replies []string
		for _, reply := range h.Replies {
			replies = append(replies, reply.XID)
		}
		if len(replies) > 0 {
			jr := junk.New()
			jr["type"] = "Collection"
			jr["totalItems"] = len(replies)
			jr["items"] = replies
			jo["replies"] = jr
		}

		var tags []junk.Junk
		for _, m := range h.Mentions {
			t := junk.New()
			t["type"] = "Mention"
			t["name"] = m.Who
			t["href"] = m.Where
			tags = append(tags, t)
		}
		for _, o := range h.Onts {
			t := junk.New()
			t["type"] = "Hashtag"
			o = strings.ToLower(o)
			t["href"] = serverURL("/o/%s", o[1:])
			t["name"] = o
			tags = append(tags, t)
		}
		for _, e := range herdofemus(h.Noise) {
			t := junk.New()
			t["id"] = e.ID
			t["type"] = "Emoji"
			t["name"] = e.Name
			i := junk.New()
			i["type"] = "Image"
			i["mediaType"] = e.Type
			i["url"] = e.ID
			t["icon"] = i
			tags = append(tags, t)
		}
		for _, e := range fixupflags(h) {
			t := junk.New()
			t["id"] = e.ID
			t["type"] = "Emoji"
			t["name"] = e.Name
			i := junk.New()
			i["type"] = "Image"
			i["mediaType"] = "image/png"
			i["url"] = e.ID
			t["icon"] = i
			tags = append(tags, t)
		}
		if len(tags) > 0 {
			jo["tag"] = tags
		}
		if p := h.Place; p != nil {
			t := junk.New()
			t["type"] = "Place"
			if p.Name != "" {
				t["name"] = p.Name
			}
			if p.Latitude != 0 {
				t["latitude"] = p.Latitude
			}
			if p.Longitude != 0 {
				t["longitude"] = p.Longitude
			}
			if p.Url != "" {
				t["url"] = p.Url
			}
			jo["location"] = t
		}
		if t := h.Time; t != nil {
			jo["startTime"] = t.StartTime.Format(time.RFC3339)
			if t.Duration != 0 {
				jo["duration"] = "PT" + strings.ToUpper(t.Duration.String())
			}
		}
		atts := activatedonks(h.Donks)
		if h.Link != "" {
			jo["type"] = "Page"
			jl := junk.New()
			jl["type"] = "Link"
			jl["href"] = h.Link
			atts = append(atts, jl)
		}
		if tooooFancy(h.Noise) {
			jo["type"] = "Article"
		}
		if h.What == "event" {
			jo["type"] = "Event"
		}
		if len(atts) > 0 {
			jo["attachment"] = atts
		}
		if h.LegalName != "" {
			jo["name"] = h.LegalName
		}
		if h.Precis != "" {
			jo["summary"] = h.Precis
		}
		jo["content"] = h.Noise
		j["object"] = jo
	case "bonk":
		j["type"] = "Announce"
		if h.Convoy != "" {
			j["context"] = h.Convoy
		}
		j["object"] = h.XID
	case "unbonk":
		b := junk.New()
		b["id"] = user.URL + "/" + "bonk" + "/" + shortxid(h.XID)
		b["type"] = "Announce"
		b["actor"] = user.URL
		if h.Convoy != "" {
			b["context"] = h.Convoy
		}
		b["object"] = h.XID
		j["type"] = "Undo"
		j["object"] = b
	case "zonk":
		j["type"] = "Delete"
		j["object"] = h.XID
	case "ack":
		j["type"] = "Read"
		j["object"] = h.XID
		if h.Convoy != "" {
			j["context"] = h.Convoy
		}
	case "react":
		j["type"] = "EmojiReact"
		j["object"] = h.XID
		if h.Convoy != "" {
			j["context"] = h.Convoy
		}
		j["content"] = h.Noise
	case "deack":
		b := junk.New()
		b["id"] = user.URL + "/" + "ack" + "/" + shortxid(h.XID)
		b["type"] = "Read"
		b["actor"] = user.URL
		b["object"] = h.XID
		if h.Convoy != "" {
			b["context"] = h.Convoy
		}
		j["type"] = "Undo"
		j["object"] = b
	}

	return j, jo
}

func tooooFancy(noise string) bool {
	return strings.Contains(noise, "<img") || strings.Contains(noise, "<table")
}

var oldjonks = gencache.New(gencache.Options[string, []byte]{Fill: func(xid string) ([]byte, bool) {
	row := stmtAnyXonk.QueryRow(xid)
	honk := scanhonk(row)
	if honk == nil || !honk.Public {
		return nil, true
	}
	user, _ := getUserBio(honk.Username)
	rawhonks := gethonksbyconvoy(honk.UserID, honk.Convoy, 0)
	reversehonks(rawhonks)
	for _, h := range rawhonks {
		if h.RID == honk.XID && h.Public && (h.Whofore == WhoPublic || h.IsAcked()) {
			honk.Replies = append(honk.Replies, h)
		}
	}
	donksforhonks([]*ActivityPubActivity{honk})
	_, j := jonkjonk(user, honk)
	if j == nil {
		elog.Fatalf("what just happened? %v", honk)
	}
	j["@context"] = itiswhatitis

	return j.ToBytes(), true
}, Limit: 128})

func gimmejonk(xid string) ([]byte, bool) {
	j, ok := oldjonks.Get(xid)
	return j, ok
}

func boxuprcpts(user *WhatAbout, addresses []string, useshared bool) map[string]bool {
	rcpts := make(map[string]bool)
	var wg sync.WaitGroup
	var mtx sync.Mutex
	for i := range addresses {
		a := addresses[i]
		if a == "" || a == atContextString || a == user.URL || strings.HasSuffix(a, "/followers") {
			continue
		}
		if a[0] == '%' {
			mtx.Lock()
			rcpts[a] = true
			mtx.Unlock()
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			box, _ := boxofboxes.Get(a)
			mtx.Lock()
			if box != nil && useshared && box.Shared != "" {
				rcpts["%"+box.Shared] = true
			} else {
				rcpts[a] = true
			}
			mtx.Unlock()
		}()
	}
	wg.Wait()
	return rcpts
}

func chonkifymsg(user *WhatAbout, rcpt string, ch *Chonk) []byte {
	dt := ch.Date.Format(time.RFC3339)
	aud := []string{ch.Target}

	jo := junk.New()
	jo["id"] = ch.XID
	jo["type"] = "ChatMessage"
	jo["published"] = dt
	jo["attributedTo"] = user.URL
	jo["to"] = aud
	content := string(ch.HTML)
	if user.ChatSecKey.key != nil {
		if pubkey, ok := getchatkey(rcpt); ok {
			enc, err := encryptString(content, user.ChatSecKey, pubkey)
			if err != nil {
				ilog.Printf("failure encrypting chonk: %s", err)
			} else {
				content = enc
				jo[chatKeyProp] = user.Options.ChatPubKey
			}
		}
	}
	jo["content"] = content

	atts := activatedonks(ch.Donks)
	if len(atts) > 0 {
		jo["attachment"] = atts
	}
	var tags []junk.Junk
	for _, e := range herdofemus(ch.Noise) {
		t := junk.New()
		t["id"] = e.ID
		t["type"] = "Emoji"
		t["name"] = e.Name
		i := junk.New()
		i["type"] = "Image"
		i["mediaType"] = e.Type
		i["url"] = e.ID
		t["icon"] = i
		tags = append(tags, t)
	}
	if len(tags) > 0 {
		jo["tag"] = tags
	}

	j := junk.New()
	j["@context"] = itiswhatitis
	j["id"] = user.URL + "/" + "honk" + "/" + shortxid(ch.XID)
	j["type"] = "Create"
	j["actor"] = user.URL
	j["published"] = dt
	j["to"] = aud
	j["object"] = jo

	return j.ToBytes()
}

func sendchonk(user *WhatAbout, ch *Chonk) {
	rcpts := make(map[string]bool)
	rcpts[ch.Target] = true
	for a := range rcpts {
		msg := chonkifymsg(user, a, ch)
		go deliverate(user.ID, a, msg)
	}
}

func honkworldwide(user *WhatAbout, honk *ActivityPubActivity) {
	jonk, _ := jonkjonk(user, honk)
	jonk["@context"] = itiswhatitis
	msg := jonk.ToBytes()

	aud := honk.Audience

	if honk.Public {
		for _, h := range getdubs(user.ID) {
			if h.XID == user.URL {
				continue
			}
			aud = append(aud, h.XID)
		}
		if honk.What == "update" {
			for _, f := range getbacktracks(honk.XID) {
				aud = append(aud, f)
			}
		}
	}
	rcpts := boxuprcpts(user, aud, honk.Public)

	for a := range rcpts {
		go deliverate(user.ID, a, msg)
	}
	if (honk.What == "honk" || honk.What == "bonk") && honk.Public && len(honk.Onts) > 0 {
		collectiveaction(honk)
	}
}

func collectiveaction(honk *ActivityPubActivity) {
	user := getserveruser()
	for _, ont := range honk.Onts {
		dubs := getnameddubs(firstUserUID, ont)
		if len(dubs) == 0 {
			continue
		}
		j := junk.New()
		j["@context"] = itiswhatitis
		j["type"] = "Add"
		j["id"] = user.URL + "/add/" + shortxid(ont+honk.XID)
		j["actor"] = user.URL
		j["object"] = honk.XID
		j["target"] = serverURL("/o/%s", ont[1:])
		rcpts := make(map[string]bool)
		for _, dub := range dubs {
			box, _ := boxofboxes.Get(dub.XID)
			if box != nil && box.Shared != "" {
				rcpts["%"+box.Shared] = true
			} else {
				rcpts[dub.XID] = true
			}
		}
		msg := j.ToBytes()
		for a := range rcpts {
			go deliverate(user.ID, a, msg)
		}
	}
}

func junkuser(user *WhatAbout, private bool) junk.Junk {
	j := junk.New()
	j["@context"] = []string{itiswhatitis, w3idSecurityString}
	j["id"] = user.URL
	j["inbox"] = user.URL + "/inbox"
	j["outbox"] = user.URL + "/outbox"
	j["name"] = user.Display
	j["preferredUsername"] = user.Name
	j["manuallyApprovesFollowers"] = false // Have to set this or some clients assume you are private
	j["discoverable"] = true
	j["indexable"] = true
	j["memorial"] = false // Only alive users please
	j["summary"] = user.HTAbout
	var tags []junk.Junk
	for _, o := range user.Onts {
		t := junk.New()
		t["type"] = "Hashtag"
		o = strings.ToLower(o)
		t["href"] = serverURL("/o/%s", o[1:])
		t["name"] = o
		tags = append(tags, t)
	}
	if len(tags) > 0 {
		j["tag"] = tags
	}

	if user.ID > 0 {
		j["type"] = "Person"
		j["url"] = user.URL
		j["followers"] = user.URL + "/followers"
		j["following"] = user.URL + "/following"
		a := junk.New()
		a["type"] = "Image"
		a["mediaType"] = "image/png"
		a["url"] = avatarURL(user)
		j["icon"] = a
		if ban := user.Options.Banner; ban != "" {
			a := junk.New()
			a["type"] = "Image"
			a["mediaType"] = "image/jpg"
			a["url"] = ban
			j["image"] = a
		}
	} else {
		j["type"] = "Service"
	}
	k := junk.New()
	k["id"] = user.URL + "#key"
	k["owner"] = user.URL
	k["publicKeyPem"] = user.Key
	j["publicKey"] = k
	j[chatKeyProp] = user.Options.ChatPubKey

	return j
}

var oldjonkers = gencache.New(gencache.Options[string, []byte]{Fill: func(name string) ([]byte, bool) {
	user, err := getUserBio(name)
	if err != nil {
		return nil, false
	}
	j := junkuser(user, false)
	return j.ToBytes(), true
}, Duration: 1 * time.Minute})

func asjonker(name string) []byte {
	j, _ := oldjonkers.Get(name)
	return j
}

var handfull = gencache.New(gencache.Options[string, string]{Fill: func(name string) (string, bool) {
	m := strings.Split(name, "@")
	if len(m) != 2 {
		dlog.Printf("bad fish name: %s", name)
		return "", true
	}
	var href string
	row := stmtGetXonker.QueryRow(name, "fishname")
	err := row.Scan(&href)
	if err == nil {
		return href, true
	}
	dlog.Printf("fishing for %s", name)
	j, err := GetJunkFast(firstUserUID, fmt.Sprintf("https://%s/.well-known/webfinger?resource=acct:%s", m[1], name))
	if err != nil {
		ilog.Printf("failed to go fish %s: %s", name, err)
		return "", true
	}
	links, _ := j.GetArray("links")
	for _, li := range links {
		l, ok := li.(junk.Junk)
		if !ok {
			continue
		}
		href, _ := l.GetString("href")
		rel, _ := l.GetString("rel")
		t, _ := l.GetString("type")
		if rel == "self" && friendorfoe(t) {
			savexonker(name, href, "fishname")
			return href, true
		}
	}
	return href, true
}, Invalidator: &xonkInvalidator})

func gofish(name string) string {
	if name[0] == '@' {
		name = name[1:]
	}
	href, _ := handfull.Get(name)
	return href
}

func investigate(name string) (*SomeThing, junk.Junk, error) {
	if name == "" {
		return nil, nil, fmt.Errorf("no name")
	}
	if name[0] == '@' {
		name = gofish(name)
	}
	if name == "" {
		return nil, nil, fmt.Errorf("no name")
	}
	obj, err := GetJunkFast(firstUserUID, name)
	if err != nil {
		return nil, nil, err
	}
	allinjest(originate(name), obj)
	info, err := somethingabout(obj)
	return info, obj, err
}

func somethingabout(obj junk.Junk) (*SomeThing, error) {
	info := new(SomeThing)
	t, _ := obj.GetString("type")
	isowned := false
	switch t {
	case "Person":
		fallthrough
	case "Group":
		fallthrough
	case "Organization":
		fallthrough
	case "Application":
		fallthrough
	case "Service":
		fallthrough
	case "Team":
		fallthrough
	case "Project":
		fallthrough
	case "Repository":
		info.What = SomeActor
	case "OrderedCollection":
		isowned = true
		fallthrough
	case "Collection":
		info.What = SomeCollection
	default:
		return nil, fmt.Errorf("unknown object type")
	}
	info.XID, _ = obj.GetString("id")
	info.Name, _ = obj.GetString("preferredUsername")
	if info.Name == "" {
		info.Name, _ = obj.GetString("name")
	}
	if isowned {
		info.Owner, _ = obj.GetString("attributedTo")
	}
	if info.Owner == "" {
		info.Owner = info.XID
	}

	iconInfo, ok := obj.GetMap("icon")
	if ok {
		mType, _ := iconInfo.GetString("mediaType")
		if strings.HasPrefix(mType, "image/") {
			AvatarUrl, ok := iconInfo.GetString("url")
			if ok {
				info.AvatarURL = AvatarUrl
			}
		}
	}

	return info, nil
}

func allinjest(origin string, obj junk.Junk) {
	ident, _ := obj.GetString("id")
	if ident == "" {
		return
	}
	if originate(ident) != origin {
		return
	}
	keyobj, ok := obj.GetMap("publicKey")
	if ok {
		ingestpubkey(origin, keyobj)
	}
	ingestboxes(origin, obj)
	ingesthandle(origin, obj)
	chatkey, ok := obj.GetString(chatKeyProp)
	if ok {
		savexonker(ident, chatkey, chatKeyProp)
	}
}

func ingestpubkey(origin string, obj junk.Junk) {
	keyobj, ok := obj.GetMap("publicKey")
	if ok {
		obj = keyobj
	}
	keyname, ok := obj.GetString("id")
	var data string
	row := stmtGetXonker.QueryRow(keyname, "pubkey")
	err := row.Scan(&data)
	if err == nil {
		return
	}
	if !ok || origin != originate(keyname) {
		ilog.Printf("bad key origin %s <> %s", origin, keyname)
		return
	}
	dlog.Printf("ingesting a needed pubkey: %s", keyname)
	owner, ok := obj.GetString("owner")
	if !ok {
		ilog.Printf("error finding %s pubkey owner", keyname)
		return
	}
	data, ok = obj.GetString("publicKeyPem")
	if !ok {
		ilog.Printf("error finding %s pubkey", keyname)
		return
	}
	if originate(owner) != origin {
		ilog.Printf("bad key owner: %s <> %s", owner, origin)
		return
	}
	_, _, err = httpsig.DecodeKey(data)
	if err != nil {
		ilog.Printf("error decoding %s pubkey: %s", keyname, err)
		return
	}
	when := time.Now().UTC().Format(dbtimeformat)
	_, err = stmtSaveXonker.Exec(keyname, data, "pubkey", when)
	if err != nil {
		elog.Printf("error saving key: %s", err)
	}
}

func ingestboxes(origin string, obj junk.Junk) {
	ident, _ := obj.GetString("id")
	if ident == "" {
		return
	}
	if originate(ident) != origin {
		return
	}
	var info string
	row := stmtGetXonker.QueryRow(ident, "boxes")
	err := row.Scan(&info)
	if err == nil {
		return
	}
	dlog.Printf("ingesting boxes: %s", ident)
	inbox, _ := obj.GetString("inbox")
	outbox, _ := obj.GetString("outbox")
	sbox, _ := obj.GetString("endpoints", "sharedInbox")
	if inbox != "" {
		m := strings.Join([]string{inbox, outbox, sbox}, " ")
		savexonker(ident, m, "boxes")
	}
}

func ingesthandle(origin string, obj junk.Junk) {
	xid, _ := obj.GetString("id")
	if xid == "" {
		return
	}
	if originate(xid) != origin {
		return
	}
	var handle string
	row := stmtGetXonker.QueryRow(xid, "handle")
	err := row.Scan(&handle)
	if err == nil {
		return
	}
	handle, _ = obj.GetString("preferredUsername")
	if handle != "" {
		savexonker(xid, handle, "handle")
	}
}

func updateMe(username string) {
	user, _ := somenamedusers.Get(username)
	dt := time.Now().UTC().Format(time.RFC3339)
	j := junk.New()
	j["@context"] = itiswhatitis
	j["id"] = fmt.Sprintf("%s/upme/%s/%d", user.URL, user.Name, time.Now().Unix())
	j["actor"] = user.URL
	j["published"] = dt
	j["to"] = atContextString
	j["type"] = "Update"
	j["object"] = junkuser(user, false)

	msg := j.ToBytes()

	rcpts := make(map[string]bool)
	for _, f := range getdubs(user.ID) {
		if f.XID == user.URL {
			continue
		}
		box, _ := boxofboxes.Get(f.XID)
		if box != nil && box.Shared != "" {
			rcpts["%"+box.Shared] = true
		} else {
			rcpts[f.XID] = true
		}
	}
	for a := range rcpts {
		go deliverate(user.ID, a, msg)
	}
}

func followme(user *WhatAbout, who string, name string, j junk.Junk) {
	folxid, _ := j.GetString("id")

	ilog.Printf("updating honker follow: %s %s", who, folxid)

	var x string
	db := opendatabase()
	row := db.QueryRow("select xid from honkers where name = ? and xid = ? and userid = ? and flavor in ('dub', 'undub')", name, who, user.ID)
	err := row.Scan(&x)
	if err != sql.ErrNoRows {
		ilog.Printf("duplicate follow request: %s", who)
		_, err = stmtUpdateFlavor.Exec("dub", folxid, user.ID, name, who, "undub")
		if err != nil {
			elog.Printf("error updating honker: %s", err)
		}
	} else {
		stmtSaveDub.Exec(user.ID, name, who, "dub", folxid)
	}
	calculateFollowersForMetrics()
	go rubadubdub(user, j)
}

func unfollowme(user *WhatAbout, who string, name string, j junk.Junk) {
	var folxid string
	if who == "" {
		folxid, _ = j.GetString("object")

		db := opendatabase()
		row := db.QueryRow("select xid, name from honkers where userid = ? and folxid = ? and flavor in ('dub', 'undub')", user.ID, folxid)
		err := row.Scan(&who, &name)
		if err != nil {
			if err != sql.ErrNoRows {
				elog.Printf("error scanning honker: %s", err)
			}
			return
		}
	}

	ilog.Printf("updating honker undo: %s %s", who, folxid)
	_, err := stmtUpdateFlavor.Exec("undub", folxid, user.ID, name, who, "dub")
	if err != nil {
		elog.Printf("error updating honker: %s", err)
		return
	}
}

func followyou(user *WhatAbout, honkerid int64, sync bool) {
	var url, owner string
	db := opendatabase()
	row := db.QueryRow("select xid, owner from honkers where honkerid = ? and userid = ? and flavor in ('unsub', 'peep', 'presub', 'sub')",
		honkerid, user.ID)
	err := row.Scan(&url, &owner)
	if err != nil {
		elog.Printf("can't get honker xid: %s", err)
		return
	}
	folxid := make18CharRandomString()
	ilog.Printf("subscribing to %s", url)
	_, err = db.Exec("update honkers set flavor = ?, folxid = ? where honkerid = ?", "presub", folxid, honkerid)
	if err != nil {
		elog.Printf("error updating honker: %s", err)
		return
	}
	if sync {
		subsub(user, url, owner, folxid)
	} else {
		go subsub(user, url, owner, folxid)
	}

}
func unfollowyou(user *WhatAbout, honkerid int64, sync bool) {
	db := opendatabase()
	row := db.QueryRow("select xid, owner, folxid, flavor from honkers where honkerid = ? and userid = ? and flavor in ('unsub', 'peep', 'presub', 'sub')",
		honkerid, user.ID)
	var url, owner, folxid, flavor string
	err := row.Scan(&url, &owner, &folxid, &flavor)
	if err != nil {
		elog.Printf("can't get honker xid: %s", err)
		return
	}
	if flavor == "peep" {
		return
	}
	ilog.Printf("unsubscribing from %s", url)
	_, err = db.Exec("update honkers set flavor = ? where honkerid = ?", "unsub", honkerid)
	if err != nil {
		elog.Printf("error updating honker: %s", err)
		return
	}
	if sync {
		itakeitallback(user, url, owner, folxid)
	} else {
		go itakeitallback(user, url, owner, folxid)
	}
}

func followyou2(user *WhatAbout, j junk.Junk) {
	who, _ := j.GetString("actor")

	ilog.Printf("updating honker accept: %s", who)
	db := opendatabase()
	row := db.QueryRow("select name, folxid from honkers where userid = ? and xid = ? and flavor in ('presub', 'sub')",
		user.ID, who)
	var name, folxid string
	err := row.Scan(&name, &folxid)
	if err != nil {
		elog.Printf("can't get honker name: %s", err)
		return
	}
	_, err = stmtUpdateFlavor.Exec("sub", folxid, user.ID, name, who, "presub")
	if err != nil {
		elog.Printf("error updating honker: %s", err)
		return
	}
}

func nofollowyou2(user *WhatAbout, j junk.Junk) {
	who, _ := j.GetString("actor")

	ilog.Printf("updating honker reject: %s", who)
	db := opendatabase()
	row := db.QueryRow("select name, folxid from honkers where userid = ? and xid = ? and flavor in ('presub', 'sub')",
		user.ID, who)
	var name, folxid string
	err := row.Scan(&name, &folxid)
	if err != nil {
		elog.Printf("can't get honker name: %s", err)
		return
	}
	_, err = stmtUpdateFlavor.Exec("unsub", folxid, user.ID, name, who, "presub")
	_, err = stmtUpdateFlavor.Exec("unsub", folxid, user.ID, name, who, "sub")
	if err != nil {
		elog.Printf("error updating honker: %s", err)
		return
	}
}
