package main

import (
	"encoding/json"
	"net/http"
)

/*
{
  "uri": "karmalife.io",
  "title": "Mastodon",
  "short_description": "",
  "description": "",
  "email": "",
  "version": "4.0.2",
  "urls": {
    "streaming_api": "wss://karmalife.io"
  },
  "stats": {
    "user_count": 1,
    "status_count": 3,
    "domain_count": 2
  },
  "thumbnail": "https://karmalife.io/packs/media/images/preview-6399aebd96ccf025654e2977454f168f.png",
  "languages": [
    "en"
  ],
  "registrations": true,
  "approval_required": false,
  "invites_enabled": true,
  "configuration": {
    "accounts": {
      "max_featured_tags": 10
    },
    "statuses": {
      "max_characters": 500,
      "max_media_attachments": 4,
      "characters_reserved_per_url": 23
    },
    "media_attachments": {
      "supported_mime_types": [
        "image/jpeg",
        "image/png",
        "image/gif",
        "image/heic",
        "image/heif",
        "image/webp",
        "image/avif",
        "video/webm",
        "video/mp4",
        "video/quicktime",
        "video/ogg",
        "audio/wave",
        "audio/wav",
        "audio/x-wav",
        "audio/x-pn-wave",
        "audio/vnd.wave",
        "audio/ogg",
        "audio/vorbis",
        "audio/mpeg",
        "audio/mp3",
        "audio/webm",
        "audio/flac",
        "audio/aac",
        "audio/m4a",
        "audio/x-m4a",
        "audio/mp4",
        "audio/3gpp",
        "video/x-ms-asf"
      ],
      "image_size_limit": 10485760,
      "image_matrix_limit": 16777216,
      "video_size_limit": 41943040,
      "video_frame_rate_limit": 60,
      "video_matrix_limit": 2304000
    },
    "polls": {
      "max_options": 4,
      "max_characters_per_option": 50,
      "min_expiration": 300,
      "max_expiration": 2629746
    }
  },
  "contact_account": null,
  "rules": []
}
*/

func handleAPIInstance(w http.ResponseWriter, r *http.Request) {
	Info := APIInstanceResponse{
		ApprovalRequired: true,
		ContactAccount:   nil,
		Description:      "",
		Email:            "",
		InvitesEnabled:   false,
		Registrations:    false,
		ShortDescription: "",
		Stats: struct {
			DomainCount int "json:\"domain_count\""
			StatusCount int "json:\"status_count\""
			UserCount   int "json:\"user_count\""
		}{},
		Title:   "Benjojo's Honk Instance",
		Uri:     "",
		Version: "",
	}

	getConfigValue("servermsg", &Info.Description)
	getConfigValue("aboutmsg", &Info.ShortDescription)
	getConfigValue("servername", &Info.Uri)

	alreadyopendb.QueryRow(`SELECT COUNT(*) FROM honks WHERE what = "honk" and whofore = 2;`).Scan(&Info.Stats.StatusCount)
	alreadyopendb.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&Info.Stats.UserCount)

	json.NewEncoder(w).Encode(Info)
}

type APIInstanceResponse struct {
	ApprovalRequired bool          `json:"approval_required"`
	ContactAccount   interface{}   `json:"contact_account"`
	Description      string        `json:"description"`
	Email            string        `json:"email"`
	InvitesEnabled   bool          `json:"invites_enabled"`
	Languages        []string      `json:"languages"`
	Registrations    bool          `json:"registrations"`
	Rules            []interface{} `json:"rules"`
	ShortDescription string        `json:"short_description"`
	Stats            struct {
		DomainCount int `json:"domain_count"`
		StatusCount int `json:"status_count"`
		UserCount   int `json:"user_count"`
	} `json:"stats"`
	Thumbnail string `json:"thumbnail"`
	Title     string `json:"title"`
	Uri       string `json:"uri"`
	Version   string `json:"version"`
}

func handleNodeInfo(w http.ResponseWriter, r *http.Request) {
	Info := NodeInfo{
		Metadata:          struct{}{},
		OpenRegistrations: false,
		Protocols:         []string{"activitypub"},
		Software: struct {
			Name    string "json:\"name\""
			Version string "json:\"version\""
		}{
			Name:    "honk",
			Version: "",
		},
		Usage: struct {
			LocalPosts int "json:\"localPosts\""
			Users      struct {
				ActiveHalfyear int "json:\"activeHalfyear\""
				ActiveMonth    int "json:\"activeMonth\""
				Total          int "json:\"total\""
			} "json:\"users\""
		}{},
		Version: "",
	}

	alreadyopendb.QueryRow(`SELECT COUNT(*) FROM honks WHERE what = "honk" and whofore = 2;`).Scan(&Info.Usage.LocalPosts)
	alreadyopendb.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&Info.Usage.Users.Total)
	Info.Usage.Users.ActiveHalfyear = Info.Usage.Users.Total
	Info.Usage.Users.ActiveMonth = Info.Usage.Users.Total

	json.NewEncoder(w).Encode(Info)
}

type NodeInfo struct {
	Metadata          struct{} `json:"metadata"`
	OpenRegistrations bool     `json:"openRegistrations"`
	Protocols         []string `json:"protocols"`
	Software          struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"software"`
	Usage struct {
		LocalPosts int `json:"localPosts"`
		Users      struct {
			ActiveHalfyear int `json:"activeHalfyear"`
			ActiveMonth    int `json:"activeMonth"`
			Total          int `json:"total"`
		} `json:"users"`
	} `json:"usage"`
	Version string `json:"version"`
}
