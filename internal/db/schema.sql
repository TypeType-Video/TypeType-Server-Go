CREATE TABLE IF NOT EXISTS users (
    id TEXT NOT NULL,
    email TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    name TEXT NOT NULL,
    role TEXT NOT NULL,
    avatar_url TEXT NULL,
    avatar_type TEXT NULL,
    avatar_code TEXT NULL,
    public_username TEXT NULL,
    bio TEXT NULL,
    oidc_issuer TEXT NULL,
    oidc_subject TEXT NULL,
    verified BOOLEAN NOT NULL DEFAULT FALSE,
    suspended BOOLEAN NOT NULL DEFAULT FALSE,
    created_at BIGINT NOT NULL,
    updated_at BIGINT NOT NULL,
    CONSTRAINT pk_users PRIMARY KEY (id)
);
DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'users_email_unique') THEN ALTER TABLE users ADD CONSTRAINT users_email_unique UNIQUE (email); END IF; END $$;
DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_class WHERE relname = 'users_public_username_unique') THEN ALTER TABLE users ADD CONSTRAINT users_public_username_unique UNIQUE (public_username); END IF; END $$;
CREATE TABLE IF NOT EXISTS profile_accounts (
    profile_id TEXT NOT NULL,
    owner_user_id TEXT NOT NULL,
    display_name TEXT NOT NULL,
    is_default BOOLEAN NOT NULL,
    last_used_at BIGINT NOT NULL,
    created_at BIGINT NOT NULL,
    updated_at BIGINT NOT NULL,
    CONSTRAINT pk_profile_accounts PRIMARY KEY (profile_id)
);
CREATE INDEX IF NOT EXISTS profile_accounts_owner_user_id ON profile_accounts (owner_user_id);
CREATE INDEX IF NOT EXISTS profile_accounts_owner_user_id_is_default ON profile_accounts (owner_user_id, is_default);
CREATE TABLE IF NOT EXISTS presence_keys (
    id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    name TEXT NOT NULL,
    token_prefix TEXT NOT NULL,
    token_hash TEXT NOT NULL,
    created_at BIGINT NOT NULL,
    last_used_at BIGINT NULL,
    CONSTRAINT pk_presence_keys PRIMARY KEY (id)
);
DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'presence_keys_token_hash_unique') THEN ALTER TABLE presence_keys ADD CONSTRAINT presence_keys_token_hash_unique UNIQUE (token_hash); END IF; END $$;
CREATE INDEX IF NOT EXISTS presence_keys_user_id ON presence_keys (user_id);
CREATE TABLE IF NOT EXISTS user_avatars (
    user_id TEXT NOT NULL,
    media_type TEXT NOT NULL,
    content bytea NOT NULL,
    version TEXT NOT NULL,
    updated_at BIGINT NOT NULL,
    CONSTRAINT pk_user_avatars PRIMARY KEY (user_id)
);
CREATE TABLE IF NOT EXISTS user_seekbar_icons (
    user_id TEXT NOT NULL,
    media_type TEXT NOT NULL,
    content bytea NOT NULL,
    version TEXT NOT NULL,
    updated_at BIGINT NOT NULL,
    CONSTRAINT pk_user_seekbar_icons PRIMARY KEY (user_id)
);
CREATE TABLE IF NOT EXISTS sessions (
    id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    token TEXT NOT NULL,
    refresh_token_hash TEXT NULL,
    expires_at BIGINT NOT NULL,
    created_at BIGINT NOT NULL DEFAULT 0,
    revoked_at BIGINT NULL,
    CONSTRAINT pk_sessions PRIMARY KEY (id)
);
DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'sessions_token_unique') THEN ALTER TABLE sessions ADD CONSTRAINT sessions_token_unique UNIQUE (token); END IF; END $$;
CREATE TABLE IF NOT EXISTS admin_settings (
    id INT NOT NULL DEFAULT 1,
    name TEXT NOT NULL DEFAULT 'TypeType',
    tagline TEXT NULL,
    logo_url TEXT NULL,
    banner_url TEXT NULL,
    min_android_client_version TEXT NULL,
    allow_registration BOOLEAN NOT NULL DEFAULT TRUE,
    allow_guest BOOLEAN NOT NULL DEFAULT TRUE,
    force_email_verification BOOLEAN NOT NULL DEFAULT FALSE,
    active_sessions_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    local_login_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    oidc_auto_redirect BOOLEAN NOT NULL DEFAULT FALSE,
    youtube_remote_login_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    device_login_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    bilibili_mirror_probing_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    access_mode TEXT NOT NULL DEFAULT 'unrestricted',
    rss_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    rss_public_base_url TEXT NULL,
    rss_max_feeds_per_user INT NOT NULL DEFAULT 10,
    rss_max_items INT NOT NULL DEFAULT 50,
    rss_minimum_poll_minutes INT NOT NULL DEFAULT 5,
    rss_rate_limit_per_minute INT NOT NULL DEFAULT 30,
    recommender_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    recommender_default_on BOOLEAN NOT NULL DEFAULT FALSE,
    recommender_default_algorithm TEXT NULL,
    recommender_allowed_algorithms TEXT NOT NULL DEFAULT '',
    recommender_timeout_ms INT NOT NULL DEFAULT 0,
    recommender_stream_requests_per_minute INT NOT NULL DEFAULT 0,
    CONSTRAINT pk_admin_settings PRIMARY KEY (id)
);
CREATE TABLE IF NOT EXISTS history (
    id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    url TEXT NOT NULL,
    title TEXT NOT NULL,
    thumbnail TEXT NOT NULL,
    channel_name TEXT NOT NULL,
    channel_url TEXT NOT NULL,
    channel_avatar TEXT NOT NULL DEFAULT '',
    duration BIGINT NOT NULL,
    progress BIGINT NOT NULL,
    watched_at BIGINT NOT NULL,
    CONSTRAINT pk_history PRIMARY KEY (id)
);
CREATE TABLE IF NOT EXISTS subscriptions (
    user_id TEXT NOT NULL,
    channel_url TEXT NOT NULL,
    name TEXT NOT NULL,
    avatar_url TEXT NOT NULL,
    subscribed_at BIGINT NOT NULL,
    CONSTRAINT pk_subscriptions PRIMARY KEY (user_id, channel_url)
);
CREATE TABLE IF NOT EXISTS subscription_groups (
    id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    name TEXT NOT NULL,
    normalized_name TEXT NOT NULL,
    created_at BIGINT NOT NULL,
    updated_at BIGINT NOT NULL,
    CONSTRAINT pk_subscription_groups PRIMARY KEY (id)
);
DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'subscription_groups_user_id_normalized_name_unique') THEN ALTER TABLE subscription_groups ADD CONSTRAINT subscription_groups_user_id_normalized_name_unique UNIQUE (user_id, normalized_name); END IF; END $$;
CREATE TABLE IF NOT EXISTS subscription_group_memberships (
    group_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    channel_url TEXT NOT NULL,
    added_at BIGINT NOT NULL,
    CONSTRAINT pk_subscription_group_memberships PRIMARY KEY (group_id, channel_url),
    CONSTRAINT fk_subscription_group_memberships_group_id__id FOREIGN KEY (group_id) REFERENCES subscription_groups(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS subscription_group_memberships_user_id_channel_url ON subscription_group_memberships (user_id, channel_url);
CREATE TABLE IF NOT EXISTS focus_settings (
    user_id TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    entertainment_group_id TEXT NULL,
    study_group_id TEXT NULL,
    schedule TEXT NOT NULL DEFAULT 'always',
    timezone TEXT NULL,
    transition_days INT NOT NULL DEFAULT 60,
    archive_days INT NOT NULL DEFAULT 7,
    strict_blocking BOOLEAN NOT NULL DEFAULT FALSE,
    grayscale_thumbnails BOOLEAN NOT NULL DEFAULT FALSE,
    started_at BIGINT NOT NULL DEFAULT 0,
    disable_at BIGINT NULL,
    updated_at BIGINT NOT NULL,
    CONSTRAINT pk_focus_settings PRIMARY KEY (user_id)
);
CREATE TABLE IF NOT EXISTS playlists (
    id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    name TEXT NOT NULL,
    description TEXT NOT NULL,
    created_at BIGINT NOT NULL,
    CONSTRAINT pk_playlists PRIMARY KEY (id)
);
CREATE TABLE IF NOT EXISTS playlist_videos (
    id TEXT NOT NULL,
    playlist_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    url TEXT NOT NULL,
    title TEXT NOT NULL,
    thumbnail TEXT NOT NULL,
    duration BIGINT NOT NULL,
    position INT NOT NULL,
    channel_name TEXT NOT NULL DEFAULT '',
    channel_url TEXT NOT NULL DEFAULT '',
    channel_avatar TEXT NOT NULL DEFAULT '',
    view_count BIGINT NOT NULL DEFAULT 0,
    added_at BIGINT NOT NULL DEFAULT 0,
    published_at BIGINT NOT NULL DEFAULT -1,
    metadata_refreshed_at BIGINT NOT NULL DEFAULT 0,
    CONSTRAINT pk_playlist_videos PRIMARY KEY (id),
    CONSTRAINT fk_playlist_videos_playlist_id__id FOREIGN KEY (playlist_id) REFERENCES playlists(id) ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS watch_later (
    user_id TEXT NOT NULL,
    url TEXT NOT NULL,
    title TEXT NOT NULL,
    thumbnail TEXT NOT NULL,
    duration BIGINT NOT NULL,
    added_at BIGINT NOT NULL,
    channel_name TEXT NOT NULL DEFAULT '',
    channel_url TEXT NOT NULL DEFAULT '',
    channel_avatar TEXT NOT NULL DEFAULT '',
    view_count BIGINT NOT NULL DEFAULT 0,
    published_at BIGINT NOT NULL DEFAULT -1,
    metadata_refreshed_at BIGINT NOT NULL DEFAULT 0,
    CONSTRAINT pk_watch_later PRIMARY KEY (user_id, url)
);
CREATE TABLE IF NOT EXISTS progress (
    user_id TEXT NOT NULL,
    video_url TEXT NOT NULL,
    position BIGINT NOT NULL,
    updated_at BIGINT NOT NULL,
    CONSTRAINT pk_progress PRIMARY KEY (user_id, video_url)
);
CREATE TABLE IF NOT EXISTS saved_playlists (
    id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    public_playlist_id TEXT NOT NULL,
    url TEXT NOT NULL,
    title TEXT NOT NULL,
    thumbnail_url TEXT NOT NULL,
    uploader_name TEXT NOT NULL,
    stream_count BIGINT NOT NULL,
    playlist_type TEXT NOT NULL,
    saved_at BIGINT NOT NULL,
    CONSTRAINT pk_saved_playlists PRIMARY KEY (id)
);
CREATE TABLE IF NOT EXISTS favorites (
    user_id TEXT NOT NULL,
    video_url TEXT NOT NULL,
    favorited_at BIGINT NOT NULL,
    title TEXT NOT NULL DEFAULT '',
    thumbnail TEXT NOT NULL DEFAULT '',
    duration BIGINT NOT NULL DEFAULT 0,
    channel_name TEXT NOT NULL DEFAULT '',
    channel_url TEXT NOT NULL DEFAULT '',
    channel_avatar TEXT NOT NULL DEFAULT '',
    view_count BIGINT NOT NULL DEFAULT 0,
    published_at BIGINT NOT NULL DEFAULT -1,
    metadata_refreshed_at BIGINT NOT NULL DEFAULT 0,
    CONSTRAINT pk_favorites PRIMARY KEY (user_id, video_url)
);
CREATE TABLE IF NOT EXISTS settings (
    user_id TEXT NOT NULL,
    default_service INT NOT NULL DEFAULT 0,
    default_quality TEXT NOT NULL DEFAULT '1080p',
    default_playback_speed DOUBLE PRECISION NOT NULL DEFAULT 1.0,
    hold_playback_speed DOUBLE PRECISION NOT NULL DEFAULT 2.0,
    default_landing_page TEXT NOT NULL DEFAULT 'home',
    video_grid_columns INT NOT NULL DEFAULT 0,
    related_video_size TEXT NOT NULL DEFAULT 'default',
    autoplay BOOLEAN NOT NULL DEFAULT TRUE,
    autoplay_on_open BOOLEAN NOT NULL DEFAULT TRUE,
    autoplay_countdown_seconds INT NOT NULL DEFAULT 10,
    skip_playlist_autoplay_screen BOOLEAN NOT NULL DEFAULT FALSE,
    hide_upcoming_playlist_videos BOOLEAN NOT NULL DEFAULT FALSE,
    ambient_light BOOLEAN NOT NULL DEFAULT FALSE,
    ambient_light_spread INT NOT NULL DEFAULT 100,
    ambient_light_blur INT NOT NULL DEFAULT 50,
    ambient_light_brightness INT NOT NULL DEFAULT 100,
    volume DOUBLE PRECISION NOT NULL DEFAULT 1.0,
    muted BOOLEAN NOT NULL DEFAULT FALSE,
    notification_popups_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    subtitles_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    default_subtitle_language TEXT NOT NULL DEFAULT '',
    default_audio_language TEXT NOT NULL DEFAULT '',
    caption_styles TEXT NOT NULL DEFAULT '{}',
    prefer_original_language BOOLEAN NOT NULL DEFAULT FALSE,
    enable_high_quality_playback BOOLEAN NOT NULL DEFAULT FALSE,
    sponsor_block_mode TEXT NOT NULL DEFAULT 'auto_skip',
    sponsor_block_category_actions TEXT NOT NULL DEFAULT '{}',
    sponsor_block_minimum_duration INT NOT NULL DEFAULT 0,
    sponsor_block_show_current_segment BOOLEAN NOT NULL DEFAULT TRUE,
    sponsor_block_show_chapters BOOLEAN NOT NULL DEFAULT FALSE,
    sponsor_block_show_full_video_labels BOOLEAN NOT NULL DEFAULT TRUE,
    sponsor_block_manual_skip_on_full_video BOOLEAN NOT NULL DEFAULT TRUE,
    sponsor_block_skip_non_music_only_on_music_videos BOOLEAN NOT NULL DEFAULT FALSE,
    sponsor_block_mute_instead_of_skip BOOLEAN NOT NULL DEFAULT FALSE,
    hide_home_recommendations BOOLEAN NOT NULL DEFAULT FALSE,
    hide_continue_watching BOOLEAN NOT NULL DEFAULT FALSE,
    hide_related_videos BOOLEAN NOT NULL DEFAULT FALSE,
    hide_end_screens BOOLEAN NOT NULL DEFAULT FALSE,
    hide_comments BOOLEAN NOT NULL DEFAULT FALSE,
    hide_shorts BOOLEAN NOT NULL DEFAULT FALSE,
    hide_subscription_live_streams BOOLEAN NOT NULL DEFAULT FALSE,
    hide_members_only_content BOOLEAN NOT NULL DEFAULT FALSE,
    disable_watch_history BOOLEAN NOT NULL DEFAULT FALSE,
    dearrow_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    dearrow_title_mode TEXT NOT NULL DEFAULT 'dearrow',
    dearrow_thumbnail_mode TEXT NOT NULL DEFAULT 'dearrow_or_random',
    dearrow_trust_mode TEXT NOT NULL DEFAULT 'accepted',
    access_mode TEXT NOT NULL DEFAULT 'unrestricted',
    access_mode_admin_managed BOOLEAN NOT NULL DEFAULT FALSE,
    access_mode_admin_managed_at BIGINT NOT NULL DEFAULT 0,
    seekbar_icon TEXT NOT NULL DEFAULT 'typetype',
    seekbar_icon_url TEXT NOT NULL DEFAULT '',
    player_engine TEXT NOT NULL DEFAULT 'videojs',
    hide_watched_subscription_videos BOOLEAN NOT NULL DEFAULT FALSE,
    watched_threshold_percent INT NOT NULL DEFAULT 0,
    mini_player_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    hide_search_shorts BOOLEAN NOT NULL DEFAULT TRUE,
    CONSTRAINT pk_settings PRIMARY KEY (user_id)
);
CREATE TABLE IF NOT EXISTS allowed_channels (
    user_id TEXT NOT NULL,
    scope TEXT NOT NULL DEFAULT 'user',
    channel_url TEXT NOT NULL,
    name TEXT NULL,
    thumbnail_url TEXT NULL,
    allowed_at BIGINT NOT NULL,
    CONSTRAINT pk_allowed_channels PRIMARY KEY (user_id, channel_url)
);
CREATE TABLE IF NOT EXISTS allowed_playlists (
    user_id TEXT NOT NULL,
    scope TEXT NOT NULL DEFAULT 'user',
    playlist_url TEXT NOT NULL,
    title TEXT NULL,
    thumbnail_url TEXT NULL,
    uploader_name TEXT NULL,
    allowed_at BIGINT NOT NULL
);
CREATE TABLE IF NOT EXISTS search_history (
    id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    term TEXT NOT NULL,
    searched_at BIGINT NOT NULL,
    CONSTRAINT pk_search_history PRIMARY KEY (id)
);
CREATE TABLE IF NOT EXISTS blocked_channels (
    user_id TEXT NOT NULL,
    scope TEXT NOT NULL DEFAULT 'user',
    channel_url TEXT NOT NULL,
    name TEXT NULL,
    thumbnail_url TEXT NULL,
    blocked_at BIGINT NOT NULL,
    CONSTRAINT pk_blocked_channels PRIMARY KEY (user_id, channel_url)
);
CREATE TABLE IF NOT EXISTS blocked_keywords (
    user_id TEXT NOT NULL,
    scope TEXT NOT NULL DEFAULT 'user',
    keyword TEXT NOT NULL,
    blocked_at BIGINT NOT NULL,
    CONSTRAINT pk_blocked_keywords PRIMARY KEY (user_id, keyword)
);
CREATE TABLE IF NOT EXISTS blocked_videos (
    user_id TEXT NOT NULL,
    scope TEXT NOT NULL DEFAULT 'user',
    video_url TEXT NOT NULL,
    blocked_at BIGINT NOT NULL,
    CONSTRAINT pk_blocked_videos PRIMARY KEY (user_id, video_url)
);
CREATE TABLE IF NOT EXISTS password_resets (
    id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    token TEXT NOT NULL,
    expires_at BIGINT NOT NULL,
    used_at BIGINT NULL,
    CONSTRAINT pk_password_resets PRIMARY KEY (id)
);
DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'password_resets_token_unique') THEN ALTER TABLE password_resets ADD CONSTRAINT password_resets_token_unique UNIQUE (token); END IF; END $$;
CREATE TABLE IF NOT EXISTS youtube_takeout_import_jobs (
    id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    status TEXT NOT NULL,
    phase TEXT NOT NULL,
    progress INT NOT NULL,
    parse_completed BOOLEAN NOT NULL,
    import_started BOOLEAN NOT NULL,
    import_completed BOOLEAN NOT NULL,
    archive_path TEXT NULL,
    preview_json TEXT NULL,
    error TEXT NULL,
    report_json TEXT NULL,
    created_at BIGINT NOT NULL,
    updated_at BIGINT NOT NULL,
    expires_at BIGINT NOT NULL,
    CONSTRAINT pk_youtube_takeout_import_jobs PRIMARY KEY (id)
);
CREATE TABLE IF NOT EXISTS youtube_takeout_playlist_keys (
    user_id TEXT NOT NULL,
    source_key TEXT NOT NULL,
    playlist_id TEXT NOT NULL,
    updated_at BIGINT NOT NULL,
    CONSTRAINT pk_youtube_takeout_playlist_keys PRIMARY KEY (user_id, source_key)
);
CREATE TABLE IF NOT EXISTS youtube_sessions (
    user_id TEXT NOT NULL,
    encrypted_cookies TEXT NOT NULL,
    encrypted_po_token TEXT NOT NULL,
    auth_user INT NOT NULL DEFAULT 0,
    status TEXT NOT NULL,
    created_at BIGINT NOT NULL,
    updated_at BIGINT NOT NULL,
    last_used_at BIGINT NOT NULL DEFAULT 0,
    auth_type TEXT NOT NULL DEFAULT 'cookies',
    encrypted_access_token TEXT NOT NULL DEFAULT '',
    encrypted_refresh_token TEXT NOT NULL DEFAULT '',
    access_expires_at BIGINT NOT NULL DEFAULT 0,
    account_label TEXT NOT NULL DEFAULT '',
    CONSTRAINT pk_youtube_sessions PRIMARY KEY (user_id)
);
CREATE TABLE IF NOT EXISTS youtube_session_pairings (
    code TEXT NOT NULL,
    user_id TEXT NOT NULL,
    created_at BIGINT NOT NULL,
    expires_at BIGINT NOT NULL,
    CONSTRAINT pk_youtube_session_pairings PRIMARY KEY (code)
);
CREATE TABLE IF NOT EXISTS bilibili_sessions (
    user_id TEXT NOT NULL,
    encrypted_cookies TEXT NOT NULL,
    status TEXT NOT NULL,
    created_at BIGINT NOT NULL,
    updated_at BIGINT NOT NULL,
    last_used_at BIGINT NOT NULL DEFAULT 0,
    expires_at BIGINT NOT NULL DEFAULT 0,
    encrypted_refresh_token TEXT NULL,
    encrypted_pending_confirm_token TEXT NULL,
    refresh_checked_at BIGINT NOT NULL DEFAULT 0,
    refresh_claimed_until BIGINT NOT NULL DEFAULT 0,
    CONSTRAINT pk_bilibili_sessions PRIMARY KEY (user_id)
);
CREATE TABLE IF NOT EXISTS twitch_accounts (
    user_id TEXT NOT NULL,
    encrypted_access_token TEXT NOT NULL,
    encrypted_refresh_token TEXT NULL,
    login TEXT NOT NULL,
    display_name TEXT NOT NULL,
    avatar_url TEXT NOT NULL DEFAULT '',
    expires_at BIGINT NOT NULL DEFAULT 0,
    created_at BIGINT NOT NULL,
    updated_at BIGINT NOT NULL,
    CONSTRAINT pk_twitch_accounts PRIMARY KEY (user_id)
);
CREATE TABLE IF NOT EXISTS bug_reports (
    id VARCHAR(64) NOT NULL,
    user_id TEXT NOT NULL,
    category VARCHAR(50) NOT NULL,
    description TEXT NOT NULL,
    context TEXT NOT NULL,
    status VARCHAR(20) NOT NULL,
    github_issue_url TEXT NULL,
    created_at BIGINT NOT NULL,
    updated_at BIGINT NOT NULL,
    CONSTRAINT pk_bug_reports PRIMARY KEY (id)
);
CREATE INDEX IF NOT EXISTS bug_reports_status ON bug_reports (status);
CREATE INDEX IF NOT EXISTS bug_reports_category ON bug_reports (category);
CREATE TABLE IF NOT EXISTS notification_states (
    user_id TEXT NOT NULL,
    subscription_last_seen_uploaded BIGINT NOT NULL,
    updated_at BIGINT NOT NULL,
    CONSTRAINT pk_notification_states PRIMARY KEY (user_id)
);
CREATE TABLE IF NOT EXISTS notification_read_items (
    user_id TEXT NOT NULL,
    notification_id TEXT NOT NULL,
    read_at BIGINT NOT NULL,
    CONSTRAINT pk_notification_read_items PRIMARY KEY (user_id, notification_id)
);
CREATE INDEX IF NOT EXISTS notification_read_items_user_id_read_at ON notification_read_items (user_id, read_at);
CREATE TABLE IF NOT EXISTS channel_notification_preferences (
    user_id TEXT NOT NULL,
    channel_url TEXT NOT NULL,
    enabled BOOLEAN NOT NULL,
    updated_at BIGINT NOT NULL,
    CONSTRAINT pk_channel_notification_preferences PRIMARY KEY (user_id, channel_url)
);
CREATE INDEX IF NOT EXISTS channel_notification_preferences_user_id ON channel_notification_preferences (user_id);
CREATE TABLE IF NOT EXISTS push_devices (
    id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    device_id TEXT NOT NULL,
    platform TEXT NOT NULL,
    endpoint TEXT NOT NULL,
    endpoint_hash TEXT NOT NULL,
    p256dh TEXT NULL,
    auth_secret TEXT NULL,
    expires_at BIGINT NULL,
    created_at BIGINT NOT NULL,
    updated_at BIGINT NOT NULL,
    CONSTRAINT pk_push_devices PRIMARY KEY (id)
);
DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'push_devices_endpoint_hash_unique') THEN ALTER TABLE push_devices ADD CONSTRAINT push_devices_endpoint_hash_unique UNIQUE (endpoint_hash); END IF; END $$;
CREATE INDEX IF NOT EXISTS push_devices_user_id ON push_devices (user_id);
DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'push_devices_user_id_device_id_unique') THEN ALTER TABLE push_devices ADD CONSTRAINT push_devices_user_id_device_id_unique UNIQUE (user_id, device_id); END IF; END $$;
CREATE TABLE IF NOT EXISTS push_notification_baselines (
    user_id TEXT NOT NULL,
    initialized_at BIGINT NOT NULL,
    CONSTRAINT pk_push_notification_baselines PRIMARY KEY (user_id)
);
CREATE TABLE IF NOT EXISTS push_notification_seen_videos (
    user_id TEXT NOT NULL,
    service_id INT NOT NULL,
    channel_id TEXT NOT NULL,
    video_id TEXT NOT NULL,
    first_seen_at BIGINT NOT NULL,
    CONSTRAINT pk_push_notification_seen_videos PRIMARY KEY (user_id, service_id, channel_id, video_id)
);
CREATE INDEX IF NOT EXISTS push_notification_seen_videos_user_id ON push_notification_seen_videos (user_id);
CREATE TABLE IF NOT EXISTS push_notification_events (
    event_id TEXT NOT NULL,
    event_type TEXT NOT NULL,
    instance_id TEXT NOT NULL,
    service_id INT NOT NULL,
    channel_id TEXT NOT NULL,
    video_id TEXT NOT NULL,
    video_url TEXT NOT NULL,
    title TEXT NOT NULL,
    channel_name TEXT NOT NULL,
    channel_avatar_url TEXT NOT NULL,
    published_at BIGINT NOT NULL,
    created_at BIGINT NOT NULL,
    CONSTRAINT pk_push_notification_events PRIMARY KEY (event_id)
);
CREATE INDEX IF NOT EXISTS push_notification_events_created_at ON push_notification_events (created_at);
CREATE TABLE IF NOT EXISTS push_notification_deliveries (
    event_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    device_id TEXT NOT NULL,
    status TEXT NOT NULL,
    attempts INT NOT NULL,
    last_error TEXT NULL,
    updated_at BIGINT NOT NULL,
    CONSTRAINT pk_push_notification_deliveries PRIMARY KEY (event_id, user_id, device_id)
);
CREATE INDEX IF NOT EXISTS push_notification_deliveries_user_id_status ON push_notification_deliveries (user_id, status);
CREATE INDEX IF NOT EXISTS push_notification_deliveries_device_id ON push_notification_deliveries (device_id);
CREATE TABLE IF NOT EXISTS user_channel_interest (
    user_id TEXT NOT NULL,
    uploader_url TEXT NOT NULL,
    score DOUBLE PRECISION NOT NULL,
    updated_at BIGINT NOT NULL,
    CONSTRAINT pk_user_channel_interest PRIMARY KEY (user_id, uploader_url)
);
CREATE TABLE IF NOT EXISTS user_topic_interest (
    user_id TEXT NOT NULL,
    topic TEXT NOT NULL,
    score DOUBLE PRECISION NOT NULL,
    updated_at BIGINT NOT NULL,
    CONSTRAINT pk_user_topic_interest PRIMARY KEY (user_id, topic)
);
CREATE TABLE IF NOT EXISTS recommendation_events (
    id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    event_type TEXT NOT NULL,
    video_url TEXT NULL,
    uploader_url TEXT NULL,
    title TEXT NULL,
    watch_ratio DOUBLE PRECISION NULL,
    watch_duration_ms BIGINT NULL,
    context_key TEXT NULL,
    occurred_at BIGINT NOT NULL,
    CONSTRAINT pk_recommendation_events PRIMARY KEY (id)
);
CREATE TABLE IF NOT EXISTS recommendation_feed_history (
    user_id TEXT NOT NULL,
    video_url TEXT NOT NULL,
    show_count INT NOT NULL,
    last_shown BIGINT NOT NULL,
    CONSTRAINT pk_recommendation_feed_history PRIMARY KEY (user_id, video_url)
);
CREATE TABLE IF NOT EXISTS recommendation_feedback (
    id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    feedback_type TEXT NOT NULL,
    video_url TEXT NULL,
    uploader_url TEXT NULL,
    created_at BIGINT NOT NULL,
    CONSTRAINT pk_recommendation_feedback PRIMARY KEY (id)
);
CREATE TABLE IF NOT EXISTS recommendation_onboarding_preferences (
    user_id TEXT NOT NULL,
    preference_type TEXT NOT NULL,
    value TEXT NOT NULL,
    created_at BIGINT NOT NULL,
    CONSTRAINT pk_recommendation_onboarding_preferences PRIMARY KEY (user_id, preference_type, value)
);
CREATE TABLE IF NOT EXISTS recommendation_onboarding_state (
    user_id TEXT NOT NULL,
    completed_at BIGINT NULL,
    updated_at BIGINT NOT NULL,
    CONSTRAINT pk_recommendation_onboarding_state PRIMARY KEY (user_id)
);
CREATE TABLE IF NOT EXISTS recommender_user_settings (
    user_id TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    algorithm_id TEXT NULL,
    settings_json TEXT NOT NULL DEFAULT '{}',
    updated_at BIGINT NOT NULL,
    CONSTRAINT pk_recommender_user_settings PRIMARY KEY (user_id)
);
CREATE TABLE IF NOT EXISTS recommender_feedback (
    id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    kind TEXT NOT NULL,
    verdict TEXT NOT NULL,
    comment TEXT NULL,
    video_url TEXT NULL,
    reason TEXT NULL,
    title TEXT NULL,
    channel_name TEXT NULL,
    thumbnail_url TEXT NULL,
    algorithm_id TEXT NOT NULL,
    parameters_json TEXT NOT NULL DEFAULT '{}',
    created_at BIGINT NOT NULL,
    CONSTRAINT pk_recommender_feedback PRIMARY KEY (id)
);
CREATE INDEX IF NOT EXISTS recommender_feedback_user_id ON recommender_feedback (user_id);
CREATE TABLE IF NOT EXISTS rss_feeds (
    id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    name TEXT NOT NULL,
    token_hash TEXT NOT NULL,
    scope TEXT NOT NULL,
    include_videos BOOLEAN NOT NULL,
    include_shorts BOOLEAN NOT NULL,
    include_live BOOLEAN NOT NULL,
    include_upcoming BOOLEAN NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at BIGINT NOT NULL,
    updated_at BIGINT NOT NULL,
    last_used_at BIGINT NULL,
    CONSTRAINT pk_rss_feeds PRIMARY KEY (id)
);
CREATE INDEX IF NOT EXISTS rss_feeds_user_id ON rss_feeds (user_id);
CREATE INDEX IF NOT EXISTS rss_feeds_created_at ON rss_feeds (created_at);
CREATE TABLE IF NOT EXISTS rss_feed_channels (
    feed_id TEXT NOT NULL,
    channel_url TEXT NOT NULL,
    CONSTRAINT pk_rss_feed_channels PRIMARY KEY (feed_id, channel_url)
);
CREATE TABLE IF NOT EXISTS rss_feed_services (
    feed_id TEXT NOT NULL,
    service_id INT NOT NULL,
    CONSTRAINT pk_rss_feed_services PRIMARY KEY (feed_id, service_id)
);
CREATE TABLE IF NOT EXISTS rss_user_policies (
    user_id TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    updated_at BIGINT NOT NULL,
    CONSTRAINT pk_rss_user_policies PRIMARY KEY (user_id)
);
CREATE TABLE IF NOT EXISTS twitch_live_notifications (
    user_id TEXT NOT NULL,
    stream_id TEXT NOT NULL,
    channel_login TEXT NOT NULL,
    channel_name TEXT NOT NULL,
    channel_avatar_url TEXT NOT NULL DEFAULT '',
    title TEXT NOT NULL DEFAULT '',
    category TEXT NOT NULL DEFAULT '',
    started_at BIGINT NOT NULL,
    created_at BIGINT NOT NULL,
    CONSTRAINT pk_twitch_live_notifications PRIMARY KEY (user_id, stream_id)
);
CREATE TABLE IF NOT EXISTS twitch_live_notification_settings (
    user_id TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    watching_since BIGINT NOT NULL,
    updated_at BIGINT NOT NULL,
    CONSTRAINT pk_twitch_live_notification_settings PRIMARY KEY (user_id)
);
