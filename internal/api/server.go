package api

import (
	"errors"
	"log/slog"
	"net/http"
	"time"
	"typetype.dev/server/internal/bilibili"
	"typetype.dev/server/internal/niconico"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"typetype.dev/server/internal/auth"
	"typetype.dev/server/internal/cache"
	"typetype.dev/server/internal/config"
	"typetype.dev/server/internal/httpx"
	"typetype.dev/server/internal/sabr"
	"typetype.dev/server/internal/store"
	"typetype.dev/server/internal/youtube"
)

// Build data, set with -ldflags at release time.
var (
	Version       = "dev"
	Revision      = "unknown"
	ShortRevision = "unknown"
	BuildTime     = "unknown"
)

const instanceCacheControl = "no-store, no-cache, must-revalidate, max-age=0"

// Server holds what the handlers share. Each route family lives in its own file and registers itself in routes.
type Server struct {
	Cfg         *config.Config
	DB          *pgxpool.Pool
	Cache       *cache.Cache
	Log         *slog.Logger
	Admin       *store.Admin
	Auth        *auth.Service
	Sabr        *sabr.Store
	YT          *youtube.Client
	sponsor     *sponsorBlock
	proxyClient *http.Client
	notices     *noticeHub
	activity    *activity
	feeds       *feeds
	warm        *warmup
	mirrorWatch *mirrorWatch
	homeWarm    *homeWarm
	acceptance  *acceptance
	recs        *recPools
	rec         *recRelay
	subs        *subtitles
	deArrow     *dearrow
	bili        *bilibili.Client
	nico        *niconico.Client
	links       *shortLinks
	emoji       *openMoji
	twitch      *twitch
	dl          *downloader
	features    features
	zones       zones
}

type zones struct {
	extraction, dearrow, streams, channel, proxy, storyboard, userData *httpx.Zone
}

func New(cfg *config.Config, db *pgxpool.Pool, c *cache.Cache, log *slog.Logger) *Server {
	w := time.Minute
	srv := &Server{Cfg: cfg, DB: db, Cache: c, Log: log, Admin: store.NewAdmin(db),
		Auth: auth.NewService(db, cfg.JWTSecret, auth.SessionConfigFromEnv()), features: loadFeatures(cfg),
		Sabr: sabr.NewStore(sabr.Options{TokenURL: cfg.SubtitleService, ProxyURL: cfg.YoutubeProxyURL,
			MaxSessions: max(1, config.IntEnv("SABR_MAX_SESSIONS", 24)),
			Idle:        time.Duration(max(60, config.IntEnv("SABR_IDLE_EVICTION_SECONDS", 240))) * time.Second,
			BudgetBytes: int64(max(64, config.IntEnv("SABR_SESSION_BYTE_BUDGET_MB", 512))) << 20}),
		YT: youtube.NewClient(cfg.YoutubeProxyURL), sponsor: newSponsorBlock(c, log), proxyClient: newProxyClient(), notices: newNoticeHub(), activity: newActivity(), emoji: newOpenMoji(), twitch: newTwitch(), dl: newDownloader(cfg.DownloaderURL),
		zones: zones{
			extraction: httpx.NewZone(60, w), dearrow: httpx.NewZone(600, w), streams: httpx.NewZone(360, w),
			channel: httpx.NewZone(180, w), proxy: httpx.NewZone(6000, w), storyboard: httpx.NewZone(1200, w),
			userData: httpx.NewZone(120, w),
		}}
	srv.Sabr.SetOnFirstRound(func(videoID string, t sabr.RoundTiming) {
		log.Info("sabr_first_round", "videoId", videoID, "round", t.Round, "reused", t.Reused, "connMs", t.ConnMs, "tlsMs", t.TLSMs, "firstByteMs", t.FirstByteMs,
			"doneMs", t.DoneMs, "media", t.Media, "backoffMs", t.BackoffMs, "reload", t.Reload, "fallback", t.Fallback)
	})
	go srv.logSabrMemory()
	srv.feeds = newFeeds(srv)
	srv.warm = newWarmup(srv)
	srv.startMirrorWatch()
	srv.startHomeWarm()
	srv.startBiliRefresh()
	srv.startAcceptance()
	srv.startYoutubeRefresh()
	srv.startPushPoller()
	srv.startTwitchLivePoller()
	srv.recs = newRecPools(srv)
	srv.rec = newRecRelay(srv)
	srv.subs = newSubtitles(srv)
	srv.deArrow = newDeArrow(srv)
	srv.bili = bilibili.NewClient(nil)
	srv.nico = niconico.NewClient(nil)
	srv.links = newShortLinks()
	return srv
}

func (s *Server) Routes() (http.Handler, *Router) {
	rt := NewRouter()
	s.metaRoutes(rt)
	s.authRoutes(rt)
	s.sabrRoutes(rt)
	s.startupMetricsRoutes(rt)
	s.streamRoutes(rt)
	s.proxyRoutes(rt)
	s.eventRoutes(rt)
	s.commentRoutes(rt)
	s.settingsRoutes(rt)
	s.blockedRoutes(rt)
	s.historyRoutes(rt)
	s.profileRoutes(rt)
	s.sessionRoutes(rt)
	s.collectionRoutes(rt)
	s.playlistRoutes(rt)
	s.channelRoutes(rt)
	s.feedRoutes(rt)
	s.notificationRoutes(rt)
	s.twitchLiveNotificationRoutes(rt)
	s.avatarRoutes(rt)
	s.focusRoutes(rt)
	s.searchRoutes(rt)
	s.publicPlaylistRoutes(rt)
	s.deviceRoutes(rt)
	s.profileAccountRoutes(rt)
	s.podcastRoutes(rt)
	s.twitchRoutes(rt)
	s.groupRoutes(rt)
	s.downloaderRoutes(rt)
	s.recommendationRoutes(rt)
	s.recommenderRoutes(rt)
	s.shortsFeedRoutes(rt)
	s.subtitleRoutes(rt)
	s.dearrowRoutes(rt)
	s.hlsRoutes(rt)
	s.membershipRoutes(rt)
	s.mediaRoutes(rt)
	rt.Handle("GET", "/streams/bilibili", s.bilibiliStream)
	rt.Handle("GET", "/streams/niconico", s.niconicoStream)
	s.bulletRoutes(rt)
	s.titleLinkRoutes(rt)
	s.adminRoutes(rt)
	s.allowedRoutes(rt)
	s.bugRoutes(rt)
	s.presenceRoutes(rt)
	s.internalRoutes(rt)
	s.egressResetRoute(rt)
	s.backupRoutes(rt)
	s.portabilityRoutes(rt)
	s.takeoutRoutes(rt)
	s.youtubeSessionRoutes(rt)
	s.bilibiliSessionRoutes(rt)
	s.rssRoutes(rt)
	s.oidcRoutes(rt)
	s.pipePipeRoutes(rt)
	s.recommenderAdminRoutes(rt)
	s.uploadRoutes(rt)
	h := httpx.Gzip(rt.Handler())
	h = httpx.CORS(s.Cfg.AllowedOrigins, s.Cfg.AllowAnyOrigin, h)
	return httpx.Observe(s.Log, h), rt
}

func (s *Server) metaRoutes(rt *Router) {
	rt.Handle("GET", "/health", func(w http.ResponseWriter, r *http.Request) {
		httpx.RawJSON(w, http.StatusOK, []byte(`{"status":"ok"}`))
	})
	rt.Handle("GET", "/version", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", instanceCacheControl)
		httpx.JSON(w, http.StatusOK, map[string]string{"service": "server", "version": Version, "revision": Revision,
			"shortRevision": ShortRevision, "buildTime": BuildTime})
	})
	rt.Handle("GET", "/instance", s.instance)
}

func isNoRows(err error) bool { return errors.Is(err, pgx.ErrNoRows) }

// Close announces the restart to open event streams and ends them, before the HTTP server drains.
func (s *Server) Close() { s.notices.shutdown() }
