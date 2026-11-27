package youtube

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"sync"
	"time"
)

type Channel struct {
	Name            string      `json:"name"`
	Description     string      `json:"description"`
	AvatarURL       string      `json:"avatarUrl"`
	BannerURL       string      `json:"bannerUrl"`
	SubscriberCount int64       `json:"subscriberCount"`
	IsVerified      bool        `json:"isVerified"`
	Videos          []VideoItem `json:"videos"`
	NextPage        *string     `json:"nextpage"`
	Verification    *string     `json:"verification"`
	HasCourses      bool        `json:"hasCourses"`
	ID              string      `json:"-"`
}

var tabParams = map[string]string{
	"videos":  "EgZ2aWRlb3PyBgQKAjoA",
	"streams": "EgdzdHJlYW1z8gYECgJ6AA==",
	"shorts":  "EgZzaG9ydHPyBgUKA5oBAA==",
}

var ErrNotChannel = errors.New("not a YouTube channel URL")

// resolved remembers handle and custom URLs already turned into channel ids.
var resolved sync.Map

// ChannelTarget splits a channel URL into its browse id (resolving handles once) and the tab it points to.
func (c *Client) ChannelTarget(ctx context.Context, raw string) (string, string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !strings.Contains(u.Hostname(), "youtube.com") {
		return "", "", ErrNotChannel
	}
	segs := strings.FieldsFunc(u.Path, func(r rune) bool { return r == '/' })
	if len(segs) == 0 {
		return "", "", ErrNotChannel
	}
	tab := "videos"
	if last := segs[len(segs)-1]; len(segs) >= 2 && (last == "videos" || last == "streams" || last == "shorts" || last == "featured" ||
		last == "playlists" || last == "community" || last == "about" || last == "podcasts" || last == "releases") {
		if _, ok := tabParams[last]; ok {
			tab = last
		}
		segs = segs[:len(segs)-1]
	}
	if segs[0] == "channel" && len(segs) >= 2 {
		return segs[1], tab, nil
	}
	key := strings.ToLower(strings.Join(segs, "/"))
	if v, ok := resolved.Load(key); ok {
		return v.(string), tab, nil
	}
	res, err := c.Call(ctx, Web, "navigation/resolve_url", map[string]any{"url": "https://www.youtube.com/" + strings.Join(segs, "/")})
	if err != nil {
		return "", "", err
	}
	id := Str(res, "endpoint", "browseEndpoint", "browseId")
	if !strings.HasPrefix(id, "UC") {
		return "", "", ErrNotChannel
	}
	resolved.Store(key, id)
	return id, tab, nil
}

// Channel reads a channel header and the first page of one of its video tabs, or the page a token points to.
func (c *Client) Channel(ctx context.Context, raw, page string) (*Channel, error) {
	if page != "" {
		packed, ok := decodePage(page)
		parts := strings.SplitN(packed, "\n", 4)
		if !ok || len(parts) != 4 {
			return nil, ErrBadPage
		}
		res, err := c.call(ctx, Web, "browse", map[string]any{"continuation": parts[3]}, channelSkip)
		if err != nil {
			return nil, err
		}
		ch := &Channel{Videos: []VideoItem{}, SubscriberCount: -1, ID: parts[0], Name: parts[1], AvatarURL: parts[2]}
		var items []any
		for _, a := range List(res, "onResponseReceivedActions") {
			items = append(items, List(a, "appendContinuationItemsAction", "continuationItems")...)
		}
		c.fillGrid(ch, items)
		return ch, nil
	}
	id, tab, err := c.ChannelTarget(ctx, raw)
	if err != nil {
		return nil, err
	}
	res, err := c.call(ctx, Web, "browse", map[string]any{"browseId": id, "params": tabParams[tab]}, channelSkip)
	if err != nil {
		return nil, err
	}
	return c.channelPage(id, res), nil
}

// channelPage reads a channel's first browse page: its header and the selected tab's videos.
func (c *Client) channelPage(id string, res map[string]any) *Channel {
	ch := &Channel{Videos: []VideoItem{}, SubscriberCount: -1, ID: id}
	parseChannelHeader(ch, res)
	for _, t := range List(res, "contents", "twoColumnBrowseResultsRenderer", "tabs") {
		tr := Obj(t, "tabRenderer")
		if !Bool(tr, "selected") {
			continue
		}
		items := List(tr, "content", "richGridRenderer", "contents")
		if items == nil {
			items = List(tr, "content", "sectionListRenderer", "contents", 0, "itemSectionRenderer", "contents", 0, "gridRenderer", "items")
		}
		c.fillGrid(ch, items)
	}
	return ch
}

func (c *Client) fillGrid(ch *Channel, items []any) {
	now := time.Now()
	for _, it := range items {
		content := Dig(it, "richItemRenderer", "content")
		if content == nil {
			content = it
		}
		var v VideoItem
		ok := false
		switch {
		case Obj(content, "videoRenderer") != nil:
			v, ok = FromVideoRenderer(Obj(content, "videoRenderer"), now)
		case Obj(content, "gridVideoRenderer") != nil:
			v, ok = FromVideoRenderer(Obj(content, "gridVideoRenderer"), now)
		case Obj(content, "lockupViewModel") != nil:
			v, ok = FromLockup(Obj(content, "lockupViewModel"), now)
		case Obj(content, "shortsLockupViewModel") != nil:
			v, ok = fromShortsLockup(Obj(content, "shortsLockupViewModel"))
		}
		if t := Str(it, "continuationItemRenderer", "continuationEndpoint", "continuationCommand", "token"); t != "" {
			ch.NextPage = encodePage(ch.ID + "\n" + ch.Name + "\n" + ch.AvatarURL + "\n" + t)
		}
		if !ok {
			continue
		}
		// A video of the channel's own tab carries a byline only when several channels made it, and then only YouTube's
		// sentence ("A and B", "I-A ne-B" in Zulu) without a link: the channel shown is one of them.
		if v.UploaderURL == "" && ch.ID != "" {
			v.UploaderURL = ChannelURL(ch.ID)
			v.UploaderName = ""
		}
		if v.UploaderName == "" {
			v.UploaderName = ch.Name
		}
		if v.UploaderAvatarURL == "" {
			v.UploaderAvatarURL = ch.AvatarURL
		}
		ch.Videos = append(ch.Videos, v)
	}
}

func fromShortsLockup(s map[string]any) (VideoItem, bool) {
	id := Str(s, "onTap", "innertubeCommand", "reelWatchEndpoint", "videoId")
	if id == "" {
		return VideoItem{}, false
	}
	it := VideoItem{ID: WatchURL(id), URL: WatchURL(id), Title: Str(s, "overlayMetadata", "primaryText", "content"), Duration: -1,
		ViewCount: -1, Uploaded: -1, StreamType: "video_stream", IsShortFormContent: true}
	it.ThumbnailURL = LastURL(List(s, "thumbnail", "sources"))
	if n, ok := MixedNumber(Str(s, "overlayMetadata", "secondaryText", "content")); ok {
		it.ViewCount = n
	}
	return it, it.Title != ""
}

func parseChannelHeader(ch *Channel, res map[string]any) {
	meta := Obj(res, "metadata", "channelMetadataRenderer")
	ch.Name, ch.Description = Str(meta, "title"), Str(meta, "description")
	ch.AvatarURL = LastURL(List(meta, "avatar", "thumbnails"))
	h := Obj(res, "header", "pageHeaderRenderer", "content", "pageHeaderViewModel")
	if h != nil {
		if n := Str(h, "title", "dynamicTextViewModel", "text", "content"); n != "" {
			ch.Name = n
		}
		if a := LastURL(List(h, "image", "decoratedAvatarViewModel", "avatar", "avatarViewModel", "image", "sources")); a != "" {
			ch.AvatarURL = a
		}
		ch.BannerURL = LastURL(List(h, "banner", "imageBannerViewModel", "image", "sources"))
		for _, row := range List(h, "metadata", "contentMetadataViewModel", "metadataRows") {
			for _, p := range List(row, "metadataParts") {
				t := Str(p, "text", "content")
				if mentionsSubscribers(t) {
					if n, ok := MixedNumber(t); ok {
						ch.SubscriberCount = n
					}
				}
			}
		}
		for _, a := range List(h, "title", "dynamicTextViewModel", "text", "attachmentRuns") {
			name := Str(a, "element", "type", "imageType", "image", "sources", 0, "clientResource", "imageName")
			if strings.Contains(name, "CHECK_CIRCLE") || strings.Contains(name, "VERIFIED") || strings.Contains(name, "AUDIO_BADGE") ||
				strings.Contains(name, "MUSIC") {
				ch.IsVerified = true
			}
		}
	}
	if c4 := Obj(res, "header", "c4TabbedHeaderRenderer"); c4 != nil {
		ch.BannerURL = LastURL(List(c4, "banner", "thumbnails"))
		if n, ok := MixedNumber(Text(c4["subscriberCountText"])); ok {
			ch.SubscriberCount = n
		}
	}
}

var playlistTabParams = map[string]string{"playlists": "EglwbGF5bGlzdHPyBgQKAkIA", "courses": "Egdjb3Vyc2Vz8gYFCgOiAQA=",
	"podcasts": "Eghwb2RjYXN0c_IGBQoDugEA"}

type ChannelPlaylists struct {
	Playlists   []PlaylistResult `json:"playlists"`
	NextPage    *string          `json:"nextpage"`
	ChannelName string           `json:"-"`
}

// ChannelPlaylists lists a channel's playlists (or courses) tab, or the page a token points to.
func (c *Client) ChannelPlaylists(ctx context.Context, raw, tab, page string) (ChannelPlaylists, error) {
	out := ChannelPlaylists{Playlists: []PlaylistResult{}}
	var res map[string]any
	var err error
	if page != "" {
		token, ok := decodePage(page)
		if !ok {
			return out, ErrBadPage
		}
		res, err = c.Call(ctx, Web, "browse", map[string]any{"continuation": token})
	} else {
		id, _, terr := c.ChannelTarget(ctx, raw)
		if terr != nil {
			return out, terr
		}
		res, err = c.Call(ctx, Web, "browse", map[string]any{"browseId": id, "params": playlistTabParams[tab]})
	}
	if err != nil {
		return out, err
	}
	out.ChannelName = Str(res, "metadata", "channelMetadataRenderer", "title")
	var items []any
	for _, t := range List(res, "contents", "twoColumnBrowseResultsRenderer", "tabs") {
		tr := Obj(t, "tabRenderer")
		if !Bool(tr, "selected") {
			continue
		}
		for _, sec := range List(tr, "content", "sectionListRenderer", "contents") {
			for _, it := range List(sec, "itemSectionRenderer", "contents") {
				items = append(items, List(it, "gridRenderer", "items")...)
				items = append(items, List(it, "shelfRenderer", "content", "horizontalListRenderer", "items")...)
			}
		}
		items = append(items, List(tr, "content", "richGridRenderer", "contents")...)
	}
	for _, a := range List(res, "onResponseReceivedActions") {
		items = append(items, List(a, "appendContinuationItemsAction", "continuationItems")...)
	}
	for _, it := range items {
		if t := Str(it, "continuationItemRenderer", "continuationEndpoint", "continuationCommand", "token"); t != "" {
			out.NextPage = encodePage(t)
			continue
		}
		l := Obj(it, "lockupViewModel")
		if l == nil {
			l = Obj(it, "richItemRenderer", "content", "lockupViewModel")
		}
		if l != nil {
			if p, ok := playlistFromLockup(l); ok {
				out.Playlists = append(out.Playlists, p)
			}
			continue
		}
		if g := Obj(it, "gridPlaylistRenderer"); g != nil {
			id := Str(g, "playlistId")
			out.Playlists = append(out.Playlists, PlaylistResult{ID: id, Title: Text(g["title"]), URL: "https://www.youtube.com/playlist?list=" + id,
				ThumbnailURL: LastURL(List(g, "thumbnail", "thumbnails")), StreamCount: Digits(Text(g["videoCountShortText"])), PlaylistType: "NORMAL"})
		}
	}
	return out, nil
}
