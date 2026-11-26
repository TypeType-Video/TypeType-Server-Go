package youtube

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type PlaylistResult struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	URL          string `json:"url"`
	ThumbnailURL string `json:"thumbnailUrl"`
	UploaderName string `json:"uploaderName"`
	StreamCount  int64  `json:"streamCount"`
	PlaylistType string `json:"playlistType"`
}

type ChannelResult struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	URL             string  `json:"url"`
	ThumbnailURL    string  `json:"thumbnailUrl"`
	Description     string  `json:"description"`
	SubscriberCount int64   `json:"subscriberCount"`
	StreamCount     int64   `json:"streamCount"`
	IsVerified      bool    `json:"isVerified"`
	Verification    *string `json:"verification"`
}

type SearchPage struct {
	Items            []VideoItem      `json:"items"`
	NextPage         *string          `json:"nextpage"`
	SearchSuggestion *string          `json:"searchSuggestion"`
	IsCorrected      bool             `json:"isCorrectedSearch"`
	Playlists        []PlaylistResult `json:"playlists"`
	Channels         []ChannelResult  `json:"channels"`
}

// Search runs a YouTube search with the search params PipePipe's filters encode.
func (c *Client) Search(ctx context.Context, query, params, page string) (SearchPage, error) {
	out := SearchPage{Items: []VideoItem{}, Playlists: []PlaylistResult{}, Channels: []ChannelResult{}}
	var res map[string]any
	var err error
	if page != "" {
		token, ok := decodePage(page)
		if !ok {
			return out, ErrBadPage
		}
		res, err = c.Call(ctx, Web, "search", map[string]any{"continuation": token})
	} else {
		body := map[string]any{"query": query}
		if params != "" {
			body["params"] = params
		}
		res, err = c.Call(ctx, Web, "search", body)
	}
	if err != nil {
		return out, err
	}
	var sections []any
	sections = append(sections, List(res, "contents", "twoColumnSearchResultsRenderer", "primaryContents", "sectionListRenderer", "contents")...)
	for _, cmd := range List(res, "onResponseReceivedCommands") {
		sections = append(sections, List(cmd, "appendContinuationItemsAction", "continuationItems")...)
	}
	now := time.Now()
	for _, sec := range sections {
		if t := Str(sec, "continuationItemRenderer", "continuationEndpoint", "continuationCommand", "token"); t != "" {
			out.NextPage = encodePage(t)
			continue
		}
		for _, it := range List(sec, "itemSectionRenderer", "contents") {
			switch {
			case Obj(it, "videoRenderer") != nil:
				if v, ok := FromVideoRenderer(Obj(it, "videoRenderer"), now); ok {
					out.Items = append(out.Items, v)
				}
			case Obj(it, "lockupViewModel") != nil:
				l := Obj(it, "lockupViewModel")
				if v, ok := FromLockup(l, now); ok {
					out.Items = append(out.Items, v)
				} else if p, ok := playlistFromLockup(l); ok {
					out.Playlists = append(out.Playlists, p)
				}
			case Obj(it, "channelRenderer") != nil:
				out.Channels = append(out.Channels, channelFromRenderer(Obj(it, "channelRenderer")))
			case Obj(it, "playlistRenderer") != nil:
				p := Obj(it, "playlistRenderer")
				id := Str(p, "playlistId")
				out.Playlists = append(out.Playlists, PlaylistResult{ID: id, Title: Text(p["title"]), URL: "https://www.youtube.com/playlist?list=" + id,
					ThumbnailURL: LastURL(List(p, "thumbnails", 0, "thumbnails")), UploaderName: Text(p["shortBylineText"]),
					StreamCount: Digits(Str(p, "videoCount")), PlaylistType: "NORMAL"})
			case Obj(it, "showingResultsForRenderer") != nil:
				s := Text(Dig(it, "showingResultsForRenderer", "correctedQuery"))
				out.SearchSuggestion, out.IsCorrected = &s, true
			case Obj(it, "didYouMeanRenderer") != nil:
				s := Text(Dig(it, "didYouMeanRenderer", "correctedQuery"))
				out.SearchSuggestion = &s
			}
		}
	}
	return out, nil
}

func playlistFromLockup(l map[string]any) (PlaylistResult, bool) {
	t := Str(l, "contentType")
	if t != "LOCKUP_CONTENT_TYPE_PLAYLIST" && t != "LOCKUP_CONTENT_TYPE_PODCAST" {
		return PlaylistResult{}, false
	}
	id := Str(l, "contentId")
	meta := Obj(l, "metadata", "lockupMetadataViewModel")
	p := PlaylistResult{ID: id, Title: Str(meta, "title", "content"), URL: "https://www.youtube.com/playlist?list=" + id, PlaylistType: "NORMAL"}
	if strings.HasPrefix(id, "RD") {
		p.PlaylistType = "MIX_STREAM"
	}
	img := Obj(l, "contentImage", "collectionThumbnailViewModel", "primaryThumbnail", "thumbnailViewModel")
	p.ThumbnailURL = LastURL(List(img, "image", "sources"))
	for _, o := range List(img, "overlays") {
		for _, b := range List(o, "thumbnailOverlayBadgeViewModel", "thumbnailBadges") {
			if n := Digits(Str(b, "thumbnailBadgeViewModel", "text")); n > 0 {
				p.StreamCount = n
			}
		}
	}
	for _, row := range List(meta, "metadata", "contentMetadataViewModel", "metadataRows") {
		for _, part := range List(row, "metadataParts") {
			t := Str(part, "text", "content")
			l := strings.ToLower(t)
			if p.UploaderName == "" && t != "" && !strings.HasPrefix(l, "view ") && !strings.HasPrefix(l, "buka ") &&
				!strings.Contains(l, "updated") && !mentionsVideos(l) && !strings.Contains(l, "episode") && !mentionsPlaylist(l) {
				p.UploaderName = t
			}
		}
	}
	return p, p.Title != ""
}

func channelFromRenderer(c map[string]any) ChannelResult {
	id := Str(c, "channelId")
	r := ChannelResult{ID: ChannelURL(id), Name: Text(c["title"]), URL: ChannelURL(id), ThumbnailURL: LastURL(List(c, "thumbnail", "thumbnails")),
		Description: Text(c["descriptionSnippet"]), SubscriberCount: -1, StreamCount: -1}
	for _, k := range []string{"videoCountText", "subscriberCountText"} {
		t := Text(c[k])
		if mentionsSubscribers(t) {
			if n, ok := MixedNumber(t); ok {
				r.SubscriberCount = n
			}
		} else if mentionsVideos(t) {
			r.StreamCount = Digits(t)
		}
	}
	for _, b := range List(c, "ownerBadges") {
		st := Str(b, "metadataBadgeRenderer", "style")
		if strings.Contains(st, "VERIFIED") {
			r.IsVerified = true
		}
	}
	return r
}

// Suggestions asks the same endpoint YouTube's search box does, like YoutubeSuggestionExtractor.
func (c *Client) Suggestions(ctx context.Context, query string) ([]string, error) {
	u := "https://suggestqueries.google.com/complete/search?client=youtube&jsonp=JP&ds=yt&gl=US&q=" + url.QueryEscape(query)
	req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
	req.Header.Set("User-Agent", Web.UA)
	res, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	c.noteStatus(res.StatusCode)
	b, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	s := strings.TrimSpace(string(b))
	s = strings.TrimSuffix(strings.TrimPrefix(s, "JP("), ")")
	var arr []any
	if err := json.Unmarshal([]byte(s), &arr); err != nil {
		return nil, err
	}
	out := []string{}
	for _, e := range List(arr, 1) {
		if v, ok := Dig(e, 0).(string); ok {
			out = append(out, v)
		}
	}
	return out, nil
}

// Trending reads the first tab of YouTube's trending page, like YoutubeTrendingExtractor.
func (c *Client) Trending(ctx context.Context) ([]VideoItem, error) {
	res, err := c.Call(ctx, Web, "browse", map[string]any{"browseId": "UC4R8DWoMoI7CAwX8_LjQHig"})
	if err != nil {
		return nil, err
	}
	out := []VideoItem{}
	now := time.Now()
	tabs := List(res, "contents", "twoColumnBrowseResultsRenderer", "tabs")
	for _, t := range tabs {
		tr := Obj(t, "tabRenderer")
		if tr == nil {
			continue
		}
		content := Obj(tr, "content")
		seen := map[string]bool{}
		add := func(v VideoItem, ok bool) {
			if ok && !seen[v.URL] {
				seen[v.URL] = true
				out = append(out, v)
			}
		}
		for _, it := range List(content, "richGridRenderer", "contents") {
			add(FromVideoRenderer(Obj(it, "richItemRenderer", "content", "videoRenderer"), now))
			for _, sub := range List(it, "richSectionRenderer", "content", "richShelfRenderer", "contents") {
				add(FromVideoRenderer(Obj(sub, "richItemRenderer", "content", "videoRenderer"), now))
			}
		}
		for _, sec := range List(content, "sectionListRenderer", "contents") {
			for _, item := range List(sec, "itemSectionRenderer", "contents") {
				for _, v := range List(item, "shelfRenderer", "content", "expandedShelfContentsRenderer", "items") {
					if vi, ok := FromVideoRenderer(Obj(v, "videoRenderer"), now); ok {
						out = append(out, vi)
					}
				}
			}
		}
		break
	}
	return out, nil
}
