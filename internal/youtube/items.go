package youtube

import (
	"strings"
	"time"
)

// VideoItem is the API VideoItem, field for field.
type VideoItem struct {
	ID                 string  `json:"id"`
	Title              string  `json:"title"`
	URL                string  `json:"url"`
	ThumbnailURL       string  `json:"thumbnailUrl"`
	UploaderName       string  `json:"uploaderName"`
	UploaderURL        string  `json:"uploaderUrl"`
	UploaderAvatarURL  string  `json:"uploaderAvatarUrl"`
	Duration           int64   `json:"duration"`
	ViewCount          int64   `json:"viewCount"`
	UploadDate         string  `json:"uploadDate"`
	Uploaded           int64   `json:"uploaded"`
	StreamType         string  `json:"streamType"`
	IsShortFormContent bool    `json:"isShortFormContent"`
	UploaderVerified   bool    `json:"uploaderVerified"`
	ShortDescription   *string `json:"shortDescription"`
	PublishedAt        *int64  `json:"publishedAt"`
	IsLive             bool    `json:"isLive"`
	IsPostLive         bool    `json:"isPostLive"`
	IsLiveContent      bool    `json:"isLiveContent"`
	RequiresMembership bool    `json:"requiresMembership"`
}

func WatchURL(id string) string { return "https://www.youtube.com/watch?v=" + id }

func ChannelURL(id string) string { return "https://www.youtube.com/channel/" + id }

// endpointURL reads a navigation endpoint the way getUrlFromNavigationEndpoint does for channels and videos.
func endpointURL(ep any) string {
	if id := Str(ep, "browseEndpoint", "browseId"); strings.HasPrefix(id, "UC") {
		return ChannelURL(id)
	}
	if id := Str(ep, "watchEndpoint", "videoId"); id != "" {
		return WatchURL(id)
	}
	if u := Str(ep, "commandMetadata", "webCommandMetadata", "url"); strings.HasPrefix(u, "/") {
		return "https://www.youtube.com" + u
	}
	return ""
}

// durationSeconds reads "1:02:03" or "12:34".
func durationSeconds(s string) int64 {
	var total int64
	for _, part := range strings.Split(strings.TrimSpace(s), ":") {
		n := Digits(part)
		if part == "" {
			return -1
		}
		total = total*60 + n
	}
	return total
}

func setDates(it *VideoItem, text string, now time.Time) {
	it.UploadDate, it.Uploaded = text, -1
	if t, ok := TimeAgo(text, now); ok {
		it.Uploaded = t.UnixMilli()
	} else if t, ok := ParseDate(text); ok {
		it.Uploaded = t.UnixMilli()
	}
	if it.Uploaded > 0 {
		v := it.Uploaded
		it.PublishedAt = &v
	}
}

func isViewText(s string) bool {
	return mentionsViews(s) || mentionsWatching(s)
}

func liveTypes(it *VideoItem, live bool) {
	it.StreamType = "video_stream"
	if live {
		it.StreamType, it.IsLive, it.IsLiveContent = "live_stream", true, true
	}
}

// FromLockup reads a lockupViewModel video card (related videos, channel grids, home).
func FromLockup(l any, now time.Time) (VideoItem, bool) {
	if Str(l, "contentType") != "LOCKUP_CONTENT_TYPE_VIDEO" {
		return VideoItem{}, false
	}
	id := Str(l, "contentId")
	meta := Obj(l, "metadata", "lockupMetadataViewModel")
	it := VideoItem{ID: WatchURL(id), URL: WatchURL(id), Title: Str(meta, "title", "content"), Duration: -1, ViewCount: -1}
	thumb := Obj(l, "contentImage", "thumbnailViewModel")
	it.ThumbnailURL = LastURL(List(thumb, "image", "sources"))
	live := false
	for _, o := range List(thumb, "overlays") {
		for _, path := range [][]any{{"thumbnailBottomOverlayViewModel", "badges"}, {"thumbnailOverlayBadgeViewModel", "thumbnailBadges"}} {
			for _, b := range List(o, path...) {
				badge := Obj(b, "thumbnailBadgeViewModel")
				t := Str(badge, "text")
				switch {
				case Str(badge, "badgeStyle") == "THUMBNAIL_OVERLAY_BADGE_STYLE_LIVE" || strings.EqualFold(t, "LIVE"):
					live = true
				case strings.Contains(t, ":"):
					it.Duration = durationSeconds(t)
				}
			}
		}
	}
	liveTypes(&it, live)
	if live {
		it.Duration = -1
	}
	now = now.UTC()
	var uploaderCmd any
	for _, row := range List(meta, "metadata", "contentMetadataViewModel", "metadataRows") {
		for _, b := range List(row, "badges") {
			if Str(b, "badgeViewModel", "badgeStyle") == "BADGE_MEMBERS_ONLY" {
				it.RequiresMembership = true
			}
		}
		for _, p := range List(row, "metadataParts") {
			txt := Obj(p, "text")
			content := Str(txt, "content")
			switch {
			case content == "":
			case isViewText(content) || isViewText(Str(p, "accessibilityLabel")) || Str(p, "leadingIcon", "name") == "PLAY_ARROW_OUTLINED":
				if noViews(content) {
					it.ViewCount = 0
				} else if n, ok := MixedNumber(content); ok {
					it.ViewCount = n
				}
			case looksLikeDate(content, now):
				setDates(&it, content, now)
			case it.UploaderName == "":
				it.UploaderName = content
				if runs := List(txt, "commandRuns"); len(runs) > 0 {
					uploaderCmd = Dig(runs[0], "onTap", "innertubeCommand")
					it.UploaderURL = endpointURL(uploaderCmd)
				}
				for _, a := range List(txt, "attachmentRuns") {
					name := Str(a, "element", "type", "imageType", "image", "sources", 0, "clientResource", "imageName")
					if strings.Contains(name, "CHECK_CIRCLE") || strings.Contains(name, "AUDIO_BADGE") || strings.Contains(name, "VERIFIED") {
						it.UploaderVerified = true
					}
				}
			}
		}
	}
	img := Obj(meta, "image")
	if av := Obj(img, "decoratedAvatarViewModel"); av != nil {
		it.UploaderAvatarURL = FixURL(Str(av, "avatar", "avatarViewModel", "image", "sources", 0, "url"))
		if it.UploaderURL == "" {
			it.UploaderURL = endpointURL(Dig(av, "rendererContext", "commandContext", "onTap", "innertubeCommand"))
		}
	} else if st := Obj(img, "avatarStackViewModel"); st != nil {
		it.UploaderAvatarURL = FixURL(Str(st, "avatars", 0, "avatarViewModel", "image", "sources", 0, "url"))
		if uploaderCmd == nil {
			uploaderCmd = Dig(st, "rendererContext", "commandContext", "onTap", "innertubeCommand")
		}
	}
	firstCollaborator(&it, uploaderCmd)
	cmd := Obj(l, "rendererContext", "commandContext", "onTap", "innertubeCommand")
	it.IsShortFormContent = cmd["reelWatchEndpoint"] != nil ||
		Str(cmd, "commandMetadata", "webCommandMetadata", "webPageType") == "WEB_PAGE_TYPE_SHORTS"
	return it, it.Title != ""
}

// firstCollaborator is the uploader PipePipe gives a video several channels made together: the byline is a
// sentence YouTube writes in the request language ("A and B", "I-A ne-B" in Zulu), so the uploader is the first
// channel of the collaborators dialog.
func firstCollaborator(it *VideoItem, cmd any) {
	ep, _ := cmd.(map[string]any)
	if Obj(ep, "showDialogCommand") == nil {
		return
	}
	item := Obj(Dig(ep, "showDialogCommand", "panelLoadingStrategy", "inlineContent", "dialogViewModel", "customContent",
		"listViewModel", "listItems", 0), "listItemViewModel")
	if name := Str(item, "title", "content"); name != "" {
		it.UploaderName = name
	}
	tap := Dig(item, "title", "commandRuns", 0, "onTap", "innertubeCommand")
	if tap == nil {
		tap = Dig(item, "rendererContext", "commandContext", "onTap", "innertubeCommand")
	}
	if u := endpointURL(tap); u != "" {
		it.UploaderURL = u
	} else if u := LinkURL(ep); u != "" {
		it.UploaderURL = u
	}
	if av := FixURL(Str(item, "leadingAccessory", "avatarViewModel", "image", "sources", 0, "url")); av != "" {
		it.UploaderAvatarURL = av
	}
}

func looksLikeDate(s string, now time.Time) bool {
	if _, ok := TimeAgo(s, now); ok {
		return true
	}
	_, ok := ParseDate(s)
	return ok
}

// FromVideoRenderer reads the older videoRenderer and compactVideoRenderer cards (search, app clients).
func FromVideoRenderer(v any, now time.Time) (VideoItem, bool) {
	id := Str(v, "videoId")
	if id == "" {
		return VideoItem{}, false
	}
	it := VideoItem{ID: WatchURL(id), URL: WatchURL(id), Title: Text(Dig(v, "title")), Duration: -1, ViewCount: -1}
	it.ThumbnailURL = LastURL(List(v, "thumbnail", "thumbnails"))
	owner := Dig(v, "ownerText")
	if owner == nil {
		owner = Dig(v, "longBylineText")
	}
	if owner == nil {
		owner = Dig(v, "shortBylineText")
	}
	it.UploaderName = Text(owner)
	it.UploaderURL = endpointURL(Dig(owner, "runs", 0, "navigationEndpoint"))
	it.UploaderAvatarURL = LastURL(List(v, "channelThumbnailSupportedRenderers", "channelThumbnailWithLinkRenderer", "thumbnail", "thumbnails"))
	if it.UploaderAvatarURL == "" {
		it.UploaderAvatarURL = LastURL(List(v, "channelThumbnail", "thumbnails"))
	}
	firstCollaborator(&it, Dig(owner, "runs", 0, "navigationEndpoint"))
	live := false
	for _, b := range List(v, "badges") {
		if strings.Contains(Str(b, "metadataBadgeRenderer", "style"), "LIVE") {
			live = true
		}
		if Str(b, "metadataBadgeRenderer", "style") == "BADGE_STYLE_TYPE_MEMBERS_ONLY" {
			it.RequiresMembership = true
		}
	}
	for _, b := range List(v, "ownerBadges") {
		if strings.Contains(Str(b, "metadataBadgeRenderer", "style"), "VERIFIED") {
			it.UploaderVerified = true
		}
	}
	if d := Text(Dig(v, "lengthText")); d != "" {
		it.Duration = durationSeconds(d)
	} else {
		live = live || mentionsWatching(Text(Dig(v, "viewCountText")))
	}
	liveTypes(&it, live)
	views := Text(Dig(v, "viewCountText"))
	if noViews(views) {
		it.ViewCount = 0
	} else if n := Digits(views); n > 0 {
		it.ViewCount = n
	} else if n, ok := MixedNumber(Text(Dig(v, "shortViewCountText"))); ok {
		it.ViewCount = n
	}
	if d := Text(Dig(v, "publishedTimeText")); d != "" {
		setDates(&it, d, now.UTC())
	}
	if desc := Text(Dig(v, "detailedMetadataSnippets", 0, "snippetText")); desc != "" {
		it.ShortDescription = &desc
	}
	ep := Obj(v, "navigationEndpoint")
	it.IsShortFormContent = ep["reelWatchEndpoint"] != nil ||
		Str(ep, "commandMetadata", "webCommandMetadata", "webPageType") == "WEB_PAGE_TYPE_SHORTS"
	return it, it.Title != ""
}
