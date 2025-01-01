//
// Copyright (c) 2019-2024 Ted Unangst <tedu@tedunangst.com>
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
	"errors"
	"flag"
	"fmt"
	"html/template"
	"io"
	"log"
	notrand "math/rand"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"regexp"
	"runtime/pprof"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/crypto/acme/autocert"
	"humungus.tedunangst.com/r/gonix"
	"humungus.tedunangst.com/r/webs/gencache"
	"humungus.tedunangst.com/r/webs/httpsig"
	"humungus.tedunangst.com/r/webs/junk"
	"humungus.tedunangst.com/r/webs/login"
	"humungus.tedunangst.com/r/webs/rss"
	"humungus.tedunangst.com/r/webs/templates"
	"humungus.tedunangst.com/r/webs/totp"
)

var readviews *templates.Template

var userSep = "u"
var honkSep = "h"

var develMode = false

var allemus []Emu

func getuserstyle(u *login.UserInfo) template.HTMLAttr {
	if u == nil {
		return ""
	}
	user, _ := getUserBio(u.Username)
	class := template.HTMLAttr("")
	if user.Options.SkinnyCSS {
		class += `class="skinny"`
	}
	return class
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
	templinfo["CommonJSParam"] = getassetparam(viewDir + "/views/common.js")
	templinfo["JSParam"] = getassetparam(viewDir + "/views/honkpage.js")
	templinfo["MiscJSParam"] = getassetparam(viewDir + "/views/misc.js")
	templinfo["LocalJSParam"] = getassetparam(dataDir + "/views/local.js")
	templinfo["ServerName"] = serverName
	templinfo["IconName"] = iconName
	templinfo["UserSep"] = userSep
	if r == nil {
		return templinfo
	}
	if u := login.GetUserInfo(r); u != nil {
		templinfo["UserInfo"], _ = getUserBio(u.Username)
		templinfo["UserStyle"] = getuserstyle(u)
		combos, _ := combocache.Get(UserID(u.UserID))
		templinfo["Combos"] = combos
	}
	return templinfo
}

var oldnews = gencache.New(gencache.Options[string, []byte]{
	Fill: func(url string) ([]byte, bool) {
		templinfo := getInfo(nil)
		var honks []*ActivityPubActivity
		var userid UserID = -1

		templinfo["ServerMessage"] = serverMsg
		switch url {
		case "/events":
			honks = geteventhonks(userid)
			templinfo["ServerMessage"] = "some recent and upcoming events"
		default:
			templinfo["ShowRSS"] = true
			honks = getpublichonks()
		}
		reverbolate(userid, honks)
		templinfo["Honks"] = honks
		templinfo["MapLink"] = getmaplink(nil)
		var buf bytes.Buffer
		err := readviews.Execute(&buf, "honkpage.html", templinfo)
		if err != nil {
			elog.Print(err)
		}
		return buf.Bytes(), true

	},
	Duration: 1 * time.Minute,
})

func lonelypage(w http.ResponseWriter, r *http.Request) {
	page, _ := oldnews.Get(r.URL.Path)
	if !develMode {
		w.Header().Set("Cache-Control", "max-age=60")
	}
	w.Write(page)
}

func homepage(w http.ResponseWriter, r *http.Request) {
	u := login.GetUserInfo(r)
	if u == nil {
		lonelypage(w, r)
		return
	}
	templinfo := getInfo(r)
	var honks []*ActivityPubActivity
	var userid UserID = -1

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
		userid = UserID(u.UserID)
		switch r.URL.Path {
		case "/atme":
			templinfo["ServerMessage"] = "at me!"
			templinfo["PageName"] = "atme"
			honks = gethonksforme(userid, 0)
			honks = osmosis(honks, userid, false)
			menewnone(userid)
			templinfo["UserInfo"], _ = getUserBio(u.Username)
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

func showemus(w http.ResponseWriter, r *http.Request) {
	templinfo := getInfo(r)
	templinfo["Emus"] = allemus
	err := readviews.Execute(w, "emus.html", templinfo)
	if err != nil {
		elog.Print(err)
	}
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
		honks = gethonksbyuser(name, false, 0, true)
	} else {
		honks = getpublichonks()
	}
	reverbolate(-1, honks)

	home := serverURL("/")
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
	t := firstofmany(j, "type")
	if t == "Delete" {
		a, _ := j.GetString("actor")
		o, _ := j.GetString("object")
		if a == o {
			dlog.Printf("crappola from %s", a)
			return true
		}
	}
	if t == "Announce" {
		if obj, ok := j.GetMap("object"); ok {
			j = obj
			t = firstofmany(j, "type")
		}
	}
	if t == "Undo" {
		if obj, ok := j.GetMap("object"); ok {
			j = obj
			t = firstofmany(j, "type")
		}
	}
	if t == "Listen" {
		return true
	}
	if t == "EmojiReact" {
		o, _ := j.GetString("object")
		if originate(o) != serverName {
			return true
		}
	}
	return false
}

func ping(user *WhatAbout, who string) {
	if targ := fullname(who, user.ID); targ != "" {
		who = targ
	}
	if !strings.HasPrefix(who, "https://") {
		who = gofish(who)
	}
	if who == "" {
		ilog.Printf("nobody to ping!")
		return
	}
	box, _ := boxofboxes.Get(who)
	if box == nil {
		ilog.Printf("no inbox to ping %s", who)
		return
	}
	ilog.Printf("sending ping to %s", box.In)
	j := junk.New()
	j["@context"] = itiswhatitis
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
	box, _ := boxofboxes.Get(who)
	if box == nil {
		ilog.Printf("no inbox to pong %s", who)
		return
	}
	j := junk.New()
	j["@context"] = itiswhatitis
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

func getinbox(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	user, err := getUserBio(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	u := login.GetUserInfo(r)
	if user.ID != UserID(u.UserID) {
		http.Error(w, "that's not you!", http.StatusForbidden)
		return
	}

	honks := gethonksforuser(user.ID, 0)
	if len(honks) > 60 {
		honks = honks[0:60]
	}

	jonks := make([]junk.Junk, 0, len(honks))
	for _, h := range honks {
		j, _ := jonkjonk(user, h)
		jonks = append(jonks, j)
	}

	j := junk.New()
	j["@context"] = itiswhatitis
	j["id"] = user.URL + "/inbox"
	j["attributedTo"] = user.URL
	j["type"] = "OrderedCollection"
	j["totalItems"] = len(jonks)
	j["orderedItems"] = jonks

	w.Header().Set("Content-Type", ldjsonContentType)
	j.Write(w)
}

func postinbox(w http.ResponseWriter, r *http.Request) {
	if !friendorfoe(r.Header.Get("Content-Type")) {
		http.Error(w, "speak activity please", http.StatusNotAcceptable)
		return
	}
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
	what := firstofmany(j, "type")
	who, _ := j.GetString("actor")
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
		}
		http.Error(w, "what did you call me?", http.StatusUnauthorized)
		return
	}
	origin := keymatch(keyname, who)
	if origin == "" {
		ilog.Printf("keyname actor mismatch: %s <> %s", keyname, who)
		if collectForwards && what == "Create" {
			var xid string
			obj, ok := j.GetMap("object")
			if ok {
				xid, _ = obj.GetString("id")
			} else {
				xid, _ = j.GetString("object")
			}
			if xid != "" {
				dlog.Printf("getting forwarded create from %s: %s", keyname, xid)
				go grabhonk(user, xid)
			}
		}
		return
	}
	metricFediEvent.WithLabelValues(what).Inc()

	switch what {
	case "Ping":
		id, _ := j.GetString("id")
		ilog.Printf("ping from %s: %s", who, id)
		pong(user, who, id)
	case "Pong":
		obj, _ := j.GetString("object")
		ilog.Printf("pong from %s: %s", who, obj)
	case "Follow":
		obj, _ := j.GetString("object")
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
			what := firstofmany(obj, "type")
			switch what {
			case "Service":
				fallthrough
			case "Person":
				return
			case "Question":
				return
			}
		}
		go xonksaver(user, j, origin)
	case "Undo":
		obj, ok := j.GetMap("object")
		if !ok {
			folxid, ok := j.GetString("object")
			if ok && originate(folxid) == origin {
				unfollowme(user, "", "", j)
			}
			return
		}
		what := firstofmany(obj, "type")
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
			addreaction(user, obj, who, content)
		}
	case "Like":
		obj, ok := j.GetString("object")
		if ok {
			log.Printf("%v obj was liked by %v - well done", obj, who)
		}
	default:
		go saveandcheck(user, j, origin)
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

func saveandcheck(user *WhatAbout, j junk.Junk, origin string) {
	xonk := xonksaver(user, j, origin)
	if xonk == nil {
		return
	}
	if sname := shortname(user.ID, xonk.Honker); sname == "" {
		dlog.Printf("received unexpected activity from %s", xonk.Honker)
		if xonk.Whofore == 0 {
			dlog.Printf("it's not even for me!")
		}
	}
}

func serverinbox(w http.ResponseWriter, r *http.Request) {
	if !friendorfoe(r.Header.Get("Content-Type")) {
		http.Error(w, "speak activity please", http.StatusNotAcceptable)
		return
	}
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
		}
		http.Error(w, "what did you call me?", http.StatusUnauthorized)
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
	re_ont := regexp.MustCompile(serverURL("/o") + "/([\\pL[:digit:]]+)")
	what := firstofmany(j, "type")
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
		what := firstofmany(obj, "type")
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
	j := junkuser(user, false)
	w.Header().Set("Content-Type", ldjsonContentType)
	j.Write(w)
}

func ximport(w http.ResponseWriter, r *http.Request) {
	u := login.GetUserInfo(r)
	xid := strings.TrimSpace(r.FormValue("q"))
	xonk := getActivityPubActivity(UserID(u.UserID), xid)
	if xonk == nil {
		info, j, err := investigate(xid)
		if j == nil {
			http.Error(w, "error getting external object", http.StatusInternalServerError)
			ilog.Printf("error getting external object: %s", err)
			return
		}
		if info != nil {
			xid = info.XID
		}
		user, _ := getUserBio(u.Username)

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
	honkers := make([]Honker, 0, 256)
	for rows.Next() {
		var xid string
		rows.Scan(&xid)
		honkers = append(honkers, Honker{XID: xid})
	}
	rows.Close()
	for i := range honkers {
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

var oldoutbox = gencache.New(gencache.Options[string, []byte]{Fill: func(name string) ([]byte, bool) {
	user, err := getUserBio(name)
	if err != nil {
		return nil, false
	}
	honks := gethonksbyuser(name, false, 0, false)
	if len(honks) > 20 {
		honks = honks[0:20]
	}

	jonks := make([]junk.Junk, 0, len(honks))
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

func getoutbox(w http.ResponseWriter, r *http.Request) {
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
	j, _ := oldoutbox.Get(name)
	w.Header().Set("Content-Type", ldjsonContentType)
	w.Write(j)
}

func postoutbox(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	user, err := getUserBio(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	u := login.GetUserInfo(r)
	if user.ID != UserID(u.UserID) {
		http.Error(w, "that's not you!", http.StatusForbidden)
		return
	}

	limiter := io.LimitReader(r.Body, 1*1024*1024)
	j, err := junk.Read(limiter)
	if err != nil {
		http.Error(w, "that's not json!", http.StatusBadRequest)
		return
	}

	who, _ := j.GetString("actor")
	if who != user.URL {
		http.Error(w, "that's not you!", http.StatusForbidden)
		return
	}
	what := firstofmany(j, "type")
	switch what {
	case "Create":
		honk := xonksaver2(user, j, serverName, true)
		if honk == nil {
			dlog.Printf("returned nil")
			return
		}
		go honkworldwide(user, honk)
	case "Follow":
		defer honkerinvalidator.Clear(user.ID)
		url, _ := j.GetString("object")
		honkerid, flavor, err := savehonker(user, url, "", "presub", "", "{}")
		if err != nil {
			http.Error(w, "had some trouble with that: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if flavor == "presub" {
			followyou(user, honkerid, false)
		}
	default:
		http.Error(w, "not sure about that", http.StatusBadRequest)
	}
}

var oldempties = gencache.New(gencache.Options[string, []byte]{Fill: func(url string) ([]byte, bool) {
	colname := "/followers"
	if strings.HasSuffix(url, "/following") {
		colname = "/following"
	}
	user := serverURL("%s", url[:len(url)-10])
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
	user, err := getUserBio(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if stealthmode(user.ID, r) {
		http.NotFound(w, r)
		return
	}
	if strings.HasSuffix(r.URL.Path, "/following") {
		u, ok := login.CheckToken(r)
		if ok && u.Username == name {
			honkers := gethonkers(user.ID)
			items := make([]string, 0, len(honkers))
			for _, h := range honkers {
				if h.Flavor == "sub" {
					items = append(items, h.XID)
				}
			}
			j := junk.New()
			j["@context"] = itiswhatitis
			j["id"] = user.URL + "/following"
			j["attributedTo"] = user.URL
			j["type"] = "Collection"
			j["totalItems"] = len(items)
			j["items"] = items
			w.Header().Set("Content-Type", ldjsonContentType)
			j.Write(w)
			return
		}
	}
	j, _ := oldempties.Get(r.URL.Path)
	w.Header().Set("Content-Type", ldjsonContentType)
	w.Write(j)
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
	wantjson := false
	if strings.HasSuffix(r.URL.Path, ".json") {
		wantjson = true
	}
	if friendorfoe(r.Header.Get("Accept")) || wantjson {
		j := asjonker(name)
		u, ok := login.CheckToken(r)
		if ok && u.Username == name {
			j = junkuser(user, true).ToBytes()
		}
		w.Header().Set("Content-Type", ldjsonContentType)
		w.Write(j)
		return
	}
	u := login.GetUserInfo(r)
	if u != nil && u.Username != name {
		u = nil
	}
	honks := gethonksbyuser(name, u != nil, 0, login.GetUserInfo(r) == nil)
	templinfo := getInfo(r)
	templinfo["PageName"] = "user"
	templinfo["PageArg"] = name
	templinfo["Name"] = user.Name
	templinfo["Honkology"] = oguser(user)
	templinfo["WhatAbout"] = user.HTAbout
	templinfo["ServerMessage"] = ""
	templinfo["APAltLink"] = templates.Sprintf("<link href='%s' rel='alternate' type='application/activity+json'>", user.URL)
	if u != nil {
		templinfo["HonkCSRF"] = login.GetCSRF("honkhonk", r)
	}
	honkpage(w, u, honks, templinfo)
}

func showhonker(w http.ResponseWriter, r *http.Request) {
	u := login.GetUserInfo(r)
	name := mux.Vars(r)["name"]
	var honks []*ActivityPubActivity
	if name == "" {
		name = r.FormValue("xid")
		honks = gethonksbyxonker(UserID(u.UserID), name, 0)
	} else {
		honks = gethonksbyhonker(UserID(u.UserID), name, 0)
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
	honks := gethonksbycombo(UserID(u.UserID), name, 0)
	honks = osmosis(honks, UserID(u.UserID), true)
	if friendorfoe(r.Header.Get("Accept")) {
		user, _ := getUserBio(u.Username)
		if len(honks) > 40 {
			honks = honks[0:40]
		}

		items := make([]junk.Junk, len(honks))
		for i, h := range honks {
			_, items[i] = jonkjonk(user, h)
		}

		j := junk.New()
		j["@context"] = itiswhatitis
		j["id"] = serverURL("/c/%s", name)
		j["name"] = name
		j["attributedTo"] = user.URL
		j["type"] = "OrderedCollection"
		j["totalItems"] = len(items)
		j["orderedItems"] = items

		j.Write(w)
		return
	}
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
	honks := gethonksbyconvoy(UserID(u.UserID), c, 0)
	templinfo := getInfo(r)
	if len(honks) > 0 {
		templinfo["TopHID"] = honks[0].ID
	}
	honks = osmosis(honks, UserID(u.UserID), false)
	//reversehonks(honks)
	honks = threadsort(honks)
	templinfo["PageName"] = "convoy"
	templinfo["PageArg"] = c
	templinfo["ServerMessage"] = "honks in convoy: " + c
	templinfo["HonkCSRF"] = login.GetCSRF("honkhonk", r)
	honkpage(w, u, honks, templinfo)
}
func showsearch(w http.ResponseWriter, r *http.Request) {
	q := r.FormValue("q")
	if strings.HasPrefix(q, "https://") {
		ximport(w, r)
		return
	}
	u := login.GetUserInfo(r)
	honks := gethonksbysearch(UserID(u.UserID), q, 0)
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
	var userid UserID = -1
	if u != nil {
		userid = UserID(u.UserID)
	}
	honks := gethonksbyontology(int64(userid), "#"+name, 0)
	if friendorfoe(r.Header.Get("Accept")) {
		if len(honks) > 40 {
			honks = honks[0:40]
		}

		xids := make([]string, 0, len(honks))
		for _, h := range honks {
			xids = append(xids, h.XID)
		}

		user := getserveruser()

		j := junk.New()
		j["@context"] = itiswhatitis
		j["id"] = serverURL("/o/%s", name)
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
	var userid UserID = -1
	if u != nil {
		userid = UserID(u.UserID)
	}
	rows, err := stmtAllOnts.Query(userid)
	if err != nil {
		elog.Printf("selection error: %s", err)
		return
	}
	defer rows.Close()
	onts := make([]Ont, 0, 1024)
	pops := make([]Ont, 0, 128)
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
		if o.Count < 3 {
			continue
		}
		o.Name = o.Name[1:]
		onts = append(onts, o)
		pops = append(pops, o)
	}
	if len(onts) > 1024 {
		sort.Slice(onts, func(i, j int) bool {
			return onts[i].Count > onts[j].Count
		})
		onts = onts[:1024]
	}
	sort.Slice(onts, func(i, j int) bool {
		return onts[i].Name < onts[j].Name
	})
	sort.Slice(pops, func(i, j int) bool {
		return pops[i].Count > pops[j].Count
	})
	if len(pops) > 40 {
		pops = pops[:40]
	}
	letters := make([]string, 0, 64)
	var lastrune rune = -1
	for _, o := range onts {
		if r := firstRune(o.Name); r != lastrune {
			letters = append(letters, string(r))
			lastrune = r
		}
	}
	if u == nil && !develMode {
		w.Header().Set("Cache-Control", "max-age=300")
	}
	templinfo := getInfo(r)
	templinfo["Pops"] = pops
	templinfo["Onts"] = onts
	templinfo["Letters"] = letters
	templinfo["FirstRune"] = firstRune
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

var trackchan = make(chan Track, 4)
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
				tracks = make(map[string][]string)
			}
			c <- true
		case <-endoftheworld:
			if len(tracks) > 0 {
				savetracks(tracks)
				tracks = make(map[string][]string)
			}
			readyalready <- true
			return
		}
	}
}

var re_keyholder = regexp.MustCompile(`keyId="([^"]+)"`)

func requestActor(r *http.Request) string {
	if sig := r.Header.Get("Signature"); sig != "" {
		if m := re_keyholder.FindStringSubmatch(sig); len(m) == 2 {
			return m[1]
		}
	}
	return ""
}

func trackback(xid string, r *http.Request) {
	who := requestActor(r)
	if who != "" {
		select {
		case trackchan <- Track{xid: xid, who: who}:
		default:
		}
	}
}

func sameperson(h1, h2 *ActivityPubActivity) bool {
	n1, n2 := h1.Honker, h2.Honker
	if h1.Oonker != "" {
		n1 = h1.Oonker
	}
	if h2.Oonker != "" {
		n2 = h2.Oonker
	}
	return n1 == n2
}

func threadposes(honks []*ActivityPubActivity, wanted int64) ([]*ActivityPubActivity, []int) {
	var poses []int
	var newhonks []*ActivityPubActivity
	for i, honk := range honks {
		if honk.ID > wanted {
			newhonks = append(newhonks, honk)
			poses = append(poses, i)
		}
	}
	return newhonks, poses
}

func threadsort(honks []*ActivityPubActivity) []*ActivityPubActivity {
	sort.Slice(honks, func(i, j int) bool {
		return honks[i].Date.Before(honks[j].Date)
	})
	honkx := make(map[string]*ActivityPubActivity, len(honks))
	kids := make(map[string][]*ActivityPubActivity, len(honks))
	for _, h := range honks {
		honkx[h.XID] = h
		rid := h.RID
		kids[rid] = append(kids[rid], h)
	}
	done := make(map[*ActivityPubActivity]bool, len(honks))
	thread := make([]*ActivityPubActivity, 0, len(honks))
	var nextlevel func(p *ActivityPubActivity)
	level := 0
	hasreply := func(p *ActivityPubActivity, who string) bool {
		for _, h := range kids[p.XID] {
			if h.Honker == who {
				return true
			}
		}
		return false
	}
	nextlevel = func(p *ActivityPubActivity) {
		levelup := level < 4
		if pp := honkx[p.RID]; p.RID == "" || (pp != nil && sameperson(p, pp)) {
			levelup = false
		}
		if level > 0 && len(kids[p.RID]) == 1 {
			if pp := honkx[p.RID]; pp != nil && len(kids[pp.RID]) == 1 {
				levelup = false
			}
		}
		if levelup {
			level++
		}
		p.Style += fmt.Sprintf(" level%d", level)
		childs := kids[p.XID]
		sort.SliceStable(childs, func(i, j int) bool {
			var ipts, jpts int
			if sameperson(childs[i], p) {
				ipts += 1
			} else if hasreply(childs[i], p.Honker) {
				ipts += 2
			}
			if sameperson(childs[j], p) {
				jpts += 1
			} else if hasreply(childs[j], p.Honker) {
				jpts += 2
			}
			return ipts > jpts
		})
		for _, h := range childs {
			if !done[h] {
				done[h] = true
				thread = append(thread, h)
				nextlevel(h)
			}
		}
		if levelup {
			level--
		}
	}
	for _, h := range honks {
		if !done[h] && h.RID == "" {
			done[h] = true
			thread = append(thread, h)
			nextlevel(h)
		}
	}
	for _, h := range honks {
		if !done[h] {
			done[h] = true
			thread = append(thread, h)
			nextlevel(h)
		}
	}
	return thread
}

func oguser(user *WhatAbout) template.HTML {
	short := user.About
	if len(short) > 160 {
		short = short[0:160] + "..."
	}
	title := user.Display
	imgurl := avatarURL(user)
	return templates.Sprintf(
		`<meta property="og:title" content="%s" />
<meta property="og:type" content="website" />
<meta property="og:url" content="%s" />
<meta property="og:image" content="%s" />
<meta property="og:description" content="%s" />`,
		title, user.URL, imgurl, short)
}

func honkology(honk *ActivityPubActivity) template.HTML {
	user, ok := somenumberedusers.Get(honk.UserID)
	if !ok {
		return ""
	}
	title := fmt.Sprintf("%s: %s", user.Display, honk.Precis)
	imgurl := avatarURL(user)
	for _, d := range honk.Donks {
		if d.Local && strings.HasPrefix(d.Media, "image") {
			imgurl = d.URL
			break
		}
	}
	short := honk.Noise
	if len(short) > 160 {
		short = short[0:160] + "..."
	}
	return templates.Sprintf(
		`<meta property="og:title" content="%s" />
<meta property="og:type" content="article" />
<meta property="article:author" content="%s" />
<meta property="og:url" content="%s" />
<meta property="og:image" content="%s" />
<meta property="og:description" content="%s" />`,
		title, user.URL, honk.XID, imgurl, short)
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
	wantjson := false
	path := r.URL.Path
	if strings.HasSuffix(path, ".json") {
		path = path[:len(path)-5]
		wantjson = true
	}
	xid := serverURL("%s", path)

	if friendorfoe(r.Header.Get("Accept")) || wantjson {
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
	if u != nil && UserID(u.UserID) != user.ID {
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

	templinfo := getInfo(r)
	rawhonks := gethonksbyconvoy(honk.UserID, honk.Convoy, 0)
	//reversehonks(rawhonks)
	rawhonks = threadsort(rawhonks)
	honks := make([]*ActivityPubActivity, 0, len(rawhonks))
	for i, h := range rawhonks {
		if h.XID == xid {
			templinfo["Honkology"] = honkology(h)
			if i > 0 {
				h.Style += " glow"
			}
		}
		if h.Public && (h.Whofore == WhoPublic || h.IsAcked()) {
			honks = append(honks, h)
		}
	}

	templinfo["ServerMessage"] = "one honk maybe more"
	if u != nil {
		templinfo["HonkCSRF"] = login.GetCSRF("honkhonk", r)
	}
	templinfo["APAltLink"] = templates.Sprintf("<link href='%s' rel='alternate' type='application/activity+json'>", xid)
	honkpage(w, u, honks, templinfo)
}

func honkpage(w http.ResponseWriter, u *login.UserInfo, honks []*ActivityPubActivity, templinfo map[string]interface{}) {
	var userid UserID = -1
	if u != nil {
		userid = UserID(u.UserID)
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
	whatabout := r.FormValue("whatabout")
	whatabout = strings.Replace(whatabout, "\r", "", -1)
	u := login.GetUserInfo(r)
	user, _ := getUserBio(u.Username)
	db := opendatabase()

	options := user.Options
	options.SkinnyCSS = r.FormValue("skinny") == "skinny"
	options.OmitImages = r.FormValue("omitimages") == "omitimages"
	options.MentionAll = r.FormValue("mentionall") == "mentionall"
	options.InlineQuotes = r.FormValue("inlineqts") == "inlineqts"
	options.MapLink = r.FormValue("maps")
	options.Reaction = r.FormValue("reaction")
	enabletotp := r.FormValue("enabletotp") == "enabletotp"
	if enabletotp {
		if options.TOTP == "" {
			options.TOTP = totp.NewSecret()
		}
	} else {
		if options.TOTP != "" {
			options.TOTP = ""
		}
	}

	sendupdate := false
	ava := re_avatar.FindString(whatabout)
	if ava != "" {
		whatabout = re_avatar.ReplaceAllString(whatabout, "")
		ava = ava[7:]
		if ava[0] == ' ' {
			ava = ava[1:]
		}
		ava = serverURL("/meme/%s", ava)
	}
	if ava != options.Avatar {
		options.Avatar = ava
		sendupdate = true
	}
	ban := re_banner.FindString(whatabout)
	if ban != "" {
		whatabout = re_banner.ReplaceAllString(whatabout, "")
		ban = ban[7:]
		if ban[0] == ' ' {
			ban = ban[1:]
		}
		ban = serverURL("/meme/%s", ban)
	}
	if ban != options.Banner {
		options.Banner = ban
		sendupdate = true
	}
	whatabout = strings.TrimSpace(whatabout)
	if whatabout != user.About {
		sendupdate = true
	}
	j, err := encodeJson(options)
	if err == nil {
		_, err = db.Exec("update users set about = ?, options = ? where username = ?", whatabout, j, u.Username)
	}
	if err != nil {
		elog.Printf("error bouting what: %s", err)
	}
	somenamedusers.Clear(u.Username)
	somenumberedusers.Clear(user.ID)
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
		Whofore:  WhoPublic,
		Convoy:   xonk.Convoy,
		Audience: []string{atContextString, oonker},
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
		Honker:   user.URL,
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
	u := login.GetUserInfo(r)
	user, _ := getUserBio(u.Username)

	if wherefore == "save" {
		xonk := getActivityPubActivity(user.ID, what)
		if xonk != nil {
			_, err := stmtUpdateFlags.Exec(flagIsSaved, xonk.ID)
			if err != nil {
				elog.Printf("error saving: %s", err)
			}
		}
		return
	}

	if wherefore == "unsave" {
		xonk := getActivityPubActivity(user.ID, what)
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
		xonk := getActivityPubActivity(user.ID, what)
		if xonk != nil {
			_, err := stmtUpdateFlags.Exec(flagIsReacted, xonk.ID)
			if err != nil {
				elog.Printf("error saving: %s", err)
			}
			sendzonkofsorts(xonk, user, "react", reaction)
		}
		return
	}

	// my hammer is too big, oh well
	defer oldjonks.Flush()

	if wherefore == "ack" {
		xonk := getActivityPubActivity(user.ID, what)
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
		xonk := getActivityPubActivity(user.ID, what)
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
		bonkit(what, user)
		return
	}

	if wherefore == "unbonk" {
		xonk := getbonk(user.ID, what)
		if xonk != nil {
			deletehonk(xonk.ID)
			xonk = getActivityPubActivity(user.ID, what)
			_, err := stmtClearFlags.Exec(flagIsBonked, xonk.ID)
			if err != nil {
				elog.Printf("error unbonking: %s", err)
			}
			sendzonkofsorts(xonk, user, "unbonk", "")
		}
		return
	}

	if wherefore == "untag" {
		xonk := getActivityPubActivity(user.ID, what)
		if xonk != nil {
			_, err := stmtUpdateFlags.Exec(flagIsUntagged, xonk.ID)
			if err != nil {
				elog.Printf("error untagging: %s", err)
			}
		}
		var badparents map[string]bool
		untagged.GetAndLock(user.ID, &badparents)
		badparents[what] = true
		untagged.Unlock()
		return
	}

	ilog.Printf("zonking %s %s", wherefore, what)
	if wherefore == "zonk" {
		xonk := getActivityPubActivity(user.ID, what)
		if xonk != nil {
			deletehonk(xonk.ID)
			if xonk.Whofore == WhoPublic || xonk.Whofore == WhoPrivate {
				sendzonkofsorts(xonk, user, "zonk", "")
			}
		}
	}
	_, err := stmtSaveZonker.Exec(user.ID, what, wherefore)
	if err != nil {
		elog.Printf("error saving zonker: %s", err)
		return
	}
}

func edithonkpage(w http.ResponseWriter, r *http.Request) {
	u := login.GetUserInfo(r)
	user, _ := getUserBio(u.Username)
	xid := r.FormValue("xid")
	honk := getActivityPubActivity(user.ID, xid)
	if !canedithonk(user, honk) {
		http.Error(w, "no editing that please", http.StatusInternalServerError)
		return
	}
	noise := honk.Noise

	honks := []*ActivityPubActivity{honk}
	donksforhonks(honks)
	var savedfiles []string
	for _, d := range honk.Donks {
		savedfiles = append(savedfiles, fmt.Sprintf("%s:%d", d.XID, d.FileID))
	}
	reverbolate(user.ID, honks)

	templinfo := getInfo(r)
	templinfo["HonkCSRF"] = login.GetCSRF("honkhonk", r)
	templinfo["Honks"] = honks
	templinfo["MapLink"] = getmaplink(u)
	templinfo["Noise"] = noise
	templinfo["SavedPlace"] = honk.Place
	if tm := honk.Time; tm != nil {
		templinfo["ShowTime"] = " "
		templinfo["StartTime"] = tm.StartTime.Format("2006-01-02 15:04")
		if tm.Duration != 0 {
			templinfo["Duration"] = tm.Duration
		}
	}
	templinfo["Onties"] = honk.Onties
	templinfo["SeeAlso"] = honk.SeeAlso
	templinfo["Link"] = honk.Link
	templinfo["LegalName"] = honk.LegalName
	templinfo["ServerMessage"] = "honk edit"
	templinfo["IsPreview"] = true
	templinfo["UpdateXID"] = honk.XID
	if len(savedfiles) > 0 {
		templinfo["SavedFile"] = strings.Join(savedfiles, ",")
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

	xonk := getActivityPubActivity(UserID(u.UserID), rid)
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

func submitdonk(w http.ResponseWriter, r *http.Request) ([]*Donk, error) {
	if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "multipart/form-data") {
		return nil, nil
	}
	var donks []*Donk
	for i, hdr := range r.MultipartForm.File["donk"] {
		if i > 16 {
			break
		}
		donk, err := formtodonk(w, r, hdr)
		if err != nil {
			return nil, err
		}
		donks = append(donks, donk)
	}
	return donks, nil
}

func formtodonk(w http.ResponseWriter, r *http.Request, filehdr *multipart.FileHeader) (*Donk, error) {
	file, err := filehdr.Open()
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
	var donkmeta DonkMeta
	img, err := bigshrink(data)
	if err == nil {
		data = img.Data
		donkmeta.Width = img.Width
		donkmeta.Height = img.Height
		format := img.Format
		media = "image/" + format
		if format == "jpeg" {
			format = "jpg"
		}
		if format == "svg+xml" {
			format = "svg"
		}
		name = make18CharRandomString() + "." + format
	} else {
		ct := http.DetectContentType(data)
		switch ct {
		case "application/pdf":
			maxsize := 10000000 // 10MB
			if len(data) > maxsize {
				ilog.Printf("bad image: %s too much pdf: %d", err, len(data))
				http.Error(w, "didn't like your attachment", http.StatusUnsupportedMediaType)
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
				http.Error(w, "didn't like your attachment", http.StatusUnsupportedMediaType)
				return nil, err
			}
			for i := 0; i < len(data); i++ {
				if data[i] < 32 && data[i] != '\t' && data[i] != '\r' && data[i] != '\n' {
					ilog.Printf("bad image: %s not text: %d", err, data[i])
					http.Error(w, "didn't like your attachment", http.StatusUnsupportedMediaType)
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
	donkmeta.Length = len(data)
	desc := strings.TrimSpace(r.FormValue("donkdesc"))
	if desc == "" {
		desc = name
	}
	fileid, xid, err := savefileandxid(name, desc, "", media, true, data, &donkmeta)
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

func websubmithonk(w http.ResponseWriter, r *http.Request) {
	h := submithonk(w, r)
	if h == nil {
		return
	}
	redir := h.XID[len(serverURL("")):]
	http.Redirect(w, r, redir, http.StatusSeeOther)
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

	u := login.GetUserInfo(r)
	user, _ := getUserBio(u.Username)

	dt := time.Now().UTC()
	updatexid := r.FormValue("updatexid")
	var honk *ActivityPubActivity
	if updatexid != "" {
		honk = getActivityPubActivity(user.ID, updatexid)
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
		honk = &ActivityPubActivity{
			UserID:   user.ID,
			Username: user.Name,
			What:     what,
			Honker:   user.URL,
			XID:      xid,
			Date:     dt,
			Format:   format,
		}
	}
	honk.SeeAlso = strings.TrimSpace(r.FormValue("seealso"))
	honk.Onties = strings.TrimSpace(r.FormValue("onties"))
	honk.Link = strings.TrimSpace(r.FormValue("link"))
	honk.LegalName = strings.TrimSpace(r.FormValue("legalname"))

	var convoy string
	noise = strings.Replace(noise, "\r", "", -1)
	if updatexid == "" && rid == "" {
		noise = re_convoy.ReplaceAllStringFunc(noise, func(m string) string {
			convoy = m[7:]
			convoy = strings.TrimSpace(convoy)
			if !re_convalidate.MatchString(convoy) {
				convoy = ""
			}
			return ""
		})
	}
	noise = quickrename(noise, user.ID)
	honk.Noise = noise
	precipitate(honk)
	noise = honk.Noise
	translate(honk)

	if rid != "" {
		xonk := getActivityPubActivity(user.ID, rid)
		if xonk == nil {
			http.Error(w, "replyto disappeared", http.StatusNotFound)
			return nil
		}
		if xonk.Public {
			honk.Audience = append(honk.Audience, xonk.Audience...)
		}
		convoy = xonk.Convoy
		for i, a := range honk.Audience {
			if a == atContextString {
				honk.Audience[0], honk.Audience[i] = honk.Audience[i], honk.Audience[0]
				break
			}
		}
		honk.RID = rid
		if xonk.Precis != "" && honk.Precis == "" {
			honk.Precis = xonk.Precis
			if !re_dangerous.MatchString(honk.Precis) {
				honk.Precis = "re: " + honk.Precis
			}
		}
	} else if updatexid == "" {
		honk.Audience = []string{atContextString}
	}
	if noise != "" && noise[0] == '@' {
		honk.Audience = append(grapevine(honk.Mentions), honk.Audience...)
	} else {
		honk.Audience = append(honk.Audience, grapevine(honk.Mentions)...)
	}
	honk.Convoy = convoy

	if honk.Convoy == "" {
		honk.Convoy = "data:,electrichonkytonk-" + make18CharRandomString()
	}
	butnottooloud(honk.Audience)
	honk.Audience = stringArrayTrimUntilDupe(honk.Audience)
	if len(honk.Audience) == 0 {
		ilog.Printf("honk to nowhere")
		http.Error(w, "honk to nowhere...", http.StatusNotFound)
		return nil
	}
	honk.Public = loudandproud(honk.Audience)

	donkxid := strings.Join(r.Form["donkxid"], ",")
	if donkxid == "" {
		donks, err := submitdonk(w, r)
		if err != nil && err != http.ErrMissingFile {
			return nil
		}
		if len(donks) > 0 {
			honk.Donks = append(honk.Donks, donks...)
			var xids []string
			for _, d := range honk.Donks {
				xids = append(xids, fmt.Sprintf("%s:%d", d.XID, d.FileID))
			}
			donkxid = strings.Join(xids, ",")
		}
	} else {
		xids := strings.Split(donkxid, ",")
		for i, xid := range xids {
			if i > 16 {
				break
			}
			p := strings.Split(xid, ":")
			xid = p[0]
			url := serverURL("/d/%s", xid)
			var donk *Donk
			if len(p) > 1 {
				fileid, _ := strconv.ParseInt(p[1], 10, 0)
				donk = finddonkid(fileid, url)
			} else {
				donk = finddonk(url)
			}
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
		// fuck it, the form has the final say
		honk.Public = r.FormValue("privacy") != "on"
		if !honk.Public {
			honk.Audience = []string{user.URL, user.URL + "/followers"}
		}
	}

	if honk.Public {
		honk.Whofore = WhoPublic
	} else {
		honk.Whofore = WhoPrivate
	}

	// back to markdown
	honk.Noise = noise

	if r.FormValue("preview") == "preview" {
		honks := []*ActivityPubActivity{honk}
		reverbolate(user.ID, honks)
		templinfo := getInfo(r)
		templinfo["HonkCSRF"] = login.GetCSRF("honkhonk", r)
		templinfo["Honks"] = honks
		templinfo["MapLink"] = getmaplink(u)
		templinfo["InReplyTo"] = r.FormValue("rid")
		templinfo["Noise"] = r.FormValue("noise")
		templinfo["Onties"] = honk.Onties
		templinfo["SeeAlso"] = honk.SeeAlso
		templinfo["Link"] = honk.Link
		templinfo["LegalName"] = honk.LegalName
		templinfo["SavedFile"] = donkxid
		if tm := honk.Time; tm != nil {
			templinfo["ShowTime"] = " "
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
		updatehonk(honk)
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

func firstRune(s string) rune { r, _ := utf8.DecodeRuneInString(s); return r }

func showhonkers(w http.ResponseWriter, r *http.Request) {
	userid := UserID(login.GetUserInfo(r).UserID)
	honkers := gethonkers(userid)
	letters := make([]string, 0, 64)
	var lastrune rune = -1
	for _, h := range honkers {
		if r := firstRune(h.Name); r != lastrune {
			letters = append(letters, string(r))
			lastrune = r
		}
	}
	templinfo := getInfo(r)
	templinfo["FirstRune"] = firstRune
	templinfo["Letters"] = letters
	templinfo["Honkers"] = honkers
	templinfo["HonkerCSRF"] = login.GetCSRF("submithonker", r)
	err := readviews.Execute(w, "honkers.html", templinfo)
	if err != nil {
		elog.Print(err)
	}
}

func showchatter(w http.ResponseWriter, r *http.Request) {
	u := login.GetUserInfo(r)
	chatnewnone(UserID(u.UserID))
	chatter := loadchatter(UserID(u.UserID), 0)
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
		target = fullname(target, user.ID)
	}
	if target == "" {
		http.Error(w, "who is that?", http.StatusInternalServerError)
		return
	}
	ch := Chonk{
		UserID: user.ID,
		XID:    xid,
		Who:    user.URL,
		Target: target,
		Date:   dt,
		Noise:  noise,
		Format: format,
	}
	donks, err := submitdonk(w, r)
	if err != nil && err != http.ErrMissingFile {
		return
	}
	if len(donks) > 0 {
		ch.Donks = append(ch.Donks, donks...)
	}

	translatechonk(&ch)
	savechonk(&ch)
	// reload for consistency
	ch.Donks = nil
	donksforchonks([]*Chonk{&ch})
	go sendchonk(user, &ch)

	http.Redirect(w, r, "/chatter", http.StatusSeeOther)
}

var combocache = gencache.New(gencache.Options[UserID, []string]{Fill: func(userid UserID) ([]string, bool) {
	honkers := gethonkers(userid)
	combos := make([]string, 0, len(honkers))
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
	templinfo := getInfo(r)
	err := readviews.Execute(w, "combos.html", templinfo)
	if err != nil {
		elog.Print(err)
	}
}

func websubmithonker(w http.ResponseWriter, r *http.Request) {
	h := submithonker(w, r)
	if h == nil {
		return
	}
	http.Redirect(w, r, "/honkers", http.StatusSeeOther)
}

func submithonker(w http.ResponseWriter, r *http.Request) *Honker {
	u := login.GetUserInfo(r)
	user, _ := getUserBio(u.Username)
	name := strings.TrimSpace(r.FormValue("name"))
	url := strings.TrimSpace(r.FormValue("url"))
	peep := r.FormValue("peep")
	combos := strings.TrimSpace(r.FormValue("combos"))
	combos = " " + combos + " "
	honkerid, _ := strconv.ParseInt(r.FormValue("honkerid"), 10, 0)

	re_namecheck := regexp.MustCompile("^[\\pL[:digit:]_.-]+$")
	if name != "" && !re_namecheck.MatchString(name) {
		http.Error(w, "please use a plainer name", http.StatusInternalServerError)
		return nil
	}

	var meta HonkerMeta
	meta.Notes = strings.TrimSpace(r.FormValue("notes"))
	mj, _ := encodeJson(&meta)

	defer honkerinvalidator.Clear(user.ID)

	// mostly dummy, fill in later...
	h := &Honker{
		ID: honkerid,
	}

	if honkerid > 0 {
		if r.FormValue("delete") == "delete" {
			unfollowyou(user, honkerid, false)
			stmtDeleteHonker.Exec(honkerid)
			return h
		}
		if r.FormValue("unsub") == "unsub" {
			unfollowyou(user, honkerid, false)
		}
		if r.FormValue("sub") == "sub" {
			followyou(user, honkerid, false)
		}
		_, err := stmtUpdateHonker.Exec(name, combos, mj, honkerid, user.ID)
		if err != nil {
			elog.Printf("update honker err: %s", err)
			return nil
		}
		return h
	}

	if url == "" {
		http.Error(w, "subscribing to nothing?", http.StatusInternalServerError)
		return nil
	}

	flavor := "presub"
	if peep == "peep" {
		flavor = "peep"
	}

	var err error
	honkerid, flavor, err = savehonker(user, url, name, flavor, combos, mj)
	if err != nil {
		http.Error(w, "had some trouble with that: "+err.Error(), http.StatusInternalServerError)
		return nil
	}
	if flavor == "presub" {
		followyou(user, honkerid, false)
	}
	h.ID = honkerid
	return h
}

func hfcspage(w http.ResponseWriter, r *http.Request) {
	userinfo := login.GetUserInfo(r)

	filters := getfilters(UserID(userinfo.UserID), filtAny)

	templinfo := getInfo(r)
	templinfo["Filters"] = filters
	templinfo["FilterCSRF"] = login.GetCSRF("filter", r)
	err := readviews.Execute(w, "hfcs.html", templinfo)
	if err != nil {
		elog.Print(err)
	}
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

var oldfingers = gencache.New(gencache.Options[string, []byte]{Fill: func(orig string) ([]byte, bool) {
	if strings.HasPrefix(orig, "acct:") {
		orig = orig[5:]
	}
	name := orig
	idx := strings.LastIndexByte(name, '/')
	if idx != -1 {
		name = name[idx+1:]
		if serverURL("/%s/%s", userSep, name) != orig {
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
		return nil, false
	}

	j := junk.New()
	j["subject"] = fmt.Sprintf("acct:%s@%s", user.Name, masqName)
	j["aliases"] = []string{user.URL}
	l := junk.New()
	l["rel"] = "self"
	l["type"] = `application/activity+json`
	l["href"] = user.URL

	stupidAvatarThingForStreetPass := junk.New()
	stupidAvatarThingForStreetPass["rel"] = "http://webfinger.net/rel/avatar"
	stupidAvatarThingForStreetPass["type"] = `image/jpeg`
	stupidAvatarThingForStreetPass["href"] = fmt.Sprintf("https://benjojo.co.uk/a?a=%s", url.QueryEscape(user.URL))

	stupidProfileThingForStreetPass := junk.New()
	stupidProfileThingForStreetPass["rel"] = "http://webfinger.net/rel/profile-page"
	stupidProfileThingForStreetPass["type"] = `text/html`
	stupidProfileThingForStreetPass["href"] = user.URL

	j["links"] = []junk.Junk{l, stupidAvatarThingForStreetPass, stupidProfileThingForStreetPass}
	return j.ToBytes(), true
}})

func fingerlicker(w http.ResponseWriter, r *http.Request) {
	orig := r.FormValue("resource")

	dlog.Printf("finger lick: %s", orig)

	j, ok := oldfingers.Get(orig)
	if ok {
		w.Header().Set("Content-Type", "application/jrd+json")
		w.Write(j)
	} else {
		http.NotFound(w, r)
	}
}

func somedays() string {
	secs := 432000 + notrand.Int63n(432000)
	return fmt.Sprintf("%d", secs)
}

func lookatme(ava string) string {
	if strings.Contains(ava, serverName+"/"+userSep) {
		idx := strings.LastIndexByte(ava, '/')
		if idx < len(ava) {
			name := ava[idx+1:]
			user, _ := getUserBio(name)
			if user != nil && user.URL == ava {
				return user.Options.Avatar
			}
		}
	}
	return ""
}

func avatate(w http.ResponseWriter, r *http.Request) {
	if develMode {
		loadAvatarColors()
	}
	n := r.FormValue("a")
	forceHexAva := r.FormValue("hex") == "1"
	if forceHexAva {
		easyAvatar(r, n, w)
		return
	}

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
	j, err := GetJunk(1, /*Terrible hack, this hard codes avatar fetchs to be done by the first user of the instance*/
		xid)
	if err != nil {
		easyAvatar(r, n, w)
		return
	}

	info, _ := somethingabout(j)
	if info.AvatarURL == "" {
		easyAvatar(r, n, w)
		return
	}
	if strings.Contains(xid, serverName) && !strings.Contains(info.AvatarURL, "meme") {
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
	a := genAvatar(n)
	if !develMode {
		w.Header().Set("Cache-Control", "max-age="+somedays())
	}
	w.Write(a)
}

func serveviewasset(w http.ResponseWriter, r *http.Request) {
	serveasset(w, r, viewDir)
}
func servedataasset(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/favicon.ico" {
		r.URL.Path = "/icon.png"
	}
	serveasset(w, r, dataDir)
}

func serveasset(w http.ResponseWriter, r *http.Request, basedir string) {
	if !develMode {
		w.Header().Set("Cache-Control", "max-age=7776000")
	}
	http.ServeFile(w, r, basedir+"/views"+r.URL.Path)
	metricFileServed.WithLabelValues().Inc()
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
	_, err := os.Stat(dataDir + "/memes/" + meme)
	if err == nil {
		http.ServeFile(w, r, dataDir+"/memes/"+meme)
	} else {
		mux.Vars(r)["xid"] = meme
		servefile(w, r)
	}
}

func refetchfile(xid string) ([]byte, error) {
	donk := getfileinfo(xid)
	if donk == nil {
		return nil, errors.New("filemeta not found")
	}
	dlog.Printf("refetching missing file data %s", donk.URL)
	return fetchsome(donk.URL)
}

func servefile(w http.ResponseWriter, r *http.Request) {
	if friendorfoe(r.Header.Get("Accept")) {
		dlog.Printf("incompatible accept for donk")
		http.Error(w, "there are no activities here", http.StatusNotAcceptable)
		return
	}
	xid := mux.Vars(r)["xid"]

	servefiledata(w, r, xid)
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
	Poses     []int
}

func webhydra(w http.ResponseWriter, r *http.Request) {
	u := login.GetUserInfo(r)
	userid := UserID(u.UserID)
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
		honks = gethonksbyconvoy(userid, c, 0)
		honks = osmosis(honks, userid, false)
		honks = threadsort(honks)
		honks, hydra.Poses = threadposes(honks, wanted)
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
		honks = gethonksbyuser(uname, u != nil && u.Username == uname, wanted, false)
		hydra.Srvmsg = templates.Sprintf("honks by user: %s", uname)
	default:
		http.NotFound(w, r)
	}

	if len(honks) > 0 {
		if page == "convoy" {
			hydra.Tophid = honks[len(honks)-1].ID
		} else {
			hydra.Tophid = honks[0].ID
		}
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
	userid := UserID(u.UserID)
	action := r.FormValue("action")
	wait, _ := strconv.ParseInt(r.FormValue("wait"), 10, 0)
	dlog.Printf("api request '%s' on behalf of %s", action, u.Username)
	switch action {
	case "honk":
		h := submithonk(w, r)
		if h == nil {
			return
		}
		fmt.Fprintf(w, "%s", h.XID)
	case "donk":
		donks, err := submitdonk(w, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if len(donks) == 0 {
			http.Error(w, "missing donk", http.StatusBadRequest)
			return
		}
		d := donks[0]
		donkxid := fmt.Sprintf("%s:%d", d.XID, d.FileID)
		w.Write([]byte(donkxid))
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
		case "myhonks":
			honks = gethonksbyuser(u.Username, true, wanted, false)
			honks = osmosis(honks, userid, true)
		case "saved":
			honks = getsavedhonks(userid, wanted)
		case "combo":
			c := r.FormValue("c")
			honks = gethonksbycombo(userid, c, wanted)
			honks = osmosis(honks, userid, false)
		case "convoy":
			c := r.FormValue("c")
			honks = gethonksbyconvoy(userid, c, 0)
			honks = osmosis(honks, userid, false)
			honks = threadsort(honks)
			honks, _ = threadposes(honks, wanted)
		case "honker":
			xid := r.FormValue("xid")
			honks = gethonksbyxonker(userid, xid, wanted)
		case "search":
			q := r.FormValue("q")
			honks = gethonksbysearch(userid, q, wanted)
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
		user, _ := getUserBio(u.Username)
		j := junk.New()
		j["honks"] = honks
		j["mecount"] = user.Options.MeCount
		j["chatcount"] = user.Options.ChatCount
		j.Write(w)
	case "sendactivity":
		public := r.FormValue("public") == "1"
		user, _ := getUserBio(u.Username)
		rcpts := boxuprcpts(user, r.Form["rcpt"], public)
		msg := []byte(r.FormValue("msg"))
		for rcpt := range rcpts {
			go deliverate(userid, rcpt, msg)
		}
	case "gethonkers":
		j := junk.New()
		j["honkers"] = gethonkers(userid)
		j.Write(w)
	case "savehonker":
		h := submithonker(w, r)
		if h == nil {
			return
		}
		fmt.Fprintf(w, "%d", h.ID)
	case "getchatter":
		wanted, _ := strconv.ParseInt(r.FormValue("after"), 10, 0)
		chatnewnone(UserID(u.UserID))
		user, _ := getUserBio(u.Username)
		chatter := loadchatter(UserID(u.UserID), wanted)
		for _, chat := range chatter {
			for _, ch := range chat.Chonks {
				filterchonk(ch)
			}
		}
		j := junk.New()
		j["chatter"] = chatter
		j["mecount"] = user.Options.MeCount
		j["chatcount"] = user.Options.ChatCount
		j.Write(w)
	default:
		http.Error(w, "unknown action", http.StatusNotFound)
		return
	}
}

func fiveoh(w http.ResponseWriter, r *http.Request) {
	if !develMode {
		return
	}
	fd, err := os.OpenFile("violations.json", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		elog.Printf("error opening violations! %s", err)
		return
	}
	defer fd.Close()
	io.Copy(fd, r.Body)
	fd.WriteString("\n")
}

var endoftheworld = make(chan bool)
var readyalready = make(chan bool)
var workinprogress = 0
var requestWG sync.WaitGroup
var listenSocket net.Listener

func enditall() {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	<-sig
	ilog.Printf("stopping...")
	listenSocket.Close()
	for i := 0; i < workinprogress; i++ {
		endoftheworld <- true
	}
	if *cpuprofile != "" {
		pprof.StopCPUProfile()
	}
	if *memprofile != "" {
		pprof.WriteHeapProfile(memprofilefd)
		memprofilefd.Close()
	}
	ilog.Printf("waiting...")
	go func() {
		time.Sleep(10 * time.Second)
		elog.Printf("timed out waiting for requests to finish")
		os.Exit(0)
	}()
	for i := 0; i < workinprogress; i++ {
		<-readyalready
	}
	requestWG.Wait()
	ilog.Printf("apocalypse")
	closedatabases()
	os.Exit(0)
}

func bgmonitor() {
	for {
		time.Sleep(150 * time.Minute)
		continue
		when := time.Now().Add(-2 * 24 * time.Hour).UTC().Format(dbtimeformat)
		_, err := stmtDeleteOldXonkers.Exec(when)
		if err != nil {
			elog.Printf("error deleting old xonkers: %s", err)
		}
		xonkInvalidator.Flush()
	}
}

var coolGuysISuppose = []string{
	"chrome-extension://",
	"safari-extension://",
	"moz-extension://",
	"ms-browser-extension://",
}

func addcspheaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestWG.Add(1)
		defer requestWG.Done()

		origin := r.Header.Get("Origin")
		for _, v := range coolGuysISuppose {
			if strings.HasPrefix(origin, v) {
				w.Header().Set("access-control-allow-origin", "*")
			}
		}

		policy := "default-src 'none'; script-src 'self'; connect-src 'self'; style-src 'self'; img-src 'self'; media-src 'self'"
		if develMode {
			policy += "; report-uri /csp-violation"
		}
		w.Header().Set("Content-Security-Policy", policy)
		next.ServeHTTP(w, r)
	})
}

func emuinit() {
	var emunames []string
	dir, err := os.Open(dataDir + "/emus")
	if err == nil {
		emunames, _ = dir.Readdirnames(0)
		dir.Close()
	}
	allemus = make([]Emu, 0, len(emunames))
	for _, e := range emunames {
		if len(e) <= 4 {
			continue
		}
		ext := e[len(e)-4:]
		emu := Emu{
			ID:   fmt.Sprintf("/emu/%s", e),
			Name: e[:len(e)-4],
			Type: "image/" + ext[1:],
		}
		allemus = append(allemus, emu)
	}
	sort.Slice(allemus, func(i, j int) bool {
		return allemus[i].Name < allemus[j].Name
	})
}

var savedassetparams = make(map[string]string)

func getassetparam(file string) string {
	if p, ok := savedassetparams[file]; ok {
		return p
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return ""
	}
	hasher := sha512.New()
	hasher.Write(data)

	return fmt.Sprintf("?v=%.8x", hasher.Sum(nil))
}

func startWatcher() {
	watcher, err := gonix.NewWatcher()
	if err != nil {
		return
	}
	go func() {
		s := dataDir + "/views/local.css"
		for {
			err := watcher.WatchFile(s)
			if err != nil {
				break
			}
			err = watcher.WaitForChange()
			if err != nil {
				dlog.Printf("can't wait: %s", err)
				break
			}
			dlog.Printf("local.css changed")
			delete(savedassetparams, s)
			savedassetparams[s] = getassetparam(s)
		}
	}()
}

func doubleCheck(username string, r *http.Request) bool {
	user, err := getUserBio(username)
	if err != nil {
		return false
	}
	if user.Options.TOTP != "" {
		code, _ := strconv.Atoi(r.FormValue("totpcode"))
		return totp.CheckCode(user.Options.TOTP, code)
	}
	return true
}

func serve() {
	db := opendatabase()
	login.Init(login.InitArgs{
		Db:             db,
		Logger:         ilog,
		Insecure:       develMode,
		SameSiteStrict: !develMode,
		SecondFactor:   doubleCheck,
	})

	runBackendServer()
	go enditall()
	go redeliverator()
	go tracker()
	go syndicator()
	go bgmonitor()
	go qotd()
	loadLingo()
	extractViewsToTmpDir()
	emuinit()

	var toload []string
	dents, _ := os.ReadDir(viewDir + "/views")
	for _, dent := range dents {
		name := dent.Name()
		if strings.HasSuffix(name, ".html") {
			toload = append(toload, viewDir+"/views/"+name)
		}
	}

	readviews = templates.Load(develMode, toload...)
	if !develMode {
		assets := []string{
			viewDir + "/views/style.css",
			dataDir + "/views/local.css",
			viewDir + "/views/common.js",
			viewDir + "/views/honkpage.js",
			viewDir + "/views/misc.js",
			dataDir + "/views/local.js",
		}
		for _, s := range assets {
			savedassetparams[s] = getassetparam(s)
		}
		loadAvatarColors()
	}
	startWatcher()

	securitizeweb()

	mux := mux.NewRouter()
	mux.Use(addcspheaders)
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
	GetSubrouter.HandleFunc("/"+userSep+"/{name:[\\pL[:digit:]]+}.json", showuser)
	GetSubrouter.HandleFunc("/"+userSep+"/{name:[\\pL[:digit:]]+}/"+honkSep+"/{xid:[\\pL[:digit:]]+}", showonehonk)
	GetSubrouter.HandleFunc("/"+userSep+"/{name:[\\pL[:digit:]]+}/"+honkSep+"/{xid:[\\pL[:digit:]]+}.json", showonehonk)
	GetSubrouter.HandleFunc("/"+userSep+"/{name:[\\pL[:digit:]]+}/rss", showrss)
	PostSubRouter.HandleFunc("/"+userSep+"/{name:[\\pL[:digit:]]+}/inbox", postinbox)
	GetSubrouter.Handle("/"+userSep+"/{name:[\\pL[:digit:]]+}/inbox", login.TokenRequired(http.HandlerFunc(getinbox)))
	GetSubrouter.HandleFunc("/"+userSep+"/{name:[\\pL[:digit:]]+}/outbox", getoutbox)
	PostSubRouter.Handle("/"+userSep+"/{name:[\\pL[:digit:]]+}/outbox", login.TokenRequired(http.HandlerFunc(postoutbox)))
	GetSubrouter.HandleFunc("/"+userSep+"/{name:[\\pL[:digit:]]+}/followers", emptiness)
	GetSubrouter.HandleFunc("/"+userSep+"/{name:[\\pL[:digit:]]+}/following", emptiness)
	GetSubrouter.HandleFunc("/a", avatate)
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

	PostSubRouter.HandleFunc("/csp-violation", fiveoh)

	GetSubrouter.HandleFunc("/style.css", serveviewasset)
	GetSubrouter.HandleFunc("/common.js", serveviewasset)
	GetSubrouter.HandleFunc("/honkpage.js", serveviewasset)
	GetSubrouter.HandleFunc("/misc.js", serveviewasset)
	GetSubrouter.HandleFunc("/local.css", servedataasset)
	GetSubrouter.HandleFunc("/local.js", servedataasset)
	GetSubrouter.HandleFunc("/icon.png", servedataasset)
	GetSubrouter.HandleFunc("/favicon.ico", servedataasset)

	GetSubrouter.HandleFunc("/about", servehtml)
	GetSubrouter.HandleFunc("/login", servehtml)
	PostSubRouter.HandleFunc("/dologin", login.LoginFunc)
	GetSubrouter.HandleFunc("/logout", login.LogoutFunc)
	GetSubrouter.HandleFunc("/help/{name:[\\pL[:digit:]_.-]+}", servehelp)

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
	LoggedInRouter.Handle("/honk", login.CSRFWrap("honkhonk", http.HandlerFunc(websubmithonk)))
	LoggedInRouter.Handle("/bonk", login.CSRFWrap("honkhonk", http.HandlerFunc(submitbonk)))
	LoggedInRouter.Handle("/zonkit", login.CSRFWrap("honkhonk", http.HandlerFunc(zonkit)))
	LoggedInRouter.Handle("/savehfcs", login.CSRFWrap("filter", http.HandlerFunc(savehfcs)))
	LoggedInRouter.Handle("/saveuser", login.CSRFWrap("saveuser", http.HandlerFunc(saveuser)))
	LoggedInRouter.Handle("/ximport", login.CSRFWrap("ximport", http.HandlerFunc(ximport)))
	LoggedInRouter.HandleFunc("/honkers", showhonkers)
	LoggedInRouter.HandleFunc("/h/{name:[\\pL[:digit:]_.-]+}", showhonker)
	LoggedInRouter.HandleFunc("/h", showhonker)
	LoggedInRouter.HandleFunc("/c/{name:[\\pL[:digit:]#_.-]+}", showcombo)
	LoggedInRouter.HandleFunc("/c", showcombos)
	LoggedInRouter.HandleFunc("/t", showconvoy)
	LoggedInRouter.HandleFunc("/q", showsearch)
	LoggedInRouter.HandleFunc("/hydra", webhydra)
	LoggedInRouter.HandleFunc("/emus", showemus)
	LoggedInRouter.Handle("/submithonker", login.CSRFWrap("submithonker", http.HandlerFunc(websubmithonker)))

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
var tlsCertPath = flag.String("tls.cert", "", "The file path where a TLS certificate can be found. If enabled with tls.key, Lets Encrypt will be disabled.")
var tlsKeyPath = flag.String("tls.key", "", "The file path where a TLS key can be found. If enabled with tls.key, Lets Encrypt will be disabled.")

func serveStaticSiteInstead(w http.ResponseWriter, r *http.Request) {
	http.FileServer(http.FS(os.DirFS(*webRoot))).ServeHTTP(w, r)
}

func setupSSLConfig(httpsSrv *http.Server, httpMux *http.ServeMux) (handler http.Handler) {
	var minimumTlsVersion uint16 = tls.VersionTLS12
	// If a static TLS certificate and key path were specified, use them
	if *tlsCertPath != "" && *tlsKeyPath != "" {
		certData, err := os.ReadFile(*tlsCertPath)
		if err != nil {
			log.Fatal(err)
		}
		keyData, err := os.ReadFile(*tlsKeyPath)
		if err != nil {
			log.Fatal(err)
		}
		cert, err := tls.X509KeyPair(certData, keyData)
		if err != nil {
			log.Fatal(err)
		}
		httpsSrv.TLSConfig = &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: minimumTlsVersion}
		return nil
	}
	// Otherwise, if a TLS certificate and key path were not specified, default onto using ACME	getcertFunction, httpHandler := getNormalACMEConfig(httpMux)
	getcertFunction, httpHandler := getNormalACMEConfig(httpMux)

	//Only enable cert failover logic where a cert is provided.
	httpsSrv.TLSConfig = &tls.Config{GetCertificate: getcertFunction, MinVersion: minimumTlsVersion}
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
