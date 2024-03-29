// Code generated by cmd/lexgen (see Makefile's lexgen); DO NOT EDIT.

package bsky

// schema: app.bsky.embed.images

import (
	"github.com/bluesky-social/indigo/lex/util"
)

func init() {
	util.RegisterType("app.bsky.embed.images#main", &EmbedImages{})
} // EmbedImages is a "main" in the app.bsky.embed.images schema.
// RECORDTYPE: EmbedImages
type EmbedImages struct {
	LexiconTypeID string               `json:"$type,const=app.bsky.embed.images" cborgen:"$type,const=app.bsky.embed.images"`
	Images        []*EmbedImages_Image `json:"images" cborgen:"images"`
}

// EmbedImages_Image is a "image" in the app.bsky.embed.images schema.
type EmbedImages_Image struct {
	Alt   string        `json:"alt" cborgen:"alt"`
	Image *util.LexBlob `json:"image" cborgen:"image"`
}

// EmbedImages_View is a "view" in the app.bsky.embed.images schema.
//
// RECORDTYPE: EmbedImages_View
type EmbedImages_View struct {
	LexiconTypeID string                   `json:"$type,const=app.bsky.embed.images#view" cborgen:"$type,const=app.bsky.embed.images#view"`
	Images        []*EmbedImages_ViewImage `json:"images" cborgen:"images"`
}

// EmbedImages_ViewImage is a "viewImage" in the app.bsky.embed.images schema.
type EmbedImages_ViewImage struct {
	Alt      string `json:"alt" cborgen:"alt"`
	Fullsize string `json:"fullsize" cborgen:"fullsize"`
	Thumb    string `json:"thumb" cborgen:"thumb"`
}
