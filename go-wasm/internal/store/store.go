// Package store is the PostgreSQL persistence layer for published pages and
// their click/view metrics.
package store

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"connect-with-playlist-wasm/internal/state"
	"connect-with-playlist-wasm/internal/token"
)

//go:embed schema.sql
var schemaSQL string

// Sentinel errors mapped to HTTP statuses by the API layer.
var (
	ErrNotFound = errors.New("playlist not found")
	ErrBadToken = errors.New("invalid edit token")
	ErrNoHandle = errors.New("could not allocate a unique handle")
)

// Store wraps a pgx connection pool.
type Store struct {
	pool *pgxpool.Pool
}

// Playlist is a stored page row (without the secret token hash).
type Playlist struct {
	ID        string
	Handle    string
	State     state.State
	ViewCount int64
	CreatedAt time.Time
	UpdatedAt time.Time
}

// SitemapPage is the minimal public-page row needed to build sitemap.xml.
type SitemapPage struct {
	Handle    string
	UpdatedAt time.Time
}

// New opens a pool and verifies connectivity.
func New(ctx context.Context, dsn string) (*Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return &Store{pool: pool}, nil
}

// Close releases the pool.
func (s *Store) Close() { s.pool.Close() }

// Ping checks DB connectivity (used by /healthz).
func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

// ApplySchema runs the embedded idempotent DDL.
func (s *Store) ApplySchema(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, schemaSQL)
	return err
}

// CreatePlaylist inserts a new page, auto-suffixing the handle on collision so
// publishing never fails. The stored state.user.handle is kept in sync with the
// final allocated handle. Returns the final handle.
func (s *Store) CreatePlaylist(ctx context.Context, desiredHandle string, st state.State, editTokenHash []byte) (string, error) {
	const maxAttempts = 60
	base := desiredHandle
	candidate := desiredHandle
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		st.User.Handle = candidate
		stateJSON, err := json.Marshal(st)
		if err != nil {
			return "", err
		}
		_, err = s.pool.Exec(ctx,
			`INSERT INTO playlists (handle, title, display_name, state, edit_token_hash)
			 VALUES ($1, $2, $3, $4, $5)`,
			candidate, st.Playlist.Title, st.User.DisplayName, stateJSON, editTokenHash)
		if err == nil {
			return candidate, nil
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			candidate = suffixHandle(base, attempt+1)
			continue
		}
		return "", err
	}
	return "", ErrNoHandle
}

// suffixHandle appends -N, trimming the base so the result stays within 32 chars.
func suffixHandle(base string, n int) string {
	suffix := fmt.Sprintf("-%d", n)
	maxBase := 32 - len(suffix)
	if maxBase < 1 {
		maxBase = 1
	}
	if len(base) > maxBase {
		base = base[:maxBase]
	}
	return base + suffix
}

// GetByHandle returns the public page (no token).
func (s *Store) GetByHandle(ctx context.Context, handle string) (*Playlist, error) {
	var p Playlist
	var stateJSON []byte
	err := s.pool.QueryRow(ctx,
		`SELECT id, handle, state, view_count, created_at, updated_at
		 FROM playlists WHERE handle = $1`, handle).
		Scan(&p.ID, &p.Handle, &stateJSON, &p.ViewCount, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(stateJSON, &p.State); err != nil {
		return nil, err
	}
	return &p, nil
}

// ListSitemapPages returns every published public page in stable URL order.
func (s *Store) ListSitemapPages(ctx context.Context) ([]SitemapPage, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT handle, updated_at FROM playlists ORDER BY handle`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	pages := make([]SitemapPage, 0)
	for rows.Next() {
		var page SitemapPage
		if err := rows.Scan(&page.Handle, &page.UpdatedAt); err != nil {
			return nil, err
		}
		pages = append(pages, page)
	}
	return pages, rows.Err()
}

// IncrementViewCount bumps and returns the page's view counter (raw hits).
func (s *Store) IncrementViewCount(ctx context.Context, handle string) (int64, error) {
	var count int64
	err := s.pool.QueryRow(ctx,
		`UPDATE playlists SET view_count = view_count + 1 WHERE handle = $1 RETURNING view_count`,
		handle).Scan(&count)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNotFound
	}
	return count, err
}

// GetForEdit returns the page only if the edit token verifies.
func (s *Store) GetForEdit(ctx context.Context, handle, editToken string) (*Playlist, error) {
	var p Playlist
	var stateJSON, hash []byte
	err := s.pool.QueryRow(ctx,
		`SELECT id, handle, state, view_count, created_at, updated_at, edit_token_hash
		 FROM playlists WHERE handle = $1`, handle).
		Scan(&p.ID, &p.Handle, &stateJSON, &p.ViewCount, &p.CreatedAt, &p.UpdatedAt, &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if !token.Equal(hash, editToken) {
		return nil, ErrBadToken
	}
	if err := json.Unmarshal(stateJSON, &p.State); err != nil {
		return nil, err
	}
	return &p, nil
}

// UpdateByToken overwrites the stored state if the edit token verifies. The
// handle in the state is forced to the URL handle so it cannot be changed.
func (s *Store) UpdateByToken(ctx context.Context, handle, editToken string, st state.State) error {
	var hash []byte
	err := s.pool.QueryRow(ctx,
		`SELECT edit_token_hash FROM playlists WHERE handle = $1`, handle).Scan(&hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if !token.Equal(hash, editToken) {
		return ErrBadToken
	}
	st.User.Handle = handle
	stateJSON, err := json.Marshal(st)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx,
		`UPDATE playlists SET state = $2, title = $3, display_name = $4, updated_at = now()
		 WHERE handle = $1`,
		handle, stateJSON, st.Playlist.Title, st.User.DisplayName)
	return err
}

// HandleExists reports whether a handle is already taken.
func (s *Store) HandleExists(ctx context.Context, handle string) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM playlists WHERE handle = $1)`, handle).Scan(&exists)
	return exists, err
}

// InsertEvent records a metric event for the page identified by handle. Unknown
// handles insert nothing (no error) so the fire-and-forget client never breaks.
//
// event_day is stamped from the DB clock (UTC) and the daily dedup partial unique
// indexes collapse a visitor's repeated views/plays to one per day via
// ON CONFLICT DO NOTHING. A nil visitorHash/ipHash is stored as NULL (the event
// is kept for diagnostics but excluded from ranking aggregation).
func (s *Store) InsertEvent(ctx context.Context, handle, eventType, linkID, platform, referrer, userAgent string, visitorHash, ipHash []byte, isBot bool) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO events
		   (playlist_id, link_id, event_type, platform, referrer, user_agent,
		    visitor_hash, ip_hash, is_bot, event_day)
		 SELECT id, $2, $3, $4, $5, $6, $7, $8, $9, (now() AT TIME ZONE 'UTC')::date
		 FROM playlists WHERE handle = $1
		 ON CONFLICT DO NOTHING`,
		handle, nullify(linkID), eventType, nullify(platform), nullify(referrer), nullify(userAgent),
		visitorHash, ipHash, isBot)
	return err
}

func nullify(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// Ranking is one row of the precomputed discovery feed.
type Ranking struct {
	Handle        string
	Title         string
	DisplayName   string
	Platform      string
	LinkCount     int
	UniqueViews   int64
	EngagedPlays  int64
	Views24h      int64
	Plays24h      int64
	TrendingScore float64
	PopularScore  float64
}

// rankingColumns maps a validated section name to the ORDER BY column. The value
// is never user input — callers pass a section that was checked against this map's
// keys — so concatenating it into SQL is safe.
var rankingColumns = map[string]string{
	"trending": "trending_score",
	"today":    "today_score",
	"popular":  "popular_score",
	"new":      "created_at",
}

// ListRankings returns the top eligible playlists for a section, ordered by that
// section's precomputed score. Unknown sections fall back to trending.
func (s *Store) ListRankings(ctx context.Context, section string, limit int) ([]Ranking, error) {
	col, ok := rankingColumns[section]
	if !ok {
		col = "trending_score"
	}
	rows, err := s.pool.Query(ctx,
		`SELECT handle, title, display_name, COALESCE(primary_platform, ''), link_count,
		        unique_views, engaged_plays, views_24h, plays_24h, trending_score, popular_score
		 FROM playlist_rankings
		 WHERE eligible
		 ORDER BY `+col+` DESC
		 LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Ranking
	for rows.Next() {
		var r Ranking
		if err := rows.Scan(&r.Handle, &r.Title, &r.DisplayName, &r.Platform, &r.LinkCount,
			&r.UniqueViews, &r.EngagedPlays, &r.Views24h, &r.Plays24h, &r.TrendingScore, &r.PopularScore); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// RefreshRankings recomputes the playlist_rankings feed from deduped, non-bot
// events. One upsert covers every playlist (rows with no events get zeros). Cheap
// enough to run on a short timer at current scale; the 7-day window on trending
// and the partial dedup indexes bound the scan.
func (s *Store) RefreshRankings(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, refreshRankingsSQL)
	return err
}

// refreshRankingsSQL aggregates events and upserts scores. Weights: page_view 1,
// play 3. trending = decayed weighted sum (24h half-life, 7d window); today =
// 24h weighted volume; popular = Wilson-lower-bound play rate * ln(views+1) so
// small samples and raw-volume inflation are both damped.
const refreshRankingsSQL = `
WITH links AS (
  SELECT p.id,
         CASE WHEN jsonb_typeof(p.state->'playlist'->'links') = 'array'
              THEN p.state->'playlist'->'links' ELSE '[]'::jsonb END AS arr
  FROM playlists p
),
meta AS (
  SELECT l.id,
         jsonb_array_length(l.arr) AS link_count,
         (SELECT mode() WITHIN GROUP (ORDER BY e->>'platform')
            FROM jsonb_array_elements(l.arr) e
            WHERE e->>'platform' IS NOT NULL) AS primary_platform
  FROM links l
),
agg AS (
  SELECT e.playlist_id,
         count(*) FILTER (WHERE e.event_type = 'page_view') AS unique_views,
         count(*) FILTER (WHERE e.event_type IN ('link_play','link_open')) AS engaged_plays,
         count(*) FILTER (WHERE e.event_type = 'page_view'
                            AND e.created_at > now() - interval '24 hours') AS views_24h,
         count(*) FILTER (WHERE e.event_type IN ('link_play','link_open')
                            AND e.created_at > now() - interval '24 hours') AS plays_24h,
         COALESCE(sum(
           (CASE e.event_type WHEN 'page_view' THEN 1.0::float8 ELSE 3.0::float8 END)
           * exp(-0.6931471805599453 * extract(epoch FROM (now() - e.created_at)) / 3600.0 / 24.0)
         ) FILTER (WHERE e.created_at > now() - interval '7 days'), 0) AS trending_score
  FROM events e
  WHERE e.visitor_hash IS NOT NULL AND e.is_bot = false
  GROUP BY e.playlist_id
)
INSERT INTO playlist_rankings
  (playlist_id, handle, title, display_name, primary_platform, link_count,
   unique_views, engaged_plays, views_24h, plays_24h,
   trending_score, today_score, popular_score, eligible, created_at, refreshed_at)
SELECT p.id, p.handle, p.title, p.display_name, m.primary_platform, COALESCE(m.link_count, 0),
       COALESCE(a.unique_views, 0), COALESCE(a.engaged_plays, 0),
       COALESCE(a.views_24h, 0), COALESCE(a.plays_24h, 0),
       COALESCE(a.trending_score, 0),
       (1.0::float8 * COALESCE(a.views_24h, 0) + 3.0::float8 * COALESCE(a.plays_24h, 0)),
       wilson_lb(COALESCE(a.engaged_plays, 0), COALESCE(a.unique_views, 0))
         * ln((COALESCE(a.unique_views, 0) + 1)::float8),
       (COALESCE(m.link_count, 0) >= 1 AND length(COALESCE(p.title, '')) > 0),
       p.created_at, now()
FROM playlists p
LEFT JOIN meta m ON m.id = p.id
LEFT JOIN agg a ON a.playlist_id = p.id
ON CONFLICT (playlist_id) DO UPDATE SET
  handle = excluded.handle, title = excluded.title, display_name = excluded.display_name,
  primary_platform = excluded.primary_platform, link_count = excluded.link_count,
  unique_views = excluded.unique_views, engaged_plays = excluded.engaged_plays,
  views_24h = excluded.views_24h, plays_24h = excluded.plays_24h,
  trending_score = excluded.trending_score, today_score = excluded.today_score,
  popular_score = excluded.popular_score, eligible = excluded.eligible,
  created_at = excluded.created_at, refreshed_at = now();
`
