package youtube

import (
	"testing"
	"time"
)

// The Zulu texts below are taken from YouTube answers to hl=zu requests (search, channel, playlist, comments, posts).
func TestTimeAgoReadsZulu(t *testing.T) {
	now := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)
	cases := map[string]time.Time{
		"ezinsukwini ezingu-11 ezedlule.": now.AddDate(0, 0, -11).Truncate(time.Hour),
		"ezinsukwini ezingu-1 ezedlule.":  now.AddDate(0, 0, -1).Truncate(time.Hour),
		"emahoreni angu-13 edlule":        now.Add(-13 * time.Hour),
		"2 izinyanga ezedlule":            now.AddDate(0, -2, 0).Truncate(time.Hour),
		"2 iminyaka edlule":               now.AddDate(-2, 0, 0).Truncate(time.Hour),
		"emaminithini angu-5 edlule":      now.Add(-5 * time.Minute),
		"emasontweni angu-3 edlule":       now.AddDate(0, 0, -21).Truncate(time.Hour),
		"manje":                           now,
		"izolo":                           now.AddDate(0, 0, -1).Truncate(time.Hour),
		"3 years ago":                     now.AddDate(-3, 0, 0).Truncate(time.Hour),
		"Streamed 2 days ago":             now.AddDate(0, 0, -2).Truncate(time.Hour),
		"13 hours ago (edited)":           now.Add(-13 * time.Hour),
	}
	for in, want := range cases {
		got, ok := TimeAgo(in, now)
		if !ok || !got.Equal(want) {
			t.Errorf("TimeAgo(%q) = %v, %v; want %v", in, got, ok, want)
		}
	}
	// Metadata parts next to the date must not pass for one: view counts, durations, names.
	for _, in := range []string{"1M ukubukwa", "20 amaminithi, 48 amasekhondi", "Amavidiyo angu-395", "Papaplatte",
		"414 ababukele", "1.35M ababhalisile", "usuku olumnandi", "Izolo Records"} {
		if _, ok := TimeAgo(in, now); ok {
			t.Errorf("TimeAgo(%q) read a date", in)
		}
	}
}

func TestZuluMetadataWords(t *testing.T) {
	check := func(name string, f func(string) bool, yes, no []string) {
		for _, s := range yes {
			if !f(s) {
				t.Errorf("%s(%q) = false", name, s)
			}
		}
		for _, s := range no {
			if f(s) {
				t.Errorf("%s(%q) = true", name, s)
			}
		}
	}
	check("mentionsViews", mentionsViews, []string{"1.4M ukubukwa", "1,045,679,475 ukubukwa", "782M views"}, []string{"Papaplatte"})
	check("mentionsWatching", mentionsWatching, []string{"414 ababukele", "3 inkulungwane ababukele", "361 watching"}, []string{"1M ukubukwa"})
	check("noViews", noViews, []string{"Akukho ukubukwa", "No views"}, []string{"1M ukubukwa"})
	check("mentionsSubscribers", mentionsSubscribers, []string{"891K ababhalisile", "1.35M subscribers"}, []string{"@papaplatte"})
	check("mentionsVideos", mentionsVideos, []string{"Amavidiyo angu-182", "395 videos"}, []string{"1M ukubukwa"})
	check("mentionsPlaylist", mentionsPlaylist, []string{"Uhlu lokudlalayo", "Playlist"}, []string{"Lwith2L"})
	for in, want := range map[string]int64{
		"891K ababhalisile":         891000,
		"3 inkulungwane ababukele":  3000,
		"1.2 inkulungwane ukubukwa": 1200,
		"2 izigidi ukubukwa":        2000000,
		"1 isigidi ukubukwa":        1000000,
		"1.5 izigidigidi ukubukwa":  1500000000,
		"434,773 ukubukwa":          434773,
		"1.2M views":                1200000,
		"12K":                       12000,
	} {
		if n, ok := MixedNumber(in); !ok || n != want {
			t.Errorf("MixedNumber(%q) = %d, %v; want %d", in, n, ok, want)
		}
	}
}

func TestPlaylistOwner(t *testing.T) {
	for in, want := range map[string]string{
		"ka-Umculo":                    "Umculo",
		"ka-Papaplatte nomunye ongu-1": "Papaplatte",
		"ka-Lea nabanye abangu-3":      "Lea",
		"by Music":                     "Music",
		"Maurice":                      "Maurice",
	} {
		if got := playlistOwner(in); got != want {
			t.Errorf("playlistOwner(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRequestLanguageKeepsThePlayerInEnglish(t *testing.T) {
	if got := requestLanguage("player"); got != "en" {
		t.Errorf("player asks for %q", got)
	}
	for _, ep := range []string{"browse", "search", "next", "live_chat/get_live_chat", "navigation/resolve_url"} {
		if got := requestLanguage(ep); got != "zu" {
			t.Errorf("%s asks for %q", ep, got)
		}
	}
}
