package client

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	comatproto "github.com/bluesky-social/indigo/api/atproto"
	"github.com/bluesky-social/indigo/lex/util"

	"github.com/bluesky-social/indigo/api/atproto"
	"github.com/bluesky-social/indigo/api/bsky"
	"github.com/bluesky-social/indigo/atproto/syntax"
	"github.com/dyatlov/go-htmlinfo/htmlinfo"
)

type Image struct {
	Reader  io.Reader
	AltText string
}

type PostArgs struct {
	Text         string
	Images       []Image
	Tags         []string
	Labels       []string
	ReplyTo      *syntax.ATURI
	Quoting      *syntax.ATURI
	Languages    []string
	EmbeddedLink string
	CreatedAt    time.Time
}

// CreatePost creates a new post
func (c *Client) CreatePost(ctx context.Context, args PostArgs) (*syntax.ATURI, error) {
	post := bsky.FeedPost{
		Text: args.Text,
		Tags: args.Tags,
	}

	if args.CreatedAt.IsZero() {
		post.CreatedAt = time.Now().Format(time.RFC3339Nano)
	} else {
		post.CreatedAt = args.CreatedAt.Format(time.RFC3339Nano)
	}

	if args.Languages == nil {
		args.Languages = []string{"en"}
	}

	post.Langs = args.Languages

	if args.Labels != nil {
		labels := []*atproto.LabelDefs_SelfLabel{}
		for _, label := range args.Labels {
			labels = append(labels, &atproto.LabelDefs_SelfLabel{
				Val: label,
			})
		}
		post.Labels = &bsky.FeedPost_Labels{
			LabelDefs_SelfLabels: &atproto.LabelDefs_SelfLabels{
				Values: labels,
			},
		}
	}

	if args.Images != nil {
		images := []*bsky.EmbedImages_Image{}
		for _, image := range args.Images {
			ref, err := c.UploadImage(ctx, image.Reader)
			if err != nil {
				return nil, fmt.Errorf("failed to upload image: %w", err)
			}
			images = append(images, &bsky.EmbedImages_Image{
				Image: ref,
				Alt:   image.AltText,
			})
		}
		post.Embed = &bsky.FeedPost_Embed{
			EmbedImages: &bsky.EmbedImages{
				Images: images,
			},
		}
	}

	if args.ReplyTo != nil {
		postCid, rootURI, rootCid, err := c.resolveRoot(ctx, *args.ReplyTo)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve root for reply: %w", err)
		}

		post.Reply = &bsky.FeedPost_ReplyRef{
			Parent: &atproto.RepoStrongRef{
				Uri: args.ReplyTo.String(),
				Cid: *postCid,
			},
			Root: &atproto.RepoStrongRef{
				Uri: rootURI.String(),
				Cid: *rootCid,
			},
		}
	}

	if args.Quoting != nil {
		postCid, _, _, err := c.resolveRoot(ctx, *args.Quoting)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve root for quote: %w", err)
		}
		if post.Embed == nil {
			post.Embed = &bsky.FeedPost_Embed{}
		}
		post.Embed.EmbedRecord = &bsky.EmbedRecord{
			Record: &atproto.RepoStrongRef{
				Uri: args.Quoting.String(),
				Cid: *postCid,
			},
		}
	}

	if args.EmbeddedLink != "" {
		if post.Embed == nil {
			post.Embed = &bsky.FeedPost_Embed{}
		}

		externalMeta, err := c.getExternalMetadata(ctx, args.EmbeddedLink)
		if err != nil {
			return nil, fmt.Errorf("failed to get external metadata: %w", err)
		}

		var imageRef *util.LexBlob
		if externalMeta.Image != nil {
			imageRef, err = c.UploadImage(ctx, externalMeta.Image.Reader)
			if err != nil {
				return nil, fmt.Errorf("failed to upload external metadata image: %w", err)
			}
		}

		post.Embed.EmbedExternal = &bsky.EmbedExternal{
			External: &bsky.EmbedExternal_External{
				Uri:         args.EmbeddedLink,
				Title:       externalMeta.Title,
				Description: externalMeta.Description,
				Thumb:       imageRef,
			},
		}
	}

	if c.rl != nil {
		c.rl.Wait(ctx)
	}
	resp, err := comatproto.RepoCreateRecord(ctx, c.xrpcc, &comatproto.RepoCreateRecord_Input{
		Collection: "app.bsky.feed.post",
		Repo:       c.ActorDID.String(),
		Record:     &util.LexiconTypeDecoder{Val: &post},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create post: %w", err)
	}

	postURI, err := syntax.ParseATURI(resp.Uri)
	if err != nil {
		return nil, fmt.Errorf("failed to parse post URI from creation response: %w", err)
	}

	return &postURI, nil
}

// UploadImage uploads an image to the PDS
func (c *Client) UploadImage(ctx context.Context, image io.Reader) (*util.LexBlob, error) {
	if c.rl != nil {
		c.rl.Wait(ctx)
	}
	blob, err := comatproto.RepoUploadBlob(ctx, c.xrpcc, image)
	if err != nil {
		return nil, fmt.Errorf("failed to upload image: %w", err)
	}

	return blob.Blob, nil
}

type externalMetadata struct {
	Title       string
	Description string
	Image       *Image
}

func (c *Client) getExternalMetadata(ctx context.Context, url string) (*externalMetadata, error) {
	cl := http.Client{}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/51.0.2704.103 Safari/537.36")

	resp, err := cl.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get external metadata: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to get external metadata: status code %d", resp.StatusCode)
	}

	info := htmlinfo.NewHTMLInfo()
	err = info.Parse(resp.Body, &url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to parse external metadata: %w", err)
	}

	// Trim title and description to 500 characters
	if len(info.Title) > 500 {
		info.Title = info.Title[:500]
	}

	if len(info.Description) > 500 {
		info.Description = info.Description[:500]
	}

	meta := externalMetadata{
		Title:       info.Title,
		Description: info.Description,
	}

	if info.ImageSrcURL != "" {
		req, err := http.NewRequestWithContext(ctx, "GET", info.ImageSrcURL, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create request for external metadata image: %w", err)
		}

		req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/51.0.2704.103 Safari/537.36")

		resp, err := cl.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to get external metadata image: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("failed to get external metadata image: status code %d", resp.StatusCode)
		}

		buf := make([]byte, resp.ContentLength)
		_, err = resp.Body.Read(buf)
		if err != nil {
			return nil, fmt.Errorf("failed to read external metadata image: %w", err)
		}

		meta.Image = &Image{
			Reader: bytes.NewReader(buf),
		}
	}

	return &meta, nil
}

func (c *Client) resolveRoot(ctx context.Context, uri syntax.ATURI) (postCid *string, rootURI *syntax.ATURI, rootCid *string, err error) {
	if uri.Collection().String() != "app.bsky.feed.post" {
		return nil, nil, nil, fmt.Errorf("uri is not a post")
	}

	if c.rl != nil {
		c.rl.Wait(ctx)
	}

	resp, err := comatproto.RepoGetRecord(ctx, c.xrpcc, "", uri.Collection().String(), uri.Authority().String(), uri.RecordKey().String())
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to resolve root: %w", err)
	}

	postCid = resp.Cid

	post, ok := resp.Value.Val.(*bsky.FeedPost)
	if !ok {
		return nil, nil, nil, fmt.Errorf("failed to resolve root: record at %s couldn't be decoded as a post", uri)
	}

	if post.Reply == nil || post.Reply.Root == nil {
		rootCid = postCid
		rootURI = &uri
		return postCid, rootURI, rootCid, nil
	}

	rURI, err := syntax.ParseATURI(post.Reply.Root.Uri)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to parse root URI from record: %w", err)
	}

	rootURI = &rURI
	rootCid = &post.Reply.Root.Cid

	return postCid, rootURI, rootCid, nil

}
