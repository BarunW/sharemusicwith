-- Applied at startup (idempotent). Uses CREATE ... IF NOT EXISTS and
-- ALTER TABLE ... ADD COLUMN IF NOT EXISTS, so it is safe to re-run and also
-- migrates existing tables forward.

CREATE EXTENSION IF NOT EXISTS pgcrypto;   -- gen_random_uuid()
CREATE EXTENSION IF NOT EXISTS citext;     -- case-insensitive handle uniqueness

CREATE TABLE IF NOT EXISTS playlists (
    id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    handle          citext      NOT NULL UNIQUE,
    title           text        NOT NULL DEFAULT '',
    display_name    text        NOT NULL DEFAULT '',
    state           jsonb       NOT NULL,
    edit_token_hash bytea       NOT NULL,
    view_count      bigint      NOT NULL DEFAULT 0,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_playlists_created_at ON playlists (created_at DESC);

CREATE TABLE IF NOT EXISTS events (
    id           bigserial   PRIMARY KEY,
    playlist_id  uuid        NOT NULL REFERENCES playlists(id) ON DELETE CASCADE,
    link_id      text,
    event_type   text        NOT NULL,
    platform     text,
    referrer     text,
    user_agent   text,
    created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_events_playlist_type_time
    ON events (playlist_id, event_type, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_events_link
    ON events (playlist_id, link_id) WHERE link_id IS NOT NULL;

-- ---------------------------------------------------------------------------
-- Metrics hardening (gaming-resistant ranking)
-- ---------------------------------------------------------------------------

-- Visitor identity is stored ONLY as salted hashes; the raw IP is never
-- persisted. event_day buckets each event by UTC day (set at insert time, not a
-- generated column, so the dedup indexes stay immutable) so a visitor's repeated
-- views/plays of the same page collapse to one per day.
ALTER TABLE events ADD COLUMN IF NOT EXISTS visitor_hash bytea;
ALTER TABLE events ADD COLUMN IF NOT EXISTS ip_hash      bytea;
ALTER TABLE events ADD COLUMN IF NOT EXISTS is_bot       boolean NOT NULL DEFAULT false;
ALTER TABLE events ADD COLUMN IF NOT EXISTS event_day    date;

-- Daily dedup: at most one counted view AND one counted play per
-- (playlist, visitor, UTC day). Same columns, different predicates (allowed).
-- Enforced at write time via INSERT ... ON CONFLICT DO NOTHING.
CREATE UNIQUE INDEX IF NOT EXISTS uq_events_view_daily
    ON events (playlist_id, visitor_hash, event_day)
    WHERE event_type = 'page_view' AND visitor_hash IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uq_events_play_daily
    ON events (playlist_id, visitor_hash, event_day)
    WHERE event_type IN ('link_play', 'link_open') AND visitor_hash IS NOT NULL;

-- Supports a future per-IP contribution cap during aggregation.
CREATE INDEX IF NOT EXISTS idx_events_ip_day
    ON events (playlist_id, ip_hash, event_day) WHERE ip_hash IS NOT NULL;

-- Wilson lower bound of the play/view rate: small-sample pages get a
-- conservative score so they cannot top charts on a 1/1 ratio.
CREATE OR REPLACE FUNCTION wilson_lb(k bigint, n bigint, z double precision DEFAULT 1.96)
RETURNS double precision LANGUAGE sql IMMUTABLE AS $$
  SELECT CASE WHEN n = 0 THEN 0 ELSE
    ( (k::float8 / n) + z*z/(2*n)
      - z * sqrt( ((k::float8/n) * (1 - (k::float8/n)) + z*z/(4*n)) / n ) ) / (1 + z*z/n)
  END
$$;

-- Precomputed ranking feed, upserted by a background ticker so reads never do a
-- heavy GROUP BY and never block writers.
CREATE TABLE IF NOT EXISTS playlist_rankings (
    playlist_id      uuid             PRIMARY KEY REFERENCES playlists(id) ON DELETE CASCADE,
    handle           citext           NOT NULL,
    title            text             NOT NULL DEFAULT '',
    display_name     text             NOT NULL DEFAULT '',
    primary_platform text,
    link_count       int              NOT NULL DEFAULT 0,
    unique_views     bigint           NOT NULL DEFAULT 0,
    engaged_plays    bigint           NOT NULL DEFAULT 0,
    views_24h        bigint           NOT NULL DEFAULT 0,
    plays_24h        bigint           NOT NULL DEFAULT 0,
    trending_score   double precision NOT NULL DEFAULT 0,
    today_score      double precision NOT NULL DEFAULT 0,
    popular_score    double precision NOT NULL DEFAULT 0,
    eligible         boolean          NOT NULL DEFAULT false,
    created_at       timestamptz      NOT NULL,
    refreshed_at     timestamptz      NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_rank_trending ON playlist_rankings (trending_score DESC) WHERE eligible;
CREATE INDEX IF NOT EXISTS idx_rank_today    ON playlist_rankings (today_score    DESC) WHERE eligible;
CREATE INDEX IF NOT EXISTS idx_rank_popular  ON playlist_rankings (popular_score  DESC) WHERE eligible;
CREATE INDEX IF NOT EXISTS idx_rank_new      ON playlist_rankings (created_at     DESC) WHERE eligible;
