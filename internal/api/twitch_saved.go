package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// twitchVideoPrefix starts the URL a saved Twitch VOD is kept under in playlists, favourites and Watch later.
const twitchVideoPrefix = "https://www.twitch.tv/videos/"

// twitchVideoLookup gives the saved-video repair what TypeType-Live knows of a VOD; a VOD Twitch removed is not worth
// retrying, so the stored title and cover stay.
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
	var v struct {
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
	}
	if err := json.Unmarshal(res.body, &v); err != nil {
		return nil, true
	}
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
