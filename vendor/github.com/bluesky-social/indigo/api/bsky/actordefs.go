// Code generated by cmd/lexgen (see Makefile's lexgen); DO NOT EDIT.

package bsky

// schema: app.bsky.actor.defs

import (
	"encoding/json"
	"fmt"

	comatprototypes "github.com/bluesky-social/indigo/api/atproto"
	"github.com/bluesky-social/indigo/lex/util"
)

// ActorDefs_AdultContentPref is a "adultContentPref" in the app.bsky.actor.defs schema.
//
// RECORDTYPE: ActorDefs_AdultContentPref
type ActorDefs_AdultContentPref struct {
	LexiconTypeID string `json:"$type,const=app.bsky.actor.defs#adultContentPref" cborgen:"$type,const=app.bsky.actor.defs#adultContentPref"`
	Enabled       bool   `json:"enabled" cborgen:"enabled"`
}

// ActorDefs_ContentLabelPref is a "contentLabelPref" in the app.bsky.actor.defs schema.
//
// RECORDTYPE: ActorDefs_ContentLabelPref
type ActorDefs_ContentLabelPref struct {
	LexiconTypeID string `json:"$type,const=app.bsky.actor.defs#contentLabelPref" cborgen:"$type,const=app.bsky.actor.defs#contentLabelPref"`
	Label         string `json:"label" cborgen:"label"`
	Visibility    string `json:"visibility" cborgen:"visibility"`
}

type ActorDefs_Preferences_Elem struct {
	ActorDefs_AdultContentPref *ActorDefs_AdultContentPref
	ActorDefs_ContentLabelPref *ActorDefs_ContentLabelPref
	ActorDefs_SavedFeedsPref   *ActorDefs_SavedFeedsPref
}

func (t *ActorDefs_Preferences_Elem) MarshalJSON() ([]byte, error) {
	if t.ActorDefs_AdultContentPref != nil {
		t.ActorDefs_AdultContentPref.LexiconTypeID = "app.bsky.actor.defs#adultContentPref"
		return json.Marshal(t.ActorDefs_AdultContentPref)
	}
	if t.ActorDefs_ContentLabelPref != nil {
		t.ActorDefs_ContentLabelPref.LexiconTypeID = "app.bsky.actor.defs#contentLabelPref"
		return json.Marshal(t.ActorDefs_ContentLabelPref)
	}
	if t.ActorDefs_SavedFeedsPref != nil {
		t.ActorDefs_SavedFeedsPref.LexiconTypeID = "app.bsky.actor.defs#savedFeedsPref"
		return json.Marshal(t.ActorDefs_SavedFeedsPref)
	}
	return nil, fmt.Errorf("cannot marshal empty enum")
}
func (t *ActorDefs_Preferences_Elem) UnmarshalJSON(b []byte) error {
	typ, err := util.TypeExtract(b)
	if err != nil {
		return err
	}

	switch typ {
	case "app.bsky.actor.defs#adultContentPref":
		t.ActorDefs_AdultContentPref = new(ActorDefs_AdultContentPref)
		return json.Unmarshal(b, t.ActorDefs_AdultContentPref)
	case "app.bsky.actor.defs#contentLabelPref":
		t.ActorDefs_ContentLabelPref = new(ActorDefs_ContentLabelPref)
		return json.Unmarshal(b, t.ActorDefs_ContentLabelPref)
	case "app.bsky.actor.defs#savedFeedsPref":
		t.ActorDefs_SavedFeedsPref = new(ActorDefs_SavedFeedsPref)
		return json.Unmarshal(b, t.ActorDefs_SavedFeedsPref)

	default:
		return nil
	}
}

// ActorDefs_ProfileView is a "profileView" in the app.bsky.actor.defs schema.
type ActorDefs_ProfileView struct {
	Avatar      *string                            `json:"avatar,omitempty" cborgen:"avatar,omitempty"`
	Description *string                            `json:"description,omitempty" cborgen:"description,omitempty"`
	Did         string                             `json:"did" cborgen:"did"`
	DisplayName *string                            `json:"displayName,omitempty" cborgen:"displayName,omitempty"`
	Handle      string                             `json:"handle" cborgen:"handle"`
	IndexedAt   *string                            `json:"indexedAt,omitempty" cborgen:"indexedAt,omitempty"`
	Labels      []*comatprototypes.LabelDefs_Label `json:"labels,omitempty" cborgen:"labels,omitempty"`
	Viewer      *ActorDefs_ViewerState             `json:"viewer,omitempty" cborgen:"viewer,omitempty"`
}

// ActorDefs_ProfileViewBasic is a "profileViewBasic" in the app.bsky.actor.defs schema.
type ActorDefs_ProfileViewBasic struct {
	Avatar      *string                            `json:"avatar,omitempty" cborgen:"avatar,omitempty"`
	Did         string                             `json:"did" cborgen:"did"`
	DisplayName *string                            `json:"displayName,omitempty" cborgen:"displayName,omitempty"`
	Handle      string                             `json:"handle" cborgen:"handle"`
	Labels      []*comatprototypes.LabelDefs_Label `json:"labels,omitempty" cborgen:"labels,omitempty"`
	Viewer      *ActorDefs_ViewerState             `json:"viewer,omitempty" cborgen:"viewer,omitempty"`
}

// ActorDefs_ProfileViewDetailed is a "profileViewDetailed" in the app.bsky.actor.defs schema.
type ActorDefs_ProfileViewDetailed struct {
	Avatar         *string                            `json:"avatar,omitempty" cborgen:"avatar,omitempty"`
	Banner         *string                            `json:"banner,omitempty" cborgen:"banner,omitempty"`
	Description    *string                            `json:"description,omitempty" cborgen:"description,omitempty"`
	Did            string                             `json:"did" cborgen:"did"`
	DisplayName    *string                            `json:"displayName,omitempty" cborgen:"displayName,omitempty"`
	FollowersCount *int64                             `json:"followersCount,omitempty" cborgen:"followersCount,omitempty"`
	FollowsCount   *int64                             `json:"followsCount,omitempty" cborgen:"followsCount,omitempty"`
	Handle         string                             `json:"handle" cborgen:"handle"`
	IndexedAt      *string                            `json:"indexedAt,omitempty" cborgen:"indexedAt,omitempty"`
	Labels         []*comatprototypes.LabelDefs_Label `json:"labels,omitempty" cborgen:"labels,omitempty"`
	PostsCount     *int64                             `json:"postsCount,omitempty" cborgen:"postsCount,omitempty"`
	Viewer         *ActorDefs_ViewerState             `json:"viewer,omitempty" cborgen:"viewer,omitempty"`
}

// ActorDefs_SavedFeedsPref is a "savedFeedsPref" in the app.bsky.actor.defs schema.
//
// RECORDTYPE: ActorDefs_SavedFeedsPref
type ActorDefs_SavedFeedsPref struct {
	LexiconTypeID string   `json:"$type,const=app.bsky.actor.defs#savedFeedsPref" cborgen:"$type,const=app.bsky.actor.defs#savedFeedsPref"`
	Pinned        []string `json:"pinned" cborgen:"pinned"`
	Saved         []string `json:"saved" cborgen:"saved"`
}

// ActorDefs_ViewerState is a "viewerState" in the app.bsky.actor.defs schema.
type ActorDefs_ViewerState struct {
	BlockedBy   *bool                    `json:"blockedBy,omitempty" cborgen:"blockedBy,omitempty"`
	Blocking    *string                  `json:"blocking,omitempty" cborgen:"blocking,omitempty"`
	FollowedBy  *string                  `json:"followedBy,omitempty" cborgen:"followedBy,omitempty"`
	Following   *string                  `json:"following,omitempty" cborgen:"following,omitempty"`
	Muted       *bool                    `json:"muted,omitempty" cborgen:"muted,omitempty"`
	MutedByList *GraphDefs_ListViewBasic `json:"mutedByList,omitempty" cborgen:"mutedByList,omitempty"`
}