package youtube

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Dig walks decoded JSON by keys and indexes; any missing step gives nil.
func Dig(v any, path ...any) any {
	for _, p := range path {
		switch k := p.(type) {
		case string:
			m, ok := v.(map[string]any)
			if !ok {
				return nil
			}
			v = m[k]
		case int:
			a, ok := v.([]any)
			if !ok || k < 0 || k >= len(a) {
				return nil
			}
			v = a[k]
		}
	}
	return v
}

func Str(v any, path ...any) string {
	s, _ := Dig(v, path...).(string)
	return s
}

func List(v any, path ...any) []any {
	a, _ := Dig(v, path...).([]any)
	return a
}

func Obj(v any, path ...any) map[string]any {
	m, _ := Dig(v, path...).(map[string]any)
	return m
}

func Bool(v any, path ...any) bool {
	b, _ := Dig(v, path...).(bool)
	return b
}

func Num(v any, path ...any) (float64, bool) {
	switch n := Dig(v, path...).(type) {
	case float64:
		return n, true
	case string:
		f, err := strconv.ParseFloat(n, 64)
		return f, err == nil
	}
	return 0, false
}

// Text reads the three text shapes YouTube uses: simpleText, runs, and view-model content.
func Text(v any) string {
	if s := Str(v, "simpleText"); s != "" {
		return s
	}
	if s := Str(v, "content"); s != "" {
		return s
	}
	runs := List(v, "runs")
	if len(runs) == 0 {
		return ""
	}
	var b strings.Builder
	for _, r := range runs {
		b.WriteString(Str(r, "text"))
	}
	return b.String()
}

// FixURL gives protocol-relative and http thumbnail URLs an https scheme, like fixThumbnailUrl.
func FixURL(u string) string {
	switch {
	case strings.HasPrefix(u, "//"):
		return "https:" + u
	case strings.HasPrefix(u, "http://"):
		return "https://" + strings.TrimPrefix(u, "http://")
	}
	return u
}

// LastURL is the URL of the last (largest) entry of a thumbnails or sources array.
func LastURL(arr []any) string {
	if len(arr) == 0 {
		return ""
	}
	return FixURL(Str(arr[len(arr)-1], "url"))
}

var nonDigits = regexp.MustCompile(`\D`)

func Digits(s string) int64 {
	n, _ := strconv.ParseInt(nonDigits.ReplaceAllString(s, ""), 10, 64)
	return n
}

var mixedNumber = regexp.MustCompile(`([\d.,]+)\s*([KkMmBb])?`)

// MixedNumber reads "1.2M views", "3,4 k" or "12K" like Utils.mixedNumberWordToLong.
func MixedNumber(s string) (int64, bool) {
	m := mixedNumber.FindStringSubmatch(s)
	if m == nil {
		return 0, false
	}
	num := strings.ReplaceAll(m[1], ",", ".")
	if m[2] == "" {
		return Digits(m[1]), true
	}
	f, err := strconv.ParseFloat(num, 64)
	if err != nil {
		return 0, false
	}
	switch strings.ToUpper(m[2]) {
	case "K":
		f *= 1e3
	case "M":
		f *= 1e6
	case "B":
		f *= 1e9
	}
	return int64(f + 0.5), true
}

var timeAgo = regexp.MustCompile(`(?i)(\d+)\s*(second|minute|hour|day|week|month|year)s?\s+ago`)

// TimeAgo turns "3 years ago" (and "Streamed 2 days ago") into a time, as the English TimeAgoParser does, and the
// Zulu forms the metadata requests get ("ezinsukwini ezingu-2 ezedlule").
func TimeAgo(s string, now time.Time) (time.Time, bool) {
	unit, n, ok := timeAgoUnit(s)
	if !ok {
		return time.Time{}, false
	}
	return agoFrom(now, unit, n)
}

// ParseDate reads the ISO dates of the microformat: "2024-06-24", "2024-06-24T08:00:00-07:00".
func ParseDate(s string) (time.Time, bool) {
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}
