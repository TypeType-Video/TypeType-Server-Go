package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type noWarmKey struct{}

// internalLookup marks a stream lookup made by the server for itself, which must not warm provider media.
func internalLookup(ctx context.Context) bool { v, _ := ctx.Value(noWarmKey{}).(bool); return v }

// lookupStream is StreamService.getStreamInfo for background work: the public stream response of a URL, and whether
// a failure is worth retrying (anything but a bad request).
func (s *Server) lookupStream(ctx context.Context, raw string) (*streamResponse, bool) {
	if id, ok := strings.CutPrefix(raw, twitchVideoPrefix); ok && twitchIDRe.MatchString(id) {
		return s.twitchVideoLookup(ctx, id)
	}
	r, _ := http.NewRequestWithContext(context.WithValue(ctx, noWarmKey{}, true), "GET", "/streams?url="+url.QueryEscape(raw), nil)
	resp, c := s.streamFor(r, raw)
	if resp != nil {
		return resp, false
	}
	return nil, c == nil || c.status != http.StatusBadRequest
}

// savedTable is SavedVideoColumns: a saved-video table and its URL column.
type savedTable struct{ table, url string }

var savedTables = []savedTable{{"playlist_videos", "url"}, {"watch_later", "url"}, {"favorites", "video_url"}}

// repairs is UserVideoMetadataRepairService: after a list is read, up to three batches of eight of its videos with
// missing or day-old metadata are looked up again and written back to every saved list of the user.
type repairs struct {
	mu       sync.Mutex
	inFlight map[string]bool
	last     map[string]time.Time
}

var metadataRepairs = &repairs{inFlight: map[string]bool{}, last: map[string]time.Time{}}

func (s *Server) scheduleRepair(key, user string, t savedTable, playlist string) {
	m := metadataRepairs
	m.mu.Lock()
	if at, ok := m.last[key]; ok && time.Since(at) < 5*time.Minute || m.inFlight[key] {
		m.mu.Unlock()
		return
	}
	m.inFlight[key], m.last[key] = true, time.Now()
	m.mu.Unlock()
	go func() {
		defer func() {
			m.mu.Lock()
			delete(m.inFlight, key)
			m.mu.Unlock()
		}()
		excluded := map[string]bool{}
		for batch := 1; batch <= 3; batch++ {
			tried := s.repairBatch(user, t, playlist, excluded)
			if tried == 0 {
				break
			}
			if batch < 3 {
				time.Sleep(1500 * time.Millisecond)
			}
		}
	}()
}

func (s *Server) repairBatch(user string, t savedTable, playlist string, excluded map[string]bool) int {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	now := time.Now().UnixMilli()
	q := `SELECT ` + t.url + ` FROM ` + t.table + ` WHERE user_id = $1 AND (title LIKE 'YouTube video %' OR thumbnail = '' OR duration <= 0
OR channel_name = '' OR channel_url = '' OR metadata_refreshed_at < $2 OR channel_avatar = '' AND metadata_refreshed_at < $3)`
	args := []any{user, now - 24*3600*1000, now - 3600*1000}
	if playlist != "" {
		q += " AND playlist_id = $4"
		args = append(args, playlist)
	}
	rows, err := s.DB.Query(ctx, q+" ORDER BY metadata_refreshed_at ASC", args...)
	if err != nil {
		return 0
	}
	var urls []string
	seen := map[string]bool{}
	for rows.Next() {
		var u string
		if rows.Scan(&u) == nil && !seen[u] && !excluded[u] && len(urls) < 8 {
			seen[u] = true
			urls = append(urls, u)
		}
	}
	rows.Close()
	for _, u := range urls {
		excluded[u] = true
	}
	resolved, retry := s.resolveMetadata(ctx, urls)
	s.applyMetadata(ctx, user, resolved)
	if len(retry) > 0 {
		time.Sleep(3 * time.Second)
		again, _ := s.resolveMetadata(ctx, retry)
		s.applyMetadata(ctx, user, again)
		for u := range again {
			resolved[u] = again[u]
		}
	}
	var gone []string
	for _, u := range urls {
		if _, ok := resolved[u]; !ok && !slicesContains(retry, u) {
			gone = append(gone, u)
		}
	}
	if len(gone) > 0 {
		for _, st := range savedTables {
			s.DB.Exec(ctx, "UPDATE "+st.table+" SET metadata_refreshed_at = $3 WHERE user_id = $1 AND "+st.url+" = ANY($2)", user, gone, now)
		}
	}
	return len(urls)
}

func slicesContains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// resolveMetadata is VideoMetadataResolver.resolveDetailed, two lookups at a time.
func (s *Server) resolveMetadata(ctx context.Context, urls []string) (map[string]*streamResponse, []string) {
	out := map[string]*streamResponse{}
	var retry []string
	var mu sync.Mutex
	slots := make(chan struct{}, 2)
	var wg sync.WaitGroup
	for _, u := range urls {
		wg.Go(func() {
			slots <- struct{}{}
			defer func() { <-slots }()
			resp, retryable := s.lookupStream(ctx, u)
			mu.Lock()
			defer mu.Unlock()
			if resp != nil {
				out[u] = resp
			} else if retryable {
				retry = append(retry, u)
			}
		})
	}
	wg.Wait()
	return out, retry
}

// applyMetadata is SavedVideoColumns.apply on every saved list: only non-blank values overwrite.
func (s *Server) applyMetadata(ctx context.Context, user string, resolved map[string]*streamResponse) {
	now := time.Now().UnixMilli()
	for u, r := range resolved {
		published := r.Uploaded
		if r.PublishedAt != nil {
			published = *r.PublishedAt
		}
		for _, st := range savedTables {
			s.DB.Exec(ctx, `UPDATE `+st.table+` SET title = COALESCE(NULLIF($3, ''), title), thumbnail = COALESCE(NULLIF($4, ''), thumbnail),
duration = CASE WHEN $5::bigint > 0 THEN $5::bigint ELSE duration END, channel_name = COALESCE(NULLIF($6, ''), channel_name),
channel_url = COALESCE(NULLIF($7, ''), channel_url), channel_avatar = COALESCE(NULLIF($8, ''), channel_avatar),
view_count = CASE WHEN $9::bigint > 0 THEN $9::bigint ELSE view_count END, published_at = CASE WHEN $10::bigint > 0 THEN $10::bigint ELSE published_at END,
metadata_refreshed_at = $11::bigint WHERE user_id = $1 AND `+st.url+` = $2`, user, u, r.Title, r.ThumbnailURL, r.Duration, r.UploaderName, r.UploaderURL,
				r.UploaderAvatarURL, r.ViewCount, published, now)
		}
	}
}

// avatarWarmup is SubscriptionAvatarWarmupService: subscriptions listed without an avatar get it from their channel
// page, eight per listing, two lookups at a time, a failed channel left alone for 15 minutes.
type avatarWarmup struct {
	mu       sync.Mutex
	inFlight map[string]bool
	failed   map[string]time.Time
}

var avatarWarm = &avatarWarmup{inFlight: map[string]bool{}, failed: map[string]time.Time{}}

func (s *Server) warmAvatars(user string, items []subscriptionItem) {
	a := avatarWarm
	a.mu.Lock()
	var picked []string
	for _, it := range items {
		if it.AvatarURL != "" || a.inFlight[it.ChannelURL] || len(picked) >= 8 {
			continue
		}
		if at, ok := a.failed[it.ChannelURL]; ok && time.Since(at) < 15*time.Minute {
			continue
		}
		a.inFlight[it.ChannelURL] = true
		picked = append(picked, it.ChannelURL)
	}
	a.mu.Unlock()
	if len(picked) == 0 {
		return
	}
	go func() {
		defer func() {
			a.mu.Lock()
			for _, c := range picked {
				delete(a.inFlight, c)
			}
			a.mu.Unlock()
		}()
		for i := 0; i < len(picked); i += 2 {
			var wg sync.WaitGroup
			for _, c := range picked[i:min(i+2, len(picked))] {
				wg.Go(func() { s.warmAvatar(user, c) })
			}
			wg.Wait()
			time.Sleep(750 * time.Millisecond)
		}
	}()
}

func (s *Server) warmAvatar(user, channel string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	r, _ := http.NewRequestWithContext(ctx, "GET", "/channel?url="+url.QueryEscape(channel), nil)
	c := &captured{header: http.Header{}}
	s.channelPage(c, r, channel, "")
	var ch struct {
		AvatarURL string `json:"avatarUrl"`
	}
	fail := func() {
		avatarWarm.mu.Lock()
		avatarWarm.failed[channel] = time.Now()
		avatarWarm.mu.Unlock()
	}
	if c.status != http.StatusOK || json.Unmarshal(c.body.Bytes(), &ch) != nil || ch.AvatarURL == "" {
		fail()
		return
	}
	s.DB.Exec(ctx, "UPDATE subscriptions SET avatar_url = $3 WHERE user_id = $1 AND channel_url = $2 AND avatar_url = ''", user, channel, ch.AvatarURL)
	avatarWarm.mu.Lock()
	delete(avatarWarm.failed, channel)
	avatarWarm.mu.Unlock()
}
