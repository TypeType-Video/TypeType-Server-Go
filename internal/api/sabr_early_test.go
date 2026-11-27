package api

import (
	"encoding/json"
	"testing"

	"typetype.dev/server/internal/sabr"
)

func TestResumeFromMatchesTheWebPlayer(t *testing.T) {
	cases := []struct{ saved, duration, want int64 }{
		{0, 600_000, 0},
		{4_999, 600_000, 0},
		{95_000, 600_000, 95_000},
		{590_000, 600_000, 0},
		{3_420_000, 3_600_000, 0},
		{3_400_000, 3_600_000, 3_400_000},
		{95_000, 0, 95_000},
	}
	for _, c := range cases {
		if got := resumeFrom(c.saved, c.duration); got != c.want {
			t.Fatalf("resumeFrom(%d, %d) = %d, want %d", c.saved, c.duration, got, c.want)
		}
	}
}

func TestDefaultWebAudioIsTheFirstListedStreamOfTheDefaultTrack(t *testing.T) {
	info := func(formats string) *sabr.Info {
		var in sabr.Info
		if err := json.Unmarshal([]byte(formats), &in.Formats); err != nil {
			t.Fatal(err)
		}
		return &in
	}
	single := info(`[
		{"itag":140,"mimeType":"audio/mp4; codecs=\"mp4a.40.2\"","bitrate":130000},
		{"itag":251,"mimeType":"audio/webm; codecs=\"opus\"","bitrate":143000},
		{"itag":258,"mimeType":"audio/mp4; codecs=\"mp4a.40.2\"","bitrate":389000},
		{"itag":328,"mimeType":"audio/mp4; codecs=\"ec-3\"","bitrate":385000}]`)
	if got := defaultWebAudio("x", single); got == nil || got.Itag != 140 {
		t.Fatalf("single track: got %+v, want itag 140", got)
	}
	dubbed := info(`[
		{"itag":140,"mimeType":"audio/mp4; codecs=\"mp4a.40.2\"","bitrate":130000,"audioTrack":{"id":"de.3","displayName":"German"}},
		{"itag":140,"mimeType":"audio/mp4; codecs=\"mp4a.40.2\"","bitrate":131000,"audioTrack":{"id":"en.4","displayName":"English original"}},
		{"itag":251,"mimeType":"audio/webm; codecs=\"opus\"","bitrate":143000,"audioTrack":{"id":"en.4","displayName":"English original"}}]`)
	got := defaultWebAudio("x", dubbed)
	if got == nil || got.AudioTrackID() == nil || *got.AudioTrackID() != "en.4" || got.Itag != 140 {
		t.Fatalf("dubbed: got %+v, want the original track at itag 140", got)
	}
}
