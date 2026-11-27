package auth

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
)

type AccountProfile struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	IsActive       bool    `json:"isActive"`
	IsDefault      bool    `json:"isDefault"`
	LastUsedAt     int64   `json:"lastUsedAt"`
	PublicUsername *string `json:"publicUsername"`
	AvatarURL      *string `json:"avatarUrl"`
	AvatarType     *string `json:"avatarType"`
	AvatarCode     *string `json:"avatarCode"`
}

type AccountProfiles struct {
	Profiles         []AccountProfile `json:"profiles"`
	ActiveProfileID  string           `json:"activeProfileId"`
	DefaultProfileID string           `json:"defaultProfileId"`
}

var (
	ErrInvalidName        = errors.New("PROFILE_NAME_INVALID")
	ErrProfileNotFound    = errors.New("profile not found")
	ErrCannotDeleteOwner  = errors.New("owner")
	ErrCannotDeleteActive = errors.New("active")
)

func normalizeProfileName(v string) (string, bool) {
	t := strings.TrimSpace(v)
	n := JavaLength(t)
	return t, n >= 1 && n <= 40
}

func (p *Profiles) profileRow(ctx context.Context, q conn, id, active string) (AccountProfile, error) {
	a := AccountProfile{ID: id}
	err := q.QueryRow(ctx, `SELECT pa.display_name, pa.is_default, pa.last_used_at, u.public_username, u.avatar_url, u.avatar_type, u.avatar_code
FROM profile_accounts pa JOIN users u ON u.id = pa.profile_id WHERE pa.profile_id = $1`, id).
		Scan(&a.Name, &a.IsDefault, &a.LastUsedAt, &a.PublicUsername, &a.AvatarURL, &a.AvatarType, &a.AvatarCode)
	a.IsActive = id == active
	return a, err
}

func (p *Profiles) List(ctx context.Context, active string) (*AccountProfiles, error) {
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	owner, err := ownerID(ctx, tx, active)
	if err != nil || owner == "" {
		return nil, err
	}
	if err := ensureDefault(ctx, tx, owner); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, "SELECT profile_id FROM profile_accounts WHERE owner_user_id = $1", owner)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		rows.Scan(&id)
		ids = append(ids, id)
	}
	rows.Close()
	out := &AccountProfiles{Profiles: []AccountProfile{}, ActiveProfileID: active, DefaultProfileID: active}
	for _, id := range ids {
		a, err := p.profileRow(ctx, tx, id, active)
		if err != nil {
			return nil, err
		}
		out.Profiles = append(out.Profiles, a)
	}
	sort.SliceStable(out.Profiles, func(i, j int) bool {
		a, b := out.Profiles[i], out.Profiles[j]
		if a.IsDefault != b.IsDefault {
			return a.IsDefault
		}
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	})
	for _, a := range out.Profiles {
		if a.IsDefault {
			out.DefaultProfileID = a.ID
			break
		}
	}
	return out, tx.Commit(ctx)
}

func (p *Profiles) Create(ctx context.Context, active, name string) (AccountProfile, error) {
	n, ok := normalizeProfileName(name)
	if !ok {
		return AccountProfile{}, ErrInvalidName
	}
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return AccountProfile{}, err
	}
	defer tx.Rollback(ctx)
	owner, err := ownerID(ctx, tx, active)
	if err != nil {
		return AccountProfile{}, err
	}
	if owner == "" {
		return AccountProfile{}, ErrProfileNotFound
	}
	id, now := UUID(), nowMs()
	if _, err := tx.Exec(ctx, `INSERT INTO users (id, email, password_hash, name, role, verified, created_at, updated_at)
VALUES ($1, $2, $3, $4, 'user', TRUE, $5, $5) ON CONFLICT DO NOTHING`, id, "profile-"+id+"@profiles.invalid", "profile:"+id, n, now); err != nil {
		return AccountProfile{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO profile_accounts (profile_id, owner_user_id, display_name, is_default, last_used_at, created_at, updated_at)
VALUES ($1, $2, $3, FALSE, 0, $4, $4) ON CONFLICT DO NOTHING`, id, owner, n, now); err != nil {
		return AccountProfile{}, err
	}
	a, err := p.profileRow(ctx, tx, id, active)
	if err != nil {
		return a, err
	}
	return a, tx.Commit(ctx)
}

// target checks that profileID belongs to the account of the active profile.
func (p *Profiles) target(ctx context.Context, tx pgx.Tx, active, profileID string) (string, string, error) {
	owner, err := ownerID(ctx, tx, active)
	if err != nil {
		return "", "", err
	}
	if owner == "" {
		return "", "", ErrProfileNotFound
	}
	var id string
	err = tx.QueryRow(ctx, "SELECT profile_id FROM profile_accounts WHERE owner_user_id = $1 AND profile_id = $2", owner, profileID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return owner, "", ErrProfileNotFound
	}
	return owner, id, err
}

func (p *Profiles) mutate(ctx context.Context, active, profileID string, fn func(tx pgx.Tx, owner, id string) error, as string) (AccountProfile, error) {
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return AccountProfile{}, err
	}
	defer tx.Rollback(ctx)
	owner, id, err := p.target(ctx, tx, active, profileID)
	if err != nil {
		return AccountProfile{}, err
	}
	if err := fn(tx, owner, id); err != nil {
		return AccountProfile{}, err
	}
	if as == "" {
		as = active
	}
	a, err := p.profileRow(ctx, tx, id, as)
	if err != nil {
		return a, err
	}
	return a, tx.Commit(ctx)
}

func (p *Profiles) Rename(ctx context.Context, active, profileID, name string) (AccountProfile, error) {
	n, ok := normalizeProfileName(name)
	if !ok {
		return AccountProfile{}, ErrInvalidName
	}
	return p.mutate(ctx, active, profileID, func(tx pgx.Tx, _, id string) error {
		_, err := tx.Exec(ctx, "UPDATE profile_accounts SET display_name = $2, updated_at = $3 WHERE profile_id = $1", id, n, nowMs())
		return err
	}, "")
}

func (p *Profiles) SetDefault(ctx context.Context, active, profileID string) (AccountProfile, error) {
	return p.mutate(ctx, active, profileID, func(tx pgx.Tx, owner, id string) error {
		if _, err := tx.Exec(ctx, "UPDATE profile_accounts SET is_default = FALSE WHERE owner_user_id = $1", owner); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, "UPDATE profile_accounts SET is_default = TRUE, updated_at = $2 WHERE profile_id = $1", id, nowMs())
		return err
	}, "")
}

func (p *Profiles) Switch(ctx context.Context, active, profileID string) (AccountProfile, error) {
	return p.mutate(ctx, active, profileID, func(tx pgx.Tx, _, id string) error {
		_, err := tx.Exec(ctx, "UPDATE profile_accounts SET last_used_at = $2 WHERE profile_id = $1", id, nowMs())
		return err
	}, profileID)
}

// userTables are the tables keyed by user_id that a profile's deletion clears, ProfileDataDeletionService's list.
var userTables = []string{"history", "favorites", "progress", "watch_later", "subscriptions", "subscription_groups", "saved_playlists",
	"search_history", "settings", "allowed_channels", "allowed_playlists", "blocked_channels", "blocked_keywords", "blocked_videos",
	"bug_reports", "notification_states", "notification_read_items", "push_notification_deliveries", "push_notification_seen_videos",
	"push_notification_baselines", "push_devices", "channel_notification_preferences", "password_resets", "sessions", "user_avatars",
	"user_seekbar_icons", "user_channel_interest", "user_topic_interest", "youtube_sessions", "twitch_accounts", "youtube_session_pairings",
	"youtube_takeout_import_jobs", "youtube_takeout_playlist_keys", "recommendation_events", "recommendation_feed_history",
	"recommendation_feedback", "recommendation_onboarding_preferences", "recommendation_onboarding_state", "recommender_user_settings",
	"focus_settings", "recommender_feedback", "rss_user_policies", "twitch_live_notifications",
	"twitch_live_notification_settings"}

func (p *Profiles) Delete(ctx context.Context, active, profileID string) error {
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	owner, id, err := p.target(ctx, tx, active, profileID)
	if err != nil {
		return err
	}
	if id == owner {
		return ErrCannotDeleteOwner
	}
	if id == active {
		return ErrCannotDeleteActive
	}
	stmts := []string{
		"DELETE FROM rss_feed_channels WHERE feed_id IN (SELECT id FROM rss_feeds WHERE user_id = $1)",
		"DELETE FROM rss_feed_services WHERE feed_id IN (SELECT id FROM rss_feeds WHERE user_id = $1)",
		"DELETE FROM rss_feeds WHERE user_id = $1",
		"DELETE FROM subscription_group_memberships WHERE user_id = $1 OR group_id IN (SELECT id FROM subscription_groups WHERE user_id = $1)",
		"DELETE FROM playlist_videos WHERE user_id = $1",
		"DELETE FROM playlists WHERE user_id = $1",
	}
	for _, t := range userTables {
		stmts = append(stmts, "DELETE FROM "+t+" WHERE user_id = $1")
	}
	stmts = append(stmts, "DELETE FROM users WHERE id = $1", "DELETE FROM profile_accounts WHERE profile_id = $1")
	for _, q := range stmts {
		if _, err := tx.Exec(ctx, q, id); err != nil {
			return err
		}
	}
	if err := ensureDefault(ctx, tx, owner); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
