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
	"crypto/sha512"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"

	"golang.org/x/net/html"
	"humungus.tedunangst.com/r/webs/cache"
	"humungus.tedunangst.com/r/webs/htfilter"
	"humungus.tedunangst.com/r/webs/httpsig"
	"humungus.tedunangst.com/r/webs/templates"
)

var allowedclasses = make(map[string]bool)

func init() {
	allowedclasses["kw"] = true
	allowedclasses["bi"] = true
	allowedclasses["st"] = true
	allowedclasses["nm"] = true
	allowedclasses["tp"] = true
	allowedclasses["op"] = true
	allowedclasses["cm"] = true
	allowedclasses["al"] = true
	allowedclasses["dl"] = true
}

func reverbolate(userid int64, honks []*Honk) {
	for _, h := range honks {
		h.What += "ed"
		if h.What == "tonked" {
			h.What = "honked back"
			h.Style += " subtle"
		}
		if !h.Public {
			h.Style += " limited"
		}
		translate(h, false)
		if h.Whofore == 2 || h.Whofore == 3 {
			h.URL = h.XID
			if h.What != "bonked" {
				h.Noise = re_memes.ReplaceAllString(h.Noise, "")
				h.Noise = mentionize(h.Noise)
				h.Noise = ontologize(h.Noise)
			}
			h.Username, h.Handle = handles(h.Honker)
		} else {
			_, h.Handle = handles(h.Honker)
			short := shortname(userid, h.Honker)
			if short != "" {
				h.Username = short
			} else {
				h.Username = h.Handle
				if len(h.Username) > 20 {
					h.Username = h.Username[:20] + ".."
				}
			}
			if h.URL == "" {
				h.URL = h.XID
			}
		}
		if h.Oonker != "" {
			_, h.Oondle = handles(h.Oonker)
		}
		h.Precis = demoji(h.Precis)
		h.Noise = demoji(h.Noise)
		h.Open = "open"

		zap := make(map[string]bool)
		{
			var htf htfilter.Filter
			htf.Imager = replaceimgsand(zap, false)
			htf.SpanClasses = allowedclasses
			p, _ := htf.String(h.Precis)
			n, _ := htf.String(h.Noise)
			h.Precis = string(p)
			h.Noise = string(n)
		}

		if userid == -1 {
			if h.Precis != "" {
				h.Open = ""
			}
		} else {
			unsee(userid, h)
			if h.Open == "open" && h.Precis == "unspecified horror" {
				h.Precis = ""
			}
		}
		if len(h.Noise) > 6000 && h.Open == "open" {
			if h.Precis == "" {
				h.Precis = "really freaking long"
			}
			h.Open = ""
		}

		emuxifier := func(e string) string {
			for _, d := range h.Donks {
				if d.Name == e {
					zap[d.XID] = true
					if d.Local {
						return fmt.Sprintf(`<img class="emu" title="%s" src="/d/%s">`, d.Name, d.XID)
					}
				}
			}
			return e
		}
		h.Precis = re_emus.ReplaceAllStringFunc(h.Precis, emuxifier)
		h.Noise = re_emus.ReplaceAllStringFunc(h.Noise, emuxifier)

		j := 0
		for i := 0; i < len(h.Donks); i++ {
			if !zap[h.Donks[i].XID] {
				h.Donks[j] = h.Donks[i]
				j++
			}
		}
		h.Donks = h.Donks[:j]

		h.HTPrecis = template.HTML(h.Precis)
		h.HTML = template.HTML(h.Noise)
	}
}

func replaceimgsand(zap map[string]bool, absolute bool) func(node *html.Node) string {
	return func(node *html.Node) string {
		src := htfilter.GetAttr(node, "src")
		alt := htfilter.GetAttr(node, "alt")
		//title := GetAttr(node, "title")
		if htfilter.HasClass(node, "Emoji") && alt != "" {
			return alt
		}
		d := finddonk(src)
		if d != nil {
			zap[d.XID] = true
			base := ""
			if absolute {
				base = "https://" + serverName
			}
			return string(templates.Sprintf(`<img alt="%s" title="%s" src="%s/d/%s">`, alt, alt, base, d.XID))
		}
		return string(templates.Sprintf(`&lt;img alt="%s" src="<a href="%s">%s<a>"&gt;`, alt, src, src))
	}
}

func inlineimgsfor(honk *Honk) func(node *html.Node) string {
	return func(node *html.Node) string {
		src := htfilter.GetAttr(node, "src")
		alt := htfilter.GetAttr(node, "alt")
		d := savedonk(src, "image", alt, "image", true)
		if d != nil {
			honk.Donks = append(honk.Donks, d)
		}
		log.Printf("inline img with src: %s", src)
		return ""
	}
}

func imaginate(honk *Honk) {
	var htf htfilter.Filter
	htf.Imager = inlineimgsfor(honk)
	htf.String(honk.Noise)
}

func translate(honk *Honk, redoimages bool) {
	if honk.Format == "html" {
		return
	}
	noise := honk.Noise
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
	honk.Precis = markitzero(strings.TrimSpace(honk.Precis))

	noise = strings.TrimSpace(noise)
	noise = quickrename(noise, honk.UserID)
	noise = markitzero(noise)
	honk.Noise = noise
	honk.Onts = oneofakind(ontologies(honk.Noise))

	if redoimages {
		zap := make(map[string]bool)
		{
			var htf htfilter.Filter
			htf.Imager = replaceimgsand(zap, true)
			htf.SpanClasses = allowedclasses
			p, _ := htf.String(honk.Precis)
			n, _ := htf.String(honk.Noise)
			honk.Precis = string(p)
			honk.Noise = string(n)
		}
		j := 0
		for i := 0; i < len(honk.Donks); i++ {
			if !zap[honk.Donks[i].XID] {
				honk.Donks[j] = honk.Donks[i]
				j++
			}
		}
		honk.Donks = honk.Donks[:j]

		honk.Noise = re_memes.ReplaceAllString(honk.Noise, "")
		honk.Noise = ontologize(mentionize(honk.Noise))
		honk.Noise = strings.Replace(honk.Noise, "<a href=", "<a class=\"mention u-url\" href=", -1)
	}
}

func xcelerate(b []byte) string {
	letters := "BCDFGHJKLMNPQRSTVWXYZbcdfghjklmnpqrstvwxyz1234567891234567891234"
	for i, c := range b {
		b[i] = letters[c&63]
	}
	s := string(b)
	return s
}

func shortxid(xid string) string {
	h := sha512.New512_256()
	io.WriteString(h, xid)
	return xcelerate(h.Sum(nil)[:20])
}

func xfiltrate() string {
	var b [18]byte
	rand.Read(b[:])
	return xcelerate(b[:])
}

var re_hashes = regexp.MustCompile(`(?:^| )#[[:alnum:]][[:alnum:]_-]*`)

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
		fileid, err := savefile("", name, name, url, ct, false, nil)
		if err != nil {
			log.Printf("error saving meme: %s", err)
			return x
		}
		d := &Donk{
			FileID: fileid,
			XID:    "",
			Name:   name,
			Media:  ct,
			URL:    url,
			Local:  false,
		}
		honk.Donks = append(honk.Donks, d)
		return ""
	}
	honk.Noise = re_memes.ReplaceAllStringFunc(honk.Noise, repl)
}

var re_quickmention = regexp.MustCompile("(^| )@[[:alnum:]]+( |$)")

func quickrename(s string, userid int64) string {
	nonstop := true
	for nonstop {
		nonstop = false
		s = re_quickmention.ReplaceAllStringFunc(s, func(m string) string {
			prefix := ""
			if m[0] == ' ' {
				prefix = " "
				m = m[1:]
			}
			prefix += "@"
			m = m[1:]
			if m[len(m)-1] == ' ' {
				m = m[:len(m)-1]
			}

			xid := fullname(m, userid)

			if xid != "" {
				_, name := handles(xid)
				if name != "" {
					nonstop = true
					m = name
				}
			}
			return prefix + m + " "
		})
	}
	return s
}

var shortnames = cache.New(cache.Options{Filler: func(userid int64) (map[string]string, bool) {
	honkers := gethonkers(userid)
	m := make(map[string]string)
	for _, h := range honkers {
		m[h.XID] = h.Name
	}
	return m, true
}, Invalidator: &honkerinvalidator})

func shortname(userid int64, xid string) string {
	var m map[string]string
	ok := shortnames.Get(userid, &m)
	if ok {
		return m[xid]
	}
	return ""
}

var fullnames = cache.New(cache.Options{Filler: func(userid int64) (map[string]string, bool) {
	honkers := gethonkers(userid)
	m := make(map[string]string)
	for _, h := range honkers {
		m[h.Name] = h.XID
	}
	return m, true
}, Invalidator: &honkerinvalidator})

func fullname(name string, userid int64) string {
	var m map[string]string
	ok := fullnames.Get(userid, &m)
	if ok {
		return m[name]
	}
	return ""
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

func ontologize(s string) string {
	s = re_hashes.ReplaceAllStringFunc(s, func(o string) string {
		if o[0] == '&' {
			return o
		}
		p := ""
		h := o
		if h[0] != '#' {
			p = h[:1]
			h = h[1:]
		}
		return fmt.Sprintf(`%s<a href="https://%s/o/%s">%s</a>`, p, serverName,
			strings.ToLower(h[1:]), h)
	})
	return s
}

var re_unurl = regexp.MustCompile("https://([^/]+).*/([^/]+)")
var re_urlhost = regexp.MustCompile("https://([^/ ]+)")

func originate(u string) string {
	m := re_urlhost.FindStringSubmatch(u)
	if len(m) > 1 {
		return m[1]
	}
	return ""
}

var allhandles = cache.New(cache.Options{Filler: func(xid string) (string, bool) {
	row := stmtGetXonker.QueryRow(xid, "handle")
	var handle string
	err := row.Scan(&handle)
	if err != nil {
		info, err := investigate(xid)
		if err != nil {
			m := re_unurl.FindStringSubmatch(xid)
			if len(m) > 2 {
				handle = m[2]
			} else {
				handle = xid
			}
		} else {
			handle = info.Name
			_, err = stmtSaveXonker.Exec(xid, handle, "handle")
			if err != nil {
				log.Printf("error saving handle: %s", err)
			}
		}
	}
	return handle, true
}})

// handle, handle@host
func handles(xid string) (string, string) {
	if xid == "" {
		return "", ""
	}
	var handle string
	allhandles.Get(xid, &handle)
	if handle == xid {
		return xid, xid
	}
	return handle, handle + "@" + originate(xid)
}

func prepend(s string, x []string) []string {
	return append([]string{s}, x...)
}

func butnottooloud(aud []string) {
	for i, a := range aud {
		if strings.HasSuffix(a, "/followers") {
			aud[i] = ""
		}
	}
}

func loudandproud(aud []string) bool {
	for _, a := range aud {
		if a == thewholeworld {
			return true
		}
	}
	return false
}

func firstclass(honk *Honk) bool {
	return honk.Audience[0] == thewholeworld
}

func oneofakind(a []string) []string {
	seen := make(map[string]bool)
	seen[""] = true
	j := 0
	for _, s := range a {
		if !seen[s] {
			seen[s] = true
			a[j] = s
			j++
		}
	}
	return a[:j]
}

var ziggies = cache.New(cache.Options{Filler: func(userid int64) (*KeyInfo, bool) {
	var user *WhatAbout
	ok := somenumberedusers.Get(userid, &user)
	if !ok {
		return nil, false
	}
	ki := new(KeyInfo)
	ki.keyname = user.URL + "#key"
	ki.seckey = user.SecKey
	return ki, true
}})

func ziggy(userid int64) *KeyInfo {
	var ki *KeyInfo
	ziggies.Get(userid, &ki)
	return ki
}

var zaggies = cache.New(cache.Options{Filler: func(keyname string) (*rsa.PublicKey, bool) {
	row := stmtGetXonker.QueryRow(keyname, "pubkey")
	var data string
	err := row.Scan(&data)
	if err == nil {
		_, key, err := httpsig.DecodeKey(data)
		if err != nil {
			log.Printf("error decoding %s pubkey: %s", keyname, err)
		}
		return key, true
	}
	log.Printf("hitting the webs for missing pubkey: %s", keyname)
	j, err := GetJunk(keyname)
	if err != nil {
		log.Printf("error getting %s pubkey: %s", keyname, err)
		return nil, true
	}
	keyobj, ok := j.GetMap("publicKey")
	if ok {
		j = keyobj
	}
	data, ok = j.GetString("publicKeyPem")
	if !ok {
		log.Printf("error finding %s pubkey", keyname)
		return nil, true
	}
	_, ok = j.GetString("owner")
	if !ok {
		log.Printf("error finding %s pubkey owner", keyname)
		return nil, true
	}
	_, key, err := httpsig.DecodeKey(data)
	if err != nil {
		log.Printf("error decoding %s pubkey: %s", keyname, err)
		return nil, true
	}
	_, err = stmtSaveXonker.Exec(keyname, data, "pubkey")
	if err != nil {
		log.Printf("error saving key: %s", err)
	}
	return key, true
}})

func zaggy(keyname string) *rsa.PublicKey {
	var key *rsa.PublicKey
	zaggies.Get(keyname, &key)
	return key
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
