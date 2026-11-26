package youtube

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

const webVersion = "2.20261120.01.00"

type ClientInfo struct {
	Name, Version, UA string
	ID                int
	Extra             map[string]any
}

var (
	Web = ClientInfo{Name: "WEB", Version: webVersion, ID: 1,
		UA: "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0 Safari/537.36"}
	AndroidVR = ClientInfo{Name: "ANDROID_VR", Version: "1.65.10", ID: 28,
		UA:    "com.google.android.apps.youtube.vr.oculus/1.65.10 (Linux; U; Android 12L; eureka-user Build/SQ3A.220605.009.A1) gzip",
		Extra: map[string]any{"deviceMake": "Oculus", "deviceModel": "Quest 3", "androidSdkVersion": 32, "osName": "Android", "osVersion": "12L"}}
)

// Client calls innertube with one warm connection pool and a visitor id shared by every call, refreshed hourly.
type Client struct {
	http *http.Client
	mu   sync.Mutex
	vis  string
	at   time.Time
	// fetching lets one caller download the embed page while the others wait for its answer; failedAt keeps
	// them from queueing one 20 s timeout each when YouTube does not answer.
	fetching sync.Mutex
	failedAt time.Time
	// commentTokens keeps the comments continuation of each video page fetched, so the comments that open right
	// after the page skip a second /next.
	commentTokens sync.Map
	refused       atomic.Pointer[func()]
}

// OnRefused is OkHttpDownloader.onProviderRefusal: told of every 429 YouTube answers.
func (c *Client) OnRefused(f func()) { c.refused.Store(&f) }

func (c *Client) noteStatus(code int) {
	if f := c.refused.Load(); code == http.StatusTooManyRequests && f != nil {
		(*f)()
	}
}

type tokenAt struct {
	token string
	at    time.Time
}

func (c *Client) rememberComments(videoID, token string) {
	if token != "" {
		c.commentTokens.Store(videoID, tokenAt{token, time.Now()})
	}
}

func (c *Client) knownComments(videoID string) string {
	v, ok := c.commentTokens.Load(videoID)
	if !ok {
		return ""
	}
	t := v.(tokenAt)
	if time.Since(t.at) > 30*time.Minute {
		c.commentTokens.Delete(videoID)
		return ""
	}
	return t.token
}

func NewClient(proxy string) *Client {
	tr := &http.Transport{
		DialContext:         (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:   true,
		MaxIdleConnsPerHost: 64,
		IdleConnTimeout:     120 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}
	if proxy != "" {
		if u, err := url.Parse(proxy); err == nil {
			tr.Proxy = http.ProxyURL(u)
		}
	}
	tr.DisableCompression = true
	return &Client{http: &http.Client{Transport: inflating{tr}, Timeout: 20 * time.Second}}
}

// NewClientWithHTTP is a client that sends every YouTube call through hc, for tests that stand a fake YouTube in.
func NewClientWithHTTP(hc *http.Client) *Client {
	return &Client{http: hc}
}

var visitorRe = regexp.MustCompile(`"VISITOR_DATA":"([^"]+)"`)

// Visitor returns a visitor id from the embed page, kept for an hour.
func (c *Client) Visitor(ctx context.Context) string {
	if v := c.cachedVisitor(); v != "" {
		return v
	}
	c.fetching.Lock()
	defer c.fetching.Unlock()
	if v := c.cachedVisitor(); v != "" {
		return v
	}
	c.mu.Lock()
	failed := time.Since(c.failedAt) < 10*time.Second
	c.mu.Unlock()
	if failed {
		return ""
	}
	v := c.fetchVisitor(ctx)
	c.mu.Lock()
	if v == "" {
		c.failedAt = time.Now()
	} else {
		c.vis, c.at = v, time.Now()
	}
	c.mu.Unlock()
	return v
}

func (c *Client) cachedVisitor() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.vis != "" && time.Since(c.at) < time.Hour {
		return c.vis
	}
	return ""
}

func (c *Client) fetchVisitor(ctx context.Context) string {
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://www.youtube.com/embed/aqz-KE-bpKQ", nil)
	req.Header.Set("User-Agent", Web.UA)
	res, err := c.http.Do(req)
	if err != nil {
		return ""
	}
	defer res.Body.Close()
	c.noteStatus(res.StatusCode)
	return scanVisitor(io.LimitReader(res.Body, 4<<20))
}

// scanVisitor stops reading at the visitor id, a quarter into the 150 KB embed page.
func scanVisitor(r io.Reader) string {
	buf := make([]byte, 0, 64<<10)
	for {
		if len(buf) == cap(buf) {
			buf = slices.Grow(buf, cap(buf))
		}
		n, err := r.Read(buf[len(buf):cap(buf)])
		buf = buf[:len(buf)+n]
		if m := visitorRe.FindSubmatch(buf); m != nil {
			return string(m[1])
		}
		if err != nil {
			return ""
		}
	}
}

var ErrStatus = errors.New("innertube status")

// contentLanguage is the interface language of the metadata requests. YouTube machine-translates video titles into
// the language a request names, and it has no translations into Zulu, so "zu" keeps every title, description and
// chapter as the uploader wrote it, as PipePipe's YoutubeService does. The texts YouTube writes itself come back in
// Zulu then ("ezinsukwini ezingu-2 ezedlule", "434,773 ukubukwa"); the parsers read both languages.
var contentLanguage = "zu"

// requestLanguage keeps the player on English: it does not translate titles, and the playability reasons
// CheckPlayability reads are English.
func requestLanguage(endpoint string) string {
	if endpoint == "player" {
		return "en"
	}
	return contentLanguage
}

// Call posts to /youtubei/v1/{endpoint} as the given client and decodes the answer without its tracking members.
func (c *Client) Call(ctx context.Context, ci ClientInfo, endpoint string, body map[string]any) (map[string]any, error) {
	return c.call(ctx, ci, endpoint, body, trackingSkip)
}

// call is Call; members named in skip are left out of the decoded answer at any depth.
func (c *Client) call(ctx context.Context, ci ClientInfo, endpoint string, body map[string]any, skip map[string]bool) (map[string]any, error) {
	visitor := c.Visitor(ctx)
	client := map[string]any{"clientName": ci.Name, "clientVersion": ci.Version, "hl": requestLanguage(endpoint), "gl": "US"}
	if visitor != "" {
		client["visitorData"] = visitor
	}
	for k, v := range ci.Extra {
		client[k] = v
	}
	body["context"] = map[string]any{"client": client}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, "POST", "https://www.youtube.com/youtubei/v1/"+endpoint+"?prettyPrint=false", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", ci.UA)
	req.Header.Set("X-YouTube-Client-Name", strconv.Itoa(ci.ID))
	req.Header.Set("X-YouTube-Client-Version", ci.Version)
	if visitor != "" {
		req.Header.Set("X-Goog-Visitor-Id", visitor)
	}
	res, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		c.noteStatus(res.StatusCode)
		io.Copy(io.Discard, io.LimitReader(res.Body, 1<<20))
		return nil, fmt.Errorf("%w %d on %s", ErrStatus, res.StatusCode, endpoint)
	}
	return decodePruned(res.Body, skip)
}

// Reset drops the visitor id, the remembered comment tokens and the idle connections, after the egress changed.
func (c *Client) Reset() int {
	c.mu.Lock()
	c.vis, c.at = "", time.Time{}
	c.mu.Unlock()
	n := 0
	c.commentTokens.Range(func(k, _ any) bool {
		c.commentTokens.Delete(k)
		n++
		return true
	})
	c.http.CloseIdleConnections()
	return n
}
