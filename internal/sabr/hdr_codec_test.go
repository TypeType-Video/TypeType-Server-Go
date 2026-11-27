package sabr

import "testing"

func TestFullHDRCodecRewritesOnlyTheShortVP9Profile2(t *testing.T) {
	cases := map[string]string{
		`video/webm; codecs="vp9.2"`:                         `video/webm; codecs="vp09.02.51.10.01.09.16.09.00"`,
		`video/webm; codecs="vp9"`:                           `video/webm; codecs="vp9"`,
		`video/mp4; codecs="av01.0.13M.10.0.110.09.16.09.0"`: `video/mp4; codecs="av01.0.13M.10.0.110.09.16.09.0"`,
	}
	for in, want := range cases {
		if got := fullHDRCodec(in); got != want {
			t.Fatalf("fullHDRCodec(%q) = %q, want %q", in, got, want)
		}
	}
}
