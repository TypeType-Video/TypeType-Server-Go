package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5"
	"typetype.dev/server/internal/config"
	"typetype.dev/server/internal/httpx"
)

const (
	twitchLiveType      = "twitch_live_started"
	twitchLiveKeep      = 7 * 24 * time.Hour
	twitchLiveListLimit = 50
)

type twitchLiveStream struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	User  struct {
		Login       string `json:"login"`
		DisplayName string `json:"displayName"`
		Avatar      string `json:"avatarUrl"`
	} `json:"user"`
	Game *struct {
		Name string `json:"name"`
	} `json:"game"`
	StartedAt time.Time `json:"startedAt"`
}

type twitchLiveNotice struct {
	StreamID string `json:"streamId"`
	Login    string `json:"login"`
	Category string `json:"category"`
}

func (s *Server) startTwitchLivePoller() {
	if s.twitch == nil {
		return
	}
	interval := time.Duration(max(60, min(config.IntEnv("TYPE_TYPE_TWITCH_LIVE_NOTIFICATIONS_INTERVAL_SECONDS", 120), 3600))) * time.Second
	go func() {
		time.Sleep(15 * time.Second)
		for {
			ctx, cancel := context.WithTimeout(context.Background(), interval)
			s.pollTwitchLive(ctx)
			cancel()
			time.Sleep(interval)
		}
	}()
}

func (s *Server) pollTwitchLive(ctx context.Context) {
	rows, err := s.DB.Query(ctx, `SELECT a.user_id FROM twitch_accounts a
LEFT JOIN twitch_live_notification_settings n ON n.user_id = a.user_id WHERE n.enabled IS NOT FALSE`)
	if err != nil {
		slog.Warn("twitch_live_notifications", "event", "registry_failed", "error", err.Error())
		return
	}
	users, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		slog.Warn("twitch_live_notifications", "event", "registry_failed", "error", err.Error())
		return
	}
	for _, u := range users {
		if err := s.pollTwitchLiveUser(ctx, u); err != nil {
			slog.Warn("twitch_live_notifications", "event", "poll_failed", "user", u[:min(8, len(u))], "error", err.Error())
		}
	}
	s.DB.Exec(ctx, "DELETE FROM twitch_live_notifications WHERE created_at < $1", time.Now().Add(-twitchLiveKeep).UnixMilli())
}

// pollTwitchLiveUser records the followed streams that started after the user began receiving live notifications,
// once per stream: the streams already live then never show up.
func (s *Server) pollTwitchLiveUser(ctx context.Context, user string) error {
	res, ok := s.withTwitch(ctx, user, func(token string) liveResponse {
		return s.twitch.do(ctx, "GET", "/v1/me/live", url.Values{"first": {"100"}}, nil, token)
	})
	if !ok {
		return nil
	}
	if res.status != http.StatusOK {
		return fmt.Errorf("live service answered %d", res.status)
	}
	var page struct {
		Items []twitchLiveStream `json:"items"`
	}
	if err := json.Unmarshal(res.body, &page); err != nil {
		return err
	}
	now := time.Now().UnixMilli()
	if _, err := s.DB.Exec(ctx, `INSERT INTO twitch_live_notification_settings (user_id, enabled, watching_since, updated_at)
VALUES ($1, TRUE, $2, $2) ON CONFLICT (user_id) DO NOTHING`, user, now); err != nil {
		return err
	}
	var since int64
	if err := s.DB.QueryRow(ctx, "SELECT watching_since FROM twitch_live_notification_settings WHERE user_id = $1", user).Scan(&since); err != nil {
		return err
	}
	for _, st := range page.Items {
		started := st.StartedAt.UnixMilli()
		if st.ID == "" || st.User.Login == "" || started <= since {
			continue
		}
		category := ""
		if st.Game != nil {
			category = st.Game.Name
		}
		name := st.User.DisplayName
		if name == "" {
			name = st.User.Login
		}
		if _, err := s.DB.Exec(ctx, `INSERT INTO twitch_live_notifications (user_id, stream_id, channel_login, channel_name,
channel_avatar_url, title, category, started_at, created_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) ON CONFLICT DO NOTHING`,
			user, st.ID, st.User.Login, name, st.User.Avatar, st.Title, category, started, now); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) twitchLiveItems(ctx context.Context, user string) ([]notificationItem, error) {
	if s.twitch == nil {
		return nil, nil
	}
	rows, err := s.DB.Query(ctx, `SELECT l.stream_id, l.channel_login, l.channel_name, l.channel_avatar_url, l.title, l.category,
l.started_at, l.created_at FROM twitch_live_notifications l JOIN twitch_live_notification_settings n ON n.user_id = l.user_id
WHERE l.user_id = $1 AND n.enabled AND l.created_at >= $2 ORDER BY l.created_at DESC LIMIT $3`,
		user, time.Now().Add(-twitchLiveKeep).UnixMilli(), twitchLiveListLimit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []notificationItem
	for rows.Next() {
		var n twitchLiveNotice
		var it notificationItem
		if err := rows.Scan(&n.StreamID, &n.Login, &it.ChannelName, &it.ChannelAvatarURL, &it.Title, &n.Category,
			&it.PublishedAt, &it.CreatedAt); err != nil {
			return nil, err
		}
		sum := sha256.Sum256([]byte(twitchLiveType + "|" + n.StreamID))
		it.ID, it.Type, it.ServiceID, it.ServiceName = hex.EncodeToString(sum[:]), twitchLiveType, -1, "Twitch"
		it.ChannelURL = "https://www.twitch.tv/" + n.Login
		it.Live = &n
		items = append(items, it)
	}
	return items, rows.Err()
}

// mergeNewest merges two newest-first lists into a new one; feed items are shared, so neither input is changed.
func mergeNewest(a, b []notificationItem) []notificationItem {
	out := make([]notificationItem, 0, len(a)+len(b))
	i, j := 0, 0
	for i < len(a) || j < len(b) {
		if j == len(b) || i < len(a) && a[i].CreatedAt >= b[j].CreatedAt {
			out = append(out, a[i])
			i++
		} else {
			out = append(out, b[j])
			j++
		}
	}
	return out
}

func (s *Server) twitchLiveNotificationRoutes(rt *Router) {
	rt.Handle("GET", "/notifications/twitch-live", s.twitchSignedIn(func(w http.ResponseWriter, r *http.Request, user string) {
		enabled := true
		s.DB.QueryRow(r.Context(), "SELECT enabled FROM twitch_live_notification_settings WHERE user_id = $1", user).Scan(&enabled)
		httpx.JSON(w, http.StatusOK, map[string]any{"enabled": enabled})
	}))
	rt.Handle("PUT", "/notifications/twitch-live", s.twitchSignedIn(func(w http.ResponseWriter, r *http.Request, user string) {
		var req struct {
			Enabled *bool `json:"enabled"`
		}
		if !decode(w, r, &req) {
			return
		}
		if req.Enabled == nil {
			plain(w, r, http.StatusBadRequest, "Invalid request body")
			return
		}
		now := time.Now().UnixMilli()
		_, err := s.DB.Exec(r.Context(), `INSERT INTO twitch_live_notification_settings (user_id, enabled, watching_since, updated_at)
VALUES ($1, $2, $3, $3) ON CONFLICT (user_id) DO UPDATE SET enabled = EXCLUDED.enabled, updated_at = EXCLUDED.updated_at,
watching_since = CASE WHEN EXCLUDED.enabled AND NOT twitch_live_notification_settings.enabled THEN EXCLUDED.watching_since
ELSE twitch_live_notification_settings.watching_since END`, user, *req.Enabled, now)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		unreadCache.Delete(user)
		httpx.JSON(w, http.StatusOK, map[string]any{"enabled": *req.Enabled})
	}))
}
