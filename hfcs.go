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
	"net/http"
	"regexp"
	"sort"
	"time"

	"humungus.tedunangst.com/r/webs/cache"
)

type Filter struct {
	ID              int64      `json:"-"`
	Actions         []filtType `json:"-"`
	Name            string
	Date            time.Time
	Actor           string `json:",omitempty"`
	IncludeAudience bool   `json:",omitempty"`
	Text            string `json:",omitempty"`
	re_text         *regexp.Regexp
	IsAnnounce      bool   `json:",omitempty"`
	AnnounceOf      string `json:",omitempty"`
	Reject          bool   `json:",omitempty"`
	SkipMedia       bool   `json:",omitempty"`
	Hide            bool   `json:",omitempty"`
	Collapse        bool   `json:",omitempty"`
	Rewrite         string `json:",omitempty"`
	re_rewrite      *regexp.Regexp
	Replace         string `json:",omitempty"`
	Expiration      time.Time
	Notes           string
}

type filtType uint

const (
	filtNone filtType = iota
	filtAny
	filtReject
	filtSkipMedia
	filtHide
	filtCollapse
	filtRewrite
)

var filtNames = []string{"None", "Any", "Reject", "SkipMedia", "Hide", "Collapse", "Rewrite"}

func (ft filtType) String() string {
	return filtNames[ft]
}

type afiltermap map[filtType][]*Filter

var filtInvalidator cache.Invalidator
var filtcache *cache.Cache

func init() {
	// resolve init loop
	filtcache = cache.New(cache.Options{Filler: filtcachefiller, Invalidator: &filtInvalidator})
}

func filtcachefiller(userid int64) (afiltermap, bool) {
	rows, err := stmtGetFilters.Query(userid)
	if err != nil {
		elog.Printf("error querying filters: %s", err)
		return nil, false
	}
	defer rows.Close()

	now := time.Now()

	var expflush time.Time

	filtmap := make(afiltermap)
	for rows.Next() {
		filt := new(Filter)
		var j string
		var filterid int64
		err = rows.Scan(&filterid, &j)
		if err == nil {
			err = unjsonify(j, filt)
		}
		if err != nil {
			elog.Printf("error scanning filter: %s", err)
			continue
		}
		if !filt.Expiration.IsZero() {
			if filt.Expiration.Before(now) {
				continue
			}
			if expflush.IsZero() || filt.Expiration.Before(expflush) {
				expflush = filt.Expiration
			}
		}
		if t := filt.Text; t != "" {
			wordfront := t[0] != '#'
			wordtail := true
			t = "(?i:" + t + ")"
			if wordfront {
				t = "\\b" + t
			}
			if wordtail {
				t = t + "\\b"
			}
			filt.re_text, err = regexp.Compile(t)
			if err != nil {
				elog.Printf("error compiling filter text: %s", err)
				continue
			}
		}
		if t := filt.Rewrite; t != "" {
			wordfront := t[0] != '#'
			wordtail := true
			t = "(?i:" + t + ")"
			if wordfront {
				t = "\\b" + t
			}
			if wordtail {
				t = t + "\\b"
			}
			filt.re_rewrite, err = regexp.Compile(t)
			if err != nil {
				elog.Printf("error compiling filter rewrite: %s", err)
				continue
			}
		}
		filt.ID = filterid
		if filt.Reject {
			filt.Actions = append(filt.Actions, filtReject)
			filtmap[filtReject] = append(filtmap[filtReject], filt)
		}
		if filt.SkipMedia {
			filt.Actions = append(filt.Actions, filtSkipMedia)
			filtmap[filtSkipMedia] = append(filtmap[filtSkipMedia], filt)
		}
		if filt.Hide {
			filt.Actions = append(filt.Actions, filtHide)
			filtmap[filtHide] = append(filtmap[filtHide], filt)
		}
		if filt.Collapse {
			filt.Actions = append(filt.Actions, filtCollapse)
			filtmap[filtCollapse] = append(filtmap[filtCollapse], filt)
		}
		if filt.Rewrite != "" {
			filt.Actions = append(filt.Actions, filtRewrite)
			filtmap[filtRewrite] = append(filtmap[filtRewrite], filt)
		}
		filtmap[filtAny] = append(filtmap[filtAny], filt)
	}
	sorting := filtmap[filtAny]
	sort.Slice(filtmap[filtAny], func(i, j int) bool {
		return sorting[i].Name < sorting[j].Name
	})
	if !expflush.IsZero() {
		dur := expflush.Sub(now)
		go filtcacheclear(userid, dur)
	}
	return filtmap, true
}

func filtcacheclear(userid int64, dur time.Duration) {
	time.Sleep(dur + time.Second)
	filtInvalidator.Clear(userid)
}

func getfilters(userid int64, scope filtType) []*Filter {
	var filtmap afiltermap
	ok := filtcache.Get(userid, &filtmap)
	if ok {
		return filtmap[scope]
	}
	return nil
}

type arejectmap map[string][]*Filter

var rejectAnyKey = "..."

var rejectcache = cache.New(cache.Options{Filler: func(userid int64) (arejectmap, bool) {
	m := make(arejectmap)
	filts := getfilters(userid, filtReject)
	for _, f := range filts {
		if f.Text != "" {
			key := rejectAnyKey
			m[key] = append(m[key], f)
			continue
		}
		if f.IsAnnounce && f.AnnounceOf != "" {
			key := f.AnnounceOf
			m[key] = append(m[key], f)
		}
		if f.Actor != "" {
			key := f.Actor
			m[key] = append(m[key], f)
		}
	}
	return m, true
}, Invalidator: &filtInvalidator})

func rejectfilters(userid int64, name string) []*Filter {
	var m arejectmap
	rejectcache.Get(userid, &m)
	return m[name]
}

func rejectorigin(userid int64, origin string, isannounce bool) bool {
	if o := originate(origin); o != "" {
		origin = o
	}
	filts := rejectfilters(userid, origin)
	for _, f := range filts {
		if isannounce && f.IsAnnounce {
			if f.AnnounceOf == origin {
				return true
			}
		}
		if f.Actor == origin {
			return true
		}
	}
	return false
}

func rejectactor(userid int64, actor string) bool {
	filts := rejectfilters(userid, actor)
	for _, f := range filts {
		if f.IsAnnounce {
			continue
		}
		if f.Actor == actor {
			ilog.Printf("rejecting actor: %s", actor)
			return true
		}
	}
	origin := originate(actor)
	if origin == "" {
		return false
	}
	filts = rejectfilters(userid, origin)
	for _, f := range filts {
		if f.IsAnnounce {
			continue
		}
		if f.Actor == origin {
			ilog.Printf("rejecting actor: %s", actor)
			return true
		}
	}
	return false
}

func stealthmode(userid int64, r *http.Request) bool {
	agent := r.UserAgent()
	agent = originate(agent)
	if agent != "" {
		fake := rejectorigin(userid, agent, false)
		if fake {
			ilog.Printf("faking 404 for %s", agent)
			return true
		}
	}
	return false
}

func matchfilter(h *Honk, f *Filter) bool {
	return matchfilterX(h, f) != ""
}

func matchfilterX(h *Honk, f *Filter) string {
	rv := ""
	match := true
	if match && f.Actor != "" {
		match = false
		if f.Actor == h.Honker || f.Actor == h.Oonker {
			match = true
			rv = f.Actor
		}
		if !match && (f.Actor == originate(h.Honker) ||
			f.Actor == originate(h.Oonker) ||
			f.Actor == originate(h.XID)) {
			match = true
			rv = f.Actor
		}
		if !match && f.IncludeAudience {
			for _, a := range h.Audience {
				if f.Actor == a || f.Actor == originate(a) {
					match = true
					rv = f.Actor
					break
				}
			}
		}
	}
	if match && f.IsAnnounce {
		match = false
		if (f.AnnounceOf == "" && h.Oonker != "") || f.AnnounceOf == h.Oonker ||
			f.AnnounceOf == originate(h.Oonker) {
			match = true
			rv += " announce"
		}
	}
	if match && f.Text != "" {
		match = false
		re := f.re_text
		m := re.FindString(h.Precis)
		if m == "" {
			m = re.FindString(h.Noise)
		}
		if m == "" {
			for _, d := range h.Donks {
				m = re.FindString(d.Desc)
				if m != "" {
					break
				}
			}
		}
		if m != "" {
			match = true
			rv = m
		}
	}
	if match {
		return rv
	}
	return ""
}

func rejectxonk(xonk *Honk) bool {
	var m arejectmap
	rejectcache.Get(xonk.UserID, &m)
	filts := m[rejectAnyKey]
	filts = append(filts, m[xonk.Honker]...)
	filts = append(filts, m[originate(xonk.Honker)]...)
	filts = append(filts, m[xonk.Oonker]...)
	filts = append(filts, m[originate(xonk.Oonker)]...)
	for _, a := range xonk.Audience {
		filts = append(filts, m[a]...)
		filts = append(filts, m[originate(a)]...)
	}
	for _, f := range filts {
		if cause := matchfilterX(xonk, f); cause != "" {
			ilog.Printf("rejecting %s because %s", xonk.XID, cause)
			return true
		}
	}
	return false
}

func skipMedia(xonk *Honk) bool {
	filts := getfilters(xonk.UserID, filtSkipMedia)
	for _, f := range filts {
		if matchfilter(xonk, f) {
			return true
		}
	}
	return false
}

func unsee(honks []*Honk, userid int64) {
	if userid != -1 {
		colfilts := getfilters(userid, filtCollapse)
		rwfilts := getfilters(userid, filtRewrite)
		for _, h := range honks {
			for _, f := range colfilts {
				if bad := matchfilterX(h, f); bad != "" {
					if h.Precis == "" {
						h.Precis = bad
					}
					h.Open = ""
					break
				}
			}
			if h.Open == "open" && h.Precis == "unspecified horror" {
				h.Precis = ""
			}
			for _, f := range rwfilts {
				if matchfilter(h, f) {
					h.Noise = f.re_rewrite.ReplaceAllString(h.Noise, f.Replace)
				}
			}
			if len(h.Noise) > 6000 && h.Open == "open" {
				if h.Precis == "" {
					h.Precis = "really freaking long"
				}
				h.Open = ""
			}
		}
	}
}

var untagged = cache.New(cache.Options{Filler: func(userid int64) (map[string]bool, bool) {
	rows, err := stmtUntagged.Query(userid)
	if err != nil {
		elog.Printf("error query untagged: %s", err)
		return nil, false
	}
	defer rows.Close()
	bad := make(map[string]bool)
	for rows.Next() {
		var xid, rid string
		var flags int64
		err = rows.Scan(&xid, &rid, &flags)
		if err != nil {
			elog.Printf("error scanning untag: %s", err)
			continue
		}
		if flags&flagIsUntagged != 0 {
			bad[xid] = true
		}
		if bad[rid] {
			bad[xid] = true
		}
	}
	return bad, true
}})

func osmosis(honks []*Honk, userid int64, withfilt bool) []*Honk {
	var badparents map[string]bool
	untagged.GetAndLock(userid, &badparents)
	j := 0
	reversehonks(honks)
	for _, h := range honks {
		if badparents[h.RID] {
			badparents[h.XID] = true
			continue
		}
		honks[j] = h
		j++
	}
	untagged.Unlock()
	honks = honks[0:j]
	reversehonks(honks)
	if !withfilt {
		return honks
	}
	filts := getfilters(userid, filtHide)
	j = 0
outer:
	for _, h := range honks {
		for _, f := range filts {
			if matchfilter(h, f) {
				continue outer
			}
		}
		honks[j] = h
		j++
	}
	honks = honks[0:j]
	return honks
}
