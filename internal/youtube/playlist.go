package youtube

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type PublicPlaylist struct {
	Playlist PlaylistResult `json:"playlist"`
	Videos   []VideoItem    `json:"videos"`
	NextPage *string        `json:"nextpage"`
}

var ErrNotPlaylist = errors.New("not a YouTube playlist URL")

func PlaylistID(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	return u.Query().Get("list")
}

// Playlist reads a public playlist page (browse VL<id>), or the page a token points to.
func (c *Client) Playlist(ctx context.Context, raw, page string) (PublicPlaylist, error) {
	out := PublicPlaylist{Videos: []VideoItem{}}
	id := PlaylistID(raw)
	if id == "" {
		return out, ErrNotPlaylist
	}
	out.Playlist = PlaylistResult{ID: id, URL: "https://www.youtube.com/playlist?list=" + id, PlaylistType: "NORMAL"}
	if strings.HasPrefix(id, "RD") {
		out.Playlist.PlaylistType = "MIX_STREAM"
	}
	var res map[string]any
	var err error
	if page != "" {
		token, ok := decodePage(page)
		if !ok {
			return out, ErrBadPage
		}
		res, err = c.Call(ctx, Web, "browse", map[string]any{"continuation": token})
	} else {
		res, err = c.Call(ctx, Web, "browse", map[string]any{"browseId": "VL" + id})
	}
	if err != nil {
		return out, err
	}
	if h := Obj(res, "header", "pageHeaderRenderer"); h != nil {
		out.Playlist.Title = Str(h, "pageTitle")
		vm := Obj(h, "content", "pageHeaderViewModel")
		for _, row := range List(vm, "metadata", "contentMetadataViewModel", "metadataRows") {
			for _, p := range List(row, "metadataParts") {
				t := Str(p, "text", "content")
				switch {
				case mentionsVideos(t):
					out.Playlist.StreamCount = Digits(t)
				case out.Playlist.UploaderName == "" && !mentionsViews(t) && !mentionsPlaylist(t) &&
					Str(p, "avatarStack", "avatarStackViewModel", "text", "content") == "":
					out.Playlist.UploaderName = playlistOwner(t)
				}
				if n := Str(p, "avatarStack", "avatarStackViewModel", "text", "content"); n != "" {
					out.Playlist.UploaderName = playlistOwner(n)
				}
			}
		}
		out.Playlist.ThumbnailURL = LastURL(List(vm, "heroImage", "contentPreviewImageViewModel", "image", "sources"))
	}
	if h := Obj(res, "header", "playlistHeaderRenderer"); h != nil {
		out.Playlist.Title = Text(h["title"])
		out.Playlist.UploaderName = Text(h["ownerText"])
		out.Playlist.StreamCount = Digits(Text(h["numVideosText"]))
	}
	var items []any
	for _, t := range List(res, "contents", "twoColumnBrowseResultsRenderer", "tabs") {
		for _, sec := range List(t, "tabRenderer", "content", "sectionListRenderer", "contents") {
			for _, it := range List(sec, "itemSectionRenderer", "contents") {
				if Obj(it, "lockupViewModel") != nil {
					items = append(items, it)
					continue
				}
				items = append(items, List(it, "playlistVideoListRenderer", "contents")...)
			}
			if t := Str(sec, "continuationItemRenderer", "continuationEndpoint", "continuationCommand", "token"); t != "" {
				items = append(items, sec)
			}
		}
	}
	for _, a := range List(res, "onResponseReceivedActions") {
		items = append(items, List(a, "appendContinuationItemsAction", "continuationItems")...)
	}
	now := time.Now()
	for _, it := range items {
		if t := Str(it, "continuationItemRenderer", "continuationEndpoint", "continuationCommand", "token"); t != "" {
			out.NextPage = encodePage(t)
			continue
		}
		if l := Obj(it, "lockupViewModel"); l != nil {
			if v, ok := FromLockup(l, now); ok {
				out.Videos = append(out.Videos, v)
			}
			continue
		}
		p := Obj(it, "playlistVideoRenderer")
		if p == nil {
			continue
		}
		v, ok := FromVideoRenderer(p, now)
		if !ok {
			continue
		}
		if secs, err := strconv.ParseInt(Str(p, "lengthSeconds"), 10, 64); err == nil {
			v.Duration = secs
		}
		for _, run := range List(p, "videoInfo", "runs") {
			t := Str(run, "text")
			if mentionsViews(t) {
				if n, ok := MixedNumber(t); ok {
					v.ViewCount = n
				}
			} else if looksLikeDate(t, now) {
				setDates(&v, t, now.UTC())
			}
		}
		out.Videos = append(out.Videos, v)
	}
	if out.Playlist.ThumbnailURL == "" && len(out.Videos) > 0 {
		out.Playlist.ThumbnailURL = out.Videos[0].ThumbnailURL
	}
	return out, nil
}
