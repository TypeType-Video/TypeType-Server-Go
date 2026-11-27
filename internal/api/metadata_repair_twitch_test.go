package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"typetype.dev/server/internal/db"
)

// TestRepairFillsSavedTwitchVideos needs PostgreSQL (TEST_DATABASE_URL): a favourite Twitch VOD saved by URL alone
// gets its title, cover, length and channel from TypeType-Live, and one Twitch removed keeps what it had.
func TestRepairFillsSavedTwitchVideos(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := db.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	live := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/videos/2370123456" {
			http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
			return
		}
		w.Write([]byte(`{"id":"2370123456","title":"Speedrun VOD","previewUrl":"https://p/v.jpg","lengthSeconds":5400,"views":42,
"owner":{"login":"runner","displayName":"Runner","avatarUrl":"https://a/r.png"},"createdAt":"2026-11-20T18:00:00Z"}`))
	}))
	defer live.Close()
	t.Setenv("TWITCH_SERVICE_URL", live.URL)
	t.Setenv("TWITCH_SERVICE_SECRET", "test-only-secret")
	t.Setenv("TWITCH_TOKEN_ENCRYPTION_KEY", "test-only-twitch-key-0123456789abcdef")
	s := &Server{DB: pool, twitch: newTwitch()}
	user := "twitch-vod-test-" + contentPlaybackNonce()
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM favorites WHERE user_id = $1", user) })
	kept, gone := twitchVideoPrefix+"2370123456", twitchVideoPrefix+"1111111111"
	now := time.Now().UnixMilli()
	for _, u := range []string{kept, gone} {
		if _, err := pool.Exec(ctx, `INSERT INTO favorites (user_id, video_url, favorited_at, title, thumbnail, duration, channel_name,
channel_url, channel_avatar, view_count, published_at) VALUES ($1, $2, $3, '', '', 0, '', '', '', 0, -1)`, user, u, now); err != nil {
			t.Fatal(err)
		}
	}
	pool.Exec(ctx, "UPDATE favorites SET title = 'Old title' WHERE user_id = $1 AND video_url = $2", user, gone)
	if n := s.repairBatch(user, savedTable{"favorites", "video_url"}, "", map[string]bool{}); n != 2 {
		t.Fatalf("looked up %d videos", n)
	}
	var title, thumb, channel, channelURL string
	var duration int64
	pool.QueryRow(ctx, "SELECT title, thumbnail, duration, channel_name, channel_url FROM favorites WHERE user_id = $1 AND video_url = $2",
		user, kept).Scan(&title, &thumb, &duration, &channel, &channelURL)
	if title != "Speedrun VOD" || thumb != "https://p/v.jpg" || duration != 5400 || channel != "Runner" || channelURL != "https://www.twitch.tv/runner" {
		t.Fatalf("kept VOD: %q %q %d %q %q", title, thumb, duration, channel, channelURL)
	}
	var oldTitle string
	var refreshed int64
	pool.QueryRow(ctx, "SELECT title, metadata_refreshed_at FROM favorites WHERE user_id = $1 AND video_url = $2", user, gone).Scan(&oldTitle, &refreshed)
	if oldTitle != "Old title" || refreshed == 0 {
		t.Fatalf("removed VOD: title %q refreshed %d", oldTitle, refreshed)
	}
}
