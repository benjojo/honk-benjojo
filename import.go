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
	"archive/zip"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html"
	"log"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"humungus.tedunangst.com/r/webs/junk"
)

func importMain(username, flavor, source string) {
	switch flavor {
	case "mastodon":
		importMastodon(username, source)
	case "honk":
		importHonk(username, source)
	case "twitter":
		importTwitter(username, source)
	case "instagram":
		importInstagram(username, source)
	default:
		elog.Fatal("unknown source flavor")
	}
}

type ActivityObject struct {
	Id           string
	AttributedTo string
	Summary      string
	Content      string
	Source       struct {
		MediaType string
		Content   string
	}
	InReplyTo    string
	Conversation string
	Context      string
	Published    time.Time
	Tag          []struct {
		Type string
		Name string
	}
	Attachment []struct {
		Type      string
		MediaType string
		Url       string
		Name      string
	}
}

type PlainActivityObject ActivityObject

func (obj *ActivityObject) UnmarshalJSON(b []byte) error {
	p := (*PlainActivityObject)(obj)
	json.Unmarshal(b, p)
	return nil
}

func importMastodon(username, source string) {
	user, err := getUserBio(username)
	if err != nil {
		elog.Fatal(err)
	}

	outbox := source + "/outbox.json"
	if _, err := os.Stat(outbox); err == nil {
		importActivities(user, outbox, source)
	} else {
		ilog.Printf("skipping outbox.json!")
	}
	if _, err := os.Stat(source + "/following_accounts.csv"); err == nil {
		importMastotooters(user, source)
	} else {
		ilog.Printf("skipping following_accounts.csv!")
	}
}

func importHonk(username, source string) {
	user, err := getUserBio(username)
	if err != nil {
		elog.Fatal(err)
	}

	outbox := source + "/outbox.json"
	if _, err := os.Stat(outbox); err == nil {
		importActivities(user, outbox, source)
	} else {
		ilog.Printf("skipping outbox.json!")
	}
}

func importActivities(user *WhatAbout, filename, source string) {
	type Activity struct {
		Id     string
		Type   string
		To     interface{}
		Cc     []string
		Object ActivityObject
	}
	var outbox struct {
		OrderedItems []Activity
	}
	ilog.Println("Importing honks...")
	fd, err := os.Open(filename)
	if err != nil {
		elog.Fatal(err)
	}
	dec := json.NewDecoder(fd)
	err = dec.Decode(&outbox)
	if err != nil {
		elog.Fatalf("error parsing json: %s", err)
	}
	fd.Close()

	havetoot := func(xid string) bool {
		var id int64
		row := stmtFindXonk.QueryRow(user.ID, xid)
		err := row.Scan(&id)
		if err == nil {
			return true
		}
		return false
	}

	re_tootid := regexp.MustCompile("[^/]+$")
	items := outbox.OrderedItems
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
	for _, item := range items {
		toot := item
		if toot.Type != "Create" {
			continue
		}
		if strings.HasSuffix(toot.Id, "/activity") {
			toot.Id = strings.TrimSuffix(toot.Id, "/activity")
		}
		tootid := re_tootid.FindString(toot.Id)
		xid := fmt.Sprintf("%s/%s/%s", user.URL, honkSep, tootid)
		if havetoot(xid) {
			continue
		}

		convoy := toot.Object.Context
		if convoy == "" {
			convoy = toot.Object.Conversation
		}
		var audience []string
		to, ok := toot.To.(string)
		if ok {
			audience = append(audience, to)
		} else {
			for _, t := range toot.To.([]interface{}) {
				audience = append(audience, t.(string))
			}
		}
		content := toot.Object.Content
		format := "html"
		if toot.Object.Source.MediaType == "text/markdown" {
			content = toot.Object.Source.Content
			format = "markdown"
		}
		audience = append(audience, toot.Cc...)
		honk := ActivityPubActivity{
			UserID:   user.ID,
			What:     "honk",
			Honker:   user.URL,
			XID:      xid,
			RID:      toot.Object.InReplyTo,
			Date:     toot.Object.Published,
			URL:      xid,
			Audience: audience,
			Noise:    content,
			Convoy:   convoy,
			Whofore:  WhoPublic,
			Format:   format,
			Precis:   toot.Object.Summary,
		}
		if !loudandproud(honk.Audience) {
			honk.Whofore = WhoPrivate
		}
		for _, att := range toot.Object.Attachment {
			var meta DonkMeta
			switch att.Type {
			case "Document":
				fname := fmt.Sprintf("%s/%s", source, att.Url)
				data, err := os.ReadFile(fname)
				if err != nil {
					elog.Printf("error reading media for %s: %s", honk.XID, fname)
					continue
				}
				u := make18CharRandomString()
				name := att.Name
				desc := name
				newurl := fmt.Sprintf("https://%s/d/%s", serverName, u)
				fileid, err := savefile(name, desc, newurl, att.MediaType, true, data, &meta)
				if err != nil {
					elog.Printf("error saving media: %s", fname)
					continue
				}
				donk := &Donk{
					FileID: fileid,
				}
				honk.Donks = append(honk.Donks, donk)
			}
		}
		for _, t := range toot.Object.Tag {
			switch t.Type {
			case "Hashtag":
				honk.Onts = append(honk.Onts, t.Name)
			}
		}
		savehonk(&honk)
	}
}

func importMastotooters(user *WhatAbout, source string) {
	ilog.Println("Importing honkers...")
	fd, err := os.Open(source + "/following_accounts.csv")
	if err != nil {
		elog.Fatal(err)
	}
	r := csv.NewReader(fd)
	data, err := r.ReadAll()
	if err != nil {
		elog.Fatal(err)
	}
	fd.Close()

	var meta HonkerMeta
	mj, _ := encodeJson(&meta)

	for i, d := range data {
		if i == 0 {
			continue
		}
		url := "@" + d[0]
		name := ""
		flavor := "peep"
		combos := ""
		_, _, err := savehonker(user, url, name, flavor, combos, mj)
		if err != nil {
			elog.Printf("trouble with a honker: %s", err)
		}
	}
}

func importTwitter(username, source string) {
	user, err := getUserBio(username)
	if err != nil {
		elog.Fatal(err)
	}

	type Tweet struct {
		date   time.Time
		convoy string
		Tweet  struct {
			CreatedAt        string   `json:"created_at"`
			DisplayTextRange []string `json:"display_text_range"`
			EditInfo         struct {
				Initial struct {
					EditTweetIds   []string `json:"editTweetIds"`
					EditableUntil  string   `json:"editableUntil"`
					EditsRemaining string   `json:"editsRemaining"`
					IsEditEligible bool     `json:"isEditEligible"`
				} `json:"initial"`
			} `json:"edit_info"`
			Entities struct {
				Hashtags []struct {
					Indices []string `json:"indices"`
					Text    string   `json:"text"`
				} `json:"hashtags"`
				Media []struct {
					DisplayURL    string   `json:"display_url"`
					ExpandedURL   string   `json:"expanded_url"`
					ID            string   `json:"id"`
					IdStr         string   `json:"id_str"`
					Indices       []string `json:"indices"`
					MediaURL      string   `json:"media_url"`
					MediaUrlHttps string   `json:"media_url_https"`
					Sizes         struct {
						Large struct {
							H      string `json:"h"`
							Resize string `json:"resize"`
							W      string `json:"w"`
						} `json:"large"`
						Medium struct {
							H      string `json:"h"`
							Resize string `json:"resize"`
							W      string `json:"w"`
						} `json:"medium"`
						Small struct {
							H      string `json:"h"`
							Resize string `json:"resize"`
							W      string `json:"w"`
						} `json:"small"`
						Thumb struct {
							H      string `json:"h"`
							Resize string `json:"resize"`
							W      string `json:"w"`
						} `json:"thumb"`
					} `json:"sizes"`
					Type string `json:"type"`
					URL  string `json:"url"`
				} `json:"media"`
				Symbols []interface{} `json:"symbols"`
				Urls    []struct {
					DisplayURL  string   `json:"display_url"`
					ExpandedURL string   `json:"expanded_url"`
					Indices     []string `json:"indices"`
					URL         string   `json:"url"`
				} `json:"urls"`
				UserMentions []interface{} `json:"user_mentions"`
			} `json:"entities"`
			ExtendedEntities struct {
				Media []struct {
					DisplayURL    string   `json:"display_url"`
					ExpandedURL   string   `json:"expanded_url"`
					ID            string   `json:"id"`
					IdStr         string   `json:"id_str"`
					Indices       []string `json:"indices"`
					MediaURL      string   `json:"media_url"`
					MediaUrlHttps string   `json:"media_url_https"`
					Sizes         struct {
						Large struct {
							H      string `json:"h"`
							Resize string `json:"resize"`
							W      string `json:"w"`
						} `json:"large"`
						Medium struct {
							H      string `json:"h"`
							Resize string `json:"resize"`
							W      string `json:"w"`
						} `json:"medium"`
						Small struct {
							H      string `json:"h"`
							Resize string `json:"resize"`
							W      string `json:"w"`
						} `json:"small"`
						Thumb struct {
							H      string `json:"h"`
							Resize string `json:"resize"`
							W      string `json:"w"`
						} `json:"thumb"`
					} `json:"sizes"`
					Type string `json:"type"`
					URL  string `json:"url"`
				} `json:"media"`
			} `json:"extended_entities"`
			FavoriteCount        string `json:"favorite_count"`
			Favorited            bool   `json:"favorited"`
			FullText             string `json:"full_text"`
			ID                   string `json:"id"`
			IdStr                string `json:"id_str"`
			InReplyToScreenName  string `json:"in_reply_to_screen_name"`
			InReplyToStatusID    string `json:"in_reply_to_status_id"`
			InReplyToStatusIdStr string `json:"in_reply_to_status_id_str"`
			InReplyToUserID      string `json:"in_reply_to_user_id"`
			InReplyToUserIdStr   string `json:"in_reply_to_user_id_str"`
			Lang                 string `json:"lang"`
			PossiblySensitive    bool   `json:"possibly_sensitive"`
			RetweetCount         string `json:"retweet_count"`
			Retweeted            bool   `json:"retweeted"`
			Source               string `json:"source"`
			Truncated            bool   `json:"truncated"`
		} `json:"tweet"`
	}

	var tweets []*Tweet
	fd, err := os.Open(source + "/tweet.js")
	if err != nil {
		elog.Fatal(err)
	}
	// skip past window.YTD.tweet.part0 =
	fd.Seek(25, 0)
	dec := json.NewDecoder(fd)
	err = dec.Decode(&tweets)
	if err != nil {
		elog.Fatalf("error parsing json: %s", err)
	}
	fd.Close()
	tweetmap := make(map[string]*Tweet)
	for _, t := range tweets {
		t.date, _ = time.Parse("Mon Jan 02 15:04:05 -0700 2006", t.Tweet.CreatedAt)
		tweetmap[t.Tweet.IdStr] = t
	}
	sort.Slice(tweets, func(i, j int) bool {
		return tweets[i].date.Before(tweets[j].date)
	})
	havetwid := func(xid string) bool {
		var id int64
		row := stmtFindXonk.QueryRow(user.ID, xid)
		err := row.Scan(&id)
		if err == nil {
			log.Printf("id = %v", id)
			return true
		}
		return false
	}
	log.Printf("importing %v tweets", len(tweets))
	for _, t := range tweets {
		xid := fmt.Sprintf("%s/%s/%s", user.URL, honkSep, t.Tweet.IdStr)
		if havetwid(xid) {
			continue
		}

		what := "honk"
		noise := ""
		if parent := tweetmap[t.Tweet.InReplyToStatusID]; parent != nil {
			t.convoy = parent.convoy
		} else {
			t.convoy = "data:,acoustichonkytonk-" + t.Tweet.IdStr
			if t.Tweet.InReplyToScreenName != "" {
				noise = fmt.Sprintf("re: https://twitter.com/%s/status/%s\n\n",
					t.Tweet.InReplyToScreenName, t.Tweet.InReplyToStatusID)
			}
		}
		audience := []string{atContextString}
		honk := ActivityPubActivity{
			UserID:   user.ID,
			Username: user.Name,
			What:     what,
			Honker:   user.URL,
			XID:      xid,
			Date:     t.date,
			Format:   "markdown",
			Audience: audience,
			Convoy:   t.convoy,
			Public:   true,
			Whofore:  WhoPublic,
		}
		noise += t.Tweet.FullText
		// unbelievable
		noise = html.UnescapeString(noise)
		for _, r := range t.Tweet.Entities.Urls {
			noise = strings.Replace(noise, r.URL, r.ExpandedURL, -1)
		}
		for _, m := range t.Tweet.Entities.Media {
			var meta DonkMeta
			u := m.MediaURL
			idx := strings.LastIndexByte(u, '/')
			u = u[idx+1:]
			fname := fmt.Sprintf("%s/tweet_media/%s-%s", source, t.Tweet.IdStr, u)
			data, err := os.ReadFile(fname)
			if err != nil {
				elog.Printf("error reading media: %s", fname)
				continue
			}
			newurl := fmt.Sprintf("https://%s/d/%s", serverName, u)

			fileid, err := savefile(u, u, newurl, "image/jpg", true, data, &meta)
			if err != nil {
				elog.Printf("error saving media: %s", fname)
				continue
			}
			donk := &Donk{
				FileID: fileid,
			}
			honk.Donks = append(honk.Donks, donk)
			noise = strings.Replace(noise, m.URL, "", -1)
		}
		for _, ht := range t.Tweet.Entities.Hashtags {
			honk.Onts = append(honk.Onts, "#"+ht.Text)
		}
		honk.Noise = noise
		err := savehonk(&honk)
		log.Printf("honk saved %v -> %v", xid, err)
	}
}

func importInstagram(username, source string) {
	user, err := getUserBio(username)
	if err != nil {
		elog.Fatal(err)
	}

	type Gram struct {
		Media []struct {
			URI      string
			Creation int64 `json:"creation_timestamp"`
			Title    string
		}
	}

	var grams []*Gram
	fd, err := os.Open(source + "/content/posts_1.json")
	if err != nil {
		elog.Fatal(err)
	}
	dec := json.NewDecoder(fd)
	err = dec.Decode(&grams)
	if err != nil {
		elog.Fatalf("error parsing json: %s", err)
	}
	fd.Close()
	log.Printf("importing %d grams", len(grams))
	sort.Slice(grams, func(i, j int) bool {
		return grams[i].Media[0].Creation < grams[j].Media[0].Creation
	})
	for _, g0 := range grams {
		g := g0.Media[0]
		xid := fmt.Sprintf("%s/%s/%s", user.URL, honkSep, make18CharRandomString())
		what := "honk"
		noise := g.Title
		convoy := "data:,acoustichonkytonk-" + make18CharRandomString()
		date := time.Unix(g.Creation, 0)
		audience := []string{atContextString}
		honk := ActivityPubActivity{
			UserID:   user.ID,
			Username: user.Name,
			What:     what,
			Honker:   user.URL,
			XID:      xid,
			Date:     date,
			Format:   "markdown",
			Audience: audience,
			Convoy:   convoy,
			Public:   true,
			Whofore:  WhoPublic,
		}
		{
			var meta DonkMeta
			u := make18CharRandomString()
			fname := fmt.Sprintf("%s/%s", source, g.URI)
			data, err := os.ReadFile(fname)
			if err != nil {
				elog.Printf("error reading media: %s", fname)
				continue
			}
			newurl := fmt.Sprintf("https://%s/d/%s", serverName, u)

			fileid, err := savefile(u, u, newurl, "image/jpg", true, data, &meta)
			if err != nil {
				elog.Printf("error saving media: %s", fname)
				continue
			}
			donk := &Donk{
				FileID: fileid,
			}
			honk.Donks = append(honk.Donks, donk)
		}
		honk.Noise = noise
		err := savehonk(&honk)
		log.Printf("honk saved %v -> %v", xid, err)
	}
}

func export(username, file string) {
	user, err := getUserBio(username)
	if err != nil {
		elog.Fatal(err)
	}
	fd, err := os.OpenFile(file, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0666)
	if err != nil {
		elog.Fatal(err)
	}
	zd := zip.NewWriter(fd)
	donks := make(map[string]bool)
	{
		w, err := zd.Create("outbox.json")
		if err != nil {
			elog.Fatal("error creating outbox.json", err)
		}
		var jonks []junk.Junk
		rows, err := stmtUserHonks.Query(0, 3, user.Name, "0", 1234567)
		honks := getsomehonks(rows, err)
		for _, honk := range honks {
			for _, donk := range honk.Donks {
				donk.URL = "media/" + donk.XID
				donks[donk.XID] = true
			}
			noise := honk.Noise
			j, jo := jonkjonk(user, honk)
			if honk.Format == "markdown" {
				source := junk.New()
				source["mediaType"] = "text/markdown"
				source["content"] = noise
				jo["source"] = source
			}
			jonks = append(jonks, j)
		}
		j := junk.New()
		j["@context"] = itiswhatitis
		j["id"] = user.URL + "/outbox"
		j["attributedTo"] = user.URL
		j["type"] = "OrderedCollection"
		j["totalItems"] = len(jonks)
		j["orderedItems"] = jonks
		j.Write(w)
	}
	{
		w, err := zd.Create("inbox.json")
		if err != nil {
			elog.Fatal("error creating inbox.json", err)
		}
		var jonks []junk.Junk
		rows, err := stmtHonksForMe.Query(0, user.ID, "0", user.ID, 1234567)
		honks := getsomehonks(rows, err)
		for _, honk := range honks {
			for _, donk := range honk.Donks {
				donk.URL = "media/" + donk.XID
				donks[donk.XID] = true
			}
			j, _ := jonkjonk(user, honk)
			jonks = append(jonks, j)
		}
		j := junk.New()
		j["@context"] = itiswhatitis
		j["id"] = user.URL + "/inbox"
		j["attributedTo"] = user.URL
		j["type"] = "OrderedCollection"
		j["totalItems"] = len(jonks)
		j["orderedItems"] = jonks
		j.Write(w)
	}
	zd.Create("media/")
	for donk := range donks {
		if donk == "" {
			continue
		}
		w, err := zd.Create("media/" + donk)
		if err != nil {
			elog.Printf("error creating %s: %s", donk, err)
			continue
		}
		data, closer, err := loaddata(donk)
		if err != nil {
			elog.Printf("error scanning file %s: %s", donk, err)
			continue
		}
		w.Write(data)
		closer()
	}
	zd.Close()
	fd.Close()
}

func dumpthread(username, convoy string) {
	user, _ := getUserBio(username)
	honks := gethonksbyconvoy(user.ID, convoy, 0)
	var jonks []junk.Junk
	for _, honk := range honks {
		noise := honk.Noise
		j, jo := jonkjonk(user, honk)
		if honk.Format == "markdown" {
			source := junk.New()
			source["mediaType"] = "text/markdown"
			source["content"] = noise
			jo["source"] = source
		}
		jonks = append(jonks, j)
	}
	j := junk.New()
	j["@context"] = itiswhatitis
	j["orderedItems"] = jonks
	j.Write(os.Stdout)
}
func rawimport(username, filename string) {
	user, _ := getUserBio(username)
	type Activity struct {
		Id     string
		Type   string
		To     interface{}
		Cc     []string
		Object ActivityObject
	}
	var outbox struct {
		OrderedItems []Activity
	}
	ilog.Println("Importing honks...")
	fd, err := os.Open(filename)
	if err != nil {
		elog.Fatal(err)
	}
	dec := json.NewDecoder(fd)
	err = dec.Decode(&outbox)
	if err != nil {
		elog.Fatalf("error parsing json: %s", err)
	}
	fd.Close()

	havetoot := func(xid string) bool {
		var id int64
		row := stmtFindXonk.QueryRow(user.ID, xid)
		err := row.Scan(&id)
		if err == nil {
			return true
		}
		return false
	}

	items := outbox.OrderedItems
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
	for _, item := range items {
		toot := item
		if toot.Type != "Create" {
			continue
		}
		xid := toot.Object.Id
		if havetoot(xid) {
			continue
		}

		convoy := toot.Object.Context
		if convoy == "" {
			convoy = toot.Object.Conversation
		}
		var audience []string
		to, ok := toot.To.(string)
		if ok {
			audience = append(audience, to)
		} else {
			for _, t := range toot.To.([]interface{}) {
				audience = append(audience, t.(string))
			}
		}
		content := toot.Object.Content
		format := "html"
		if toot.Object.Source.MediaType == "text/markdown" {
			content = toot.Object.Source.Content
			format = "markdown"
		}
		audience = append(audience, toot.Cc...)
		honk := ActivityPubActivity{
			UserID:   user.ID,
			What:     "honk",
			Honker:   toot.Object.AttributedTo,
			XID:      xid,
			RID:      toot.Object.InReplyTo,
			Date:     toot.Object.Published,
			URL:      xid,
			Audience: audience,
			Noise:    content,
			Convoy:   convoy,
			Whofore:  WhoPublic,
			Format:   format,
			Precis:   toot.Object.Summary,
		}
		if !loudandproud(honk.Audience) {
			honk.Whofore = WhoPrivate
		}
		for _, t := range toot.Object.Tag {
			switch t.Type {
			case "Hashtag":
				honk.Onts = append(honk.Onts, t.Name)
			}
		}
		savehonk(&honk)
	}
}
