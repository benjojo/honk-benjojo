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
	"html/template"
	"io"
	"log"
	notrand "math/rand"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"humungus.tedunangst.com/r/webs/cache"
	"humungus.tedunangst.com/r/webs/httpsig"
	"humungus.tedunangst.com/r/webs/junk"
	"humungus.tedunangst.com/r/webs/login"
	"humungus.tedunangst.com/r/webs/rss"
	"humungus.tedunangst.com/r/webs/templates"
)

var readviews *templates.Template

var userSep = "u"
var honkSep = "h"

var debugMode = false

func getuserstyle(u *login.UserInfo) template.CSS {
	if u == nil {
		return ""
	}
	user, _ := butwhatabout(u.Username)
	css := template.CSS("")
	if user.Options.SkinnyCSS {
		css += "main { max-width: 700px; }\n"
	}
	return css
}

func getmaplink(u *login.UserInfo) string {
	if u == nil {
		return "osm"
	}
	user, _ := butwhatabout(u.Username)
	ml := user.Options.MapLink
	if ml == "" {
		ml = "osm"
	}
	return ml
}

func getInfo(r *http.Request) map[string]interface{} {
	u := login.GetUserInfo(r)
	templinfo := make(map[string]interface{})
	templinfo["StyleParam"] = getassetparam(viewDir + "/views/style.css")
	templinfo["LocalStyleParam"] = getassetparam(viewDir + "/views/local.css")
	templinfo["JSParam"] = getassetparam(viewDir + "/views/honkpage.js")
	templinfo["UserStyle"] = getuserstyle(u)
	templinfo["ServerName"] = serverName
	templinfo["IconName"] = iconName
	templinfo["UserInfo"] = u
	templinfo["UserSep"] = userSep
	if u != nil {
		var combos []string
		combocache.Get(u.UserID, &combos)
		templinfo["Combos"] = combos
	}
	return templinfo
}

func homepage(w http.ResponseWriter, r *http.Request) {
	templinfo := getInfo(r)
	u := login.GetUserInfo(r)
	var honks []*Honk
	var userid int64 = -1

	templinfo["ServerMessage"] = serverMsg
	if u == nil || r.URL.Path == "/front" {
		switch r.URL.Path {
		case "/events":
			honks = geteventhonks(userid)
			templinfo["ServerMessage"] = "some recent and upcoming events"
		default:
			templinfo["ShowRSS"] = true
			honks = getpublichonks()
		}
	} else {
		userid = u.UserID
		switch r.URL.Path {
		case "/atme":
			templinfo["ServerMessage"] = "at me!"
			templinfo["PageName"] = "atme"
			honks = gethonksforme(userid, 0)
			honks = osmosis(honks, userid, false)
		case "/longago":
			templinfo["ServerMessage"] = "long ago and far away!"
			templinfo["PageName"] = "longago"
			honks = gethonksfromlongago(userid, 0)
			honks = osmosis(honks, userid, false)
		case "/events":
			templinfo["ServerMessage"] = "some recent and upcoming events"
			templinfo["PageName"] = "events"
			honks = geteventhonks(userid)
			honks = osmosis(honks, userid, true)
		case "/first":
			templinfo["PageName"] = "first"
			honks = gethonksforuserfirstclass(userid, 0)
			honks = osmosis(honks, userid, true)
		case "/saved":
			templinfo["ServerMessage"] = "saved honks"
			templinfo["PageName"] = "saved"
			honks = getsavedhonks(userid, 0)
		default:
			templinfo["PageName"] = "home"
			honks = gethonksforuser(userid, 0)
			honks = osmosis(honks, userid, true)
		}
		templinfo["HonkCSRF"] = login.GetCSRF("honkhonk", r)
	}

	honkpage(w, u, honks, templinfo)
}

func showfunzone(w http.ResponseWriter, r *http.Request) {
	var emunames, memenames []string
	dir, err := os.Open(dataDir + "/emus")
	if err == nil {
		emunames, _ = dir.Readdirnames(0)
		dir.Close()
	}
	for i, e := range emunames {
		if len(e) > 4 {
			emunames[i] = e[:len(e)-4]
		}
	}
	dir, err = os.Open(dataDir + "/memes")
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
		honks = gethonksbyuser(name, false, 0)
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
		if !firstclass(honk) {
			continue
		}
		desc := string(honk.HTML)
		if t := honk.Time; t != nil {
			desc += fmt.Sprintf(`<p>Time: %s`, t.StartTime.Local().Format("03:04PM EDT Mon Jan 02"))
			if t.Duration != 0 {
				desc += fmt.Sprintf(`<br>Duration: %s`, t.Duration)
			}
		}
		if p := honk.Place; p != nil {
			desc += string(templates.Sprintf(`<p>Location: <a href="%s">%s</a> %f %f`,
				p.Url, p.Name, p.Latitude, p.Longitude))
		}
		for _, d := range honk.Donks {
			desc += string(templates.Sprintf(`<p><a href="%s">Attachment: %s</a>`,
				d.URL, d.Desc))
			if strings.HasPrefix(d.Media, "image") {
				desc += string(templates.Sprintf(`<img src="%s">`, d.URL))
			}
		}

		feed.Items = append(feed.Items, &rss.Item{
			Title:       fmt.Sprintf("%s %s %s", honk.Username, honk.What, honk.XID),
			Description: rss.CData{Data: desc},
			Link:        honk.URL,
			PubDate:     honk.Date.Format(time.RFC1123),
			Guid:        &rss.Guid{IsPermaLink: true, Value: honk.URL},
		})
		if honk.Date.After(modtime) {
			modtime = honk.Date
		}
	}
	if !debugMode {
		w.Header().Set("Cache-Control", "max-age=300")
		w.Header().Set("Last-Modified", modtime.Format(http.TimeFormat))
	}

	err := feed.Write(w)
	if err != nil {
		log.Printf("error writing rss: %s", err)
	}
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
	var box *Box
	ok := boxofboxes.Get(who, &box)
	if !ok {
		log.Printf("no inbox to ping %s", who)
		return
	}
	j := junk.New()
	j["@context"] = itiswhatitis
	j["type"] = "Ping"
	j["id"] = user.URL + "/ping/" + xfiltrate()
	j["actor"] = user.URL
	j["to"] = who
	ki := ziggy(user.ID)
	if ki == nil {
		return
	}
	err := PostJunk(ki.keyname, ki.seckey, box.In, j)
	if err != nil {
		log.Printf("can't send ping: %s", err)
		return
	}
	log.Printf("sent ping to %s: %s", who, j["id"])
}

func pong(user *WhatAbout, who string, obj string) {
	var box *Box
	ok := boxofboxes.Get(who, &box)
	if !ok {
		log.Printf("no inbox to pong %s", who)
		return
	}
	j := junk.New()
	j["@context"] = itiswhatitis
	j["type"] = "Pong"
	j["id"] = user.URL + "/pong/" + xfiltrate()
	j["actor"] = user.URL
	j["to"] = who
	j["object"] = obj
	ki := ziggy(user.ID)
	if ki == nil {
		return
	}
	err := PostJunk(ki.keyname, ki.seckey, box.In, j)
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
	if stealthmode(user.ID, r) {
		http.NotFound(w, r)
		return
	}
	var buf bytes.Buffer
	limiter := io.LimitReader(r.Body, 1*1024*1024)
	io.Copy(&buf, limiter)
	payload := buf.Bytes()
	j, err := junk.FromBytes(payload)
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
	what, _ := j.GetString("type")
	obj, _ := j.GetString("object")
	if what == "Like" || (what == "EmojiReact" && originate(obj) != serverName) {
		return
	}
	who, _ := j.GetString("actor")
	if rejectactor(user.ID, who) {
		return
	}

	keyname, err := httpsig.VerifyRequest(r, payload, zaggy)
	if err != nil && keyname != "" {
		savingthrow(keyname)
		keyname, err = httpsig.VerifyRequest(r, payload, zaggy)
	}
	if err != nil {
		log.Printf("inbox message failed signature for %s from %s: %s", keyname, r.Header.Get("X-Forwarded-For"), err)
		if keyname != "" {
			log.Printf("bad signature from %s", keyname)
			io.WriteString(os.Stdout, "bad payload\n")
			os.Stdout.Write(payload)
			io.WriteString(os.Stdout, "\n")
		}
		http.Error(w, "what did you call me?", http.StatusTeapot)
		return
	}
	origin := keymatch(keyname, who)
	if origin == "" {
		log.Printf("keyname actor mismatch: %s <> %s", keyname, who)
		return
	}

	switch what {
	case "Ping":
		id, _ := j.GetString("id")
		log.Printf("ping from %s: %s", who, id)
		pong(user, who, obj)
	case "Pong":
		log.Printf("pong from %s: %s", who, obj)
	case "Follow":
		if obj != user.URL {
			log.Printf("can't follow %s", obj)
			return
		}
		followme(user, who, who, j)
	case "Accept":
		followyou2(user, j)
	case "Reject":
		nofollowyou2(user, j)
	case "Update":
		obj, ok := j.GetMap("object")
		if ok {
			what, _ := obj.GetString("type")
			switch what {
			case "Person":
				return
			case "Question":
				return
			case "Note":
				go xonksaver(user, j, origin)
				return
			}
		}
		log.Printf("unknown Update activity")
		dumpactivity(j)
	case "Undo":
		obj, ok := j.GetMap("object")
		if !ok {
			folxid, ok := j.GetString("object")
			if ok && originate(folxid) == origin {
				unfollowme(user, "", "", j)
			}
			return
		}
		what, _ := obj.GetString("type")
		switch what {
		case "Follow":
			unfollowme(user, who, who, j)
		case "Announce":
			xid, _ := obj.GetString("object")
			log.Printf("undo announce: %s", xid)
		case "Like":
		default:
			log.Printf("unknown undo: %s", what)
		}
	default:
		go xonksaver(user, j, origin)
	}
}

func serverinbox(w http.ResponseWriter, r *http.Request) {
	user := getserveruser()
	if stealthmode(user.ID, r) {
		http.NotFound(w, r)
		return
	}
	var buf bytes.Buffer
	io.Copy(&buf, r.Body)
	payload := buf.Bytes()
	j, err := junk.FromBytes(payload)
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
	if err != nil && keyname != "" {
		savingthrow(keyname)
		keyname, err = httpsig.VerifyRequest(r, payload, zaggy)
	}
	if err != nil {
		log.Printf("inbox message failed signature for %s from %s: %s", keyname, r.Header.Get("X-Forwarded-For"), err)
		if keyname != "" {
			log.Printf("bad signature from %s", keyname)
			io.WriteString(os.Stdout, "bad payload\n")
			os.Stdout.Write(payload)
			io.WriteString(os.Stdout, "\n")
		}
		http.Error(w, "what did you call me?", http.StatusTeapot)
		return
	}
	who, _ := j.GetString("actor")
	origin := keymatch(keyname, who)
	if origin == "" {
		log.Printf("keyname actor mismatch: %s <> %s", keyname, who)
		return
	}
	if rejectactor(user.ID, who) {
		return
	}
	re_ont := regexp.MustCompile("https://" + serverName + "/o/([[:alnum:]]+)")
	what, _ := j.GetString("type")
	log.Printf("server got a %s", what)
	switch what {
	case "Follow":
		obj, _ := j.GetString("object")
		if obj == user.URL {
			log.Printf("can't follow the server!")
			return
		}
		m := re_ont.FindStringSubmatch(obj)
		if len(m) != 2 {
			log.Printf("not sure how to handle this")
			return
		}
		ont := "#" + m[1]

		followme(user, who, ont, j)
	case "Undo":
		obj, ok := j.GetMap("object")
		if !ok {
			log.Printf("unknown undo no object")
			return
		}
		what, _ := obj.GetString("type")
		if what != "Follow" {
			log.Printf("unknown undo: %s", what)
			return
		}
		targ, _ := obj.GetString("object")
		m := re_ont.FindStringSubmatch(targ)
		if len(m) != 2 {
			log.Printf("not sure how to handle this")
			return
		}
		ont := "#" + m[1]
		unfollowme(user, who, ont, j)
	default:
		log.Printf("unhandled server activity: %s", what)
		dumpactivity(j)
	}
}

func serveractor(w http.ResponseWriter, r *http.Request) {
	user := getserveruser()
	if stealthmode(user.ID, r) {
		http.NotFound(w, r)
		return
	}
	j := junkuser(user)
	j.Write(w)
}

func ximport(w http.ResponseWriter, r *http.Request) {
	u := login.GetUserInfo(r)
	xid := strings.TrimSpace(r.FormValue("xid"))
	xonk := getxonk(u.UserID, xid)
	if xonk == nil {
		p, _ := investigate(xid)
		if p != nil {
			xid = p.XID
		}
		j, err := GetJunk(xid)
		if err != nil {
			http.Error(w, "error getting external object", http.StatusInternalServerError)
			log.Printf("error getting external object: %s", err)
			return
		}
		allinjest(originate(xid), j)
		log.Printf("importing %s", xid)
		user, _ := butwhatabout(u.Username)

		info, _ := somethingabout(j)
		if info == nil {
			xonk = xonksaver(user, j, originate(xid))
		} else if info.What == SomeActor {
			outbox, _ := j.GetString("outbox")
			gimmexonks(user, outbox)
			http.Redirect(w, r, "/h?xid="+url.QueryEscape(xid), http.StatusSeeOther)
			return
		} else if info.What == SomeCollection {
			gimmexonks(user, xid)
			http.Redirect(w, r, "/xzone", http.StatusSeeOther)
			return
		}
	}
	convoy := ""
	if xonk != nil {
		convoy = xonk.Convoy
	}
	http.Redirect(w, r, "/t?c="+url.QueryEscape(convoy), http.StatusSeeOther)
}

func xzone(w http.ResponseWriter, r *http.Request) {
	u := login.GetUserInfo(r)
	rows, err := stmtRecentHonkers.Query(u.UserID, u.UserID)
	if err != nil {
		log.Printf("query err: %s", err)
		return
	}
	defer rows.Close()
	var honkers []Honker
	for rows.Next() {
		var xid string
		rows.Scan(&xid)
		honkers = append(honkers, Honker{XID: xid})
	}
	rows.Close()
	for i, _ := range honkers {
		_, honkers[i].Handle = handles(honkers[i].XID)
	}
	templinfo := getInfo(r)
	templinfo["XCSRF"] = login.GetCSRF("ximport", r)
	templinfo["Honkers"] = honkers
	err = readviews.Execute(w, "xzone.html", templinfo)
	if err != nil {
		log.Print(err)
	}
}

var oldoutbox = cache.New(cache.Options{Filler: func(name string) ([]byte, bool) {
	user, err := butwhatabout(name)
	if err != nil {
		return nil, false
	}
	honks := gethonksbyuser(name, false, 0)
	if len(honks) > 20 {
		honks = honks[0:20]
	}

	var jonks []junk.Junk
	for _, h := range honks {
		j, _ := jonkjonk(user, h)
		jonks = append(jonks, j)
	}

	j := junk.New()
	j["@context"] = itiswhatitis
	j["id"] = user.URL + "/outbox"
	j["attributedTo"] = user.URL
	j["type"] = "OrderedCollection"
	j["totalItems"] = len(jonks)
	j["orderedItems"] = jonks

	return j.ToBytes(), true
}, Duration: 1 * time.Minute})

func outbox(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	user, err := butwhatabout(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if stealthmode(user.ID, r) {
		http.NotFound(w, r)
		return
	}
	var j []byte
	ok := oldoutbox.Get(name, &j)
	if ok {
		w.Header().Set("Content-Type", theonetruename)
		w.Write(j)
	} else {
		http.NotFound(w, r)
	}
}

var oldempties = cache.New(cache.Options{Filler: func(url string) ([]byte, bool) {
	colname := "/followers"
	if strings.HasSuffix(url, "/following") {
		colname = "/following"
	}
	user := fmt.Sprintf("https://%s%s", serverName, url[:len(url)-10])
	j := junk.New()
	j["@context"] = itiswhatitis
	j["id"] = user + colname
	j["attributedTo"] = user
	j["type"] = "OrderedCollection"
	j["totalItems"] = 0
	j["orderedItems"] = []junk.Junk{}

	return j.ToBytes(), true
}})

func emptiness(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	user, err := butwhatabout(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if stealthmode(user.ID, r) {
		http.NotFound(w, r)
		return
	}
	var j []byte
	ok := oldempties.Get(r.URL.Path, &j)
	if ok {
		w.Header().Set("Content-Type", theonetruename)
		w.Write(j)
	} else {
		http.NotFound(w, r)
	}
}

func showuser(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	user, err := butwhatabout(name)
	if err != nil {
		log.Printf("user not found %s: %s", name, err)
		http.NotFound(w, r)
		return
	}
	if stealthmode(user.ID, r) {
		http.NotFound(w, r)
		return
	}
	if friendorfoe(r.Header.Get("Accept")) {
		j, ok := asjonker(name)
		if ok {
			w.Header().Set("Content-Type", theonetruename)
			w.Write(j)
		} else {
			http.NotFound(w, r)
		}
		return
	}
	u := login.GetUserInfo(r)
	honks := gethonksbyuser(name, u != nil && u.Username == name, 0)
	templinfo := getInfo(r)
	templinfo["Name"] = user.Name
	templinfo["WhatAbout"] = user.HTAbout
	templinfo["ServerMessage"] = ""
	templinfo["HonkCSRF"] = login.GetCSRF("honkhonk", r)
	honkpage(w, u, honks, templinfo)
}

func showhonker(w http.ResponseWriter, r *http.Request) {
	u := login.GetUserInfo(r)
	name := mux.Vars(r)["name"]
	var honks []*Honk
	if name == "" {
		name = r.FormValue("xid")
		honks = gethonksbyxonker(u.UserID, name, 0)
	} else {
		honks = gethonksbyhonker(u.UserID, name, 0)
	}
	msg := templates.Sprintf(`honks by honker: <a href="%s" ref="noreferrer">%s</a>`, name, name)
	templinfo := getInfo(r)
	templinfo["PageName"] = "honker"
	templinfo["PageArg"] = name
	templinfo["ServerMessage"] = msg
	templinfo["HonkCSRF"] = login.GetCSRF("honkhonk", r)
	honkpage(w, u, honks, templinfo)
}

func showcombo(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	u := login.GetUserInfo(r)
	honks := gethonksbycombo(u.UserID, name, 0)
	honks = osmosis(honks, u.UserID, true)
	templinfo := getInfo(r)
	templinfo["PageName"] = "combo"
	templinfo["PageArg"] = name
	templinfo["ServerMessage"] = "honks by combo: " + name
	templinfo["HonkCSRF"] = login.GetCSRF("honkhonk", r)
	honkpage(w, u, honks, templinfo)
}
func showconvoy(w http.ResponseWriter, r *http.Request) {
	c := r.FormValue("c")
	u := login.GetUserInfo(r)
	honks := gethonksbyconvoy(u.UserID, c, 0)
	templinfo := getInfo(r)
	if len(honks) > 0 {
		templinfo["TopHID"] = honks[0].ID
	}
	honks = osmosis(honks, u.UserID, false)
	reversehonks(honks)
	templinfo["PageName"] = "convoy"
	templinfo["PageArg"] = c
	templinfo["ServerMessage"] = "honks in convoy: " + c
	templinfo["HonkCSRF"] = login.GetCSRF("honkhonk", r)
	honkpage(w, u, honks, templinfo)
}
func showsearch(w http.ResponseWriter, r *http.Request) {
	q := r.FormValue("q")
	u := login.GetUserInfo(r)
	honks := gethonksbysearch(u.UserID, q, 0)
	templinfo := getInfo(r)
	templinfo["PageName"] = "search"
	templinfo["PageArg"] = q
	templinfo["ServerMessage"] = "honks for search: " + q
	templinfo["HonkCSRF"] = login.GetCSRF("honkhonk", r)
	honkpage(w, u, honks, templinfo)
}
func showontology(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	u := login.GetUserInfo(r)
	var userid int64 = -1
	if u != nil {
		userid = u.UserID
	}
	honks := gethonksbyontology(userid, "#"+name, 0)
	if friendorfoe(r.Header.Get("Accept")) {
		if len(honks) > 40 {
			honks = honks[0:40]
		}

		var xids []string
		for _, h := range honks {
			xids = append(xids, h.XID)
		}

		user := getserveruser()

		j := junk.New()
		j["@context"] = itiswhatitis
		j["id"] = fmt.Sprintf("https://%s/o/%s", serverName, name)
		j["name"] = "#" + name
		j["attributedTo"] = user.URL
		j["type"] = "OrderedCollection"
		j["totalItems"] = len(xids)
		j["orderedItems"] = xids

		j.Write(w)
		return
	}

	templinfo := getInfo(r)
	templinfo["ServerMessage"] = "honks by ontology: " + name
	templinfo["HonkCSRF"] = login.GetCSRF("honkhonk", r)
	honkpage(w, u, honks, templinfo)
}

type Ont struct {
	Name  string
	Count int64
}

func thelistingoftheontologies(w http.ResponseWriter, r *http.Request) {
	u := login.GetUserInfo(r)
	var userid int64 = -1
	if u != nil {
		userid = u.UserID
	}
	rows, err := stmtAllOnts.Query(userid)
	if err != nil {
		log.Printf("selection error: %s", err)
		return
	}
	defer rows.Close()
	var onts []Ont
	for rows.Next() {
		var o Ont
		err := rows.Scan(&o.Name, &o.Count)
		if err != nil {
			log.Printf("error scanning ont: %s", err)
			continue
		}
		if len(o.Name) > 24 {
			continue
		}
		o.Name = o.Name[1:]
		onts = append(onts, o)
	}
	sort.Slice(onts, func(i, j int) bool {
		return onts[i].Name < onts[j].Name
	})
	if u == nil && !debugMode {
		w.Header().Set("Cache-Control", "max-age=300")
	}
	templinfo := getInfo(r)
	templinfo["Onts"] = onts
	err = readviews.Execute(w, "onts.html", templinfo)
	if err != nil {
		log.Print(err)
	}
}

type Track struct {
	xid string
	who string
}

func getbacktracks(xid string) []string {
	c := make(chan bool)
	dumptracks <- c
	<-c
	row := stmtGetTracks.QueryRow(xid)
	var rawtracks string
	err := row.Scan(&rawtracks)
	if err != nil {
		if err != sql.ErrNoRows {
			log.Printf("error scanning tracks: %s", err)
		}
		return nil
	}
	var rcpts []string
	for _, f := range strings.Split(rawtracks, " ") {
		idx := strings.LastIndexByte(f, '#')
		if idx != -1 {
			f = f[:idx]
		}
		if !strings.HasPrefix(f, "https://") {
			f = fmt.Sprintf("%%https://%s/inbox", f)
		}
		rcpts = append(rcpts, f)
	}
	return rcpts
}

func savetracks(tracks map[string][]string) {
	db := opendatabase()
	tx, err := db.Begin()
	if err != nil {
		log.Printf("savetracks begin error: %s", err)
		return
	}
	defer func() {
		err := tx.Commit()
		if err != nil {
			log.Printf("savetracks commit error: %s", err)
		}

	}()
	stmtGetTracks, err := tx.Prepare("select fetches from tracks where xid = ?")
	if err != nil {
		log.Printf("savetracks error: %s", err)
		return
	}
	stmtNewTracks, err := tx.Prepare("insert into tracks (xid, fetches) values (?, ?)")
	if err != nil {
		log.Printf("savetracks error: %s", err)
		return
	}
	stmtUpdateTracks, err := tx.Prepare("update tracks set fetches = ? where xid = ?")
	if err != nil {
		log.Printf("savetracks error: %s", err)
		return
	}
	count := 0
	for xid, f := range tracks {
		count += len(f)
		var prev string
		row := stmtGetTracks.QueryRow(xid)
		err := row.Scan(&prev)
		if err == sql.ErrNoRows {
			f = oneofakind(f)
			stmtNewTracks.Exec(xid, strings.Join(f, " "))
		} else if err == nil {
			all := append(strings.Split(prev, " "), f...)
			all = oneofakind(all)
			stmtUpdateTracks.Exec(strings.Join(all, " "))
		} else {
			log.Printf("savetracks error: %s", err)
		}
	}
	log.Printf("saved %d new fetches", count)
}

var trackchan = make(chan Track)
var dumptracks = make(chan chan bool)

func tracker() {
	timeout := 4 * time.Minute
	sleeper := time.NewTimer(timeout)
	tracks := make(map[string][]string)
	workinprogress++
	for {
		select {
		case track := <-trackchan:
			tracks[track.xid] = append(tracks[track.xid], track.who)
		case <-sleeper.C:
			if len(tracks) > 0 {
				go savetracks(tracks)
				tracks = make(map[string][]string)
			}
			sleeper.Reset(timeout)
		case c := <-dumptracks:
			if len(tracks) > 0 {
				savetracks(tracks)
			}
			c <- true
		case <-endoftheworld:
			if len(tracks) > 0 {
				savetracks(tracks)
			}
			readyalready <- true
			return
		}
	}
}

var re_keyholder = regexp.MustCompile(`keyId="([^"]+)"`)

func trackback(xid string, r *http.Request) {
	agent := r.UserAgent()
	who := originate(agent)
	sig := r.Header.Get("Signature")
	if sig != "" {
		m := re_keyholder.FindStringSubmatch(sig)
		if len(m) == 2 {
			who = m[1]
		}
	}
	if who != "" {
		trackchan <- Track{xid: xid, who: who}
	}
}

func showonehonk(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	user, err := butwhatabout(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if stealthmode(user.ID, r) {
		http.NotFound(w, r)
		return
	}
	xid := fmt.Sprintf("https://%s%s", serverName, r.URL.Path)

	if friendorfoe(r.Header.Get("Accept")) {
		j, ok := gimmejonk(xid)
		if ok {
			trackback(xid, r)
			w.Header().Set("Content-Type", theonetruename)
			w.Write(j)
		} else {
			http.NotFound(w, r)
		}
		return
	}
	honk := getxonk(user.ID, xid)
	if honk == nil {
		http.NotFound(w, r)
		return
	}
	u := login.GetUserInfo(r)
	if u != nil && u.UserID != user.ID {
		u = nil
	}
	if !honk.Public {
		if u == nil {
			http.NotFound(w, r)
			return

		}
		templinfo := getInfo(r)
		templinfo["ServerMessage"] = "one honk maybe more"
		templinfo["HonkCSRF"] = login.GetCSRF("honkhonk", r)
		honkpage(w, u, []*Honk{honk}, templinfo)
		return
	}
	rawhonks := gethonksbyconvoy(honk.UserID, honk.Convoy, 0)
	reversehonks(rawhonks)
	var honks []*Honk
	for _, h := range rawhonks {
		if h.XID == xid && len(honks) != 0 {
			h.Style += " glow"
		}
		if h.Public && (h.Whofore == 2 || h.IsAcked()) {
			honks = append(honks, h)
		}
	}

	templinfo := getInfo(r)
	templinfo["ServerMessage"] = "one honk maybe more"
	templinfo["HonkCSRF"] = login.GetCSRF("honkhonk", r)
	honkpage(w, u, honks, templinfo)
}

func honkpage(w http.ResponseWriter, u *login.UserInfo, honks []*Honk, templinfo map[string]interface{}) {
	var userid int64 = -1
	if u != nil {
		userid = u.UserID
		templinfo["User"], _ = butwhatabout(u.Username)
	}
	reverbolate(userid, honks)
	templinfo["Honks"] = honks
	templinfo["MapLink"] = getmaplink(u)
	if templinfo["TopHID"] == nil {
		if len(honks) > 0 {
			templinfo["TopHID"] = honks[0].ID
		} else {
			templinfo["TopHID"] = 0
		}
	}
	if u == nil && !debugMode {
		w.Header().Set("Cache-Control", "max-age=60")
	}
	err := readviews.Execute(w, "honkpage.html", templinfo)
	if err != nil {
		log.Print(err)
	}
}

func saveuser(w http.ResponseWriter, r *http.Request) {
	whatabout := r.FormValue("whatabout")
	whatabout = strings.Replace(whatabout, "\r", "", -1)
	u := login.GetUserInfo(r)
	user, _ := butwhatabout(u.Username)
	db := opendatabase()

	options := user.Options
	if r.FormValue("skinny") == "skinny" {
		options.SkinnyCSS = true
	} else {
		options.SkinnyCSS = false
	}
	if r.FormValue("omitimages") == "omitimages" {
		options.OmitImages = true
	} else {
		options.OmitImages = false
	}
	if r.FormValue("mentionall") == "mentionall" {
		options.MentionAll = true
	} else {
		options.MentionAll = false
	}
	if r.FormValue("maps") == "apple" {
		options.MapLink = "apple"
	} else {
		options.MapLink = ""
	}
	options.Reaction = r.FormValue("reaction")

	sendupdate := false
	ava := re_avatar.FindString(whatabout)
	if ava != "" {
		whatabout = re_avatar.ReplaceAllString(whatabout, "")
		ava = ava[7:]
		if ava[0] == ' ' {
			ava = ava[1:]
		}
		ava = fmt.Sprintf("https://%s/meme/%s", serverName, ava)
	}
	if ava != options.Avatar {
		options.Avatar = ava
		sendupdate = true
	}
	whatabout = strings.TrimSpace(whatabout)
	if whatabout != user.About {
		sendupdate = true
	}
	j, err := jsonify(options)
	if err == nil {
		_, err = db.Exec("update users set about = ?, options = ? where username = ?", whatabout, j, u.Username)
	}
	if err != nil {
		log.Printf("error bouting what: %s", err)
	}
	somenamedusers.Clear(u.Username)
	somenumberedusers.Clear(u.UserID)
	oldjonkers.Clear(u.Username)

	if sendupdate {
		updateMe(u.Username)
	}

	http.Redirect(w, r, "/account", http.StatusSeeOther)
}

func bonkit(xid string, user *WhatAbout) {
	log.Printf("bonking %s", xid)

	xonk := getxonk(user.ID, xid)
	if xonk == nil {
		return
	}
	if !xonk.Public {
		return
	}
	if xonk.IsBonked() {
		return
	}
	donksforhonks([]*Honk{xonk})

	_, err := stmtUpdateFlags.Exec(flagIsBonked, xonk.ID)
	if err != nil {
		log.Printf("error acking bonk: %s", err)
	}

	oonker := xonk.Oonker
	if oonker == "" {
		oonker = xonk.Honker
	}
	dt := time.Now().UTC()
	bonk := &Honk{
		UserID:   user.ID,
		Username: user.Name,
		What:     "bonk",
		Honker:   user.URL,
		Oonker:   oonker,
		XID:      xonk.XID,
		RID:      xonk.RID,
		Noise:    xonk.Noise,
		Precis:   xonk.Precis,
		URL:      xonk.URL,
		Date:     dt,
		Donks:    xonk.Donks,
		Whofore:  2,
		Convoy:   xonk.Convoy,
		Audience: []string{thewholeworld, oonker},
		Public:   true,
		Format:   "html",
		Place:    xonk.Place,
		Onts:     xonk.Onts,
		Time:     xonk.Time,
	}

	err = savehonk(bonk)
	if err != nil {
		log.Printf("uh oh")
		return
	}

	go honkworldwide(user, bonk)
}

func submitbonk(w http.ResponseWriter, r *http.Request) {
	xid := r.FormValue("xid")
	userinfo := login.GetUserInfo(r)
	user, _ := butwhatabout(userinfo.Username)

	bonkit(xid, user)

	if r.FormValue("js") != "1" {
		templinfo := getInfo(r)
		templinfo["ServerMessage"] = "Bonked!"
		err := readviews.Execute(w, "msg.html", templinfo)
		if err != nil {
			log.Print(err)
		}
	}
}

func sendzonkofsorts(xonk *Honk, user *WhatAbout, what string, aux string) {
	zonk := &Honk{
		What:     what,
		XID:      xonk.XID,
		Date:     time.Now().UTC(),
		Audience: oneofakind(xonk.Audience),
		Noise:    aux,
	}
	zonk.Public = loudandproud(zonk.Audience)

	log.Printf("announcing %sed honk: %s", what, xonk.XID)
	go honkworldwide(user, zonk)
}

func zonkit(w http.ResponseWriter, r *http.Request) {
	wherefore := r.FormValue("wherefore")
	what := r.FormValue("what")
	userinfo := login.GetUserInfo(r)
	user, _ := butwhatabout(userinfo.Username)

	if wherefore == "save" {
		xonk := getxonk(userinfo.UserID, what)
		if xonk != nil {
			_, err := stmtUpdateFlags.Exec(flagIsSaved, xonk.ID)
			if err != nil {
				log.Printf("error saving: %s", err)
			}
		}
		return
	}

	if wherefore == "unsave" {
		xonk := getxonk(userinfo.UserID, what)
		if xonk != nil {
			_, err := stmtClearFlags.Exec(flagIsSaved, xonk.ID)
			if err != nil {
				log.Printf("error unsaving: %s", err)
			}
		}
		return
	}

	if wherefore == "react" {
		reaction := user.Options.Reaction
		if r2 := r.FormValue("reaction"); r2 != "" {
			reaction = r2
		}
		if reaction == "none" {
			return
		}
		xonk := getxonk(userinfo.UserID, what)
		if xonk != nil {
			_, err := stmtUpdateFlags.Exec(flagIsReacted, xonk.ID)
			if err != nil {
				log.Printf("error saving: %s", err)
			}
			sendzonkofsorts(xonk, user, "react", reaction)
		}
		return
	}

	// my hammer is too big, oh well
	defer oldjonks.Flush()

	if wherefore == "ack" {
		xonk := getxonk(userinfo.UserID, what)
		if xonk != nil && !xonk.IsAcked() {
			_, err := stmtUpdateFlags.Exec(flagIsAcked, xonk.ID)
			if err != nil {
				log.Printf("error acking: %s", err)
			}
			sendzonkofsorts(xonk, user, "ack", "")
		}
		return
	}

	if wherefore == "deack" {
		xonk := getxonk(userinfo.UserID, what)
		if xonk != nil && xonk.IsAcked() {
			_, err := stmtClearFlags.Exec(flagIsAcked, xonk.ID)
			if err != nil {
				log.Printf("error deacking: %s", err)
			}
			sendzonkofsorts(xonk, user, "deack", "")
		}
		return
	}

	if wherefore == "bonk" {
		user, _ := butwhatabout(userinfo.Username)
		bonkit(what, user)
		return
	}

	if wherefore == "unbonk" {
		xonk := getbonk(userinfo.UserID, what)
		if xonk != nil {
			deletehonk(xonk.ID)
			xonk = getxonk(userinfo.UserID, what)
			_, err := stmtClearFlags.Exec(flagIsBonked, xonk.ID)
			if err != nil {
				log.Printf("error unbonking: %s", err)
			}
			sendzonkofsorts(xonk, user, "unbonk", "")
		}
		return
	}

	if wherefore == "untag" {
		xonk := getxonk(userinfo.UserID, what)
		if xonk != nil {
			_, err := stmtUpdateFlags.Exec(flagIsUntagged, xonk.ID)
			if err != nil {
				log.Printf("error untagging: %s", err)
			}
		}
		var badparents map[string]bool
		untagged.GetAndLock(userinfo.UserID, &badparents)
		badparents[what] = true
		untagged.Unlock()
		return
	}

	log.Printf("zonking %s %s", wherefore, what)
	if wherefore == "zonk" {
		xonk := getxonk(userinfo.UserID, what)
		if xonk != nil {
			deletehonk(xonk.ID)
			if xonk.Whofore == 2 || xonk.Whofore == 3 {
				sendzonkofsorts(xonk, user, "zonk", "")
			}
		}
	}
	_, err := stmtSaveZonker.Exec(userinfo.UserID, what, wherefore)
	if err != nil {
		log.Printf("error saving zonker: %s", err)
		return
	}
}

func edithonkpage(w http.ResponseWriter, r *http.Request) {
	u := login.GetUserInfo(r)
	user, _ := butwhatabout(u.Username)
	xid := r.FormValue("xid")
	honk := getxonk(u.UserID, xid)
	if !canedithonk(user, honk) {
		http.Error(w, "no editing that please", http.StatusInternalServerError)
		return
	}

	noise := honk.Noise

	honks := []*Honk{honk}
	donksforhonks(honks)
	reverbolate(u.UserID, honks)
	templinfo := getInfo(r)
	templinfo["HonkCSRF"] = login.GetCSRF("honkhonk", r)
	templinfo["Honks"] = honks
	templinfo["MapLink"] = getmaplink(u)
	templinfo["Noise"] = noise
	templinfo["SavedPlace"] = honk.Place
	if tm := honk.Time; tm != nil {
		templinfo["ShowTime"] = ";"
		templinfo["StartTime"] = tm.StartTime.Format("2006-01-02 03:04")
		templinfo["Duration"] = tm.Duration
	}
	templinfo["ServerMessage"] = "honk edit 2"
	templinfo["IsPreview"] = true
	templinfo["UpdateXID"] = honk.XID
	if len(honk.Donks) > 0 {
		templinfo["SavedFile"] = honk.Donks[0].XID
	}
	err := readviews.Execute(w, "honkpage.html", templinfo)
	if err != nil {
		log.Print(err)
	}
}

func newhonkpage(w http.ResponseWriter, r *http.Request) {
	u := login.GetUserInfo(r)
	rid := r.FormValue("rid")
	noise := ""

	xonk := getxonk(u.UserID, rid)
	if xonk != nil {
		_, replto := handles(xonk.Honker)
		if replto != "" {
			noise = "@" + replto + " "
		}
	}

	templinfo := getInfo(r)
	templinfo["HonkCSRF"] = login.GetCSRF("honkhonk", r)
	templinfo["InReplyTo"] = rid
	templinfo["Noise"] = noise
	templinfo["ServerMessage"] = "compose honk"
	templinfo["IsPreview"] = true
	err := readviews.Execute(w, "honkpage.html", templinfo)
	if err != nil {
		log.Print(err)
	}
}

func canedithonk(user *WhatAbout, honk *Honk) bool {
	if honk == nil || honk.Honker != user.URL || honk.What == "bonk" {
		return false
	}
	return true
}

func submitdonk(w http.ResponseWriter, r *http.Request) (*Donk, error) {
	if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "multipart/form-data") {
		return nil, nil
	}
	file, filehdr, err := r.FormFile("donk")
	if err != nil {
		if err == http.ErrMissingFile {
			return nil, nil
		}
		log.Printf("error reading donk: %s", err)
		http.Error(w, "error reading donk", http.StatusUnsupportedMediaType)
		return nil, err
	}
	var buf bytes.Buffer
	io.Copy(&buf, file)
	file.Close()
	data := buf.Bytes()
	var media, name string
	img, err := shrinkit(data)
	if err == nil {
		data = img.Data
		format := img.Format
		media = "image/" + format
		if format == "jpeg" {
			format = "jpg"
		}
		name = xfiltrate() + "." + format
	} else {
		ct := http.DetectContentType(data)
		switch ct {
		case "application/pdf":
			maxsize := 10000000
			if len(data) > maxsize {
				log.Printf("bad image: %s too much pdf: %d", err, len(data))
				http.Error(w, "didn't like your attachment", http.StatusUnsupportedMediaType)
				return nil, err
			}
			media = ct
			name = filehdr.Filename
			if name == "" {
				name = xfiltrate() + ".pdf"
			}
		default:
			maxsize := 100000
			if len(data) > maxsize {
				log.Printf("bad image: %s too much text: %d", err, len(data))
				http.Error(w, "didn't like your attachment", http.StatusUnsupportedMediaType)
				return nil, err
			}
			for i := 0; i < len(data); i++ {
				if data[i] < 32 && data[i] != '\t' && data[i] != '\r' && data[i] != '\n' {
					log.Printf("bad image: %s not text: %d", err, data[i])
					http.Error(w, "didn't like your attachment", http.StatusUnsupportedMediaType)
					return nil, err
				}
			}
			media = "text/plain"
			name = filehdr.Filename
			if name == "" {
				name = xfiltrate() + ".txt"
			}
		}
	}
	desc := strings.TrimSpace(r.FormValue("donkdesc"))
	if desc == "" {
		desc = name
	}
	fileid, xid, err := savefileandxid(name, desc, "", media, true, data)
	if err != nil {
		log.Printf("unable to save image: %s", err)
		http.Error(w, "failed to save attachment", http.StatusUnsupportedMediaType)
		return nil, err
	}
	d := &Donk{
		FileID: fileid,
		XID:    xid,
		Desc:   desc,
		Local:  true,
	}
	return d, nil
}

func submitwebhonk(w http.ResponseWriter, r *http.Request) {
	h := submithonk(w, r)
	if h == nil {
		return
	}
	http.Redirect(w, r, h.XID[len(serverName)+8:], http.StatusSeeOther)
}

// what a hot mess this function is
func submithonk(w http.ResponseWriter, r *http.Request) *Honk {
	rid := r.FormValue("rid")
	noise := r.FormValue("noise")
	format := r.FormValue("format")
	if format == "" {
		format = "markdown"
	}
	if !(format == "markdown" || format == "html") {
		http.Error(w, "unknown format", 500)
		return nil
	}

	userinfo := login.GetUserInfo(r)
	user, _ := butwhatabout(userinfo.Username)

	dt := time.Now().UTC()
	updatexid := r.FormValue("updatexid")
	var honk *Honk
	if updatexid != "" {
		honk = getxonk(userinfo.UserID, updatexid)
		if !canedithonk(user, honk) {
			http.Error(w, "no editing that please", http.StatusInternalServerError)
			return nil
		}
		honk.Date = dt
		honk.What = "update"
		honk.Format = format
	} else {
		xid := fmt.Sprintf("%s/%s/%s", user.URL, honkSep, xfiltrate())
		what := "honk"
		if rid != "" {
			what = "tonk"
		}
		honk = &Honk{
			UserID:   userinfo.UserID,
			Username: userinfo.Username,
			What:     what,
			Honker:   user.URL,
			XID:      xid,
			Date:     dt,
			Format:   format,
		}
	}

	noise = strings.Replace(noise, "\r", "", -1)
	noise = quickrename(noise, userinfo.UserID)
	noise = hooterize(noise)
	honk.Noise = noise
	translate(honk)

	var convoy string
	if rid != "" {
		xonk := getxonk(userinfo.UserID, rid)
		if xonk == nil {
			http.Error(w, "replyto disappeared", http.StatusNotFound)
			return nil
		}
		if xonk.Public {
			honk.Audience = append(honk.Audience, xonk.Audience...)
		}
		convoy = xonk.Convoy
		for i, a := range honk.Audience {
			if a == thewholeworld {
				honk.Audience[0], honk.Audience[i] = honk.Audience[i], honk.Audience[0]
				break
			}
		}
		honk.RID = rid
		if xonk.Precis != "" && honk.Precis == "" {
			honk.Precis = xonk.Precis
			if !(strings.HasPrefix(honk.Precis, "DZ:") || strings.HasPrefix(honk.Precis, "re: re: re: ")) {
				honk.Precis = "re: " + honk.Precis
			}
		}
	} else {
		honk.Audience = []string{thewholeworld}
	}
	if honk.Noise != "" && honk.Noise[0] == '@' {
		honk.Audience = append(grapevine(honk.Mentions), honk.Audience...)
	} else {
		honk.Audience = append(honk.Audience, grapevine(honk.Mentions)...)
	}

	if convoy == "" {
		convoy = "data:,electrichonkytonk-" + xfiltrate()
	}
	butnottooloud(honk.Audience)
	honk.Audience = oneofakind(honk.Audience)
	if len(honk.Audience) == 0 {
		log.Printf("honk to nowhere")
		http.Error(w, "honk to nowhere...", http.StatusNotFound)
		return nil
	}
	honk.Public = loudandproud(honk.Audience)
	honk.Convoy = convoy

	donkxid := r.FormValue("donkxid")
	if donkxid == "" {
		d, err := submitdonk(w, r)
		if err != nil && err != http.ErrMissingFile {
			return nil
		}
		if d != nil {
			honk.Donks = append(honk.Donks, d)
			donkxid = d.XID
		}
	} else {
		xid := donkxid
		url := fmt.Sprintf("https://%s/d/%s", serverName, xid)
		donk := finddonk(url)
		if donk != nil {
			honk.Donks = append(honk.Donks, donk)
		} else {
			log.Printf("can't find file: %s", xid)
		}
	}
	memetize(honk)
	imaginate(honk)

	placename := strings.TrimSpace(r.FormValue("placename"))
	placelat := strings.TrimSpace(r.FormValue("placelat"))
	placelong := strings.TrimSpace(r.FormValue("placelong"))
	placeurl := strings.TrimSpace(r.FormValue("placeurl"))
	if placename != "" || placelat != "" || placelong != "" || placeurl != "" {
		p := new(Place)
		p.Name = placename
		p.Latitude, _ = strconv.ParseFloat(placelat, 64)
		p.Longitude, _ = strconv.ParseFloat(placelong, 64)
		p.Url = placeurl
		honk.Place = p
	}
	timestart := strings.TrimSpace(r.FormValue("timestart"))
	if timestart != "" {
		t := new(Time)
		now := time.Now().Local()
		for _, layout := range []string{"2006-01-02 3:04pm", "2006-01-02 15:04", "3:04pm", "15:04"} {
			start, err := time.ParseInLocation(layout, timestart, now.Location())
			if err == nil {
				if start.Year() == 0 {
					start = time.Date(now.Year(), now.Month(), now.Day(), start.Hour(), start.Minute(), 0, 0, now.Location())
				}
				t.StartTime = start
				break
			}
		}
		timeend := r.FormValue("timeend")
		dur := parseDuration(timeend)
		if dur != 0 {
			t.Duration = Duration(dur)
		}
		if !t.StartTime.IsZero() {
			honk.What = "event"
			honk.Time = t
		}
	}

	if honk.Public {
		honk.Whofore = 2
	} else {
		honk.Whofore = 3
	}

	// back to markdown
	honk.Noise = noise

	if r.FormValue("preview") == "preview" {
		honks := []*Honk{honk}
		reverbolate(userinfo.UserID, honks)
		templinfo := getInfo(r)
		templinfo["HonkCSRF"] = login.GetCSRF("honkhonk", r)
		templinfo["Honks"] = honks
		templinfo["MapLink"] = getmaplink(userinfo)
		templinfo["InReplyTo"] = r.FormValue("rid")
		templinfo["Noise"] = r.FormValue("noise")
		templinfo["SavedFile"] = donkxid
		if tm := honk.Time; tm != nil {
			templinfo["ShowTime"] = ";"
			templinfo["StartTime"] = tm.StartTime.Format("2006-01-02 03:04")
			templinfo["Duration"] = tm.Duration
		}
		templinfo["IsPreview"] = true
		templinfo["UpdateXID"] = updatexid
		templinfo["ServerMessage"] = "honk preview"
		err := readviews.Execute(w, "honkpage.html", templinfo)
		if err != nil {
			log.Print(err)
		}
		return nil
	}

	if updatexid != "" {
		updatehonk(honk)
		oldjonks.Clear(honk.XID)
	} else {
		err := savehonk(honk)
		if err != nil {
			log.Printf("uh oh")
			return nil
		}
	}

	// reload for consistency
	honk.Donks = nil
	donksforhonks([]*Honk{honk})

	go honkworldwide(user, honk)

	return honk
}

func showhonkers(w http.ResponseWriter, r *http.Request) {
	userinfo := login.GetUserInfo(r)
	templinfo := getInfo(r)
	templinfo["Honkers"] = gethonkers(userinfo.UserID)
	templinfo["HonkerCSRF"] = login.GetCSRF("submithonker", r)
	err := readviews.Execute(w, "honkers.html", templinfo)
	if err != nil {
		log.Print(err)
	}
}

func showchatter(w http.ResponseWriter, r *http.Request) {
	u := login.GetUserInfo(r)
	chatter := loadchatter(u.UserID)
	for _, chat := range chatter {
		for _, ch := range chat.Chonks {
			filterchonk(ch)
		}
	}

	templinfo := getInfo(r)
	templinfo["Chatter"] = chatter
	templinfo["ChonkCSRF"] = login.GetCSRF("sendchonk", r)
	err := readviews.Execute(w, "chatter.html", templinfo)
	if err != nil {
		log.Print(err)
	}
}

func submitchonk(w http.ResponseWriter, r *http.Request) {
	u := login.GetUserInfo(r)
	user, _ := butwhatabout(u.Username)
	noise := r.FormValue("noise")
	target := r.FormValue("target")
	format := "markdown"
	dt := time.Now().UTC()
	xid := fmt.Sprintf("%s/%s/%s", user.URL, "chonk", xfiltrate())

	if !strings.HasPrefix(target, "https://") {
		target = fullname(target, u.UserID)
	}
	if target == "" {
		http.Error(w, "who is that?", http.StatusInternalServerError)
		return
	}
	ch := Chonk{
		UserID: u.UserID,
		XID:    xid,
		Who:    user.URL,
		Target: target,
		Date:   dt,
		Noise:  noise,
		Format: format,
	}
	d, err := submitdonk(w, r)
	if err != nil && err != http.ErrMissingFile {
		return
	}
	if d != nil {
		ch.Donks = append(ch.Donks, d)
	}

	translatechonk(&ch)
	savechonk(&ch)
	// reload for consistency
	ch.Donks = nil
	donksforchonks([]*Chonk{&ch})
	go sendchonk(user, &ch)

	http.Redirect(w, r, "/chatter", http.StatusSeeOther)
}

var combocache = cache.New(cache.Options{Filler: func(userid int64) ([]string, bool) {
	honkers := gethonkers(userid)
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
	return combos, true
}, Invalidator: &honkerinvalidator})

func showcombos(w http.ResponseWriter, r *http.Request) {
	userinfo := login.GetUserInfo(r)
	var combos []string
	combocache.Get(userinfo.UserID, &combos)
	templinfo := getInfo(r)
	err := readviews.Execute(w, "combos.html", templinfo)
	if err != nil {
		log.Print(err)
	}
}

func submithonker(w http.ResponseWriter, r *http.Request) {
	u := login.GetUserInfo(r)
	user, _ := butwhatabout(u.Username)
	name := strings.TrimSpace(r.FormValue("name"))
	url := strings.TrimSpace(r.FormValue("url"))
	peep := r.FormValue("peep")
	combos := strings.TrimSpace(r.FormValue("combos"))
	combos = " " + combos + " "
	honkerid, _ := strconv.ParseInt(r.FormValue("honkerid"), 10, 0)

	var meta HonkerMeta
	meta.Notes = strings.TrimSpace(r.FormValue("notes"))
	mj, _ := jsonify(&meta)

	defer honkerinvalidator.Clear(u.UserID)

	if honkerid > 0 {
		goodbye := r.FormValue("goodbye")
		if goodbye == "F" {
			unfollowyou(user, honkerid)
			http.Redirect(w, r, "/honkers", http.StatusSeeOther)
			return
		}
		if goodbye == "X" {
			followyou(user, honkerid)
			http.Redirect(w, r, "/honkers", http.StatusSeeOther)
			return
		}
		_, err := stmtUpdateHonker.Exec(name, combos, mj, honkerid, u.UserID)
		if err != nil {
			log.Printf("update honker err: %s", err)
			return
		}
		http.Redirect(w, r, "/honkers", http.StatusSeeOther)
		return
	}

	if url == "" {
		http.Error(w, "subscribing to nothing?", http.StatusInternalServerError)
		return
	}

	flavor := "presub"
	if peep == "peep" {
		flavor = "peep"
	}

	if url[0] == '#' {
		flavor = "peep"
		if name == "" {
			name = url
		}
		_, err := stmtSaveHonker.Exec(u.UserID, name, url, flavor, combos, url, mj)
		if err != nil {
			log.Print(err)
			return
		}
		http.Redirect(w, r, "/honkers", http.StatusSeeOther)
		return
	}

	info, err := investigate(url)
	if err != nil {
		http.Error(w, "error investigating: "+err.Error(), http.StatusInternalServerError)
		log.Printf("failed to investigate honker: %s", err)
		return
	}
	url = info.XID

	if name == "" {
		name = info.Name
	}

	var x string
	db := opendatabase()
	row := db.QueryRow("select xid from honkers where xid = ? and userid = ? and flavor in ('sub', 'unsub', 'peep')", url, u.UserID)
	err = row.Scan(&x)
	if err != sql.ErrNoRows {
		http.Error(w, "it seems you are already subscribed to them", http.StatusInternalServerError)
		if err != nil {
			log.Printf("honker scan err: %s", err)
		}
		return
	}

	res, err := stmtSaveHonker.Exec(u.UserID, name, url, flavor, combos, info.Owner, mj)
	if err != nil {
		log.Print(err)
		return
	}
	honkerid, _ = res.LastInsertId()
	if flavor == "presub" {
		followyou(user, honkerid)
	}
	http.Redirect(w, r, "/honkers", http.StatusSeeOther)
}

func hfcspage(w http.ResponseWriter, r *http.Request) {
	userinfo := login.GetUserInfo(r)

	filters := getfilters(userinfo.UserID, filtAny)

	templinfo := getInfo(r)
	templinfo["Filters"] = filters
	templinfo["FilterCSRF"] = login.GetCSRF("filter", r)
	err := readviews.Execute(w, "hfcs.html", templinfo)
	if err != nil {
		log.Print(err)
	}
}

func savehfcs(w http.ResponseWriter, r *http.Request) {
	userinfo := login.GetUserInfo(r)
	itsok := r.FormValue("itsok")
	if itsok == "iforgiveyou" {
		hfcsid, _ := strconv.ParseInt(r.FormValue("hfcsid"), 10, 0)
		_, err := stmtDeleteFilter.Exec(userinfo.UserID, hfcsid)
		if err != nil {
			log.Printf("error deleting filter: %s", err)
		}
		filtInvalidator.Clear(userinfo.UserID)
		http.Redirect(w, r, "/hfcs", http.StatusSeeOther)
		return
	}

	filt := new(Filter)
	filt.Name = strings.TrimSpace(r.FormValue("name"))
	filt.Date = time.Now().UTC()
	filt.Actor = strings.TrimSpace(r.FormValue("actor"))
	filt.IncludeAudience = r.FormValue("incaud") == "yes"
	filt.Text = strings.TrimSpace(r.FormValue("filttext"))
	filt.IsAnnounce = r.FormValue("isannounce") == "yes"
	filt.AnnounceOf = strings.TrimSpace(r.FormValue("announceof"))
	filt.Reject = r.FormValue("doreject") == "yes"
	filt.SkipMedia = r.FormValue("doskipmedia") == "yes"
	filt.Hide = r.FormValue("dohide") == "yes"
	filt.Collapse = r.FormValue("docollapse") == "yes"
	filt.Rewrite = strings.TrimSpace(r.FormValue("filtrewrite"))
	filt.Replace = strings.TrimSpace(r.FormValue("filtreplace"))
	if dur := parseDuration(r.FormValue("filtduration")); dur > 0 {
		filt.Expiration = time.Now().UTC().Add(dur)
	}
	filt.Notes = strings.TrimSpace(r.FormValue("filtnotes"))

	if filt.Actor == "" && filt.Text == "" && !filt.IsAnnounce {
		log.Printf("blank filter")
		http.Error(w, "can't save a blank filter", http.StatusInternalServerError)
		return
	}

	j, err := jsonify(filt)
	if err == nil {
		_, err = stmtSaveFilter.Exec(userinfo.UserID, j)
	}
	if err != nil {
		log.Printf("error saving filter: %s", err)
	}

	filtInvalidator.Clear(userinfo.UserID)
	http.Redirect(w, r, "/hfcs", http.StatusSeeOther)
}

func accountpage(w http.ResponseWriter, r *http.Request) {
	u := login.GetUserInfo(r)
	user, _ := butwhatabout(u.Username)
	templinfo := getInfo(r)
	templinfo["UserCSRF"] = login.GetCSRF("saveuser", r)
	templinfo["LogoutCSRF"] = login.GetCSRF("logout", r)
	templinfo["User"] = user
	about := user.About
	if ava := user.Options.Avatar; ava != "" {
		about += "\n\navatar: " + ava[strings.LastIndexByte(ava, '/')+1:]
	}
	templinfo["WhatAbout"] = about
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
			if !(name+"@"+serverName == orig || name+"@"+masqName == orig) {
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
	if stealthmode(user.ID, r) {
		http.NotFound(w, r)
		return
	}

	j := junk.New()
	j["subject"] = fmt.Sprintf("acct:%s@%s", user.Name, masqName)
	j["aliases"] = []string{user.URL}
	l := junk.New()
	l["rel"] = "self"
	l["type"] = `application/activity+json`
	l["href"] = user.URL
	j["links"] = []junk.Junk{l}

	w.Header().Set("Content-Type", "application/jrd+json")
	j.Write(w)
}

func somedays() string {
	secs := 432000 + notrand.Int63n(432000)
	return fmt.Sprintf("%d", secs)
}

func avatate(w http.ResponseWriter, r *http.Request) {
	if debugMode {
		loadAvatarColors()
	}
	n := r.FormValue("a")
	a := genAvatar(n)
	w.Header().Set("Cache-Control", "max-age="+somedays())
	w.Write(a)
}

func serveasset(w http.ResponseWriter, r *http.Request) {
	if !debugMode {
		w.Header().Set("Cache-Control", "max-age=7776000")
	}
	dir := viewDir
	if r.URL.Path == "/local.css" {
		dir = dataDir
	}
	http.ServeFile(w, r, dir+"/views"+r.URL.Path)
}
func servehelp(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	if !debugMode {
		w.Header().Set("Cache-Control", "max-age=3600")
	}
	http.ServeFile(w, r, viewDir+"/docs/"+name)
}
func servehtml(w http.ResponseWriter, r *http.Request) {
	u := login.GetUserInfo(r)
	templinfo := getInfo(r)
	templinfo["AboutMsg"] = aboutMsg
	templinfo["LoginMsg"] = loginMsg
	templinfo["HonkVersion"] = softwareVersion
	if r.URL.Path == "/about" {
		templinfo["Sensors"] = getSensors()
	}
	if u == nil && !debugMode {
		w.Header().Set("Cache-Control", "max-age=60")
	}
	err := readviews.Execute(w, r.URL.Path[1:]+".html", templinfo)
	if err != nil {
		log.Print(err)
	}
}
func serveemu(w http.ResponseWriter, r *http.Request) {
	emu := mux.Vars(r)["emu"]

	w.Header().Set("Cache-Control", "max-age="+somedays())
	http.ServeFile(w, r, dataDir+"/emus/"+emu)
}
func servememe(w http.ResponseWriter, r *http.Request) {
	meme := mux.Vars(r)["meme"]

	w.Header().Set("Cache-Control", "max-age="+somedays())
	http.ServeFile(w, r, dataDir+"/memes/"+meme)
}

func servefile(w http.ResponseWriter, r *http.Request) {
	xid := mux.Vars(r)["xid"]
	var media string
	var data []byte
	row := stmtGetFileData.QueryRow(xid)
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
	io.WriteString(w, "Disallow: /d/\n")
	io.WriteString(w, "Disallow: /meme/\n")
	io.WriteString(w, "Disallow: /o\n")
	io.WriteString(w, "Disallow: /o/\n")
	for _, u := range allusers() {
		fmt.Fprintf(w, "Disallow: /%s/%s/%s/\n", userSep, u.Username, honkSep)
	}
}

func webhydra(w http.ResponseWriter, r *http.Request) {
	u := login.GetUserInfo(r)
	userid := u.UserID
	templinfo := getInfo(r)
	templinfo["HonkCSRF"] = login.GetCSRF("honkhonk", r)
	page := r.FormValue("page")

	wanted, _ := strconv.ParseInt(r.FormValue("tophid"), 10, 0)

	var honks []*Honk
	switch page {
	case "atme":
		honks = gethonksforme(userid, wanted)
		honks = osmosis(honks, userid, false)
		templinfo["ServerMessage"] = "at me!"
	case "longago":
		honks = gethonksfromlongago(userid, wanted)
		honks = osmosis(honks, userid, false)
		templinfo["ServerMessage"] = "from long ago"
	case "home":
		honks = gethonksforuser(userid, wanted)
		honks = osmosis(honks, userid, true)
		templinfo["ServerMessage"] = serverMsg
	case "first":
		honks = gethonksforuserfirstclass(userid, wanted)
		honks = osmosis(honks, userid, true)
		templinfo["ServerMessage"] = "first class only"
	case "saved":
		honks = getsavedhonks(userid, wanted)
		templinfo["PageName"] = "saved"
		templinfo["ServerMessage"] = "saved honks"
	case "combo":
		c := r.FormValue("c")
		honks = gethonksbycombo(userid, c, wanted)
		honks = osmosis(honks, userid, false)
		templinfo["ServerMessage"] = "honks by combo: " + c
	case "convoy":
		c := r.FormValue("c")
		honks = gethonksbyconvoy(userid, c, wanted)
		honks = osmosis(honks, userid, false)
		templinfo["ServerMessage"] = "honks in convoy: " + c
	case "honker":
		xid := r.FormValue("xid")
		honks = gethonksbyxonker(userid, xid, wanted)
		msg := templates.Sprintf(`honks by honker: <a href="%s" ref="noreferrer">%s</a>`, xid, xid)
		templinfo["ServerMessage"] = msg
	default:
		http.NotFound(w, r)
	}
	if len(honks) > 0 {
		templinfo["TopHID"] = honks[0].ID
	} else {
		templinfo["TopHID"] = wanted
	}
	reverbolate(userid, honks)
	templinfo["Honks"] = honks
	templinfo["MapLink"] = getmaplink(u)
	templinfo["User"], _ = butwhatabout(u.Username)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	err := readviews.Execute(w, "honkfrags.html", templinfo)
	if err != nil {
		log.Printf("frag error: %s", err)
	}
}

var honkline = make(chan bool)

func honkhonkline() {
	for {
		select {
		case honkline <- true:
		default:
			return
		}
	}
}

func apihandler(w http.ResponseWriter, r *http.Request) {
	u := login.GetUserInfo(r)
	userid := u.UserID
	user, _ := butwhatabout(u.Username)
	action := r.FormValue("action")
	wait, _ := strconv.ParseInt(r.FormValue("wait"), 10, 0)
	log.Printf("api request '%s' on behalf of %s", action, u.Username)
	switch action {
	case "honk":
		h := submithonk(w, r)
		if h == nil {
			return
		}
		w.Write([]byte(h.XID))
	case "donk":
		d, err := submitdonk(w, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if d == nil {
			http.Error(w, "missing donk", http.StatusBadRequest)
			return
		}
		w.Write([]byte(d.XID))
	case "zonkit":
		zonkit(w, r)
	case "gethonks":
		var honks []*Honk
		wanted, _ := strconv.ParseInt(r.FormValue("after"), 10, 0)
		page := r.FormValue("page")
		var waitchan <-chan time.Time
	requery:
		switch page {
		case "atme":
			honks = gethonksforme(userid, wanted)
			honks = osmosis(honks, userid, false)
		case "longago":
			honks = gethonksfromlongago(userid, wanted)
			honks = osmosis(honks, userid, false)
		case "home":
			honks = gethonksforuser(userid, wanted)
			honks = osmosis(honks, userid, true)
		default:
			http.Error(w, "unknown page", http.StatusNotFound)
			return
		}
		if len(honks) == 0 && wait > 0 {
			if waitchan == nil {
				waitchan = time.After(time.Duration(wait) * time.Second)
			}
			select {
			case <-honkline:
				goto requery
			case <-waitchan:
			}
		}
		reverbolate(userid, honks)
		j := junk.New()
		j["honks"] = honks
		j.Write(w)
	case "sendactivity":
		public := r.FormValue("public") == "1"
		rcpts := boxuprcpts(user, r.Form["rcpt"], public)
		msg := []byte(r.FormValue("msg"))
		for rcpt := range rcpts {
			go deliverate(0, userid, rcpt, msg, true)
		}
	default:
		http.Error(w, "unknown action", http.StatusNotFound)
		return
	}
}

var endoftheworld = make(chan bool)
var readyalready = make(chan bool)
var workinprogress = 0

func enditall() {
	sig := make(chan os.Signal)
	signal.Notify(sig, os.Interrupt)
	signal.Notify(sig, syscall.SIGTERM)
	<-sig
	log.Printf("stopping...")
	for i := 0; i < workinprogress; i++ {
		endoftheworld <- true
	}
	log.Printf("waiting...")
	for i := 0; i < workinprogress; i++ {
		<-readyalready
	}
	log.Printf("apocalypse")
	os.Exit(0)
}

var preservehooks []func()

func wait100ms() chan struct{} {
	c := make(chan struct{})
	go func() {
		time.Sleep(100 * time.Millisecond)
		close(c)
	}()
	return c
}

func bgmonitor() {
	for {
		time.Sleep(50 * time.Minute)
	}
}

func serve() {
	db := opendatabase()
	login.Init(db)

	listener, err := openListener()
	if err != nil {
		log.Fatal(err)
	}
	go runBackendServer()
	go enditall()
	go redeliverator()
	go tracker()
	go bgmonitor()
	loadLingo()
	w100 := wait100ms()

	readviews = templates.Load(debugMode,
		viewDir+"/views/honkpage.html",
		viewDir+"/views/honkfrags.html",
		viewDir+"/views/honkers.html",
		viewDir+"/views/chatter.html",
		viewDir+"/views/hfcs.html",
		viewDir+"/views/combos.html",
		viewDir+"/views/honkform.html",
		viewDir+"/views/honk.html",
		viewDir+"/views/account.html",
		viewDir+"/views/about.html",
		viewDir+"/views/funzone.html",
		viewDir+"/views/login.html",
		viewDir+"/views/xzone.html",
		viewDir+"/views/msg.html",
		viewDir+"/views/header.html",
		viewDir+"/views/onts.html",
		viewDir+"/views/honkpage.js",
	)
	if !debugMode {
		assets := []string{viewDir + "/views/style.css", dataDir + "/views/local.css", viewDir + "/views/honkpage.js"}
		for _, s := range assets {
			savedassetparams[s] = getassetparam(s)
		}
		loadAvatarColors()
	}
	<-w100

	for _, h := range preservehooks {
		h()
	}

	mux := mux.NewRouter()
	mux.Use(login.Checker)

	mux.Handle("/api", login.TokenRequired(http.HandlerFunc(apihandler)))

	posters := mux.Methods("POST").Subrouter()
	getters := mux.Methods("GET").Subrouter()

	getters.HandleFunc("/", homepage)
	getters.HandleFunc("/home", homepage)
	getters.HandleFunc("/front", homepage)
	getters.HandleFunc("/events", homepage)
	getters.HandleFunc("/robots.txt", nomoroboto)
	getters.HandleFunc("/rss", showrss)
	getters.HandleFunc("/"+userSep+"/{name:[[:alnum:]]+}", showuser)
	getters.HandleFunc("/"+userSep+"/{name:[[:alnum:]]+}/"+honkSep+"/{xid:[[:alnum:]]+}", showonehonk)
	getters.HandleFunc("/"+userSep+"/{name:[[:alnum:]]+}/rss", showrss)
	posters.HandleFunc("/"+userSep+"/{name:[[:alnum:]]+}/inbox", inbox)
	getters.HandleFunc("/"+userSep+"/{name:[[:alnum:]]+}/outbox", outbox)
	getters.HandleFunc("/"+userSep+"/{name:[[:alnum:]]+}/followers", emptiness)
	getters.HandleFunc("/"+userSep+"/{name:[[:alnum:]]+}/following", emptiness)
	getters.HandleFunc("/a", avatate)
	getters.HandleFunc("/o", thelistingoftheontologies)
	getters.HandleFunc("/o/{name:.+}", showontology)
	getters.HandleFunc("/d/{xid:[[:alnum:].]+}", servefile)
	getters.HandleFunc("/emu/{emu:[^.]*[^/]+}", serveemu)
	getters.HandleFunc("/meme/{meme:[^.]*[^/]+}", servememe)
	getters.HandleFunc("/.well-known/webfinger", fingerlicker)

	getters.HandleFunc("/flag/{code:.+}", showflag)

	getters.HandleFunc("/server", serveractor)
	posters.HandleFunc("/server/inbox", serverinbox)
	posters.HandleFunc("/inbox", serverinbox)

	getters.HandleFunc("/style.css", serveasset)
	getters.HandleFunc("/local.css", serveasset)
	getters.HandleFunc("/honkpage.js", serveasset)
	getters.HandleFunc("/about", servehtml)
	getters.HandleFunc("/login", servehtml)
	posters.HandleFunc("/dologin", login.LoginFunc)
	getters.HandleFunc("/logout", login.LogoutFunc)
	getters.HandleFunc("/help/{name:[[:alnum:]_.-]+}", servehelp)

	loggedin := mux.NewRoute().Subrouter()
	loggedin.Use(login.Required)
	loggedin.HandleFunc("/first", homepage)
	loggedin.HandleFunc("/chatter", showchatter)
	loggedin.Handle("/sendchonk", login.CSRFWrap("sendchonk", http.HandlerFunc(submitchonk)))
	loggedin.HandleFunc("/saved", homepage)
	loggedin.HandleFunc("/account", accountpage)
	loggedin.HandleFunc("/funzone", showfunzone)
	loggedin.HandleFunc("/chpass", dochpass)
	loggedin.HandleFunc("/atme", homepage)
	loggedin.HandleFunc("/longago", homepage)
	loggedin.HandleFunc("/hfcs", hfcspage)
	loggedin.HandleFunc("/xzone", xzone)
	loggedin.HandleFunc("/newhonk", newhonkpage)
	loggedin.HandleFunc("/edit", edithonkpage)
	loggedin.Handle("/honk", login.CSRFWrap("honkhonk", http.HandlerFunc(submitwebhonk)))
	loggedin.Handle("/bonk", login.CSRFWrap("honkhonk", http.HandlerFunc(submitbonk)))
	loggedin.Handle("/zonkit", login.CSRFWrap("honkhonk", http.HandlerFunc(zonkit)))
	loggedin.Handle("/savehfcs", login.CSRFWrap("filter", http.HandlerFunc(savehfcs)))
	loggedin.Handle("/saveuser", login.CSRFWrap("saveuser", http.HandlerFunc(saveuser)))
	loggedin.Handle("/ximport", login.CSRFWrap("ximport", http.HandlerFunc(ximport)))
	loggedin.HandleFunc("/honkers", showhonkers)
	loggedin.HandleFunc("/h/{name:[[:alnum:]_.-]+}", showhonker)
	loggedin.HandleFunc("/h", showhonker)
	loggedin.HandleFunc("/c/{name:[[:alnum:]_.-]+}", showcombo)
	loggedin.HandleFunc("/c", showcombos)
	loggedin.HandleFunc("/t", showconvoy)
	loggedin.HandleFunc("/q", showsearch)
	loggedin.HandleFunc("/hydra", webhydra)
	loggedin.Handle("/submithonker", login.CSRFWrap("submithonker", http.HandlerFunc(submithonker)))

	err = http.Serve(listener, mux)
	if err != nil {
		log.Fatal(err)
	}
}
