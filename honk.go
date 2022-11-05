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
	"fmt"
	"html"
	"html/template"
	"io"
	"log"
	notrand "math/rand"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"humungus.tedunangst.com/r/webs/htfilter"
	"humungus.tedunangst.com/r/webs/httpsig"
	"humungus.tedunangst.com/r/webs/image"
	"humungus.tedunangst.com/r/webs/junk"
	"humungus.tedunangst.com/r/webs/login"
	"humungus.tedunangst.com/r/webs/rss"
	"humungus.tedunangst.com/r/webs/templates"
)

type WhatAbout struct {
	ID        int64
	Name      string
	Display   string
	About     string
	Key       string
	URL       string
	SkinnyCSS bool
}

type Honk struct {
	ID       int64
	UserID   int64
	Username string
	What     string
	Honker   string
	Handle   string
	Oonker   string
	Oondle   string
	XID      string
	RID      string
	Date     time.Time
	URL      string
	Noise    string
	Precis   string
	Convoy   string
	Audience []string
	Public   bool
	Whofore  int64
	HTML     template.HTML
	Style    string
	Open     string
	Donks    []*Donk
}

type Donk struct {
	FileID  int64
	XID     string
	Name    string
	URL     string
	Media   string
	Local   bool
	Content []byte
}

type Honker struct {
	ID     int64
	UserID int64
	Name   string
	XID    string
	Flavor string
	Combos []string
}

var serverName string
var iconName = "icon.png"
var serverMsg = "Things happen."

var userSep = "u"
var honkSep = "h"

var readviews *templates.Template

func getuserstyle(u *login.UserInfo) template.CSS {
	if u == nil {
		return ""
	}
	user, _ := butwhatabout(u.Username)
	if user.SkinnyCSS {
		return "main { max-width: 700px; }"
	}
	return ""
}

func getInfo(r *http.Request) map[string]interface{} {
	u := login.GetUserInfo(r)
	templinfo := make(map[string]interface{})
	templinfo["StyleParam"] = getstyleparam("views/style.css")
	templinfo["LocalStyleParam"] = getstyleparam("views/local.css")
	templinfo["UserStyle"] = getuserstyle(u)
	templinfo["ServerName"] = serverName
	templinfo["IconName"] = iconName
	templinfo["UserInfo"] = u
	templinfo["UserSep"] = userSep
	return templinfo
}

var donotfedafterdark = make(map[string]bool)

func stealthed(r *http.Request) bool {
	addr := r.Header.Get("X-Forwarded-For")
	fake := donotfedafterdark[addr]
	if fake {
		log.Printf("faking 404 for %s", addr)
	}
	return fake
}

func homepage(w http.ResponseWriter, r *http.Request) {
	templinfo := getInfo(r)
	u := login.GetUserInfo(r)
	var honks []*Honk
	var userid int64 = -1
	if r.URL.Path == "/front" || u == nil {
		honks = getpublichonks()
	} else {
		userid = u.UserID
		if r.URL.Path == "/atme" {
			honks = gethonksforme(userid)
		} else {
			honks = gethonksforuser(userid)
			honks = osmosis(honks, userid)
		}
		templinfo["HonkCSRF"] = login.GetCSRF("honkhonk", r)
	}

	reverbolate(userid, honks)

	templinfo["Honks"] = honks
	templinfo["ShowRSS"] = true
	templinfo["ServerMessage"] = serverMsg
	if u == nil {
		w.Header().Set("Cache-Control", "max-age=60")
	} else {
		w.Header().Set("Cache-Control", "max-age=0")
	}
	err := readviews.Execute(w, "honkpage.html", templinfo)
	if err != nil {
		log.Print(err)
	}
}

func showfunzone(w http.ResponseWriter, r *http.Request) {
	var emunames, memenames []string
	dir, err := os.Open("emus")
	if err == nil {
		emunames, _ = dir.Readdirnames(0)
		dir.Close()
	}
	for i, e := range emunames {
		if len(e) > 4 {
			emunames[i] = e[:len(e)-4]
		}
	}
	dir, err = os.Open("memes")
	if err == nil {
		memenames, _ = dir.Readdirnames(0)
		dir.Close()
	}
	templinfo := getInfo(r)
	templinfo["Emus"] = emunames
	templinfo["Memes"] = memenames
	err = readviews.Execute(w, "funzone.html", templinfo)
	if err != nil {
		log.Print(err)
	}

}

func showrss(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	var honks []*Honk
	if name != "" {
		honks = gethonksbyuser(name, false)
	} else {
		honks = getpublichonks()
	}
	reverbolate(-1, honks)

	home := fmt.Sprintf("https://%s/", serverName)
	base := home
	if name != "" {
		home += "u/" + name
		name += " "
	}
	feed := rss.Feed{
		Title:       name + "honk",
		Link:        home,
		Description: name + "honk rss",
		Image: &rss.Image{
			URL:   base + "icon.png",
			Title: name + "honk rss",
			Link:  home,
		},
	}
	var modtime time.Time
	for _, honk := range honks {
		desc := string(honk.HTML)
		for _, d := range honk.Donks {
			desc += fmt.Sprintf(`<p><a href="%s">Attachment: %s</a>`,
				d.URL, html.EscapeString(d.Name))
		}

		feed.Items = append(feed.Items, &rss.Item{
			Title:       fmt.Sprintf("%s %s %s", honk.Username, honk.What, honk.XID),
			Description: rss.CData{desc},
			Link:        honk.URL,
			PubDate:     honk.Date.Format(time.RFC1123),
			Guid:        &rss.Guid{IsPermaLink: true, Value: honk.URL},
		})
		if honk.Date.After(modtime) {
			modtime = honk.Date
		}
	}
	w.Header().Set("Cache-Control", "max-age=300")
	w.Header().Set("Last-Modified", modtime.Format(http.TimeFormat))

	err := feed.Write(w)
	if err != nil {
		log.Printf("error writing rss: %s", err)
	}
}

func butwhatabout(name string) (*WhatAbout, error) {
	row := stmtWhatAbout.QueryRow(name)
	var user WhatAbout
	var options string
	err := row.Scan(&user.ID, &user.Name, &user.Display, &user.About, &user.Key, &options)
	user.URL = fmt.Sprintf("https://%s/%s/%s", serverName, userSep, user.Name)
	user.SkinnyCSS = strings.Contains(options, " skinny ")
	return &user, err
}

func crappola(j junk.Junk) bool {
	t, _ := j.GetString("type")
	a, _ := j.GetString("actor")
	o, _ := j.GetString("object")
	if t == "Delete" && a == o {
		log.Printf("crappola from %s", a)
		return true
	}
	return false
}

func ping(user *WhatAbout, who string) {
	box, err := getboxes(who)
	if err != nil {
		log.Printf("no inbox for ping: %s", err)
		return
	}
	j := junk.New()
	j["@context"] = itiswhatitis
	j["type"] = "Ping"
	j["id"] = user.URL + "/ping/" + xfiltrate()
	j["actor"] = user.URL
	j["to"] = who
	keyname, key := ziggy(user.Name)
	err = PostJunk(keyname, key, box.In, j)
	if err != nil {
		log.Printf("can't send ping: %s", err)
		return
	}
	log.Printf("sent ping to %s: %s", who, j["id"])
}

func pong(user *WhatAbout, who string, obj string) {
	box, err := getboxes(who)
	if err != nil {
		log.Printf("no inbox for pong %s : %s", who, err)
		return
	}
	j := junk.New()
	j["@context"] = itiswhatitis
	j["type"] = "Pong"
	j["id"] = user.URL + "/pong/" + xfiltrate()
	j["actor"] = user.URL
	j["to"] = who
	j["object"] = obj
	keyname, key := ziggy(user.Name)
	err = PostJunk(keyname, key, box.In, j)
	if err != nil {
		log.Printf("can't send pong: %s", err)
		return
	}
}

func inbox(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	user, err := butwhatabout(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	var buf bytes.Buffer
	io.Copy(&buf, r.Body)
	payload := buf.Bytes()
	j, err := junk.Read(bytes.NewReader(payload))
	if err != nil {
		log.Printf("bad payload: %s", err)
		io.WriteString(os.Stdout, "bad payload\n")
		os.Stdout.Write(payload)
		io.WriteString(os.Stdout, "\n")
		return
	}
	if crappola(j) {
		return
	}
	keyname, err := httpsig.VerifyRequest(r, payload, zaggy)
	if err != nil {
		log.Printf("inbox message failed signature: %s", err)
		if keyname != "" {
			keyname, err = makeitworksomehowwithoutregardforkeycontinuity(keyname, r, payload)
		}
		if err != nil {
			return
		}
	}
	what, _ := j.GetString("type")
	if what == "Like" {
		return
	}
	who, _ := j.GetString("actor")
	origin := keymatch(keyname, who)
	if origin == "" {
		log.Printf("keyname actor mismatch: %s <> %s", keyname, who)
		return
	}
	objid, _ := j.GetString("id")
	if thoudostbitethythumb(user.ID, []string{who}, objid) {
		log.Printf("ignoring thumb sucker %s", who)
		return
	}
	switch what {
	case "Ping":
		obj, _ := j.GetString("id")
		log.Printf("ping from %s: %s", who, obj)
		pong(user, who, obj)
	case "Pong":
		obj, _ := j.GetString("object")
		log.Printf("pong from %s: %s", who, obj)
	case "Follow":
		obj, _ := j.GetString("object")
		if obj == user.URL {
			log.Printf("updating honker follow: %s", who)
			stmtSaveDub.Exec(user.ID, who, who, "dub")
			go rubadubdub(user, j)
		} else {
			log.Printf("can't follow %s", obj)
		}
	case "Accept":
		log.Printf("updating honker accept: %s", who)
		_, err = stmtUpdateFlavor.Exec("sub", user.ID, who, "presub")
		if err != nil {
			log.Printf("error updating honker: %s", err)
			return
		}
	case "Update":
		obj, ok := j.GetMap("object")
		if ok {
			what, _ := obj.GetString("type")
			switch what {
			case "Person":
				return
			}
		}
		log.Printf("unknown Update activity")
		fd, _ := os.OpenFile("savedinbox.json", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		j.Write(fd)
		io.WriteString(fd, "\n")
		fd.Close()

	case "Undo":
		obj, ok := j.GetMap("object")
		if !ok {
			log.Printf("unknown undo no object")
		} else {
			what, _ := obj.GetString("type")
			switch what {
			case "Follow":
				log.Printf("updating honker undo: %s", who)
				_, err = stmtUpdateFlavor.Exec("undub", user.ID, who, "dub")
				if err != nil {
					log.Printf("error updating honker: %s", err)
					return
				}
			case "Like":
			case "Announce":
			default:
				log.Printf("unknown undo: %s", what)
			}
		}
	default:
		go consumeactivity(user, j, origin)
	}
}

func ximport(w http.ResponseWriter, r *http.Request) {
	xid := r.FormValue("xid")
	x := investigate(xid)
	if x != "" {
		xid = x
	}
	j, err := GetJunk(xid)
	if err != nil {
		http.Error(w, "error getting external object", http.StatusInternalServerError)
		log.Printf("error getting external object: %s", err)
		return
	}
	log.Printf("importing %s", xid)
	u := login.GetUserInfo(r)
	user, _ := butwhatabout(u.Username)

	what, _ := j.GetString("type")
	if isactor(what) {
		outbox, _ := j.GetString("outbox")
		gimmexonks(user, outbox)
		http.Redirect(w, r, "/h?xid="+url.QueryEscape(xid), http.StatusSeeOther)
		return
	}
	xonk := xonkxonk(user, j, originate(xid))
	convoy := ""
	if xonk != nil {
		convoy = xonk.Convoy
		savexonk(user, xonk)
	}
	http.Redirect(w, r, "/t?c="+url.QueryEscape(convoy), http.StatusSeeOther)
}

func xzone(w http.ResponseWriter, r *http.Request) {
	u := login.GetUserInfo(r)
	var honkers []string
	rows, err := stmtRecentHonkers.Query(u.UserID)
	if err != nil {
		log.Printf("query err: %s", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var s string
		rows.Scan(&s)
		honkers = append(honkers, s)
	}

	templinfo := getInfo(r)
	templinfo["XCSRF"] = login.GetCSRF("ximport", r)
	templinfo["Honkers"] = honkers
	err = readviews.Execute(w, "xzone.html", templinfo)
	if err != nil {
		log.Print(err)
	}
}

func outbox(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	user, err := butwhatabout(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if stealthed(r) {
		http.NotFound(w, r)
		return
	}

	honks := gethonksbyuser(name, false)

	var jonks []junk.Junk
	for _, h := range honks {
		j, _ := jonkjonk(user, h)
		jonks = append(jonks, j)
	}

	j := junk.New()
	j["@context"] = itiswhatitis
	j["id"] = user.URL + "/outbox"
	j["type"] = "OrderedCollection"
	j["totalItems"] = len(jonks)
	j["orderedItems"] = jonks

	w.Header().Set("Content-Type", theonetruename)
	j.Write(w)
}

func emptiness(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	user, err := butwhatabout(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	colname := "/followers"
	if strings.HasSuffix(r.URL.Path, "/following") {
		colname = "/following"
	}
	j := junk.New()
	j["@context"] = itiswhatitis
	j["id"] = user.URL + colname
	j["type"] = "OrderedCollection"
	j["totalItems"] = 0
	j["orderedItems"] = []junk.Junk{}

	w.Header().Set("Content-Type", theonetruename)
	j.Write(w)
}

func showuser(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	user, err := butwhatabout(name)
	if err != nil {
		log.Printf("user not found %s: %s", name, err)
		http.NotFound(w, r)
		return
	}
	if friendorfoe(r.Header.Get("Accept")) {
		j := asjonker(user)
		w.Header().Set("Content-Type", theonetruename)
		j.Write(w)
		return
	}
	u := login.GetUserInfo(r)
	honks := gethonksbyuser(name, u != nil && u.Username == name)
	honkpage(w, r, u, user, honks, "")
}

func showhonker(w http.ResponseWriter, r *http.Request) {
	u := login.GetUserInfo(r)
	name := mux.Vars(r)["name"]
	var honks []*Honk
	if name == "" {
		name = r.FormValue("xid")
		honks = gethonksbyxonker(u.UserID, name)
	} else {
		honks = gethonksbyhonker(u.UserID, name)
	}
	name = html.EscapeString(name)
	msg := fmt.Sprintf(`honks by honker: <a href="%s" ref="noreferrer">%s</a>`, name, name)
	honkpage(w, r, u, nil, honks, template.HTML(msg))
}

func showcombo(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	u := login.GetUserInfo(r)
	honks := gethonksbycombo(u.UserID, name)
	honks = osmosis(honks, u.UserID)
	honkpage(w, r, u, nil, honks, template.HTML(html.EscapeString("honks by combo: "+name)))
}
func showconvoy(w http.ResponseWriter, r *http.Request) {
	c := r.FormValue("c")
	u := login.GetUserInfo(r)
	honks := gethonksbyconvoy(u.UserID, c)
	honkpage(w, r, u, nil, honks, template.HTML(html.EscapeString("honks in convoy: "+c)))
}

func showhonk(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	user, err := butwhatabout(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if stealthed(r) {
		http.NotFound(w, r)
		return
	}

	xid := fmt.Sprintf("https://%s%s", serverName, r.URL.Path)
	h := getxonk(user.ID, xid)
	if h == nil {
		http.NotFound(w, r)
		return
	}
	u := login.GetUserInfo(r)
	if u != nil && u.UserID != user.ID {
		u = nil
	}
	if !h.Public {
		if u == nil {
			http.NotFound(w, r)
			return

		}
		honkpage(w, r, u, nil, []*Honk{h}, "one honk maybe more")
		return
	}
	if friendorfoe(r.Header.Get("Accept")) {
		donksforhonks([]*Honk{h})
		_, j := jonkjonk(user, h)
		j["@context"] = itiswhatitis
		w.Header().Set("Content-Type", theonetruename)
		j.Write(w)
		return
	}
	honks := gethonksbyconvoy(-1, h.Convoy)
	honkpage(w, r, u, nil, honks, "one honk maybe more")
}

func honkpage(w http.ResponseWriter, r *http.Request, u *login.UserInfo, user *WhatAbout,
	honks []*Honk, infomsg template.HTML) {
	templinfo := getInfo(r)
	var userid int64 = -1
	if u != nil {
		templinfo["HonkCSRF"] = login.GetCSRF("honkhonk", r)
		userid = u.UserID
	}
	if u == nil {
		w.Header().Set("Cache-Control", "max-age=60")
	}
	reverbolate(userid, honks)
	if user != nil {
		filt := htfilter.New()
		templinfo["Name"] = user.Name
		whatabout := user.About
		whatabout = obfusbreak(user.About)
		templinfo["WhatAbout"], _ = filt.String(whatabout)
	}
	templinfo["Honks"] = honks
	templinfo["ServerMessage"] = infomsg
	err := readviews.Execute(w, "honkpage.html", templinfo)
	if err != nil {
		log.Print(err)
	}
}

func saveuser(w http.ResponseWriter, r *http.Request) {
	whatabout := r.FormValue("whatabout")
	u := login.GetUserInfo(r)
	db := opendatabase()
	options := ""
	if r.FormValue("skinny") == "skinny" {
		options += " skinny "
	}
	_, err := db.Exec("update users set about = ?, options = ? where username = ?", whatabout, options, u.Username)
	if err != nil {
		log.Printf("error bouting what: %s", err)
	}

	http.Redirect(w, r, "/account", http.StatusSeeOther)
}

func gethonkers(userid int64) []*Honker {
	rows, err := stmtHonkers.Query(userid)
	if err != nil {
		log.Printf("error querying honkers: %s", err)
		return nil
	}
	defer rows.Close()
	var honkers []*Honker
	for rows.Next() {
		var f Honker
		var combos string
		err = rows.Scan(&f.ID, &f.UserID, &f.Name, &f.XID, &f.Flavor, &combos)
		f.Combos = strings.Split(strings.TrimSpace(combos), " ")
		if err != nil {
			log.Printf("error scanning honker: %s", err)
			return nil
		}
		honkers = append(honkers, &f)
	}
	return honkers
}

func getdubs(userid int64) []*Honker {
	rows, err := stmtDubbers.Query(userid)
	if err != nil {
		log.Printf("error querying dubs: %s", err)
		return nil
	}
	defer rows.Close()
	var honkers []*Honker
	for rows.Next() {
		var f Honker
		err = rows.Scan(&f.ID, &f.UserID, &f.Name, &f.XID, &f.Flavor)
		if err != nil {
			log.Printf("error scanning honker: %s", err)
			return nil
		}
		honkers = append(honkers, &f)
	}
	return honkers
}

func allusers() []login.UserInfo {
	var users []login.UserInfo
	rows, _ := opendatabase().Query("select userid, username from users")
	defer rows.Close()
	for rows.Next() {
		var u login.UserInfo
		rows.Scan(&u.UserID, &u.Username)
		users = append(users, u)
	}
	return users
}

func getxonk(userid int64, xid string) *Honk {
	h := new(Honk)
	var dt, aud string
	row := stmtOneXonk.QueryRow(userid, xid)
	err := row.Scan(&h.ID, &h.UserID, &h.Username, &h.What, &h.Honker, &h.Oonker, &h.XID, &h.RID,
		&dt, &h.URL, &aud, &h.Noise, &h.Precis, &h.Convoy, &h.Whofore)
	if err != nil {
		if err != sql.ErrNoRows {
			log.Printf("error scanning xonk: %s", err)
		}
		return nil
	}
	h.Date, _ = time.Parse(dbtimeformat, dt)
	h.Audience = strings.Split(aud, " ")
	h.Public = !keepitquiet(h.Audience)
	return h
}

func getpublichonks() []*Honk {
	dt := time.Now().UTC().Add(-7 * 24 * time.Hour).Format(dbtimeformat)
	rows, err := stmtPublicHonks.Query(dt)
	return getsomehonks(rows, err)
}
func gethonksbyuser(name string, includeprivate bool) []*Honk {
	dt := time.Now().UTC().Add(-7 * 24 * time.Hour).Format(dbtimeformat)
	whofore := 2
	if includeprivate {
		whofore = 3
	}
	rows, err := stmtUserHonks.Query(whofore, name, dt)
	return getsomehonks(rows, err)
}
func gethonksforuser(userid int64) []*Honk {
	dt := time.Now().UTC().Add(-7 * 24 * time.Hour).Format(dbtimeformat)
	rows, err := stmtHonksForUser.Query(userid, dt, userid, userid)
	return getsomehonks(rows, err)
}
func gethonksforme(userid int64) []*Honk {
	dt := time.Now().UTC().Add(-7 * 24 * time.Hour).Format(dbtimeformat)
	rows, err := stmtHonksForMe.Query(userid, dt, userid)
	return getsomehonks(rows, err)
}
func gethonksbyhonker(userid int64, honker string) []*Honk {
	rows, err := stmtHonksByHonker.Query(userid, honker, userid)
	return getsomehonks(rows, err)
}
func gethonksbyxonker(userid int64, xonker string) []*Honk {
	rows, err := stmtHonksByXonker.Query(userid, xonker, xonker, userid)
	return getsomehonks(rows, err)
}
func gethonksbycombo(userid int64, combo string) []*Honk {
	combo = "% " + combo + " %"
	rows, err := stmtHonksByCombo.Query(userid, combo, userid)
	return getsomehonks(rows, err)
}
func gethonksbyconvoy(userid int64, convoy string) []*Honk {
	rows, err := stmtHonksByConvoy.Query(userid, userid, convoy)
	honks := getsomehonks(rows, err)
	for i, j := 0, len(honks)-1; i < j; i, j = i+1, j-1 {
		honks[i], honks[j] = honks[j], honks[i]
	}
	return honks
}

func getsomehonks(rows *sql.Rows, err error) []*Honk {
	if err != nil {
		log.Printf("error querying honks: %s", err)
		return nil
	}
	defer rows.Close()
	var honks []*Honk
	for rows.Next() {
		var h Honk
		var dt, aud string
		err = rows.Scan(&h.ID, &h.UserID, &h.Username, &h.What, &h.Honker, &h.Oonker,
			&h.XID, &h.RID, &dt, &h.URL, &aud, &h.Noise, &h.Precis, &h.Convoy, &h.Whofore)
		if err != nil {
			log.Printf("error scanning honks: %s", err)
			return nil
		}
		h.Date, _ = time.Parse(dbtimeformat, dt)
		h.Audience = strings.Split(aud, " ")
		h.Public = !keepitquiet(h.Audience)
		honks = append(honks, &h)
	}
	rows.Close()
	donksforhonks(honks)
	return honks
}

func donksforhonks(honks []*Honk) {
	db := opendatabase()
	var ids []string
	hmap := make(map[int64]*Honk)
	for _, h := range honks {
		ids = append(ids, fmt.Sprintf("%d", h.ID))
		hmap[h.ID] = h
	}
	q := fmt.Sprintf("select honkid, donks.fileid, xid, name, url, media, local from donks join files on donks.fileid = files.fileid where honkid in (%s)", strings.Join(ids, ","))
	rows, err := db.Query(q)
	if err != nil {
		log.Printf("error querying donks: %s", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var hid int64
		var d Donk
		err = rows.Scan(&hid, &d.FileID, &d.XID, &d.Name, &d.URL, &d.Media, &d.Local)
		if err != nil {
			log.Printf("error scanning donk: %s", err)
			continue
		}
		h := hmap[hid]
		h.Donks = append(h.Donks, &d)
	}
}

func savebonk(w http.ResponseWriter, r *http.Request) {
	xid := r.FormValue("xid")
	userinfo := login.GetUserInfo(r)
	user, _ := butwhatabout(userinfo.Username)

	log.Printf("bonking %s", xid)

	xonk := getxonk(userinfo.UserID, xid)
	if xonk == nil {
		return
	}
	if !xonk.Public {
		return
	}
	donksforhonks([]*Honk{xonk})

	oonker := xonk.Oonker
	if oonker == "" {
		oonker = xonk.Honker
	}
	dt := time.Now().UTC()
	bonk := Honk{
		UserID:   userinfo.UserID,
		Username: userinfo.Username,
		What:     "bonk",
		Honker:   user.URL,
		XID:      xonk.XID,
		Date:     dt,
		Donks:    xonk.Donks,
		Convoy:   xonk.Convoy,
		Audience: []string{oonker, thewholeworld},
		Public:   true,
	}

	aud := strings.Join(bonk.Audience, " ")
	whofore := 2
	res, err := stmtSaveHonk.Exec(userinfo.UserID, "bonk", bonk.Honker, xid, "",
		dt.Format(dbtimeformat), "", aud, xonk.Noise, xonk.Convoy, whofore, "html",
		xonk.Precis, oonker)
	if err != nil {
		log.Printf("error saving bonk: %s", err)
		return
	}
	bonk.ID, _ = res.LastInsertId()
	for _, d := range bonk.Donks {
		_, err = stmtSaveDonk.Exec(bonk.ID, d.FileID)
		if err != nil {
			log.Printf("err saving donk: %s", err)
			return
		}
	}

	go honkworldwide(user, &bonk)
}

func zonkit(w http.ResponseWriter, r *http.Request) {
	wherefore := r.FormValue("wherefore")
	what := r.FormValue("what")
	switch wherefore {
	case "zonk":
	case "zonvoy":
	}

	log.Printf("zonking %s %s", wherefore, what)
	userinfo := login.GetUserInfo(r)
	if wherefore == "zonk" {
		xonk := getxonk(userinfo.UserID, what)
		if xonk != nil {
			stmtZonkDonks.Exec(xonk.ID)
			stmtZonkIt.Exec(userinfo.UserID, what)
			if xonk.Whofore == 2 || xonk.Whofore == 3 {
				zonk := Honk{
					What:     "zonk",
					XID:      xonk.XID,
					Date:     time.Now().UTC(),
					Audience: oneofakind(xonk.Audience),
				}
				zonk.Public = !keepitquiet(zonk.Audience)

				user, _ := butwhatabout(userinfo.Username)
				log.Printf("announcing deleted honk: %s", what)
				go honkworldwide(user, &zonk)
			}
		}
	}
	_, err := stmtSaveZonker.Exec(userinfo.UserID, what, wherefore)
	if err != nil {
		log.Printf("error saving zonker: %s", err)
		return
	}
}

func savehonk(w http.ResponseWriter, r *http.Request) {
	rid := r.FormValue("rid")
	noise := r.FormValue("noise")

	userinfo := login.GetUserInfo(r)
	user, _ := butwhatabout(userinfo.Username)

	dt := time.Now().UTC()
	xid := fmt.Sprintf("%s/%s/%s", user.URL, honkSep, xfiltrate())
	what := "honk"
	if rid != "" {
		what = "tonk"
	}
	honk := Honk{
		UserID:   userinfo.UserID,
		Username: userinfo.Username,
		What:     "honk",
		Honker:   user.URL,
		XID:      xid,
		Date:     dt,
	}
	if strings.HasPrefix(noise, "DZ:") {
		idx := strings.Index(noise, "\n")
		if idx == -1 {
			honk.Precis = noise
			noise = ""
		} else {
			honk.Precis = noise[:idx]
			noise = noise[idx+1:]
		}
	}
	noise = hooterize(noise)
	noise = strings.TrimSpace(noise)
	honk.Precis = strings.TrimSpace(honk.Precis)

	var convoy string
	if rid != "" {
		xonk := getxonk(userinfo.UserID, rid)
		if xonk != nil {
			if xonk.Public {
				honk.Audience = append(honk.Audience, xonk.Audience...)
			}
			convoy = xonk.Convoy
		} else {
			xonkaud, c := whosthere(rid)
			honk.Audience = append(honk.Audience, xonkaud...)
			convoy = c
		}
		for i, a := range honk.Audience {
			if a == thewholeworld {
				honk.Audience[0], honk.Audience[i] = honk.Audience[i], honk.Audience[0]
				break
			}
		}
		honk.RID = rid
	} else {
		honk.Audience = []string{thewholeworld}
	}
	if noise != "" && noise[0] == '@' {
		honk.Audience = append(grapevine(noise), honk.Audience...)
	} else {
		honk.Audience = append(honk.Audience, grapevine(noise)...)
	}
	if convoy == "" {
		convoy = "data:,electrichonkytonk-" + xfiltrate()
	}
	butnottooloud(honk.Audience)
	honk.Audience = oneofakind(honk.Audience)
	if len(honk.Audience) == 0 {
		log.Printf("honk to nowhere")
		http.Error(w, "honk to nowhere...", http.StatusNotFound)
		return
	}
	honk.Public = !keepitquiet(honk.Audience)
	noise = obfusbreak(noise)
	honk.Noise = noise
	honk.Convoy = convoy

	donkxid := r.FormValue("donkxid")
	if donkxid == "" {
		file, filehdr, err := r.FormFile("donk")
		if err == nil {
			var buf bytes.Buffer
			io.Copy(&buf, file)
			file.Close()
			data := buf.Bytes()
			xid := xfiltrate()
			var media, name string
			img, err := image.Vacuum(&buf, image.Params{MaxWidth: 2048, MaxHeight: 2048})
			if err == nil {
				data = img.Data
				format := img.Format
				media = "image/" + format
				if format == "jpeg" {
					format = "jpg"
				}
				name = xid + "." + format
				xid = name
			} else {
				maxsize := 100000
				if len(data) > maxsize {
					log.Printf("bad image: %s too much text: %d", err, len(data))
					http.Error(w, "didn't like your attachment", http.StatusUnsupportedMediaType)
					return
				}
				for i := 0; i < len(data); i++ {
					if data[i] < 32 && data[i] != '\t' && data[i] != '\r' && data[i] != '\n' {
						log.Printf("bad image: %s not text: %d", err, data[i])
						http.Error(w, "didn't like your attachment", http.StatusUnsupportedMediaType)
						return
					}
				}
				media = "text/plain"
				name = filehdr.Filename
				if name == "" {
					name = xid + ".txt"
				}
				xid += ".txt"
			}
			url := fmt.Sprintf("https://%s/d/%s", serverName, xid)
			res, err := stmtSaveFile.Exec(xid, name, url, media, 1, data)
			if err != nil {
				log.Printf("unable to save image: %s", err)
				return
			}
			var d Donk
			d.FileID, _ = res.LastInsertId()
			d.XID = name
			d.Name = name
			d.Media = media
			d.URL = url
			d.Local = true
			honk.Donks = append(honk.Donks, &d)
			donkxid = d.XID
		}
	} else {
		xid := donkxid
		url := fmt.Sprintf("https://%s/d/%s", serverName, xid)
		var donk Donk
		row := stmtFindFile.QueryRow(url)
		err := row.Scan(&donk.FileID)
		if err == nil {
			donk.XID = xid
			donk.Local = true
			donk.URL = url
			honk.Donks = append(honk.Donks, &donk)
		} else {
			log.Printf("can't find file: %s", xid)
		}
	}
	herd := herdofemus(honk.Noise)
	for _, e := range herd {
		donk := savedonk(e.ID, e.Name, "image/png", true)
		if donk != nil {
			donk.Name = e.Name
			honk.Donks = append(honk.Donks, donk)
		}
	}
	memetize(&honk)

	aud := strings.Join(honk.Audience, " ")
	whofore := 2
	if !honk.Public {
		whofore = 3
	}
	if r.FormValue("preview") == "preview" {
		honks := []*Honk{&honk}
		reverbolate(userinfo.UserID, honks)
		templinfo := getInfo(r)
		templinfo["HonkCSRF"] = login.GetCSRF("honkhonk", r)
		templinfo["Honks"] = honks
		templinfo["InReplyTo"] = r.FormValue("rid")
		templinfo["Noise"] = r.FormValue("noise")
		templinfo["SavedFile"] = donkxid
		templinfo["ServerMessage"] = "honk preview"
		err := readviews.Execute(w, "honkpage.html", templinfo)
		if err != nil {
			log.Print(err)
		}
		return
	}
	res, err := stmtSaveHonk.Exec(userinfo.UserID, what, honk.Honker, xid, rid,
		dt.Format(dbtimeformat), "", aud, honk.Noise, convoy, whofore, "html", honk.Precis, honk.Oonker)
	if err != nil {
		log.Printf("error saving honk: %s", err)
		http.Error(w, "something bad happened while saving", http.StatusInternalServerError)
		return
	}
	honk.ID, _ = res.LastInsertId()
	for _, d := range honk.Donks {
		_, err = stmtSaveDonk.Exec(honk.ID, d.FileID)
		if err != nil {
			log.Printf("err saving donk: %s", err)
			http.Error(w, "something bad happened while saving", http.StatusInternalServerError)
			return
		}
	}

	go honkworldwide(user, &honk)

	http.Redirect(w, r, xid, http.StatusSeeOther)
}

func showhonkers(w http.ResponseWriter, r *http.Request) {
	userinfo := login.GetUserInfo(r)
	templinfo := getInfo(r)
	templinfo["Honkers"] = gethonkers(userinfo.UserID)
	templinfo["HonkerCSRF"] = login.GetCSRF("savehonker", r)
	err := readviews.Execute(w, "honkers.html", templinfo)
	if err != nil {
		log.Print(err)
	}
}

func showcombos(w http.ResponseWriter, r *http.Request) {
	userinfo := login.GetUserInfo(r)
	templinfo := getInfo(r)
	honkers := gethonkers(userinfo.UserID)
	var combos []string
	for _, h := range honkers {
		combos = append(combos, h.Combos...)
	}
	for i, c := range combos {
		if c == "-" {
			combos[i] = ""
		}
	}
	combos = oneofakind(combos)
	sort.Strings(combos)
	templinfo["Combos"] = combos
	err := readviews.Execute(w, "combos.html", templinfo)
	if err != nil {
		log.Print(err)
	}
}

func savehonker(w http.ResponseWriter, r *http.Request) {
	u := login.GetUserInfo(r)
	name := r.FormValue("name")
	url := r.FormValue("url")
	peep := r.FormValue("peep")
	combos := r.FormValue("combos")
	honkerid, _ := strconv.ParseInt(r.FormValue("honkerid"), 10, 0)

	if honkerid > 0 {
		goodbye := r.FormValue("goodbye")
		if goodbye == "F" {
			db := opendatabase()
			row := db.QueryRow("select xid from honkers where honkerid = ? and userid = ?",
				honkerid, u.UserID)
			var xid string
			err := row.Scan(&xid)
			if err != nil {
				log.Printf("can't get honker xid: %s", err)
				return
			}
			log.Printf("unsubscribing from %s", xid)
			user, _ := butwhatabout(u.Username)
			go itakeitallback(user, xid)
			_, err = stmtUpdateFlavor.Exec("unsub", u.UserID, xid, "sub")
			if err != nil {
				log.Printf("error updating honker: %s", err)
				return
			}

			http.Redirect(w, r, "/honkers", http.StatusSeeOther)
			return
		}
		combos = " " + strings.TrimSpace(combos) + " "
		_, err := stmtUpdateCombos.Exec(combos, honkerid, u.UserID)
		if err != nil {
			log.Printf("update honker err: %s", err)
			return
		}
		http.Redirect(w, r, "/honkers", http.StatusSeeOther)
	}

	flavor := "presub"
	if peep == "peep" {
		flavor = "peep"
	}
	url = investigate(url)
	if url == "" {
		return
	}
	_, err := stmtSaveHonker.Exec(u.UserID, name, url, flavor, combos)
	if err != nil {
		log.Print(err)
		return
	}
	if flavor == "presub" {
		user, _ := butwhatabout(u.Username)
		go subsub(user, url)
	}
	http.Redirect(w, r, "/honkers", http.StatusSeeOther)
}

type Zonker struct {
	ID        int64
	Name      string
	Wherefore string
}

func zonkzone(w http.ResponseWriter, r *http.Request) {
	userinfo := login.GetUserInfo(r)
	rows, err := stmtGetZonkers.Query(userinfo.UserID)
	if err != nil {
		log.Printf("err: %s", err)
		return
	}
	defer rows.Close()
	var zonkers []Zonker
	for rows.Next() {
		var z Zonker
		rows.Scan(&z.ID, &z.Name, &z.Wherefore)
		zonkers = append(zonkers, z)
	}
	sort.Slice(zonkers, func(i, j int) bool {
		w1 := zonkers[i].Wherefore
		w2 := zonkers[j].Wherefore
		if w1 == w2 {
			return zonkers[i].Name < zonkers[j].Name
		}
		if w1 == "zonvoy" {
			w1 = "zzzzzzz"
		}
		if w2 == "zonvoy" {
			w2 = "zzzzzzz"
		}
		return w1 < w2
	})

	templinfo := getInfo(r)
	templinfo["Zonkers"] = zonkers
	templinfo["ZonkCSRF"] = login.GetCSRF("zonkzonk", r)
	err = readviews.Execute(w, "zonkers.html", templinfo)
	if err != nil {
		log.Print(err)
	}
}

func zonkzonk(w http.ResponseWriter, r *http.Request) {
	userinfo := login.GetUserInfo(r)
	itsok := r.FormValue("itsok")
	if itsok == "iforgiveyou" {
		zonkerid, _ := strconv.ParseInt(r.FormValue("zonkerid"), 10, 0)
		db := opendatabase()
		db.Exec("delete from zonkers where userid = ? and zonkerid = ?",
			userinfo.UserID, zonkerid)
		bitethethumbs()
		http.Redirect(w, r, "/zonkzone", http.StatusSeeOther)
		return
	}
	wherefore := r.FormValue("wherefore")
	name := r.FormValue("name")
	if name == "" {
		return
	}
	switch wherefore {
	case "zonker":
	case "zomain":
	case "zonvoy":
	case "zord":
	case "zilence":
	default:
		return
	}
	db := opendatabase()
	db.Exec("insert into zonkers (userid, name, wherefore) values (?, ?, ?)",
		userinfo.UserID, name, wherefore)
	if wherefore == "zonker" || wherefore == "zomain" || wherefore == "zord" || wherefore == "zilence" {
		bitethethumbs()
	}

	http.Redirect(w, r, "/zonkzone", http.StatusSeeOther)
}

func accountpage(w http.ResponseWriter, r *http.Request) {
	u := login.GetUserInfo(r)
	user, _ := butwhatabout(u.Username)
	templinfo := getInfo(r)
	templinfo["UserCSRF"] = login.GetCSRF("saveuser", r)
	templinfo["LogoutCSRF"] = login.GetCSRF("logout", r)
	templinfo["User"] = user
	err := readviews.Execute(w, "account.html", templinfo)
	if err != nil {
		log.Print(err)
	}
}

func dochpass(w http.ResponseWriter, r *http.Request) {
	err := login.ChangePassword(w, r)
	if err != nil {
		log.Printf("error changing password: %s", err)
	}
	http.Redirect(w, r, "/account", http.StatusSeeOther)
}

func fingerlicker(w http.ResponseWriter, r *http.Request) {
	orig := r.FormValue("resource")

	log.Printf("finger lick: %s", orig)

	if strings.HasPrefix(orig, "acct:") {
		orig = orig[5:]
	}

	name := orig
	idx := strings.LastIndexByte(name, '/')
	if idx != -1 {
		name = name[idx+1:]
		if fmt.Sprintf("https://%s/%s/%s", serverName, userSep, name) != orig {
			log.Printf("foreign request rejected")
			name = ""
		}
	} else {
		idx = strings.IndexByte(name, '@')
		if idx != -1 {
			name = name[:idx]
			if name+"@"+serverName != orig {
				log.Printf("foreign request rejected")
				name = ""
			}
		}
	}
	user, err := butwhatabout(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	j := junk.New()
	j["subject"] = fmt.Sprintf("acct:%s@%s", user.Name, serverName)
	j["aliases"] = []string{user.URL}
	var links []junk.Junk
	l := junk.New()
	l["rel"] = "self"
	l["type"] = `application/activity+json`
	l["href"] = user.URL
	links = append(links, l)
	j["links"] = links

	w.Header().Set("Cache-Control", "max-age=3600")
	w.Header().Set("Content-Type", "application/jrd+json")
	j.Write(w)
}

func somedays() string {
	secs := 432000 + notrand.Int63n(432000)
	return fmt.Sprintf("%d", secs)
}

func avatate(w http.ResponseWriter, r *http.Request) {
	n := r.FormValue("a")
	a := avatar(n)
	w.Header().Set("Cache-Control", "max-age="+somedays())
	w.Write(a)
}

func servecss(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "max-age=7776000")
	http.ServeFile(w, r, "views"+r.URL.Path)
}
func servehtml(w http.ResponseWriter, r *http.Request) {
	templinfo := getInfo(r)
	err := readviews.Execute(w, r.URL.Path[1:]+".html", templinfo)
	if err != nil {
		log.Print(err)
	}
}
func serveemu(w http.ResponseWriter, r *http.Request) {
	xid := mux.Vars(r)["xid"]
	w.Header().Set("Cache-Control", "max-age="+somedays())
	http.ServeFile(w, r, "emus/"+xid)
}
func servememe(w http.ResponseWriter, r *http.Request) {
	xid := mux.Vars(r)["xid"]
	w.Header().Set("Cache-Control", "max-age="+somedays())
	http.ServeFile(w, r, "memes/"+xid)
}

func servefile(w http.ResponseWriter, r *http.Request) {
	xid := mux.Vars(r)["xid"]
	row := stmtFileData.QueryRow(xid)
	var media string
	var data []byte
	err := row.Scan(&media, &data)
	if err != nil {
		log.Printf("error loading file: %s", err)
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", media)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "max-age="+somedays())
	w.Write(data)
}

func nomoroboto(w http.ResponseWriter, r *http.Request) {
	io.WriteString(w, "User-agent: *\n")
	io.WriteString(w, "Disallow: /a\n")
	io.WriteString(w, "Disallow: /d\n")
	io.WriteString(w, "Disallow: /meme\n")
	for _, u := range allusers() {
		fmt.Fprintf(w, "Disallow: /%s/%s/%s/\n", userSep, u.Username, honkSep)
	}
}

func serve() {
	db := opendatabase()
	login.Init(db)

	listener, err := openListener()
	if err != nil {
		log.Fatal(err)
	}
	go redeliverator()

	debug := false
	getconfig("debug", &debug)
	readviews = templates.Load(debug,
		"views/honkpage.html",
		"views/honkers.html",
		"views/zonkers.html",
		"views/combos.html",
		"views/honkform.html",
		"views/honk.html",
		"views/account.html",
		"views/about.html",
		"views/funzone.html",
		"views/login.html",
		"views/xzone.html",
		"views/header.html",
	)
	if !debug {
		s := "views/style.css"
		savedstyleparams[s] = getstyleparam(s)
		s = "views/local.css"
		savedstyleparams[s] = getstyleparam(s)
	}

	bitethethumbs()

	mux := mux.NewRouter()
	mux.Use(login.Checker)

	posters := mux.Methods("POST").Subrouter()
	getters := mux.Methods("GET").Subrouter()

	getters.HandleFunc("/", homepage)
	getters.HandleFunc("/front", homepage)
	getters.HandleFunc("/robots.txt", nomoroboto)
	getters.HandleFunc("/rss", showrss)
	getters.HandleFunc("/"+userSep+"/{name:[[:alnum:]]+}", showuser)
	getters.HandleFunc("/"+userSep+"/{name:[[:alnum:]]+}/"+honkSep+"/{xid:[[:alnum:]]+}", showhonk)
	getters.HandleFunc("/"+userSep+"/{name:[[:alnum:]]+}/rss", showrss)
	posters.HandleFunc("/"+userSep+"/{name:[[:alnum:]]+}/inbox", inbox)
	getters.HandleFunc("/"+userSep+"/{name:[[:alnum:]]+}/outbox", outbox)
	getters.HandleFunc("/"+userSep+"/{name:[[:alnum:]]+}/followers", emptiness)
	getters.HandleFunc("/"+userSep+"/{name:[[:alnum:]]+}/following", emptiness)
	getters.HandleFunc("/a", avatate)
	getters.HandleFunc("/d/{xid:[[:alnum:].]+}", servefile)
	getters.HandleFunc("/emu/{xid:[[:alnum:]_.-]+}", serveemu)
	getters.HandleFunc("/meme/{xid:[[:alnum:]_.-]+}", servememe)
	getters.HandleFunc("/.well-known/webfinger", fingerlicker)

	getters.HandleFunc("/style.css", servecss)
	getters.HandleFunc("/local.css", servecss)
	getters.HandleFunc("/about", servehtml)
	getters.HandleFunc("/login", servehtml)
	posters.HandleFunc("/dologin", login.LoginFunc)
	getters.HandleFunc("/logout", login.LogoutFunc)

	loggedin := mux.NewRoute().Subrouter()
	loggedin.Use(login.Required)
	loggedin.HandleFunc("/account", accountpage)
	loggedin.HandleFunc("/funzone", showfunzone)
	loggedin.HandleFunc("/chpass", dochpass)
	loggedin.HandleFunc("/atme", homepage)
	loggedin.HandleFunc("/zonkzone", zonkzone)
	loggedin.HandleFunc("/xzone", xzone)
	loggedin.Handle("/honk", login.CSRFWrap("honkhonk", http.HandlerFunc(savehonk)))
	loggedin.Handle("/bonk", login.CSRFWrap("honkhonk", http.HandlerFunc(savebonk)))
	loggedin.Handle("/zonkit", login.CSRFWrap("honkhonk", http.HandlerFunc(zonkit)))
	loggedin.Handle("/zonkzonk", login.CSRFWrap("zonkzonk", http.HandlerFunc(zonkzonk)))
	loggedin.Handle("/saveuser", login.CSRFWrap("saveuser", http.HandlerFunc(saveuser)))
	loggedin.Handle("/ximport", login.CSRFWrap("ximport", http.HandlerFunc(ximport)))
	loggedin.HandleFunc("/honkers", showhonkers)
	loggedin.HandleFunc("/h/{name:[[:alnum:]]+}", showhonker)
	loggedin.HandleFunc("/h", showhonker)
	loggedin.HandleFunc("/c/{name:[[:alnum:]]+}", showcombo)
	loggedin.HandleFunc("/c", showcombos)
	loggedin.HandleFunc("/t", showconvoy)
	loggedin.Handle("/savehonker", login.CSRFWrap("savehonker", http.HandlerFunc(savehonker)))

	err = http.Serve(listener, mux)
	if err != nil {
		log.Fatal(err)
	}
}

func cleanupdb(arg string) {
	db := opendatabase()
	days, err := strconv.Atoi(arg)
	if err != nil {
		honker := arg
		expdate := time.Now().UTC().Add(-3 * 24 * time.Hour).Format(dbtimeformat)
		doordie(db, "delete from donks where honkid in (select honkid from honks where dt < ? and whofore = 0 and honker = ?)", expdate, honker)
		doordie(db, "delete from honks where dt < ? and whofore = 0 and honker = ?", expdate, honker)
	} else {
		expdate := time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour).Format(dbtimeformat)
		doordie(db, "delete from donks where honkid in (select honkid from honks where dt < ? and whofore = 0 and convoy not in (select convoy from honks where whofore = 2 or whofore = 3))", expdate)
		doordie(db, "delete from honks where dt < ? and whofore = 0 and convoy not in (select convoy from honks where whofore = 2 or whofore = 3)", expdate)
	}
	doordie(db, "delete from files where fileid not in (select fileid from donks)")
	for _, u := range allusers() {
		doordie(db, "delete from zonkers where userid = ? and wherefore = 'zonvoy' and zonkerid < (select zonkerid from zonkers where userid = ? and wherefore = 'zonvoy' order by zonkerid desc limit 1 offset 200)", u.UserID, u.UserID)
	}
}

var stmtHonkers, stmtDubbers, stmtSaveHonker, stmtUpdateFlavor, stmtUpdateCombos *sql.Stmt
var stmtOneXonk, stmtPublicHonks, stmtUserHonks, stmtHonksByCombo, stmtHonksByConvoy *sql.Stmt
var stmtHonksForUser, stmtHonksForMe, stmtSaveDub, stmtHonksByXonker *sql.Stmt
var stmtHonksByHonker, stmtSaveHonk, stmtFileData, stmtWhatAbout *sql.Stmt
var stmtFindZonk, stmtFindXonk, stmtSaveDonk, stmtFindFile, stmtSaveFile *sql.Stmt
var stmtAddDoover, stmtGetDoovers, stmtLoadDoover, stmtZapDoover *sql.Stmt
var stmtHasHonker, stmtThumbBiters, stmtZonkIt, stmtZonkDonks, stmtSaveZonker *sql.Stmt
var stmtGetZonkers, stmtRecentHonkers, stmtGetXonker, stmtSaveXonker, stmtDeleteXonker *sql.Stmt

func preparetodie(db *sql.DB, s string) *sql.Stmt {
	stmt, err := db.Prepare(s)
	if err != nil {
		log.Fatalf("error %s: %s", err, s)
	}
	return stmt
}

func prepareStatements(db *sql.DB) {
	stmtHonkers = preparetodie(db, "select honkerid, userid, name, xid, flavor, combos from honkers where userid = ? and (flavor = 'sub' or flavor = 'peep' or flavor = 'unsub') order by name")
	stmtSaveHonker = preparetodie(db, "insert into honkers (userid, name, xid, flavor, combos) values (?, ?, ?, ?, ?)")
	stmtUpdateFlavor = preparetodie(db, "update honkers set flavor = ? where userid = ? and xid = ? and flavor = ?")
	stmtUpdateCombos = preparetodie(db, "update honkers set combos = ? where honkerid = ? and userid = ?")
	stmtHasHonker = preparetodie(db, "select honkerid from honkers where xid = ? and userid = ?")
	stmtDubbers = preparetodie(db, "select honkerid, userid, name, xid, flavor from honkers where userid = ? and flavor = 'dub'")

	selecthonks := "select honkid, honks.userid, username, what, honker, oonker, honks.xid, rid, dt, url, audience, noise, precis, convoy, whofore from honks join users on honks.userid = users.userid "
	limit := " order by honkid desc limit 250"
	butnotthose := " and convoy not in (select name from zonkers where userid = ? and wherefore = 'zonvoy' order by zonkerid desc limit 100)"
	stmtOneXonk = preparetodie(db, selecthonks+"where honks.userid = ? and xid = ?")
	stmtPublicHonks = preparetodie(db, selecthonks+"where whofore = 2 and dt > ?"+limit)
	stmtUserHonks = preparetodie(db, selecthonks+"where (whofore = 2 or whofore = ?) and username = ? and dt > ?"+limit)
	stmtHonksForUser = preparetodie(db, selecthonks+"where honks.userid = ? and dt > ? and honker in (select xid from honkers where userid = ? and flavor = 'sub' and combos not like '% - %')"+butnotthose+limit)
	stmtHonksForMe = preparetodie(db, selecthonks+"where honks.userid = ? and dt > ? and whofore = 1"+butnotthose+limit)
	stmtHonksByHonker = preparetodie(db, selecthonks+"join honkers on honkers.xid = honks.honker where honks.userid = ? and honkers.name = ?"+butnotthose+limit)
	stmtHonksByXonker = preparetodie(db, selecthonks+" where honks.userid = ? and (honker = ? or oonker = ?)"+butnotthose+limit)
	stmtHonksByCombo = preparetodie(db, selecthonks+"join honkers on honkers.xid = honks.honker where honks.userid = ? and honkers.combos like ?"+butnotthose+limit)
	stmtHonksByConvoy = preparetodie(db, selecthonks+"where (honks.userid = ? or (? = -1 and whofore = 2)) and convoy = ?"+limit)

	stmtSaveHonk = preparetodie(db, "insert into honks (userid, what, honker, xid, rid, dt, url, audience, noise, convoy, whofore, format, precis, oonker) values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)")
	stmtFileData = preparetodie(db, "select media, content from files where xid = ?")
	stmtFindXonk = preparetodie(db, "select honkid from honks where userid = ? and xid = ?")
	stmtSaveDonk = preparetodie(db, "insert into donks (honkid, fileid) values (?, ?)")
	stmtZonkIt = preparetodie(db, "delete from honks where userid = ? and xid = ?")
	stmtZonkDonks = preparetodie(db, "delete from donks where honkid = ?")
	stmtFindFile = preparetodie(db, "select fileid from files where url = ? and local = 1")
	stmtSaveFile = preparetodie(db, "insert into files (xid, name, url, media, local, content) values (?, ?, ?, ?, ?, ?)")
	stmtWhatAbout = preparetodie(db, "select userid, username, displayname, about, pubkey, options from users where username = ?")
	stmtSaveDub = preparetodie(db, "insert into honkers (userid, name, xid, flavor) values (?, ?, ?, ?)")
	stmtAddDoover = preparetodie(db, "insert into doovers (dt, tries, username, rcpt, msg) values (?, ?, ?, ?, ?)")
	stmtGetDoovers = preparetodie(db, "select dooverid, dt from doovers")
	stmtLoadDoover = preparetodie(db, "select tries, username, rcpt, msg from doovers where dooverid = ?")
	stmtZapDoover = preparetodie(db, "delete from doovers where dooverid = ?")
	stmtThumbBiters = preparetodie(db, "select userid, name, wherefore from zonkers where (wherefore = 'zonker' or wherefore = 'zomain' or wherefore = 'zord' or wherefore = 'zilence')")
	stmtFindZonk = preparetodie(db, "select zonkerid from zonkers where userid = ? and name = ? and wherefore = 'zonk'")
	stmtGetZonkers = preparetodie(db, "select zonkerid, name, wherefore from zonkers where userid = ? and wherefore <> 'zonk'")
	stmtSaveZonker = preparetodie(db, "insert into zonkers (userid, name, wherefore) values (?, ?, ?)")
	stmtGetXonker = preparetodie(db, "select info from xonkers where name = ? and flavor = ?")
	stmtSaveXonker = preparetodie(db, "insert into xonkers (name, info, flavor) values (?, ?, ?)")
	stmtDeleteXonker = preparetodie(db, "delete from xonkers where name = ? and flavor = ?")
	stmtRecentHonkers = preparetodie(db, "select distinct(honker) from honks where userid = ? order by honkid desc limit 100")
}

func ElaborateUnitTests() {
}

func main() {
	cmd := "run"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	switch cmd {
	case "init":
		initdb()
	case "upgrade":
		upgradedb()
	}
	db := opendatabase()
	dbversion := 0
	getconfig("dbversion", &dbversion)
	if dbversion != myVersion {
		log.Fatal("incorrect database version. run upgrade.")
	}
	getconfig("servermsg", &serverMsg)
	getconfig("servername", &serverName)
	getconfig("usersep", &userSep)
	getconfig("honksep", &honkSep)
	getconfig("dnf", &donotfedafterdark)
	prepareStatements(db)
	switch cmd {
	case "adduser":
		adduser()
	case "cleanup":
		arg := "30"
		if len(os.Args) > 2 {
			arg = os.Args[2]
		}
		cleanupdb(arg)
	case "ping":
		if len(os.Args) < 4 {
			fmt.Printf("usage: honk ping from to\n")
			return
		}
		name := os.Args[2]
		targ := os.Args[3]
		user, err := butwhatabout(name)
		if err != nil {
			log.Printf("unknown user")
			return
		}
		ping(user, targ)
	case "peep":
		peeppeep()
	case "run":
		serve()
	case "test":
		ElaborateUnitTests()
	default:
		log.Fatal("unknown command")
	}
}
