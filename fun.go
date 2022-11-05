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
	"crypto/sha512"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"golang.org/x/net/html"
	"humungus.tedunangst.com/r/webs/cache"
	"humungus.tedunangst.com/r/webs/htfilter"
	"humungus.tedunangst.com/r/webs/httpsig"
	"humungus.tedunangst.com/r/webs/mz"
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

var relingo = make(map[string]string)

func loadLingo() {
	for _, l := range []string{"honked", "bonked", "honked back", "qonked", "evented"} {
		v := l
		k := "lingo-" + strings.ReplaceAll(l, " ", "")
		getconfig(k, &v)
		relingo[l] = v
	}
}

func reverbolate(userid int64, honks []*Honk) {
	var user *WhatAbout
	somenumberedusers.Get(userid, &user)
	for _, h := range honks {
		h.What += "ed"
		if h.What == "tonked" {
			h.What = "honked back"
			h.Style += " subtle"
		}
		if !h.Public {
			h.Style += " limited"
		}
		if h.Whofore == 1 {
			h.Style += " atme"
		}
		translate(h)
		local := false
		if h.Whofore == 2 || h.Whofore == 3 {
			local = true
		}
		if local && h.What != "bonked" {
			h.Noise = re_memes.ReplaceAllString(h.Noise, "")
		}
		h.Username, h.Handle = handles(h.Honker)
		if !local {
			short := shortname(userid, h.Honker)
			if short != "" {
				h.Username = short
			} else {
				h.Username = h.Handle
				if len(h.Username) > 20 {
					h.Username = h.Username[:20] + ".."
				}
			}
		}
		if user != nil {
			if user.Options.MentionAll {
				hset := []string{"@" + h.Handle}
				for _, a := range h.Audience {
					if a == h.Honker || a == user.URL {
						continue
					}
					_, hand := handles(a)
					if hand != "" {
						hand = "@" + hand
						hset = append(hset, hand)
					}
				}
				h.Handles = strings.Join(hset, " ")
			} else if h.Honker != user.URL {
				h.Handles = "@" + h.Handle
			}
		}
		if h.URL == "" {
			h.URL = h.XID
		}
		if h.Oonker != "" {
			_, h.Oondle = handles(h.Oonker)
		}
		h.Precis = demoji(h.Precis)
		h.Noise = demoji(h.Noise)
		h.Open = "open"
		for _, m := range h.Mentions {
			if !m.IsPresent(h.Noise) {
				h.Noise = "(" + m.Who + ")" + h.Noise
			}
		}

		zap := make(map[string]bool)
		{
			var htf htfilter.Filter
			htf.Imager = replaceimgsand(zap, false)
			htf.SpanClasses = allowedclasses
			htf.BaseURL, _ = url.Parse(h.XID)
			emuxifier := func(e string) string {
				for _, d := range h.Donks {
					if d.Name == e {
						zap[d.XID] = true
						if d.Local {
							return fmt.Sprintf(`<img class="emu" title="%s" src="/d/%s">`, d.Name, d.XID)
						}
					}
				}
				if local && h.What != "bonked" {
					var emu Emu
					emucache.Get(e, &emu)
					if emu.ID != "" {
						return fmt.Sprintf(`<img class="emu" title="%s" src="%s">`, emu.Name, emu.ID)
					}
				}
				return e
			}
			htf.FilterText = func(w io.Writer, data string) {
				data = htfilter.EscapeText(data)
				data = re_emus.ReplaceAllStringFunc(data, emuxifier)
				io.WriteString(w, data)
			}
			p, _ := htf.String(h.Precis)
			n, _ := htf.String(h.Noise)
			h.Precis = string(p)
			h.Noise = string(n)
		}
		j := 0
		for i := 0; i < len(h.Donks); i++ {
			if !zap[h.Donks[i].XID] {
				h.Donks[j] = h.Donks[i]
				j++
			}
		}
		h.Donks = h.Donks[:j]
	}

	unsee(honks, userid)

	for _, h := range honks {
		renderflags(h)

		h.HTPrecis = template.HTML(h.Precis)
		h.HTML = template.HTML(h.Noise)
		if h.What == "wonked" {
			h.HTML = "? wonk ?"
		}
		if redo := relingo[h.What]; redo != "" {
			h.What = redo
		}
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
		return string(templates.Sprintf(`&lt;img alt="%s" src="<a href="%s">%s</a>"&gt;`, alt, src, src))
	}
}

func translatechonk(ch *Chonk) {
	noise := ch.Noise
	if ch.Format == "markdown" {
		noise = markitzero(noise)
	}
	var htf htfilter.Filter
	htf.SpanClasses = allowedclasses
	htf.BaseURL, _ = url.Parse(ch.XID)
	ch.HTML, _ = htf.String(noise)
}

func filterchonk(ch *Chonk) {
	translatechonk(ch)

	noise := string(ch.HTML)

	local := originate(ch.XID) == serverName

	zap := make(map[string]bool)
	emuxifier := func(e string) string {
		for _, d := range ch.Donks {
			if d.Name == e {
				zap[d.XID] = true
				if d.Local {
					return fmt.Sprintf(`<img class="emu" title="%s" src="/d/%s">`, d.Name, d.XID)
				}
			}
		}
		if local {
			var emu Emu
			emucache.Get(e, &emu)
			if emu.ID != "" {
				return fmt.Sprintf(`<img class="emu" title="%s" src="%s">`, emu.Name, emu.ID)
			}
		}
		return e
	}
	noise = re_emus.ReplaceAllStringFunc(noise, emuxifier)
	j := 0
	for i := 0; i < len(ch.Donks); i++ {
		if !zap[ch.Donks[i].XID] {
			ch.Donks[j] = ch.Donks[i]
			j++
		}
	}
	ch.Donks = ch.Donks[:j]

	if strings.HasPrefix(noise, "<p>") {
		noise = noise[3:]
	}
	ch.HTML = template.HTML(noise)
	if short := shortname(ch.UserID, ch.Who); short != "" {
		ch.Handle = short
	} else {
		ch.Handle, _ = handles(ch.Who)
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
		dlog.Printf("inline img with src: %s", src)
		return ""
	}
}

func imaginate(honk *Honk) {
	var htf htfilter.Filter
	htf.Imager = inlineimgsfor(honk)
	htf.BaseURL, _ = url.Parse(honk.XID)
	htf.String(honk.Noise)
}

func translate(honk *Honk) {
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

	var marker mz.Marker
	marker.HashLinker = ontoreplacer
	marker.AtLinker = attoreplacer
	noise = strings.TrimSpace(noise)
	noise = marker.Mark(noise)
	honk.Noise = noise
	honk.Onts = oneofakind(marker.HashTags)
	honk.Mentions = bunchofgrapes(marker.Mentions)
}

func redoimages(honk *Honk) {
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
	honk.Noise = strings.Replace(honk.Noise, "<a href=", "<a class=\"mention u-url\" href=", -1)
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

func grapevine(mentions []Mention) []string {
	var s []string
	for _, m := range mentions {
		s = append(s, m.Where)
	}
	return s
}

func bunchofgrapes(m []string) []Mention {
	var mentions []Mention
	for i := range m {
		where := gofish(m[i])
		if where != "" {
			mentions = append(mentions, Mention{Who: m[i], Where: where})
		}
	}
	return mentions
}

type Emu struct {
	ID   string
	Name string
	Type string
}

var re_emus = regexp.MustCompile(`:[[:alnum:]_-]+:`)

var emucache = cache.New(cache.Options{Filler: func(ename string) (Emu, bool) {
	fname := ename[1 : len(ename)-1]
	exts := []string{".png", ".gif"}
	for _, ext := range exts {
		_, err := os.Stat(dataDir + "/emus/" + fname + ext)
		if err != nil {
			continue
		}
		url := fmt.Sprintf("https://%s/emu/%s%s", serverName, fname, ext)
		return Emu{ID: url, Name: ename, Type: "image/" + ext[1:]}, true
	}
	return Emu{Name: ename, ID: "", Type: "image/png"}, true
}, Duration: 10 * time.Second})

func herdofemus(noise string) []Emu {
	m := re_emus.FindAllString(noise, -1)
	m = oneofakind(m)
	var emus []Emu
	for _, e := range m {
		var emu Emu
		emucache.Get(e, &emu)
		if emu.ID == "" {
			continue
		}
		emus = append(emus, emu)
	}
	return emus
}

var re_memes = regexp.MustCompile("meme: ?([^\n]+)")
var re_avatar = regexp.MustCompile("avatar: ?([^\n]+)")
var re_banner = regexp.MustCompile("banner: ?([^\n]+)")

func memetize(honk *Honk) {
	repl := func(x string) string {
		name := x[5:]
		if name[0] == ' ' {
			name = name[1:]
		}
		fd, err := os.Open(dataDir + "/memes/" + name)
		if err != nil {
			ilog.Printf("no meme for %s", name)
			return x
		}
		var peek [512]byte
		n, _ := fd.Read(peek[:])
		ct := http.DetectContentType(peek[:n])
		fd.Close()

		url := fmt.Sprintf("https://%s/meme/%s", serverName, name)
		fileid, err := savefile(name, name, url, ct, false, nil)
		if err != nil {
			elog.Printf("error saving meme: %s", err)
			return x
		}
		d := &Donk{
			FileID: fileid,
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

var re_quickmention = regexp.MustCompile("(^|[ \n])@[[:alnum:]]+([ \n.]|$)")

func quickrename(s string, userid int64) string {
	nonstop := true
	for nonstop {
		nonstop = false
		s = re_quickmention.ReplaceAllStringFunc(s, func(m string) string {
			prefix := ""
			if m[0] == ' ' || m[0] == '\n' {
				prefix = m[:1]
				m = m[1:]
			}
			prefix += "@"
			m = m[1:]
			tail := ""
			if last := m[len(m)-1]; last == ' ' || last == '\n' || last == '.' {
				tail = m[len(m)-1:]
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
			return prefix + m + tail
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

func attoreplacer(m string) string {
	fill := `<span class="h-card"><a class="u-url mention" href="%s">%s</a></span>`
	where := gofish(m)
	if where == "" {
		return m
	}
	who := m[0 : 1+strings.IndexByte(m[1:], '@')]
	return fmt.Sprintf(fill, html.EscapeString(where), html.EscapeString(who))
}

func ontoreplacer(h string) string {
	return fmt.Sprintf(`<a href="https://%s/o/%s">%s</a>`, serverName,
		strings.ToLower(h[1:]), h)
}

var re_unurl = regexp.MustCompile("https://([^/]+).*/([^/]+)")
var re_urlhost = regexp.MustCompile("https://([^/ #)]+)")

func originate(u string) string {
	m := re_urlhost.FindStringSubmatch(u)
	if len(m) > 1 {
		return m[1]
	}
	return ""
}

var allhandles = cache.New(cache.Options{Filler: func(xid string) (string, bool) {
	handle := getxonker(xid, "handle")
	if handle == "" {
		dlog.Printf("need to get a handle: %s", xid)
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
		}
	}
	return handle, true
}})

// handle, handle@host
func handles(xid string) (string, string) {
	if xid == "" || xid == thewholeworld || strings.HasSuffix(xid, "/followers") {
		return "", ""
	}
	var handle string
	allhandles.Get(xid, &handle)
	if handle == xid {
		return xid, xid
	}
	return handle, handle + "@" + originate(xid)
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

var zaggies = cache.New(cache.Options{Filler: func(keyname string) (httpsig.PublicKey, bool) {
	data := getxonker(keyname, "pubkey")
	if data == "" {
		dlog.Printf("hitting the webs for missing pubkey: %s", keyname)
		j, err := GetJunk(serverUID, keyname)
		if err != nil {
			ilog.Printf("error getting %s pubkey: %s", keyname, err)
			when := time.Now().UTC().Format(dbtimeformat)
			stmtSaveXonker.Exec(keyname, "failed", "pubkey", when)
			return httpsig.PublicKey{}, true
		}
		allinjest(originate(keyname), j)
		data = getxonker(keyname, "pubkey")
		if data == "" {
			ilog.Printf("key not found after ingesting")
			when := time.Now().UTC().Format(dbtimeformat)
			stmtSaveXonker.Exec(keyname, "failed", "pubkey", when)
			return httpsig.PublicKey{}, true
		}
	}
	if data == "failed" {
		ilog.Printf("lookup previously failed key %s", keyname)
		return httpsig.PublicKey{}, true
	}
	_, key, err := httpsig.DecodeKey(data)
	if err != nil {
		ilog.Printf("error decoding %s pubkey: %s", keyname, err)
		return key, true
	}
	return key, true
}, Limit: 512})

func zaggy(keyname string) (httpsig.PublicKey, error) {
	var key httpsig.PublicKey
	zaggies.Get(keyname, &key)
	return key, nil
}

func savingthrow(keyname string) {
	when := time.Now().Add(-30 * time.Minute).UTC().Format(dbtimeformat)
	stmtDeleteXonker.Exec(keyname, "pubkey", when)
	zaggies.Clear(keyname)
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
