package api

import (
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"typetype.dev/server/internal/db"
	"typetype.dev/server/internal/httpx"
	"typetype.dev/server/internal/youtube"
)

type notificationItem struct {
	ID               string            `json:"id"`
	Type             string            `json:"type"`
	Title            string            `json:"title"`
	CreatedAt        int64             `json:"createdAt"`
	PublishedAt      int64             `json:"publishedAt"`
	ChannelURL       string            `json:"channelUrl"`
	ChannelName      string            `json:"channelName"`
	ChannelAvatarURL string            `json:"channelAvatarUrl"`
	ServiceID        int               `json:"serviceId"`
	ServiceName      string            `json:"serviceName"`
	Read             bool              `json:"read"`
	Video            youtube.VideoItem `json:"video"`
	Live             *twitchLiveNotice `json:"live,omitempty"`
}

var unreadCache sync.Map

func videoServiceID(v youtube.VideoItem) int {
	u, _ := url.Parse(v.URL)
	host := ""
	if u != nil {
		host = strings.ToLower(u.Hostname())
	}
	switch {
	case host == "b23.tv" || host == "bilibili.com" || strings.HasSuffix(host, ".bilibili.com"):
		return 5
	case host == "nico.ms" || host == "nicovideo.jp" || strings.HasSuffix(host, ".nicovideo.jp"):
		return 6
	}
	return 0
}

func publishedMillis(v youtube.VideoItem) int64 {
	val := int64(0)
	if v.PublishedAt != nil && *v.PublishedAt > 0 {
		val = *v.PublishedAt
	} else if v.Uploaded > 0 {
		val = v.Uploaded
	}
	if val > 0 && val <= 9_999_999_999 {
		return val * 1000
	}
	return val
}

func serviceName(id int) string {
	switch id {
	case 0:
		return "YouTube"
	case 5:
		return "BiliBili"
	case 6:
		return "NicoNico"
	}
	return "Video service"
}

// notifications builds the notification list from the subscription feed: new videos of the channels the user
// switched notifications on for, one per video, newest first, marked read from the stored read items and the old
// watermark. ok is false while no feed exists yet.
// notifications returns the user's items, shared and read-only, and whether each one is read.
func (s *Server) notifications(r *http.Request, user string) ([]notificationItem, []bool, bool, error) {
	ctx := r.Context()
	s.feeds.mu.Lock()
	snap := s.feeds.current[user]
	s.feeds.touched[user] = time.Now()
	s.feeds.mu.Unlock()
	if snap == nil {
		s.feeds.refresh(user)
		return nil, nil, false, nil
	}
	enabled := map[string]bool{}
	rows, err := s.DB.Query(ctx, "SELECT channel_url FROM channel_notification_preferences WHERE user_id = $1 AND enabled", user)
	if err != nil {
		return nil, nil, true, err
	}
	for rows.Next() {
		var u string
		if rows.Scan(&u) == nil {
			enabled[db.CanonicalChannelURL(u)] = true
		}
	}
	rows.Close()
	items := snap.notificationItems(enabled)
	if slices.Contains(r.URL.Query()["include"], twitchLiveType) {
		live, err := s.twitchLiveItems(ctx, user)
		if err != nil {
			return nil, nil, true, err
		}
		if len(live) > 0 {
			items = mergeNewest(items, live)
		}
	}
	read := map[string]bool{}
	rrows, err := s.DB.Query(ctx, "SELECT notification_id FROM notification_read_items WHERE user_id = $1", user)
	if err != nil {
		return nil, nil, true, err
	}
	for rrows.Next() {
		var id string
		if rrows.Scan(&id) == nil {
			read[id] = true
		}
	}
	rrows.Close()
	var watermark int64
	s.DB.QueryRow(ctx, "SELECT subscription_last_seen_uploaded FROM notification_states WHERE user_id = $1", user).Scan(&watermark)
	unread := 0
	seen := make([]bool, len(items))
	for i := range items {
		seen[i] = read[items[i].ID] || items[i].CreatedAt <= watermark
		if !seen[i] {
			unread++
		}
	}
	unreadCache.Store(user, unread)
	return items, seen, true, nil
}

func cachedUnread(user string) int {
	if v, ok := unreadCache.Load(user); ok {
		return v.(int)
	}
	return 0
}

func (s *Server) notificationRoutes(rt *Router) {
	rt.Handle("GET", "/notifications/unread-count", s.authed(func(w http.ResponseWriter, r *http.Request, user string) {
		_, read, ok, err := s.notifications(r, user)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		n := cachedUnread(user)
		if ok {
			n = 0
			for _, seen := range read {
				if !seen {
					n++
				}
			}
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"unreadCount": n, "available": ok})
	}))
	rt.Handle("GET", "/notifications", s.authed(func(w http.ResponseWriter, r *http.Request, user string) {
		page, limit := 0, 20
		if n, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil {
			page = max(0, min(n, 10000))
		}
		if n, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil {
			limit = max(1, min(n, 100))
		}
		items, read, ok, err := s.notifications(r, user)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		out := map[string]any{"items": []notificationItem{}, "unreadCount": cachedUnread(user), "nextpage": nil, "available": ok}
		from := page * limit
		if ok && from < len(items) {
			to := min(from+limit, len(items))
			pageItems := slices.Clone(items[from:to])
			for i := range pageItems {
				pageItems[i].Read = read[from+i]
			}
			out["items"] = pageItems
			if to < len(items) {
				out["nextpage"] = strconv.Itoa(page + 1)
			}
		}
		httpx.JSON(w, http.StatusOK, out)
	}))
	rt.Handle("POST", "/notifications/read-all", s.authed(func(w http.ResponseWriter, r *http.Request, user string) {
		items, _, ok, err := s.notifications(r, user)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		now := time.Now().UnixMilli()
		if !ok {
			httpx.JSON(w, http.StatusOK, map[string]any{"readAt": now, "unreadCount": cachedUnread(user), "available": false})
			return
		}
		latest := int64(0)
		for _, it := range items {
			s.DB.Exec(r.Context(), "INSERT INTO notification_read_items (user_id, notification_id, read_at) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING", user, it.ID, now)
			latest = max(latest, it.CreatedAt)
		}
		s.DB.Exec(r.Context(), `INSERT INTO notification_states (user_id, subscription_last_seen_uploaded, updated_at) VALUES ($1, $2, $3)
ON CONFLICT (user_id) DO UPDATE SET subscription_last_seen_uploaded = EXCLUDED.subscription_last_seen_uploaded, updated_at = EXCLUDED.updated_at`, user, latest, now)
		unreadCache.Store(user, 0)
		httpx.JSON(w, http.StatusOK, map[string]any{"readAt": now, "unreadCount": 0, "available": true})
	}))
	rt.Handle("POST", "/notifications/{notificationId}/read", s.authed(func(w http.ResponseWriter, r *http.Request, user string) {
		id := r.PathValue("notificationId")
		if strings.TrimSpace(id) == "" {
			plain(w, r, http.StatusBadRequest, "Missing notificationId")
			return
		}
		items, read, ok, err := s.notifications(r, user)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		now := time.Now().UnixMilli()
		if !ok {
			httpx.JSON(w, http.StatusOK, map[string]any{"readAt": now, "unreadCount": cachedUnread(user), "available": false})
			return
		}
		unread := 0
		for i, it := range items {
			if it.ID == id && !read[i] {
				s.DB.Exec(r.Context(), "INSERT INTO notification_read_items (user_id, notification_id, read_at) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING", user, id, now)
				continue
			}
			if !read[i] {
				unread++
			}
		}
		unreadCache.Store(user, unread)
		httpx.JSON(w, http.StatusOK, map[string]any{"readAt": now, "unreadCount": unread, "available": true})
	}))
	s.preferenceRoutes(rt)
	s.pushRoutes(rt)
}

type channelPreference struct {
	ChannelURL string `json:"channelUrl"`
	Enabled    bool   `json:"enabled"`
	UpdatedAt  int64  `json:"updatedAt"`
}

func (s *Server) preferences(r *http.Request, user string) ([]channelPreference, error) {
	subs, err := s.subscriptions(r, user, "", false, false)
	if err != nil {
		return nil, err
	}
	stored := map[string]channelPreference{}
	rows, err := s.DB.Query(r.Context(), "SELECT channel_url, enabled, updated_at FROM channel_notification_preferences WHERE user_id = $1", user)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var p channelPreference
		if rows.Scan(&p.ChannelURL, &p.Enabled, &p.UpdatedAt) == nil {
			stored[p.ChannelURL] = p
		}
	}
	rows.Close()
	out := []channelPreference{}
	for _, sub := range subs {
		c := db.CanonicalChannelURL(sub.ChannelURL)
		p, ok := stored[c]
		if !ok {
			p = channelPreference{ChannelURL: c}
		}
		out = append(out, p)
	}
	return out, nil
}

func (s *Server) setPreference(r *http.Request, user, channel string, enabled bool, now int64) error {
	_, err := s.DB.Exec(r.Context(), `INSERT INTO channel_notification_preferences (user_id, channel_url, enabled, updated_at) VALUES ($1, $2, $3, $4)
ON CONFLICT (user_id, channel_url) DO UPDATE SET enabled = EXCLUDED.enabled, updated_at = EXCLUDED.updated_at`, user, channel, enabled, now)
	return err
}

func (s *Server) preferenceRoutes(rt *Router) {
	rt.Handle("GET", "/notifications/channel-preferences", s.authed(func(w http.ResponseWriter, r *http.Request, user string) {
		out, err := s.preferences(r, user)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, out)
	}))
	rt.Handle("PUT", "/notifications/channel-preferences", s.authed(func(w http.ResponseWriter, r *http.Request, user string) {
		var req struct {
			ChannelURL *string `json:"channelUrl"`
			Enabled    *bool   `json:"enabled"`
		}
		if !decodeCoded(w, r, &req, "Invalid channel notification preference", "push_request_invalid") {
			return
		}
		if req.ChannelURL == nil || req.Enabled == nil {
			httpx.Error(w, r, http.StatusBadRequest, "Invalid channel notification preference", "push_request_invalid")
			return
		}
		c := db.CanonicalChannelURL(*req.ChannelURL)
		prefs, err := s.preferences(r, user)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		subscribed := false
		for _, p := range prefs {
			subscribed = subscribed || p.ChannelURL == c
		}
		if !subscribed {
			httpx.Error(w, r, http.StatusConflict, "Channel is not subscribed", "channel_not_subscribed")
			return
		}
		now := time.Now().UnixMilli()
		if err := s.setPreference(r, user, c, *req.Enabled, now); err != nil {
			s.fail(w, r, err)
			return
		}
		if !*req.Enabled {
			s.clearPendingPush(r.Context(), user, c)
		}
		httpx.JSON(w, http.StatusOK, channelPreference{ChannelURL: c, Enabled: *req.Enabled, UpdatedAt: now})
	}))
	rt.Handle("PUT", "/notifications/channel-preferences/bulk", s.authed(func(w http.ResponseWriter, r *http.Request, user string) {
		var req struct {
			Enabled     *bool     `json:"enabled"`
			ChannelURLs *[]string `json:"channelUrls"`
		}
		if !decodeCoded(w, r, &req, "Invalid channel notification preferences", "push_request_invalid") {
			return
		}
		if req.Enabled == nil {
			httpx.Error(w, r, http.StatusBadRequest, "Invalid channel notification preferences", "push_request_invalid")
			return
		}
		prefs, err := s.preferences(r, user)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		subscribed := map[string]bool{}
		var targets []string
		for _, p := range prefs {
			subscribed[p.ChannelURL] = true
			if req.ChannelURLs == nil {
				targets = append(targets, p.ChannelURL)
			}
		}
		if req.ChannelURLs != nil {
			seen := map[string]bool{}
			for _, u := range *req.ChannelURLs {
				c := db.CanonicalChannelURL(u)
				if subscribed[c] && !seen[c] {
					seen[c] = true
					targets = append(targets, c)
				}
			}
		}
		now := time.Now().UnixMilli()
		out := []channelPreference{}
		for _, c := range targets {
			if err := s.setPreference(r, user, c, *req.Enabled, now); err != nil {
				s.fail(w, r, err)
				return
			}
			out = append(out, channelPreference{ChannelURL: c, Enabled: *req.Enabled, UpdatedAt: now})
		}
		if !*req.Enabled {
			for _, p := range out {
				s.clearPendingPush(r.Context(), user, p.ChannelURL)
			}
		}
		httpx.JSON(w, http.StatusOK, out)
	}))
}
