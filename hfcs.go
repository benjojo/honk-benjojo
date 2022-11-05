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
	"log"
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

var filtcache *cache.Cache

func init() {
	// resolve init loop
	filtcache = cache.New(cache.Options{Filler: filtcachefiller})
}

func filtcachefiller(userid int64) (afiltermap, bool) {
	rows, err := stmtGetFilters.Query(userid)
	if err != nil {
		log.Printf("error querying filters: %s", err)
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
			log.Printf("error scanning filter: %s", err)
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
		if filt.Text != "" {
			filt.re_text, err = regexp.Compile("\\b(?i:" + filt.Text + ")\\b")
			if err != nil {
				log.Printf("error compiling filter text: %s", err)
				continue
			}
		}
		if filt.Rewrite != "" {
			filt.re_rewrite, err = regexp.Compile("\\b(?i:" + filt.Rewrite + ")\\b")
			if err != nil {
				log.Printf("error compiling filter rewrite: %s", err)
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
	filtcache.Clear(userid)
}

func getfilters(userid int64, scope filtType) []*Filter {
	var filtmap afiltermap
	ok := filtcache.Get(userid, &filtmap)
	if ok {
		return filtmap[scope]
	}
	return nil
}

func rejectorigin(userid int64, origin string) bool {
	if o := originate(origin); o != "" {
		origin = o
	}
	filts := getfilters(userid, filtReject)
	for _, f := range filts {
		if f.IsAnnounce || f.Text != "" {
			continue
		}
		if f.Actor == origin {
			log.Printf("rejecting origin: %s", origin)
			return true
		}
	}
	return false
}

func rejectactor(userid int64, actor string) bool {
	origin := originate(actor)
	filts := getfilters(userid, filtReject)
	for _, f := range filts {
		if f.IsAnnounce || f.Text != "" {
			continue
		}
		if f.Actor == actor || (origin != "" && f.Actor == origin) {
			log.Printf("rejecting actor: %s", actor)
			return true
		}
	}
	return false
}

func stealthmode(userid int64, r *http.Request) bool {
	agent := r.UserAgent()
	agent = originate(agent)
	if agent != "" {
		fake := rejectorigin(userid, agent)
		if fake {
			log.Printf("faking 404 for %s", agent)
			return true
		}
	}
	return false
}

func matchfilter(h *Honk, f *Filter) bool {
	match := true
	if match && f.Actor != "" {
		match = false
		if f.Actor == h.Honker || f.Actor == h.Oonker {
			match = true
		}
		if !match && (f.Actor == originate(h.Honker) ||
			f.Actor == originate(h.Oonker) ||
			f.Actor == originate(h.XID)) {
			match = true
		}
		if !match && f.IncludeAudience {
			for _, a := range h.Audience {
				if f.Actor == a || f.Actor == originate(a) {
					match = true
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
		}
	}
	if match && f.Text != "" {
		match = false
		re := f.re_text
		if re.MatchString(h.Noise) || re.MatchString(h.Precis) {
			match = true
		}
		if !match {
			for _, d := range h.Donks {
				if re.MatchString(d.Desc) {
					match = true
				}
			}
		}
	}
	return match
}

func rejectxonk(xonk *Honk) bool {
	filts := getfilters(xonk.UserID, filtReject)
	for _, f := range filts {
		if matchfilter(xonk, f) {
			log.Printf("rejecting %s because %s", xonk.XID, f.Actor)
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

func unsee(userid int64, h *Honk) {
	filts := getfilters(userid, filtCollapse)
	for _, f := range filts {
		if matchfilter(h, f) {
			bad := f.Text
			if f.Actor != "" {
				bad = f.Actor
			}
			if h.Precis == "" {
				h.Precis = bad
			}
			h.Open = ""
			break
		}
	}
	filts = getfilters(userid, filtRewrite)
	for _, f := range filts {
		if matchfilter(h, f) {
			h.Noise = f.re_rewrite.ReplaceAllString(h.Noise, f.Replace)
		}
	}
}

var untagged = cache.New(cache.Options{Filler: func(userid int64) (map[string]bool, bool) {
	rows, err := stmtUntagged.Query(userid)
	if err != nil {
		log.Printf("error query untagged: %s", err)
		return nil, false
	}
	defer rows.Close()
	bad := make(map[string]bool)
	for rows.Next() {
		var xid, rid string
		var flags int64
		err = rows.Scan(&xid, &rid, &flags)
		if err != nil {
			log.Printf("error scanning untag: %s", err)
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
