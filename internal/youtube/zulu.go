package youtube

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// The metadata requests ask YouTube for Zulu (contentLanguage), so the texts YouTube writes itself come back in Zulu.
// These helpers read them next to the English forms, with the words PipePipe's extractor checks.

// zuluUnits are the time words of PipePipe's zu TimeAgo patterns, with the unit each one stands for.
var zuluUnits = func() map[string]string {
	m := map[string]string{}
	for unit, words := range map[string][]string{
		"second": {"amasekhondi", "isekhondi", "emasekhondini", "umzuzwana", "imizuzwana", "emizuzwaneni"},
		"minute": {"amaminithi", "iminithi", "emaminithini", "umzuzu", "imizuzu", "emizuzwini"},
		"hour":   {"amahora", "ihora", "emahoreni"},
		"day":    {"izinsuku", "usuku", "osukwini", "ezinsukwini"},
		"week":   {"amaviki", "iviki", "emavikini", "isonto", "amasonto", "emasontweni"},
		"month":  {"inyanga", "izinyanga", "ezinyangeni"},
		"year":   {"iminyaka", "unyaka", "eminyakeni"},
	} {
		for _, w := range words {
			m[w] = unit
		}
	}
	return m
}()

var (
	zuluWord   = regexp.MustCompile(`\p{L}+`)
	zuluNumber = regexp.MustCompile(`\d+`)
)

// zuluTimeAgo reads "ezinsukwini ezingu-3 ezedlule." (3 days ago), "emahoreni angu-13 edlule" and "2 iminyaka edlule".
// Every relative date YouTube writes in Zulu ends with edlule/ezedlule ("that passed"), which keeps channel and
// uploader names out; "manje" (now) and "izolo" (yesterday) stand alone.
func zuluTimeAgo(s string) (unit string, n int, ok bool) {
	l := strings.ToLower(strings.TrimSpace(s))
	words := zuluWord.FindAllString(l, -1)
	if len(words) == 1 {
		switch words[0] {
		case "manje":
			return "second", 0, true
		case "izolo":
			return "day", 1, true
		}
	}
	if !strings.Contains(l, "dlule") {
		return "", 0, false
	}
	for _, w := range words {
		if u, found := zuluUnits[w]; found {
			unit = u
			break
		}
	}
	num := zuluNumber.FindString(l)
	if unit == "" || num == "" {
		return "", 0, false
	}
	n, err := strconv.Atoi(num)
	return unit, n, err == nil
}

// timeAgoUnit reads an English or Zulu relative date into its unit and count.
func timeAgoUnit(s string) (string, int, bool) {
	if m := timeAgo.FindStringSubmatch(s); m != nil {
		n, _ := strconv.Atoi(m[1])
		return strings.ToLower(m[2]), n, true
	}
	return zuluTimeAgo(s)
}

func agoFrom(now time.Time, unit string, n int) (time.Time, bool) {
	switch unit {
	case "second":
		return now.Add(-time.Duration(n) * time.Second), true
	case "minute":
		return now.Add(-time.Duration(n) * time.Minute), true
	case "hour":
		return now.Add(-time.Duration(n) * time.Hour), true
	case "day":
		return now.AddDate(0, 0, -n).Truncate(time.Hour), true
	case "week":
		return now.AddDate(0, 0, -7*n).Truncate(time.Hour), true
	case "month":
		return now.AddDate(0, -n, 0).Truncate(time.Hour), true
	case "year":
		return now.AddDate(-n, 0, 0).Truncate(time.Hour), true
	}
	return time.Time{}, false
}

func containsAny(s string, words ...string) bool {
	l := strings.ToLower(s)
	for _, w := range words {
		if strings.Contains(l, w) {
			return true
		}
	}
	return false
}

// mentionsViews is "1.2M views" / "1.2M ukubukwa"; mentionsWatching is "361 watching" / "414 ababukele".
func mentionsViews(s string) bool    { return containsAny(s, "view", "ukubukwa") }
func mentionsWatching(s string) bool { return containsAny(s, "watching", "bukele") }

// noViews is "No views" / "Akukho ukubukwa".
func noViews(s string) bool { return containsAny(s, "no views", "akukho ukubukwa", "akukho kubukwa") }

// mentionsSubscribers is "891K subscribers" / "891K ababhalisile".
func mentionsSubscribers(s string) bool { return containsAny(s, "subscriber", "ababhalisile") }

// mentionsVideos is "395 videos" / "Amavidiyo angu-395".
func mentionsVideos(s string) bool { return containsAny(s, "video", "vidiyo") }

// mentionsPlaylist is "Playlist" / "Uhlu lokudlalayo".
func mentionsPlaylist(s string) bool { return containsAny(s, "playlist", "uhlu lokudlalayo") }

var zuluOwner = regexp.MustCompile(`^ka-(.+?)(?: (?:nomunye ongu|nabanye abangu)-\d+)?$`)

// playlistOwner drops the ownership wording of a playlist header: "by Music" and "ka-Umculo", also with the
// "nomunye ongu-1" / "nabanye abangu-2" (and 1/2 others) of a shared playlist, as PipePipe does.
func playlistOwner(s string) string {
	if m := zuluOwner.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	return strings.TrimPrefix(s, "by ")
}
