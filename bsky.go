package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/ericvolp12/bsky-client/pkg/client"
	"github.com/microcosm-cc/bluemonday"
)

//

func goBlue(xid string, user *WhatAbout) {
	dlog.Printf("reposting to bluesky %s", xid)

	xonk := getActivityPubActivity(user.ID, xid)
	if xonk == nil {
		return
	}
	if !xonk.Public {
		return
	}
	donksforhonks([]*ActivityPubActivity{xonk})

	imgs := make([]client.Image, 0)

	for _, v := range xonk.Donks {
		if v.Local {
			var media string
			var data []byte
			row := stmtGetFileData.QueryRow(v.XID)
			err := row.Scan(&media, &data)
			if err != nil {
				elog.Printf("Failed to scan donk for bsky: %v", err)
				continue
			}

			imgs = append(imgs, client.Image{
				Reader:  bytes.NewReader(data),
				AltText: "",
			})
		}
	}

	_, err := stmtUpdateFlags.Exec(flagIsBSkyd, xonk.ID)
	if err != nil {
		elog.Printf("error acking bsky: %s", err)
		return
	}

	ctx := context.Background()

	// Initialize Client
	c := client.New("https://bsky.social", "https://plc.directory")
	c.AutoRefreshAuth = false

	// Authenticate
	err = c.Login(ctx, fetchDirtyConfigStringYolo("benjojo.bsky.social"), fetchDirtyConfigStringYolo("bskypassword"))
	if err != nil {
		elog.Printf("error logging in with bluesky: %s", err)
		return
	}

	log.Printf("fuck this: debug: %#v", xonk)

	// Do this once for each unique policy, and use the policy for the life of the program
	// Policy creation/editing is not safe to use in multiple goroutines
	p := bluemonday.StripTagsPolicy()

	// The policy can then be used to sanitize lots of input and it is safe to use the policy in multiple goroutines
	html := p.Sanitize(
		string(xonk.Noise),
	)

	postArgs := client.PostArgs{
		Text: html,
		// EmbeddedLink: ,
		Images: imgs,
	}

	// Create Post
	_, err = c.CreatePost(ctx, postArgs)
	if err != nil {
		elog.Printf("error posting to bsky: %s", err)
		return
	}
}

func fetchDirtyConfigStringYolo(name string) string {
	b, err := os.ReadFile(fmt.Sprintf("/etc/honk/%s", name))
	if err != nil {
		return ""
	}
	return strings.Trim(string(b), "\r\n\t ")
}
