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
	"crypto/sha512"
	"crypto/tls"
	"database/sql"
	"encoding/hex"
	"flag"
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
	"unicode/utf8"

	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/crypto/acme/autocert"
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

var develMode = false

func getuserstyle(u *login.UserInfo) template.CSS {
	if u == nil {
		return ""
	}
	user, _ := getUserBio(u.Username)
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
	user, _ := getUserBio(u.Username)
	ml := user.Options.MapLink
	if ml == "" {
		ml = "osm"
	}
	return ml
}

func getInfo(r *http.Request) map[string]interface{} {
	templinfo := make(map[string]interface{})
	templinfo["StyleParam"] = getassetparam(viewDir + "/views/style.css")
	templinfo["LocalStyleParam"] = getassetparam(dataDir + "/views/local.css")
	templinfo["JSParam"] = getassetparam(viewDir + "/views/honkpage.js")
	templinfo["LocalJSParam"] = getassetparam(dataDir + "/views/local.js")
	templinfo["ServerName"] = serverName
	templinfo["IconName"] = iconName
	templinfo["UserSep"] = userSep
	if u := login.GetUserInfo(r); u != nil {
		templinfo["UserInfo"], _ = getUserBio(u.Username)
		templinfo["UserStyle"] = getuserstyle(u)
		var combos []string
		combocache.Get(u.UserID, &combos)
		templinfo["Combos"] = combos
	}
	return templinfo
}

func homepage(w http.ResponseWriter, r *http.Request) {
	templinfo := getInfo(r)
	u := login.GetUserInfo(r)
	var honks []*ActivityPubActivity
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
			menewnone(userid)
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
	emuext := make(map[string]string)
	dir, err := os.Open(dataDir + "/emus")
	if err == nil {
		emunames, _ = dir.Readdirnames(0)
		dir.Close()
	}
	for i, e := range emunames {
		if len(e) > 4 {
			emunames[i] = e[:len(e)-4]
			emuext[emunames[i]] = e[len(e)-4:]
		}
	}
	dir, err = os.Open(dataDir + "/memes")
	if err == nil {
		memenames, _ = dir.Readdirnames(0)
		dir.Close()
	}
	sort.Strings(emunames)
	sort.Strings(memenames)
	templinfo := getInfo(r)
	templinfo["Emus"] = emunames
	templinfo["Emuext"] = emuext
	templinfo["Memes"] = memenames
	err = readviews.Execute(w, "funzone.html", templinfo)
	if err != nil {
		elog.Print(err)
	}
}

func showrss(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	var honks []*ActivityPubActivity
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
			desc += fmt.Sprintf(`<p>Time: %s`, t.StartTime.Local().Format("03:04PM MST Mon Jan 02"))
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
			} else if strings.HasPrefix(d.Media, "video") {
				desc += string(templates.Sprintf(`<video loop="true" autoplay="true" muted="true" style="max-width: 100%;height: auto;"><source src="%s" type="video/mp4"></video>`, d.URL))
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
	if !develMode {
		w.Header().Set("Cache-Control", "max-age=300")
		w.Header().Set("Last-Modified", modtime.Format(http.TimeFormat))
	}

	err := feed.Write(w)
	if err != nil {
		elog.Printf("error writing rss: %s", err)
	}
}

func crappola(j junk.Junk) bool {
	t, _ := j.GetString("type")
	a, _ := j.GetString("actor")
	o, _ := j.GetString("object")
	if t == "Delete" && a == o {
		dlog.Printf("crappola from %s", a)
		return true
	}
	return false
}

func ping(user *WhatAbout, who string) {
	if targ := fullname(who, user.ID); targ != "" {
		who = targ
	}
	if !strings.HasPrefix(who, "https:") {
		who = gofish(who)
	}
	if who == "" {
		ilog.Printf("nobody to ping!")
		return
	}
	var box *Box
	ok := boxofboxes.Get(who, &box)
	if !ok {
		ilog.Printf("no inbox to ping %s", who)
		return
	}
	ilog.Printf("sending ping to %s", box.In)
	j := junk.New()
	j["@context"] = atContextString
	j["type"] = "Ping"
	j["id"] = user.URL + "/ping/" + make18CharRandomString()
	j["actor"] = user.URL
	j["to"] = who
	ki := getPrivateKey(user.ID)
	if ki == nil {
		return
	}
	err := PostJunk(ki.keyname, ki.seckey, box.In, j)
	if err != nil {
		elog.Printf("can't send ping: %s", err)
		return
	}
	ilog.Printf("sent ping to %s: %s", who, j["id"])
}

func pong(user *WhatAbout, who string, obj string) {
	var box *Box
	ok := boxofboxes.Get(who, &box)
	if !ok {
		ilog.Printf("no inbox to pong %s", who)
		return
	}
	j := junk.New()
	j["@context"] = atContextString
	j["type"] = "Pong"
	j["id"] = user.URL + "/pong/" + make18CharRandomString()
	j["actor"] = user.URL
	j["to"] = who
	j["object"] = obj
	ki := getPrivateKey(user.ID)
	if ki == nil {
		return
	}
	err := PostJunk(ki.keyname, ki.seckey, box.In, j)
	if err != nil {
		elog.Printf("can't send pong: %s", err)
		return
	}
}

func inbox(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	user, err := getUserBio(name)
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
		ilog.Printf("bad payload: %s", err)
		ilog.Writer().Write(payload)
		ilog.Writer().Write([]byte{'\n'})
		return
	}

	if crappola(j) {
		return
	}
	what, _ := j.GetString("type")

	obj, _ := j.GetString("object")
	metricFediEvent.WithLabelValues(what).Inc()
	if what == "EmojiReact" && originate(obj) != serverName {
		return
	}

	who, _ := j.GetString("actor")
	whourl, err := url.Parse(who)
	if err == nil {
		metricFediEventActor.WithLabelValues(whourl.Hostname()).Inc()
	}
	if rejectactor(user.ID, who) {
		return
	}

	keyname, err := httpsig.VerifyRequest(r, payload, getPubKey)
	if err != nil && keyname != "" {
		savingthrow(keyname)
		keyname, err = httpsig.VerifyRequest(r, payload, getPubKey)
	}
	if err != nil {
		ilog.Printf("inbox message failed signature for %s from %s: %s", keyname, r.Header.Get("X-Forwarded-For"), err)
		if keyname != "" {
			ilog.Printf("bad signature from %s", keyname)
			ilog.Writer().Write(payload)
			ilog.Writer().Write([]byte{'\n'})
		}
		http.Error(w, "what did you call me?", http.StatusTeapot)
		return
	}
	origin := keymatch(keyname, who)
	if origin == "" {
		ilog.Printf("keyname actor mismatch: %s <> %s", keyname, who)
		return
	}

	switch what {
	case "Ping":
		id, _ := j.GetString("id")
		ilog.Printf("ping from %s: %s", who, id)
		pong(user, who, obj)
	case "Pong":
		ilog.Printf("pong from %s: %s", who, obj)
	case "Follow":
		if obj != user.URL {
			ilog.Printf("can't follow %s", obj)
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
			case "Service":
				fallthrough
			case "Person":
				return
			case "Question":
				return
			case "Note":
				go xonksaver(user, j, origin)
				return
			}
		}
		ilog.Printf("unknown Update activity")
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
			dlog.Printf("undo announce: %s", xid)
		case "Like":
		default:
			ilog.Printf("unknown undo: %s", what)
		}
	case "EmojiReact":
		obj, ok := j.GetString("object")
		if ok {
			content, _ := j.GetString("content")
			addReaction(user, obj, who, content)
		}
	case "Like":
		obj, ok := j.GetString("object")
		if ok {
			log.Printf("%v obj was liked by %v - well done", obj, who)
		}

	default:
		go xonksaver(user, j, origin)
	}
}

var (
	metricFediEvent = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "honk_fedi_event",
			Help: "How many times an event has happened.",
		},
		[]string{"type"},
	)
	metricFediEventActor = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "honk_fedi_event_actor_domain",
			Help: "How many times an actor has triggered an event.",
		},
		[]string{"actor"},
	)
	metricFileServed = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "honk_file_served",
			Help: "",
		},
		[]string{},
	)
	metricHonkers = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "honk_honkers",
			Help: "",
		},
		[]string{"account"},
	)
)

func init() {
	prometheus.MustRegister(metricFediEvent)
	prometheus.MustRegister(metricFediEventActor)
	prometheus.MustRegister(metricFileServed)
	prometheus.MustRegister(metricHonkers)
}

func calculateFollowersForMetrics() {
	rows, err := stmtGetTopDubbed.Query()
	if err == nil {
		metricHonkers.Reset()
		for rows.Next() {
			var userid, count int
			rows.Scan(&count, &userid)
			metricHonkers.WithLabelValues(fmt.Sprint(userid)).Set(float64(count))
		}
	} else {
		log.Printf("cannot quantify fame: %v", err)
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
		ilog.Printf("bad payload: %s", err)
		ilog.Writer().Write(payload)
		ilog.Writer().Write([]byte{'\n'})
		return
	}
	if crappola(j) {
		return
	}
	keyname, err := httpsig.VerifyRequest(r, payload, getPubKey)
	if err != nil && keyname != "" {
		savingthrow(keyname)
		keyname, err = httpsig.VerifyRequest(r, payload, getPubKey)
	}
	if err != nil {
		ilog.Printf("inbox message failed signature for %s from %s: %s", keyname, r.Header.Get("X-Forwarded-For"), err)
		if keyname != "" {
			ilog.Printf("bad signature from %s", keyname)
			ilog.Writer().Write(payload)
			ilog.Writer().Write([]byte{'\n'})
		}
		http.Error(w, "what did you call me?", http.StatusTeapot)
		return
	}
	who, _ := j.GetString("actor")
	origin := keymatch(keyname, who)
	if origin == "" {
		ilog.Printf("keyname actor mismatch: %s <> %s", keyname, who)
		return
	}
	if rejectactor(user.ID, who) {
		return
	}
	re_ont := regexp.MustCompile("https://" + serverName + "/o/([\\pL[:digit:]]+)")
	what, _ := j.GetString("type")
	dlog.Printf("server got a %s", what)
	switch what {
	case "Follow":
		obj, _ := j.GetString("object")
		if obj == user.URL {
			ilog.Printf("can't follow the server!")
			return
		}
		m := re_ont.FindStringSubmatch(obj)
		if len(m) != 2 {
			ilog.Printf("not sure how to handle this")
			return
		}
		ont := "#" + m[1]

		followme(user, who, ont, j)
	case "Undo":
		obj, ok := j.GetMap("object")
		if !ok {
			ilog.Printf("unknown undo no object")
			return
		}
		what, _ := obj.GetString("type")
		if what != "Follow" {
			ilog.Printf("unknown undo: %s", what)
			return
		}
		targ, _ := obj.GetString("object")
		m := re_ont.FindStringSubmatch(targ)
		if len(m) != 2 {
			ilog.Printf("not sure how to handle this")
			return
		}
		ont := "#" + m[1]
		unfollowme(user, who, ont, j)
	default:
		ilog.Printf("unhandled server activity: %s", what)
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
	xonk := getActivityPubActivity(u.UserID, xid)
	if xonk == nil {
		p, _ := investigate(xid)
		if p != nil {
			xid = p.XID
		}
		j, err := GetJunk(u.UserID, xid)
		if err != nil {
			http.Error(w, "error getting external object", http.StatusInternalServerError)
			ilog.Printf("error getting external object: %s", err)
			return
		}
		allinjest(originate(xid), j)
		dlog.Printf("importing %s", xid)
		user, _ := getUserBio(u.Username)

		info, _ := somethingabout(j)
		log.Printf("user info %#v", info)
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
		elog.Printf("query err: %s", err)
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
		elog.Print(err)
	}
}

var oldoutbox = cache.New(cache.Options{Filler: func(name string) ([]byte, bool) {
	user, err := getUserBio(name)
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
	j["@context"] = atContextString
	j["id"] = user.URL + "/outbox"
	j["attributedTo"] = user.URL
	j["type"] = "OrderedCollection"
	j["totalItems"] = len(jonks)
	j["orderedItems"] = jonks

	return j.ToBytes(), true
}, Duration: 1 * time.Minute})

func outbox(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	user, err := getUserBio(name)
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
		w.Header().Set("Content-Type", ldjsonContentType)
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
	j["@context"] = atContextString
	j["id"] = user + colname
	j["attributedTo"] = user
	j["type"] = "OrderedCollection"
	j["totalItems"] = 0
	j["orderedItems"] = []junk.Junk{}

	return j.ToBytes(), true
}})

func emptiness(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	user, err := getUserBio(name)
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
		w.Header().Set("Content-Type", ldjsonContentType)
		w.Write(j)
	} else {
		http.NotFound(w, r)
	}
}

func showuser(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	user, err := getUserBio(name)
	if err != nil {
		ilog.Printf("user not found %s: %s", name, err)
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
			w.Header().Set("Content-Type", ldjsonContentType)
			w.Write(j)
		} else {
			http.NotFound(w, r)
		}
		return
	}
	u := login.GetUserInfo(r)
	honks := gethonksbyuser(name, u != nil && u.Username == name, 0)
	templinfo := getInfo(r)
	templinfo["PageName"] = "user"
	templinfo["PageArg"] = name
	templinfo["Name"] = user.Name
	templinfo["WhatAbout"] = user.HTAbout
	templinfo["ServerMessage"] = ""
	templinfo["HonkCSRF"] = login.GetCSRF("honkhonk", r)
	honkpage(w, u, honks, templinfo)
}

func showhonker(w http.ResponseWriter, r *http.Request) {
	u := login.GetUserInfo(r)
	name := mux.Vars(r)["name"]
	var honks []*ActivityPubActivity
	if name == "" {
		name = r.FormValue("xid")
		honks = gethonksbyxonker(u.UserID, name, 0)
	} else {
		honks = gethonksbyhonker(u.UserID, name, 0)
	}
	miniform := templates.Sprintf(`<form action="/submithonker" method="POST">
<input type="hidden" name="CSRF" value="%s">
<input type="hidden" name="url" value="%s">
<button tabindex=1 name="add honker" value="add honker">add honker</button>
</form>`, login.GetCSRF("submithonker", r), name)
	msg := templates.Sprintf(`honks by honker: <a href="%s" ref="noreferrer">%s</a>%s`, name, name, miniform)
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
		j["@context"] = atContextString
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
		elog.Printf("selection error: %s", err)
		return
	}
	defer rows.Close()
	var onts []Ont
	for rows.Next() {
		var o Ont
		err := rows.Scan(&o.Name, &o.Count)
		if err != nil {
			elog.Printf("error scanning ont: %s", err)
			continue
		}
		if utf8.RuneCountInString(o.Name) > 24 {
			continue
		}
		o.Name = o.Name[1:]
		onts = append(onts, o)
	}
	sort.Slice(onts, func(i, j int) bool {
		return onts[i].Name < onts[j].Name
	})
	if u == nil && !develMode {
		w.Header().Set("Cache-Control", "max-age=300")
	}
	templinfo := getInfo(r)
	templinfo["Onts"] = onts
	templinfo["FirstRune"] = func(s string) rune { r, _ := utf8.DecodeRuneInString(s); return r }
	err = readviews.Execute(w, "onts.html", templinfo)
	if err != nil {
		elog.Print(err)
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
			elog.Printf("error scanning tracks: %s", err)
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
		elog.Printf("savetracks begin error: %s", err)
		return
	}
	defer func() {
		err := tx.Commit()
		if err != nil {
			elog.Printf("savetracks commit error: %s", err)
		}

	}()
	stmtGetTracks, err := tx.Prepare("select fetches from tracks where xid = ?")
	if err != nil {
		elog.Printf("savetracks error: %s", err)
		return
	}
	stmtNewTracks, err := tx.Prepare("insert into tracks (xid, fetches) values (?, ?)")
	if err != nil {
		elog.Printf("savetracks error: %s", err)
		return
	}
	stmtUpdateTracks, err := tx.Prepare("update tracks set fetches = ? where xid = ?")
	if err != nil {
		elog.Printf("savetracks error: %s", err)
		return
	}
	count := 0
	for xid, f := range tracks {
		count += len(f)
		var prev string
		row := stmtGetTracks.QueryRow(xid)
		err := row.Scan(&prev)
		if err == sql.ErrNoRows {
			f = stringArrayTrimUntilDupe(f)
			stmtNewTracks.Exec(xid, strings.Join(f, " "))
		} else if err == nil {
			all := append(strings.Split(prev, " "), f...)
			all = stringArrayTrimUntilDupe(all)
			stmtUpdateTracks.Exec(strings.Join(all, " "))
		} else {
			elog.Printf("savetracks error: %s", err)
		}
	}
	dlog.Printf("saved %d new fetches", count)
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
	user, err := getUserBio(name)
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
			w.Header().Set("Content-Type", ldjsonContentType)
			w.Write(j)
		} else {
			http.NotFound(w, r)
		}
		return
	}
	honk := getActivityPubActivity(user.ID, xid)
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
		honks := []*ActivityPubActivity{honk}
		donksforhonks(honks)
		templinfo := getInfo(r)
		templinfo["ServerMessage"] = "one honk maybe more"
		templinfo["HonkCSRF"] = login.GetCSRF("honkhonk", r)
		honkpage(w, u, honks, templinfo)
		return
	}
	rawhonks := gethonksbyconvoy(honk.UserID, honk.Convoy, 0)
	reversehonks(rawhonks)
	var honks []*ActivityPubActivity
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

func honkpage(w http.ResponseWriter, u *login.UserInfo, honks []*ActivityPubActivity, templinfo map[string]interface{}) {
	var userid int64 = -1
	if u != nil {
		userid = u.UserID
		templinfo["User"], _ = getUserBio(u.Username)
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
	if u == nil && !develMode {
		w.Header().Set("Cache-Control", "max-age=60")
	}
	err := readviews.Execute(w, "honkpage.html", templinfo)
	if err != nil {
		elog.Print(err)
	}
}

func saveuser(w http.ResponseWriter, r *http.Request) {
	userBio := r.FormValue("whatabout")
	userBio = strings.Replace(userBio, "\r", "", -1)
	u := login.GetUserInfo(r)
	user, _ := getUserBio(u.Username)
	db := opendatabase()

	options := user.Options
	if r.FormValue("skinny") == "skinny" {
		options.SkinnyCSS = true
	} else {
		options.SkinnyCSS = false
	}
	if r.FormValue("avahex") == "avahex" {
		options.Avahex = true
	} else {
		options.Avahex = false
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
	log.Printf("UserBio: %v", userBio)
	ava := re_avatar.FindString(userBio)
	if ava != "" {
		userBio = re_avatar.ReplaceAllString(userBio, "")
		ava = ava[7:]
		if ava[0] == ' ' {
			ava = ava[1:]
		}
		ava = fmt.Sprintf("https://%s/meme/%s", serverName, ava)
		log.Printf("UserBio Avatar: %v", ava)
	}
	if ava != options.Avatar {
		options.Avatar = ava
		sendupdate = true
	}
	ban := re_banner.FindString(userBio)
	if ban != "" {
		userBio = re_banner.ReplaceAllString(userBio, "")
		ban = ban[7:]
		if ban[0] == ' ' {
			ban = ban[1:]
		}
		ban = fmt.Sprintf("https://%s/meme/%s", serverName, ban)
		log.Printf("UserBio Banner: %v", ava)
	}
	if ban != options.Banner {
		options.Banner = ban
		sendupdate = true
	}
	userBio = strings.TrimSpace(userBio)
	if userBio != user.About {
		sendupdate = true
	}
	j, err := encodeJson(options)
	if err == nil {
		_, err = db.Exec("update users set about = ?, options = ? where username = ?", userBio, j, u.Username)
	}
	if err != nil {
		elog.Printf("error bouting what: %s", err)
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
	dlog.Printf("bonking %s", xid)

	xonk := getActivityPubActivity(user.ID, xid)
	if xonk == nil {
		return
	}
	if !xonk.Public {
		return
	}
	if xonk.IsBonked() {
		return
	}
	donksforhonks([]*ActivityPubActivity{xonk})

	_, err := stmtUpdateFlags.Exec(flagIsBonked, xonk.ID)
	if err != nil {
		elog.Printf("error acking bonk: %s", err)
	}

	oonker := xonk.Oonker
	if oonker == "" {
		oonker = xonk.Honker
	}
	dt := time.Now().UTC()
	bonk := &ActivityPubActivity{
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
		Audience: []string{activitystreamsPublicString, oonker},
		Public:   true,
		Format:   xonk.Format,
		Place:    xonk.Place,
		Onts:     xonk.Onts,
		Time:     xonk.Time,
	}

	err = savehonk(bonk)
	if err != nil {
		elog.Printf("uh oh")
		return
	}

	go honkworldwide(user, bonk)
}

func submitbonk(w http.ResponseWriter, r *http.Request) {
	xid := r.FormValue("xid")
	userinfo := login.GetUserInfo(r)
	user, _ := getUserBio(userinfo.Username)

	bonkit(xid, user)

	if r.FormValue("js") != "1" {
		templinfo := getInfo(r)
		templinfo["ServerMessage"] = "Bonked!"
		err := readviews.Execute(w, "msg.html", templinfo)
		if err != nil {
			elog.Print(err)
		}
	}
}

func sendzonkofsorts(xonk *ActivityPubActivity, user *WhatAbout, what string, aux string) {
	zonk := &ActivityPubActivity{
		What:     what,
		XID:      xonk.XID,
		Date:     time.Now().UTC(),
		Audience: stringArrayTrimUntilDupe(xonk.Audience),
		Noise:    aux,
	}
	zonk.Public = loudandproud(zonk.Audience)

	dlog.Printf("announcing %sed honk: %s", what, xonk.XID)
	go honkworldwide(user, zonk)
}

func zonkit(w http.ResponseWriter, r *http.Request) {
	wherefore := r.FormValue("wherefore")
	what := r.FormValue("what")
	userinfo := login.GetUserInfo(r)
	user, _ := getUserBio(userinfo.Username)

	if wherefore == "save" {
		xonk := getActivityPubActivity(userinfo.UserID, what)
		if xonk != nil {
			_, err := stmtUpdateFlags.Exec(flagIsSaved, xonk.ID)
			if err != nil {
				elog.Printf("error saving: %s", err)
			}
		}
		return
	}

	if wherefore == "unsave" {
		xonk := getActivityPubActivity(userinfo.UserID, what)
		if xonk != nil {
			_, err := stmtClearFlags.Exec(flagIsSaved, xonk.ID)
			if err != nil {
				elog.Printf("error unsaving: %s", err)
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
		xonk := getActivityPubActivity(userinfo.UserID, what)
		if xonk != nil {
			_, err := stmtUpdateFlags.Exec(flagIsReacted, xonk.ID)
			if err != nil {
				elog.Printf("error saving: %s", err)
			}
			sendzonkofsorts(xonk, user, "react", reaction)
		}
		return
	}

	if wherefore == "wonk" {
		xonk := getActivityPubActivity(userinfo.UserID, what)
		if xonk != nil {
			_, err := stmtUpdateFlags.Exec(flagIsWonked, xonk.ID)
			if err == nil {
				guesses := r.FormValue("guesses")
				_, err = stmtSaveMeta.Exec(xonk.ID, "guesses", guesses)
			}
			if err != nil {
				elog.Printf("error saving: %s", err)
			}
		}
		return
	}

	// my hammer is too big, oh well
	defer oldjonks.Flush()

	if wherefore == "ack" {
		xonk := getActivityPubActivity(userinfo.UserID, what)
		if xonk != nil && !xonk.IsAcked() {
			_, err := stmtUpdateFlags.Exec(flagIsAcked, xonk.ID)
			if err != nil {
				elog.Printf("error acking: %s", err)
			}
			sendzonkofsorts(xonk, user, "ack", "")
		}
		return
	}

	if wherefore == "deack" {
		xonk := getActivityPubActivity(userinfo.UserID, what)
		if xonk != nil && xonk.IsAcked() {
			_, err := stmtClearFlags.Exec(flagIsAcked, xonk.ID)
			if err != nil {
				elog.Printf("error deacking: %s", err)
			}
			sendzonkofsorts(xonk, user, "deack", "")
		}
		return
	}

	if wherefore == "bonk" {
		user, _ := getUserBio(userinfo.Username)
		bonkit(what, user)
		return
	}

	if wherefore == "unbonk" {
		xonk := getbonk(userinfo.UserID, what)
		if xonk != nil {
			deleteHonk(xonk.ID)
			xonk = getActivityPubActivity(userinfo.UserID, what)
			_, err := stmtClearFlags.Exec(flagIsBonked, xonk.ID)
			if err != nil {
				elog.Printf("error unbonking: %s", err)
			}
			sendzonkofsorts(xonk, user, "unbonk", "")
		}
		return
	}

	if wherefore == "untag" {
		xonk := getActivityPubActivity(userinfo.UserID, what)
		if xonk != nil {
			_, err := stmtUpdateFlags.Exec(flagIsUntagged, xonk.ID)
			if err != nil {
				elog.Printf("error untagging: %s", err)
			}
		}
		var badparents map[string]bool
		untagged.GetAndLock(userinfo.UserID, &badparents)
		badparents[what] = true
		untagged.Unlock()
		return
	}

	ilog.Printf("zonking %s %s", wherefore, what)
	if wherefore == "zonk" {
		xonk := getActivityPubActivity(userinfo.UserID, what)
		if xonk != nil {
			deleteHonk(xonk.ID)
			if xonk.Whofore == 2 || xonk.Whofore == 3 {
				sendzonkofsorts(xonk, user, "zonk", "")
			}
		}
	}
	_, err := stmtSaveZonker.Exec(userinfo.UserID, what, wherefore)
	if err != nil {
		elog.Printf("error saving zonker: %s", err)
		return
	}
}

func edithonkpage(w http.ResponseWriter, r *http.Request) {
	u := login.GetUserInfo(r)
	user, _ := getUserBio(u.Username)
	xid := r.FormValue("xid")
	honk := getActivityPubActivity(u.UserID, xid)
	if !canedithonk(user, honk) {
		http.Error(w, "no editing that please", http.StatusInternalServerError)
		return
	}

	noise := honk.Noise

	honks := []*ActivityPubActivity{honk}
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
		templinfo["StartTime"] = tm.StartTime.Format("2006-01-02 15:04")
		if tm.Duration != 0 {
			templinfo["Duration"] = tm.Duration
		}
	}
	templinfo["ServerMessage"] = "honk edit 2"
	templinfo["IsPreview"] = true
	templinfo["UpdateXID"] = honk.XID
	if len(honk.Donks) > 0 {
		templinfo["SavedFile"] = honk.Donks[0].XID
	}
	err := readviews.Execute(w, "honkpage.html", templinfo)
	if err != nil {
		elog.Print(err)
	}
}

func newhonkpage(w http.ResponseWriter, r *http.Request) {
	u := login.GetUserInfo(r)
	rid := r.FormValue("rid")
	noise := ""

	xonk := getActivityPubActivity(u.UserID, rid)
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
		elog.Print(err)
	}
}

func canedithonk(user *WhatAbout, honk *ActivityPubActivity) bool {
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
		elog.Printf("error reading donk: %s", err)
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
		name = make18CharRandomString() + "." + format
	} else {
		ct := http.DetectContentType(data)
		switch ct {
		case "application/pdf":
			maxsize := 10000000
			if len(data) > maxsize {
				ilog.Printf("bad image: %s too much pdf: %d", err, len(data))
				http.Error(w, "didn't like your PDF attachment", http.StatusUnsupportedMediaType)
				return nil, err
			}
			media = ct
			name = filehdr.Filename
			if name == "" {
				name = make18CharRandomString() + ".pdf"
			}
		case "video/mp4":
			maxsize := 10000000
			if len(data) > maxsize {
				ilog.Printf("bad video: %s too much mp4: %d", err, len(data))
				http.Error(w, "didn't like your MP4 attachment", http.StatusUnsupportedMediaType)
				return nil, err
			}
			media = ct
			name = filehdr.Filename
			if name == "" {
				name = make18CharRandomString() + ".mp4"
			}
		default:
			maxsize := 100000
			if len(data) > maxsize {
				ilog.Printf("bad image: %s too much text: %d", err, len(data))
				http.Error(w, "didn't like your text attachment", http.StatusUnsupportedMediaType)
				return nil, err
			}
			for i := 0; i < len(data); i++ {
				if data[i] < 32 && data[i] != '\t' && data[i] != '\r' && data[i] != '\n' {
					ilog.Printf("bad image: %s not text: %d", err, data[i])
					http.Error(w, "didn't like your text? attachment", http.StatusUnsupportedMediaType)
					return nil, err
				}
			}
			media = "text/plain"
			name = filehdr.Filename
			if name == "" {
				name = make18CharRandomString() + ".txt"
			}
		}
	}
	desc := strings.TrimSpace(r.FormValue("donkdesc"))
	if desc == "" {
		desc = name
	}
	fileid, xid, err := savefileandxid(name, desc, "", media, true, data)
	if err != nil {
		elog.Printf("unable to save image: %s", err)
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
func submithonk(w http.ResponseWriter, r *http.Request) *ActivityPubActivity {
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
	user, _ := getUserBio(userinfo.Username)

	dt := time.Now().UTC()
	updatexid := r.FormValue("updatexid")
	var honk *ActivityPubActivity
	if updatexid != "" {
		honk = getActivityPubActivity(userinfo.UserID, updatexid)
		if !canedithonk(user, honk) {
			http.Error(w, "no editing that please", http.StatusInternalServerError)
			return nil
		}
		honk.Date = dt
		honk.What = "update"
		honk.Format = format
	} else {
		xid := fmt.Sprintf("%s/%s/%s", user.URL, honkSep, make18CharRandomString())
		what := "honk"
		if rid != "" {
			what = "tonk"
		}
		wonkles := r.FormValue("wonkles")
		if wonkles != "" {
			what = "wonk"
		}
		honk = &ActivityPubActivity{
			UserID:   userinfo.UserID,
			Username: userinfo.Username,
			What:     what,
			Honker:   user.URL,
			XID:      xid,
			Date:     dt,
			Format:   format,
			Wonkles:  wonkles,
		}
	}

	noise = strings.Replace(noise, "\r", "", -1)
	noise = quickrename(noise, userinfo.UserID)
	noise = hooterize(noise)
	honk.Noise = noise
	translate(honk)

	var convoy string
	if rid != "" {
		xonk := getActivityPubActivity(userinfo.UserID, rid)
		if xonk == nil {
			http.Error(w, "replyto disappeared", http.StatusNotFound)
			return nil
		}
		if xonk.Public {
			honk.Audience = append(honk.Audience, xonk.Audience...)
		}
		convoy = xonk.Convoy
		for i, a := range honk.Audience {
			if a == activitystreamsPublicString {
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
		honk.Audience = []string{activitystreamsPublicString}
	}
	if honk.Noise != "" && honk.Noise[0] == '@' {
		honk.Audience = append(grapevine(honk.Mentions), honk.Audience...)
	} else {
		honk.Audience = append(honk.Audience, grapevine(honk.Mentions)...)
	}

	if convoy == "" {
		convoy = "data:,electrichonkytonk-" + make18CharRandomString()
	}
	butnottooloud(honk.Audience)
	honk.Audience = stringArrayTrimUntilDupe(honk.Audience)
	if len(honk.Audience) == 0 {
		ilog.Printf("honk to nowhere")
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
		for _, xid := range r.Form["donkxid"] {
			url := fmt.Sprintf("https://%s/d/%s", serverName, xid)
			donk := finddonk(url)
			if donk != nil {
				honk.Donks = append(honk.Donks, donk)
			} else {
				ilog.Printf("can't find file: %s", xid)
			}
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
		honks := []*ActivityPubActivity{honk}
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
			templinfo["StartTime"] = tm.StartTime.Format("2006-01-02 15:04")
			if tm.Duration != 0 {
				templinfo["Duration"] = tm.Duration
			}
		}
		templinfo["IsPreview"] = true
		templinfo["UpdateXID"] = updatexid
		templinfo["ServerMessage"] = "honk preview"
		err := readviews.Execute(w, "honkpage.html", templinfo)
		if err != nil {
			elog.Print(err)
		}
		return nil
	}

	if updatexid != "" {
		updateHonk(honk)
		oldjonks.Clear(honk.XID)
	} else {
		err := savehonk(honk)
		if err != nil {
			elog.Printf("uh oh")
			return nil
		}
	}

	// reload for consistency
	honk.Donks = nil
	donksforhonks([]*ActivityPubActivity{honk})

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
		elog.Print(err)
	}
}

func showchatter(w http.ResponseWriter, r *http.Request) {
	u := login.GetUserInfo(r)
	chatnewnone(u.UserID)
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
		elog.Print(err)
	}
}

func submitchonk(w http.ResponseWriter, r *http.Request) {
	u := login.GetUserInfo(r)
	user, _ := getUserBio(u.Username)
	noise := r.FormValue("noise")
	target := r.FormValue("target")
	format := "markdown"
	dt := time.Now().UTC()
	xid := fmt.Sprintf("%s/%s/%s", user.URL, "chonk", make18CharRandomString())

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
	combos = stringArrayTrimUntilDupe(combos)
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
		elog.Print(err)
	}
}

func submithonker(w http.ResponseWriter, r *http.Request) {
	u := login.GetUserInfo(r)
	user, _ := getUserBio(u.Username)
	name := strings.TrimSpace(r.FormValue("name"))
	url := strings.TrimSpace(r.FormValue("url"))
	peep := r.FormValue("peep")
	combos := strings.TrimSpace(r.FormValue("combos"))
	combos = " " + combos + " "
	honkerid, _ := strconv.ParseInt(r.FormValue("honkerid"), 10, 0)

	re_namecheck := regexp.MustCompile("[\\pL[:digit:]_.-]+")
	if name != "" && !re_namecheck.MatchString(name) {
		http.Error(w, "please use a plainer name", http.StatusInternalServerError)
		return
	}

	var meta HonkerMeta
	meta.Notes = strings.TrimSpace(r.FormValue("notes"))
	mj, _ := encodeJson(&meta)

	defer honkerinvalidator.Clear(u.UserID)

	if honkerid > 0 {
		if r.FormValue("delete") == "delete" {
			unfollowyou(user, honkerid)
			stmtDeleteHonker.Exec(honkerid)
			http.Redirect(w, r, "/honkers", http.StatusSeeOther)
			return
		}
		if r.FormValue("unsub") == "unsub" {
			unfollowyou(user, honkerid)
		}
		if r.FormValue("sub") == "sub" {
			followyou(user, honkerid)
		}
		_, err := stmtUpdateHonker.Exec(name, combos, mj, honkerid, u.UserID)
		if err != nil {
			elog.Printf("update honker err: %s", err)
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

	err := savehonker(user, url, name, flavor, combos, mj)
	if err != nil {
		http.Error(w, "had some trouble with that: "+err.Error(), http.StatusInternalServerError)
		return
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
		elog.Print(err)
	}
}

func savehfcs(w http.ResponseWriter, r *http.Request) {
	userinfo := login.GetUserInfo(r)
	itsok := r.FormValue("itsok")
	if itsok == "iforgiveyou" {
		hfcsid, _ := strconv.ParseInt(r.FormValue("hfcsid"), 10, 0)
		_, err := stmtDeleteFilter.Exec(userinfo.UserID, hfcsid)
		if err != nil {
			elog.Printf("error deleting filter: %s", err)
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
		ilog.Printf("blank filter")
		http.Error(w, "can't save a blank filter", http.StatusInternalServerError)
		return
	}

	j, err := encodeJson(filt)
	if err == nil {
		_, err = stmtSaveFilter.Exec(userinfo.UserID, j)
	}
	if err != nil {
		elog.Printf("error saving filter: %s", err)
	}

	filtInvalidator.Clear(userinfo.UserID)
	http.Redirect(w, r, "/hfcs", http.StatusSeeOther)
}

func accountpage(w http.ResponseWriter, r *http.Request) {
	u := login.GetUserInfo(r)
	user, _ := getUserBio(u.Username)
	templinfo := getInfo(r)
	templinfo["UserCSRF"] = login.GetCSRF("saveuser", r)
	templinfo["LogoutCSRF"] = login.GetCSRF("logout", r)
	templinfo["User"] = user
	about := user.About
	if ava := user.Options.Avatar; ava != "" {
		about += "\n\navatar: " + ava[strings.LastIndexByte(ava, '/')+1:]
	}
	if ban := user.Options.Banner; ban != "" {
		about += "\n\nbanner: " + ban[strings.LastIndexByte(ban, '/')+1:]
	}
	templinfo["WhatAbout"] = about
	err := readviews.Execute(w, "account.html", templinfo)
	if err != nil {
		elog.Print(err)
	}
}

func dochpass(w http.ResponseWriter, r *http.Request) {
	err := login.ChangePassword(w, r)
	if err != nil {
		elog.Printf("error changing password: %s", err)
	}
	http.Redirect(w, r, "/account", http.StatusSeeOther)
}

func fingerlicker(w http.ResponseWriter, r *http.Request) {
	orig := r.FormValue("resource")

	dlog.Printf("finger lick: %s", orig)

	if strings.HasPrefix(orig, "acct:") {
		orig = orig[5:]
	}

	name := orig
	idx := strings.LastIndexByte(name, '/')
	if idx != -1 {
		name = name[idx+1:]
		if fmt.Sprintf("https://%s/%s/%s", serverName, userSep, name) != orig {
			ilog.Printf("foreign request rejected")
			name = ""
		}
	} else {
		idx = strings.IndexByte(name, '@')
		if idx != -1 {
			name = name[:idx]
			if !(name+"@"+serverName == orig || name+"@"+masqName == orig) {
				ilog.Printf("foreign request rejected")
				name = ""
			}
		}
	}
	user, err := getUserBio(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if stealthmode(user.ID, r) {
		log.Printf("not serving webfinger due to user agent not having a link: %v", r.UserAgent())
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

func avatarWebHandler(w http.ResponseWriter, r *http.Request) {
	if develMode {
		loadAvatarColors()
	}
	n := r.FormValue("a")
	hasher := sha512.New()
	hasher.Write([]byte(n))
	hashString := hex.EncodeToString(hasher.Sum(nil))

	fileKey := fmt.Sprintf("%s/avatarcache/%s", dataDir, hashString)
	s, err := os.Stat(fileKey)
	if err == nil {
		if time.Since(s.ModTime()) < (time.Hour * 24 * 7) { // Expire the cache
			b, _ := os.ReadFile(fileKey)
			w.Header().Set("Content-Type", http.DetectContentType(b))
			w.Write(b)
			return
		}
	}
	// Else, we fetch it now
	xid := n
	// j, err := GetJunk(u.UserID, xid)
	j, err := GetJunk(0, xid)
	if err != nil {
		easyAvatar(r, n, w)
		return
	}

	info, _ := somethingabout(j)
	if info.AvatarURL == "" {
		easyAvatar(r, n, w)
		return
	}
	if strings.Contains(xid, serverName) {
		// Hack to avoid infini loop
		easyAvatar(r, n, w)
		return
	}

	imageBytes, err := fetchsome(info.AvatarURL)
	if err != nil {
		easyAvatar(r, n, w)
		return
	}
	w.Header().Set("Content-Type", http.DetectContentType(imageBytes))
	w.Write(imageBytes)

	go func() {
		os.MkdirAll(fmt.Sprintf("%s/avatarcache/", dataDir), 0777)
		os.WriteFile(fileKey, imageBytes, 0644)
	}()
}

func easyAvatar(r *http.Request, n string, w http.ResponseWriter) {
	hex := r.FormValue("hex") == "1"
	a := genAvatar(n, hex)
	if !develMode {
		w.Header().Set("Cache-Control", "max-age="+somedays())
	}
	w.Write(a)
}

func serveviewasset(w http.ResponseWriter, r *http.Request) {
	serveasset(w, r, viewDir)
}
func servedataasset(w http.ResponseWriter, r *http.Request) {
	serveasset(w, r, dataDir)
}

func serveasset(w http.ResponseWriter, r *http.Request, basedir string) {
	if !develMode {
		w.Header().Set("Cache-Control", "max-age=7776000")
	}
	http.ServeFile(w, r, basedir+"/views"+r.URL.Path)
}
func servehelp(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	if !develMode {
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
	if u == nil && !develMode {
		w.Header().Set("Cache-Control", "max-age=60")
	}
	err := readviews.Execute(w, r.URL.Path[1:]+".html", templinfo)
	if err != nil {
		elog.Print(err)
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
		elog.Printf("error loading file: %s", err)
		http.NotFound(w, r)
		return
	}
	metricFileServed.WithLabelValues().Inc()
	w.Header().Set("Content-Type", media)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "max-age="+somedays())
	w.Write(data)
}

func robotsTxtHandler(w http.ResponseWriter, r *http.Request) {
	io.WriteString(w, "User-agent: *\n")
	io.WriteString(w, "Allow: /\n")
	io.WriteString(w, "Allow: /fp/\n")
	io.WriteString(w, "Disallow: /a\n")
	io.WriteString(w, "Disallow: /d/\n")
	io.WriteString(w, "Disallow: /meme/\n")
	io.WriteString(w, "Disallow: /o\n")
	io.WriteString(w, "Disallow: /o/\n")
	io.WriteString(w, "Disallow: /help/\n")
	for _, u := range allusers() {
		fmt.Fprintf(w, "Allow: /%s/%s/%s/\n", userSep, u.Username, honkSep)
	}
}

type Hydration struct {
	Tophid    int64
	Srvmsg    template.HTML
	Honks     string
	MeCount   int64
	ChatCount int64
}

func webhydra(w http.ResponseWriter, r *http.Request) {
	u := login.GetUserInfo(r)
	userid := u.UserID
	templinfo := getInfo(r)
	templinfo["HonkCSRF"] = login.GetCSRF("honkhonk", r)
	page := r.FormValue("page")

	wanted, _ := strconv.ParseInt(r.FormValue("tophid"), 10, 0)

	var hydra Hydration

	var honks []*ActivityPubActivity
	switch page {
	case "atme":
		honks = gethonksforme(userid, wanted)
		honks = osmosis(honks, userid, false)
		menewnone(userid)
		hydra.Srvmsg = "at me!"
	case "longago":
		honks = gethonksfromlongago(userid, wanted)
		honks = osmosis(honks, userid, false)
		hydra.Srvmsg = "from long ago"
	case "home":
		honks = gethonksforuser(userid, wanted)
		honks = osmosis(honks, userid, true)
		hydra.Srvmsg = serverMsg
	case "first":
		honks = gethonksforuserfirstclass(userid, wanted)
		honks = osmosis(honks, userid, true)
		hydra.Srvmsg = "first class only"
	case "saved":
		honks = getsavedhonks(userid, wanted)
		templinfo["PageName"] = "saved"
		hydra.Srvmsg = "saved honks"
	case "combo":
		c := r.FormValue("c")
		honks = gethonksbycombo(userid, c, wanted)
		honks = osmosis(honks, userid, false)
		hydra.Srvmsg = templates.Sprintf("honks by combo: %s", c)
	case "convoy":
		c := r.FormValue("c")
		honks = gethonksbyconvoy(userid, c, wanted)
		honks = osmosis(honks, userid, false)
		hydra.Srvmsg = templates.Sprintf("honks in convoy: %s", c)
	case "honker":
		xid := r.FormValue("xid")
		honks = gethonksbyxonker(userid, xid, wanted)
		miniform := templates.Sprintf(`<form action="/submithonker" method="POST">
			<input type="hidden" name="CSRF" value="%s">
			<input type="hidden" name="url" value="%s">
			<button tabindex=1 name="add honker" value="add honker">add honker</button>
			</form>`, login.GetCSRF("submithonker", r), xid)
		msg := templates.Sprintf(`honks by honker: <a href="%s" ref="noreferrer">%s</a>%s`, xid, xid, miniform)
		hydra.Srvmsg = msg
	case "user":
		uname := r.FormValue("uname")
		honks = gethonksbyuser(uname, u != nil && u.Username == uname, wanted)
		hydra.Srvmsg = templates.Sprintf("honks by user: %s", uname)
	default:
		http.NotFound(w, r)
	}

	if len(honks) > 0 {
		hydra.Tophid = honks[0].ID
	} else {
		hydra.Tophid = wanted
	}
	reverbolate(userid, honks)

	user, _ := getUserBio(u.Username)

	var buf strings.Builder
	templinfo["Honks"] = honks
	templinfo["MapLink"] = getmaplink(u)
	templinfo["User"], _ = getUserBio(u.Username)
	err := readviews.Execute(&buf, "honkfrags.html", templinfo)
	if err != nil {
		elog.Printf("frag error: %s", err)
		return
	}
	hydra.Honks = buf.String()
	hydra.MeCount = user.Options.MeCount
	hydra.ChatCount = user.Options.ChatCount
	w.Header().Set("Content-Type", "application/json")
	j, _ := encodeJson(&hydra)
	io.WriteString(w, j)
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
	action := r.FormValue("action")
	wait, _ := strconv.ParseInt(r.FormValue("wait"), 10, 0)
	dlog.Printf("api request '%s' on behalf of %s", action, u.Username)
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
		var honks []*ActivityPubActivity
		wanted, _ := strconv.ParseInt(r.FormValue("after"), 10, 0)
		page := r.FormValue("page")
		var waitchan <-chan time.Time
	requery:
		switch page {
		case "atme":
			honks = gethonksforme(userid, wanted)
			honks = osmosis(honks, userid, false)
			menewnone(userid)
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
		user, _ := getUserBio(u.Username)
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

func exitSignalHandler() {
	sig := make(chan os.Signal)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)
	<-sig
	ilog.Printf("stopping...")
	for i := 0; i < workinprogress; i++ {
		endoftheworld <- true
	}
	ilog.Printf("waiting...")
	for i := 0; i < workinprogress; i++ {
		<-readyalready
	}
	ilog.Printf("apocalypse")
	os.Exit(0)
}

var preservehooks []func()

func bgmonitor() {
	for {
		when := time.Now().Add(-3 * 24 * time.Hour).UTC().Format(dbtimeformat)
		_, err := stmtDeleteOldXonkers.Exec("pubkey", when)
		if err != nil {
			elog.Printf("error deleting old xonkers: %s", err)
		}
		zaggies.Flush()
		time.Sleep(50 * time.Minute)
	}
}

func extractViewsToTmpDir() {
	layouts, err := viewsDir.ReadDir("views")
	if err != nil {
		log.Fatal(err)
	}
	targetDir, err := os.MkdirTemp(os.TempDir(), "honk*")
	if err != nil {
		log.Fatal(err)
	}

	os.MkdirAll(fmt.Sprintf("%s/views/", targetDir), 0777)
	// Generate our templates map from our layouts/ and includes/ directories
	for _, file := range layouts {
		log.Printf("Loading %v -> %s", file.Name(), targetDir)
		f, _ := viewsDir.Open("views/" + file.Name())
		ff, err := os.Create(fmt.Sprintf("%s/views/%s", targetDir, file.Name()))
		if err != nil {
			log.Fatalf("cannot unpack views %v", err)
		}
		io.Copy(ff, f)
		ff.Close()
	}

	viewDir = targetDir
}

func serve() {
	db := opendatabase()
	login.Init(login.InitArgs{Db: db, Logger: ilog, Insecure: develMode})
	// login.Init(login.InitArgs{Db: db, Logger: ilog, Insecure: true})

	// listener, err := openListener()
	// if err != nil {
	// 	elog.Fatal(err)
	// }
	runBackendServer()
	go exitSignalHandler()
	go redeliveryLoop()
	go tracker()
	go bgmonitor()
	loadLingo()
	extractViewsToTmpDir()

	readviews = templates.Load(develMode,
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
	if !develMode {
		assets := []string{
			viewDir + "/views/style.css",
			dataDir + "/views/local.css",
			viewDir + "/views/honkpage.js",
			dataDir + "/views/local.js",
		}
		for _, s := range assets {
			savedassetparams[s] = getassetparam(s)
		}
		loadAvatarColors()
	}

	for _, h := range preservehooks {
		h()
	}

	mux := mux.NewRouter()
	mux.NotFoundHandler = http.HandlerFunc(serveStaticSiteInstead)
	mux.Use(login.Checker)
	mux.Handle("/api", login.TokenRequired(http.HandlerFunc(apihandler)))

	PostSubRouter := mux.Methods("POST").Subrouter()
	GetSubrouter := mux.Methods("GET").Subrouter()

	GetSubrouter.HandleFunc("/honk", homepage)
	GetSubrouter.HandleFunc("/home", homepage)
	GetSubrouter.HandleFunc("/front", homepage)
	GetSubrouter.HandleFunc("/events", homepage)
	GetSubrouter.HandleFunc("/api/v1/instance", handleAPIInstance)
	GetSubrouter.HandleFunc("/nodeinfo/2.0", handleNodeInfo)
	GetSubrouter.HandleFunc("/robots.txt", robotsTxtHandler)
	GetSubrouter.HandleFunc("/rss", showrss)
	GetSubrouter.HandleFunc("/"+userSep+"/{name:[\\pL[:digit:]]+}", showuser)
	GetSubrouter.HandleFunc("/"+userSep+"/{name:[\\pL[:digit:]]+}/"+honkSep+"/{xid:[\\pL[:digit:]]+}", showonehonk)
	GetSubrouter.HandleFunc("/"+userSep+"/{name:[\\pL[:digit:]]+}/rss", showrss)
	PostSubRouter.HandleFunc("/"+userSep+"/{name:[\\pL[:digit:]]+}/inbox", inbox)
	GetSubrouter.HandleFunc("/"+userSep+"/{name:[\\pL[:digit:]]+}/outbox", outbox)
	GetSubrouter.HandleFunc("/"+userSep+"/{name:[\\pL[:digit:]]+}/followers", emptiness)
	GetSubrouter.HandleFunc("/"+userSep+"/{name:[\\pL[:digit:]]+}/following", emptiness)
	GetSubrouter.HandleFunc("/a", avatarWebHandler)
	GetSubrouter.HandleFunc("/o", thelistingoftheontologies)
	GetSubrouter.HandleFunc("/o/{name:.+}", showontology)
	GetSubrouter.HandleFunc("/d/{xid:[\\pL[:digit:].]+}", servefile)
	GetSubrouter.HandleFunc("/emu/{emu:[^.]*[^/]+}", serveemu)
	GetSubrouter.HandleFunc("/meme/{meme:[^.]*[^/]+}", servememe)
	GetSubrouter.HandleFunc("/.well-known/webfinger", fingerlicker)
	GetSubrouter.Handle("/metrics-honk", promhttp.Handler())
	calculateFollowersForMetrics()
	GetSubrouter.HandleFunc("/flag/{code:.+}", showflag)

	GetSubrouter.HandleFunc("/server", serveractor)
	PostSubRouter.HandleFunc("/server/inbox", serverinbox)
	PostSubRouter.HandleFunc("/inbox", serverinbox)

	GetSubrouter.HandleFunc("/style.css", serveviewasset)
	GetSubrouter.HandleFunc("/honkpage.js", serveviewasset)
	GetSubrouter.HandleFunc("/wonk.js", serveviewasset)
	GetSubrouter.HandleFunc("/local.css", servedataasset)
	GetSubrouter.HandleFunc("/local.js", servedataasset)
	GetSubrouter.HandleFunc("/icon.png", servedataasset)
	GetSubrouter.HandleFunc("/favicon.ico", servedataasset)

	GetSubrouter.HandleFunc("/about", servehtml)
	GetSubrouter.HandleFunc("/login", servehtml)
	PostSubRouter.HandleFunc("/dologin", login.LoginFunc)
	GetSubrouter.HandleFunc("/logout", login.LogoutFunc)
	GetSubrouter.HandleFunc("/help/{name:[\\pL[:digit:]_.-]+}", servehelp)

	GetSubrouter.HandleFunc("/bloat/wonkles", serveWordList)

	LoggedInRouter := mux.NewRoute().Subrouter()
	LoggedInRouter.Use(login.Required)
	LoggedInRouter.HandleFunc("/first", homepage)
	LoggedInRouter.HandleFunc("/chatter", showchatter)
	LoggedInRouter.Handle("/sendchonk", login.CSRFWrap("sendchonk", http.HandlerFunc(submitchonk)))
	LoggedInRouter.HandleFunc("/saved", homepage)
	LoggedInRouter.HandleFunc("/account", accountpage)
	LoggedInRouter.HandleFunc("/funzone", showfunzone)
	LoggedInRouter.HandleFunc("/chpass", dochpass)
	LoggedInRouter.HandleFunc("/atme", homepage)
	LoggedInRouter.HandleFunc("/longago", homepage)
	LoggedInRouter.HandleFunc("/hfcs", hfcspage)
	LoggedInRouter.HandleFunc("/xzone", xzone)
	LoggedInRouter.HandleFunc("/newhonk", newhonkpage)
	LoggedInRouter.HandleFunc("/edit", edithonkpage)
	LoggedInRouter.Handle("/honk", login.CSRFWrap("honkhonk", http.HandlerFunc(submitwebhonk)))
	LoggedInRouter.Handle("/bonk", login.CSRFWrap("honkhonk", http.HandlerFunc(submitbonk)))
	LoggedInRouter.Handle("/zonkit", login.CSRFWrap("honkhonk", http.HandlerFunc(zonkit)))
	LoggedInRouter.Handle("/savehfcs", login.CSRFWrap("filter", http.HandlerFunc(savehfcs)))
	LoggedInRouter.Handle("/saveuser", login.CSRFWrap("saveuser", http.HandlerFunc(saveuser)))
	LoggedInRouter.Handle("/ximport", login.CSRFWrap("ximport", http.HandlerFunc(ximport)))
	LoggedInRouter.HandleFunc("/honkers", showhonkers)
	LoggedInRouter.HandleFunc("/h/{name:[\\pL[:digit:]_.-]+}", showhonker)
	LoggedInRouter.HandleFunc("/h", showhonker)
	LoggedInRouter.HandleFunc("/c/{name:[\\pL[:digit:]_.-]+}", showcombo)
	LoggedInRouter.HandleFunc("/c", showcombos)
	LoggedInRouter.HandleFunc("/t", showconvoy)
	LoggedInRouter.HandleFunc("/q", showsearch)
	LoggedInRouter.HandleFunc("/hydra", webhydra)
	LoggedInRouter.Handle("/submithonker", login.CSRFWrap("submithonker", http.HandlerFunc(submithonker)))

	httpHandler := http.NewServeMux()
	httpHandler.HandleFunc("/", redirect)

	hserver := &http.Server{
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 300 * time.Second,
		IdleTimeout:  120 * time.Second,
		Handler:      mux,
		Addr:         ":443",
	}

	hListener := setupSSLConfig(hserver, httpHandler)
	go func() {
		s := &http.Server{
			Handler: hListener,
			Addr:    fmt.Sprintf(":%d", 80),
		}

		go s.ListenAndServe()
	}()

	err := hserver.ListenAndServeTLS("", "")
	if err != nil {
		elog.Fatalf("httpsSrv.ListendAndServeTLS() failed with %s", err)
	}
}

func redirect(w http.ResponseWriter, req *http.Request) {
	// remove/add not default ports from req.Host
	target := "https://" + req.Host + req.URL.Path
	if len(req.URL.RawQuery) > 0 {
		target += "?" + req.URL.RawQuery
	}

	http.Redirect(w, req, target,
		// see @andreiavrammsd comment: often 307 > 301
		http.StatusTemporaryRedirect)
}

var webRoot = flag.String("www.root", "/var/www/benjojo.co.uk", "Where you want to serve your actual website from (that isnt honk)")
var tlsRoot = flag.String("tls.names", "benjojo.co.uk,www.benjojo.co.uk", "What to lets encrypt for")
var tlsEmail = flag.String("tls.email", "www@benjojo.co.uk", "What email to tell let's encrypt")

func serveStaticSiteInstead(w http.ResponseWriter, r *http.Request) {
	http.FileServer(http.FS(os.DirFS(*webRoot))).ServeHTTP(w, r)
}

func setupSSLConfig(httpsSrv *http.Server, httpMux *http.ServeMux) (handler http.Handler) {
	getcertFunction, httpHandler := getNormalACMEConfig(httpMux)

	//Only enable cert failover logic where a cert is provided.
	httpsSrv.TLSConfig = &tls.Config{GetCertificate: getcertFunction, MinVersion: tls.VersionTLS12}
	return httpHandler
}

func getNormalACMEConfig(httpMux *http.ServeMux) (h func(*tls.ClientHelloInfo) (*tls.Certificate, error), handler http.Handler) {
	// Note: use a sensible value for data directory
	// this is where cached certificates are stored
	dataDir := "."
	hostPolicy := func(ctx context.Context, host string) error {
		for _, name := range strings.Split(*tlsRoot, ",") {
			if name == host {
				return nil
			}
		}

		return fmt.Errorf("acme/autocert: only %v host is allowed", tlsRoot)
	}

	m := &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		HostPolicy: hostPolicy,
		Cache:      autocert.DirCache(dataDir),
		Email:      *tlsEmail,
	}

	return m.GetCertificate, m.HTTPHandler(httpMux)
}
