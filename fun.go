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
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"html"
	"html/template"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"

	"humungus.tedunangst.com/r/webs/htfilter"
	"humungus.tedunangst.com/r/webs/httpsig"
)

func reverbolate(userid int64, honks []*Honk) {
	filt := htfilter.New()
	zilences := getzilences(userid)
	for _, h := range honks {
		h.What += "ed"
		if h.What == "tonked" {
			h.What = "honked back"
			h.Style = "subtle"
		}
		if !h.Public {
			h.Style += " limited"
		}
		if h.Whofore == 2 || h.Whofore == 3 {
			h.URL = h.XID
			if h.What != "bonked" {
				h.Noise = mentionize(h.Noise)
			}
			h.Username, h.Handle = honkerhandle(h.Honker)
		} else {
			_, h.Handle = honkerhandle(h.Honker)
			h.Username = h.Handle
			if len(h.Username) > 20 {
				h.Username = h.Username[:20] + ".."
			}
			if h.URL == "" {
				h.URL = h.XID
			}
		}
		if h.Oonker != "" {
			_, h.Oondle = honkerhandle(h.Oonker)
		}
		zap := make(map[*Donk]bool)
		h.Noise = unpucker(h.Noise)
		h.Open = "open"
		if badword := unsee(zilences, h.Precis, h.Noise); badword != "" {
			if h.Precis == "" {
				h.Precis = badword
			}
			h.Open = ""
		} else if h.Precis == "unspecified horror" {
			h.Precis = ""
		}
		h.HTML, _ = filt.String(h.Noise)
		emuxifier := func(e string) string {
			for _, d := range h.Donks {
				if d.Name == e {
					zap[d] = true
					if d.Local {
						return fmt.Sprintf(`<img class="emu" title="%s" src="/d/%s">`, d.Name, d.XID)
					}
				}
			}
			return e
		}
		h.HTML = template.HTML(re_emus.ReplaceAllStringFunc(string(h.HTML), emuxifier))
		j := 0
		for i := 0; i < len(h.Donks); i++ {
			if !zap[h.Donks[i]] {
				h.Donks[j] = h.Donks[i]
				j++
			}
		}
		h.Donks = h.Donks[:j]
	}
}

func unsee(zilences []*regexp.Regexp, precis string, noise string) string {
	for _, z := range zilences {
		if z.MatchString(precis) || z.MatchString(noise) {
			if precis == "" {
				w := z.String()
				return w[6 : len(w)-3]
			}
			return precis
		}
	}
	return ""
}

func osmosis(honks []*Honk, userid int64) []*Honk {
	zords := getzords(userid)
	j := 0
outer:
	for _, h := range honks {
		for _, z := range zords {
			if z.MatchString(h.Precis) || z.MatchString(h.Noise) {
				continue outer
			}
		}
		honks[j] = h
		j++
	}
	honks = honks[0:j]
	return honks
}

func shortxid(xid string) string {
	idx := strings.LastIndexByte(xid, '/')
	if idx == -1 {
		return xid
	}
	return xid[idx+1:]
}

func xfiltrate() string {
	letters := "BCDFGHJKLMNPQRSTVWXYZbcdfghjklmnpqrstvwxyz1234567891234567891234"
	var b [18]byte
	rand.Read(b[:])
	for i, c := range b {
		b[i] = letters[c&63]
	}
	s := string(b[:])
	return s
}

var re_hashes = regexp.MustCompile(`(?:^|\W)#[[:alnum:]]+`)

func ontologies(s string) []string {
	m := re_hashes.FindAllString(s, -1)
	j := 0
	for _, h := range m {
		if h[0] == '&' {
			continue
		}
		if h[0] != '#' {
			h = h[1:]
		}
		m[j] = h
		j++
	}
	return m[:j]
}

type Mention struct {
	who   string
	where string
}

var re_mentions = regexp.MustCompile(`@[[:alnum:]._-]+@[[:alnum:].-]*[[:alnum:]]`)
var re_urltions = regexp.MustCompile(`@https://\S+`)

func grapevine(s string) []string {
	var mentions []string
	m := re_mentions.FindAllString(s, -1)
	for i := range m {
		where := gofish(m[i])
		if where != "" {
			mentions = append(mentions, where)
		}
	}
	m = re_urltions.FindAllString(s, -1)
	for i := range m {
		mentions = append(mentions, m[i][1:])
	}
	return mentions
}

func bunchofgrapes(s string) []Mention {
	m := re_mentions.FindAllString(s, -1)
	var mentions []Mention
	for i := range m {
		where := gofish(m[i])
		if where != "" {
			mentions = append(mentions, Mention{who: m[i], where: where})
		}
	}
	m = re_urltions.FindAllString(s, -1)
	for i := range m {
		mentions = append(mentions, Mention{who: m[i][1:], where: m[i][1:]})
	}
	return mentions
}

type Emu struct {
	ID   string
	Name string
}

var re_link = regexp.MustCompile(`@?https?://[^\s"]+[\w/)]`)
var re_emus = regexp.MustCompile(`:[[:alnum:]_-]+:`)

func herdofemus(noise string) []Emu {
	m := re_emus.FindAllString(noise, -1)
	m = oneofakind(m)
	var emus []Emu
	for _, e := range m {
		fname := e[1 : len(e)-1]
		_, err := os.Stat("emus/" + fname + ".png")
		if err != nil {
			continue
		}
		url := fmt.Sprintf("https://%s/emu/%s.png", serverName, fname)
		emus = append(emus, Emu{ID: url, Name: e})
	}
	return emus
}

var re_memes = regexp.MustCompile("meme: ?([[:alnum:]_.-]+)")

func memetize(honk *Honk) {
	repl := func(x string) string {
		name := x[5:]
		if name[0] == ' ' {
			name = name[1:]
		}
		fd, err := os.Open("memes/" + name)
		if err != nil {
			log.Printf("no meme for %s", name)
			return x
		}
		var peek [512]byte
		n, _ := fd.Read(peek[:])
		ct := http.DetectContentType(peek[:n])
		fd.Close()

		url := fmt.Sprintf("https://%s/meme/%s", serverName, name)
		res, err := stmtSaveFile.Exec("", name, url, ct, 0, "")
		if err != nil {
			log.Printf("error saving meme: %s", err)
			return x
		}
		var d Donk
		d.FileID, _ = res.LastInsertId()
		d.XID = ""
		d.Name = name
		d.Media = ct
		d.URL = url
		d.Local = false
		honk.Donks = append(honk.Donks, &d)
		log.Printf("replace with -")
		return ""
	}
	honk.Noise = re_memes.ReplaceAllStringFunc(honk.Noise, repl)
}

var re_bolder = regexp.MustCompile(`(^|\W)\*\*([\w\s,.!?'-]+)\*\*($|\W)`)
var re_italicer = regexp.MustCompile(`(^|\W)\*([\w\s,.!?'-]+)\*($|\W)`)
var re_bigcoder = regexp.MustCompile("```\n?((?s:.*?))\n?```\n?")
var re_coder = regexp.MustCompile("`([^`]*)`")
var re_quoter = regexp.MustCompile(`(?m:^&gt; (.*)\n?)`)

func markitzero(s string) string {
	var bigcodes []string
	bigsaver := func(code string) string {
		bigcodes = append(bigcodes, code)
		return "``````"
	}
	s = re_bigcoder.ReplaceAllStringFunc(s, bigsaver)
	var lilcodes []string
	lilsaver := func(code string) string {
		lilcodes = append(lilcodes, code)
		return "`x`"
	}
	s = re_coder.ReplaceAllStringFunc(s, lilsaver)
	s = re_bolder.ReplaceAllString(s, "$1<b>$2</b>$3")
	s = re_italicer.ReplaceAllString(s, "$1<i>$2</i>$3")
	s = re_quoter.ReplaceAllString(s, "<blockquote>$1</blockquote><p>")
	lilun := func(s string) string {
		code := lilcodes[0]
		lilcodes = lilcodes[1:]
		return code
	}
	s = re_coder.ReplaceAllStringFunc(s, lilun)
	bigun := func(s string) string {
		code := bigcodes[0]
		bigcodes = bigcodes[1:]
		return code
	}
	s = re_bigcoder.ReplaceAllStringFunc(s, bigun)
	s = re_bigcoder.ReplaceAllString(s, "<pre><code>$1</code></pre><p>")
	s = re_coder.ReplaceAllString(s, "<code>$1</code>")
	return s
}

func obfusbreak(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Replace(s, "\r", "", -1)
	s = html.EscapeString(s)
	// dammit go
	s = strings.Replace(s, "&#39;", "'", -1)
	linkfn := func(url string) string {
		if url[0] == '@' {
			return url
		}
		addparen := false
		adddot := false
		if strings.HasSuffix(url, ")") && strings.IndexByte(url, '(') == -1 {
			url = url[:len(url)-1]
			addparen = true
		}
		if strings.HasSuffix(url, ".") {
			url = url[:len(url)-1]
			adddot = true
		}
		url = fmt.Sprintf(`<a class="mention u-url" href="%s">%s</a>`, url, url)
		if adddot {
			url += "."
		}
		if addparen {
			url += ")"
		}
		return url
	}
	s = re_link.ReplaceAllStringFunc(s, linkfn)

	s = markitzero(s)

	s = strings.Replace(s, "\n", "<br>", -1)
	return s
}

func mentionize(s string) string {
	s = re_mentions.ReplaceAllStringFunc(s, func(m string) string {
		where := gofish(m)
		if where == "" {
			return m
		}
		who := m[0 : 1+strings.IndexByte(m[1:], '@')]
		return fmt.Sprintf(`<span class="h-card"><a class="u-url mention" href="%s">%s</a></span>`,
			html.EscapeString(where), html.EscapeString(who))
	})
	s = re_urltions.ReplaceAllStringFunc(s, func(m string) string {
		return fmt.Sprintf(`<span class="h-card"><a class="u-url mention" href="%s">%s</a></span>`,
			html.EscapeString(m[1:]), html.EscapeString(m))
	})
	return s
}

var re_unurl = regexp.MustCompile("https://([^/]+).*/([^/]+)")
var re_urlhost = regexp.MustCompile("https://([^/]+)")

func originate(u string) string {
	m := re_urlhost.FindStringSubmatch(u)
	if len(m) > 1 {
		return m[1]
	}
	return ""
}

func honkerhandle(h string) (string, string) {
	m := re_unurl.FindStringSubmatch(h)
	if len(m) > 2 {
		return m[2], fmt.Sprintf("%s@%s", m[2], m[1])
	}
	return h, h
}

func prepend(s string, x []string) []string {
	return append([]string{s}, x...)
}

// pleroma leaks followers addressed posts to followers
func butnottooloud(aud []string) {
	for i, a := range aud {
		if strings.HasSuffix(a, "/followers") {
			aud[i] = ""
		}
	}
}

func keepitquiet(aud []string) bool {
	for _, a := range aud {
		if a == thewholeworld {
			return false
		}
	}
	return true
}

func oneofakind(a []string) []string {
	var x []string
	for n, s := range a {
		if s != "" {
			x = append(x, s)
			for i := n + 1; i < len(a); i++ {
				if a[i] == s {
					a[i] = ""
				}
			}
		}
	}
	return x
}

var ziggies = make(map[string]*rsa.PrivateKey)
var zaggies = make(map[string]*rsa.PublicKey)
var ziggylock sync.Mutex

func ziggy(username string) (keyname string, key *rsa.PrivateKey) {
	ziggylock.Lock()
	key = ziggies[username]
	ziggylock.Unlock()
	if key == nil {
		db := opendatabase()
		row := db.QueryRow("select seckey from users where username = ?", username)
		var data string
		row.Scan(&data)
		var err error
		key, _, err = httpsig.DecodeKey(data)
		if err != nil {
			log.Printf("error decoding %s seckey: %s", username, err)
			return
		}
		ziggylock.Lock()
		ziggies[username] = key
		ziggylock.Unlock()
	}
	keyname = fmt.Sprintf("https://%s/%s/%s#key", serverName, userSep, username)
	return
}

func zaggy(keyname string) (key *rsa.PublicKey) {
	ziggylock.Lock()
	key = zaggies[keyname]
	ziggylock.Unlock()
	if key != nil {
		return
	}
	row := stmtGetXonker.QueryRow(keyname, "pubkey")
	var data string
	err := row.Scan(&data)
	if err != nil {
		log.Printf("hitting the webs for missing pubkey: %s", keyname)
		j, err := GetJunk(keyname)
		if err != nil {
			log.Printf("error getting %s pubkey: %s", keyname, err)
			return
		}
		keyobj, ok := j.GetMap("publicKey")
		if ok {
			j = keyobj
		}
		data, ok = j.GetString("publicKeyPem")
		if !ok {
			log.Printf("error finding %s pubkey", keyname)
			return
		}
		_, ok = j.GetString("owner")
		if !ok {
			log.Printf("error finding %s pubkey owner", keyname)
			return
		}
		_, key, err = httpsig.DecodeKey(data)
		if err != nil {
			log.Printf("error decoding %s pubkey: %s", keyname, err)
			return
		}
		_, err = stmtSaveXonker.Exec(keyname, data, "pubkey")
		if err != nil {
			log.Printf("error saving key: %s", err)
		}
	} else {
		_, key, err = httpsig.DecodeKey(data)
		if err != nil {
			log.Printf("error decoding %s pubkey: %s", keyname, err)
			return
		}
	}
	ziggylock.Lock()
	zaggies[keyname] = key
	ziggylock.Unlock()
	return
}

func makeitworksomehowwithoutregardforkeycontinuity(keyname string, r *http.Request, payload []byte) (string, error) {
	_, err := stmtDeleteXonker.Exec(keyname, "pubkey")
	if err != nil {
		log.Printf("error deleting key: %s", err)
	}
	ziggylock.Lock()
	delete(zaggies, keyname)
	ziggylock.Unlock()
	return httpsig.VerifyRequest(r, payload, zaggy)
}

var thumbbiters map[int64]map[string]bool
var zordses map[int64][]*regexp.Regexp
var zilences map[int64][]*regexp.Regexp
var thumblock sync.Mutex

func bitethethumbs() {
	rows, err := stmtThumbBiters.Query()
	if err != nil {
		log.Printf("error getting thumbbiters: %s", err)
		return
	}
	defer rows.Close()

	thumblock.Lock()
	defer thumblock.Unlock()
	thumbbiters = make(map[int64]map[string]bool)
	zordses = make(map[int64][]*regexp.Regexp)
	zilences = make(map[int64][]*regexp.Regexp)
	for rows.Next() {
		var userid int64
		var name, wherefore string
		err = rows.Scan(&userid, &name, &wherefore)
		if err != nil {
			log.Printf("error scanning zonker: %s", err)
			continue
		}
		if wherefore == "zord" || wherefore == "zilence" {
			zord := "\\b(?i:" + name + ")\\b"
			re, err := regexp.Compile(zord)
			if err != nil {
				log.Printf("error compiling zord: %s", err)
			} else {
				if wherefore == "zord" {
					zordses[userid] = append(zordses[userid], re)
				} else {
					zilences[userid] = append(zilences[userid], re)
				}
			}
			continue
		}
		m := thumbbiters[userid]
		if m == nil {
			m = make(map[string]bool)
			thumbbiters[userid] = m
		}
		m[name] = true
	}
}

func getzords(userid int64) []*regexp.Regexp {
	thumblock.Lock()
	defer thumblock.Unlock()
	return zordses[userid]
}

func getzilences(userid int64) []*regexp.Regexp {
	thumblock.Lock()
	defer thumblock.Unlock()
	return zilences[userid]
}

func thoudostbitethythumb(userid int64, who []string, objid string) bool {
	thumblock.Lock()
	biters := thumbbiters[userid]
	thumblock.Unlock()
	for _, w := range who {
		if biters[w] {
			return true
		}
		where := originate(w)
		if where != "" {
			if biters[where] {
				return true
			}
		}
	}
	return false
}

func keymatch(keyname string, actor string) string {
	hash := strings.IndexByte(keyname, '#')
	if hash == -1 {
		hash = len(keyname)
	}
	owner := keyname[0:hash]
	if owner == actor {
		return originate(actor)
	}
	return ""
}
