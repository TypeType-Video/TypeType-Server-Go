package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"typetype.dev/server/internal/db"
)

func TestMergeNewestKeepsBothListsInOrder(t *testing.T) {
	a := []notificationItem{{ID: "a1", CreatedAt: 50}, {ID: "a2", CreatedAt: 10}}
	b := []notificationItem{{ID: "b1", CreatedAt: 30}}
	got := mergeNewest(a, b)
	if len(got) != 3 || got[0].ID != "a1" || got[1].ID != "b1" || got[2].ID != "a2" {
		t.Fatalf("merged %+v", got)
	}
	if a[1].ID != "a2" || len(a) != 2 {
		t.Fatalf("the feed list changed: %+v", a)
	}
}

// TestTwitchLiveNotifications needs PostgreSQL (TEST_DATABASE_URL): a stream started after the user began receiving
// live notifications is recorded once, one already live then is not, and switching them off hides the items.
func TestTwitchLiveNotifications(t *testing.T) {
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
	since := time.Now().Add(-time.Hour)
	live := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/me/live" || r.Header.Get("X-Twitch-Token") != "access" {
			http.Error(w, "unexpected", http.StatusBadRequest)
			return
		}
		fmt.Fprintf(w, `{"items":[
{"id":"old","title":"Already on","user":{"login":"early","displayName":"Early"},"startedAt":%q},
{"id":"new","title":"Speedrun","user":{"login":"runner","displayName":"Runner","avatarUrl":"https://a/r.png"},
"game":{"name":"Celeste"},"startedAt":%q}],"hasMore":false}`,
			since.Add(-time.Minute).Format(time.RFC3339), since.Add(10*time.Minute).Format(time.RFC3339))
	}))
	defer live.Close()
	t.Setenv("TWITCH_SERVICE_URL", live.URL)
	t.Setenv("TWITCH_SERVICE_SECRET", "test-only-secret")
	t.Setenv("TWITCH_TOKEN_ENCRYPTION_KEY", "test-only-twitch-key-0123456789abcdef")
	s := &Server{DB: pool, twitch: newTwitch()}
	user := "twitch-live-test-" + contentPlaybackNonce()
	t.Cleanup(func() {
		for _, table := range []string{"twitch_accounts", "twitch_live_notifications", "twitch_live_notification_settings"} {
			pool.Exec(ctx, "DELETE FROM "+table+" WHERE user_id = $1", user)
		}
	})
	now := time.Now().UnixMilli()
	if _, err := pool.Exec(ctx, `INSERT INTO twitch_accounts (user_id, encrypted_access_token, login, display_name, created_at, updated_at)
VALUES ($1, $2, 'viewer', 'Viewer', $3, $3)`, user, s.twitch.encrypt("access"), now); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO twitch_live_notification_settings (user_id, enabled, watching_since, updated_at)
VALUES ($1, TRUE, $2, $3)`, user, since.UnixMilli(), now); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := s.pollTwitchLiveUser(ctx, user); err != nil {
			t.Fatal(err)
		}
	}
	items, err := s.twitchLiveItems(ctx, user)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("want one notification, got %+v", items)
	}
	it := items[0]
	if it.Type != twitchLiveType || it.Title != "Speedrun" || it.ChannelName != "Runner" || it.ChannelURL != "https://www.twitch.tv/runner" ||
		it.ChannelAvatarURL != "https://a/r.png" || it.Live == nil || it.Live.Login != "runner" || it.Live.Category != "Celeste" || it.ID == "" {
		t.Fatalf("item %+v live %+v", it, it.Live)
	}
	if _, err := pool.Exec(ctx, "UPDATE twitch_live_notification_settings SET enabled = FALSE WHERE user_id = $1", user); err != nil {
		t.Fatal(err)
	}
	if items, _ := s.twitchLiveItems(ctx, user); len(items) != 0 {
		t.Fatalf("switched off but still listed: %+v", items)
	}
}
