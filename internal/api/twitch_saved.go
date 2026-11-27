package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// twitchVideoPrefix starts the URL a saved Twitch VOD is kept under in playlists, favourites and Watch later.
const twitchVideoPrefix = "https://www.twitch.tv/videos/"

// twitchVideoLookup gives the saved-video repair what TypeType-Live knows of a VOD. Live also prepares its playback on
// this route, so the repair asks only for a VOD saved without metadata, never for the daily refresh; a VOD Twitch
// removed is not worth retrying, and the stored title and cover stay.
func (s *Server) twitchVideoLookup(ctx context.Context, id string) (*streamResponse, bool) {
	if s.twitch == nil {
		return nil, false
	}
	res := s.twitch.do(ctx, "GET", "/v1/videos/"+id, nil, nil, "")
	switch {
	case res.status == http.StatusNotFound || res.status == http.StatusBadRequest:
		return nil, false
	case res.status != http.StatusOK:
		return nil, true
	}
	var body struct {
		Video struct {
			Title   string `json:"title"`
			Preview string `json:"previewUrl"`
			Length  int64  `json:"lengthSeconds"`
			Views   int64  `json:"views"`
			Owner   struct {
				Login       string `json:"login"`
				DisplayName string `json:"displayName"`
				Avatar      string `json:"avatarUrl"`
			} `json:"owner"`
			CreatedAt time.Time `json:"createdAt"`
		} `json:"video"`
	}
	if err := json.Unmarshal(res.body, &body); err != nil {
		return nil, true
	}
	v := body.Video
	r := &streamResponse{Title: v.Title, ThumbnailURL: v.Preview, Duration: v.Length, ViewCount: v.Views,
		UploaderName: v.Owner.DisplayName, UploaderAvatarURL: v.Owner.Avatar}
	if v.Owner.Login != "" {
		r.UploaderURL = "https://www.twitch.tv/" + v.Owner.Login
	}
	if !v.CreatedAt.IsZero() {
		r.Uploaded = v.CreatedAt.UnixMilli()
	}
	return r, false
}
