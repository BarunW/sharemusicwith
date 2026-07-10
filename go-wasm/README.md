# Connect With Playlist Go WASM

A Go/WebAssembly app for building a shareable music page, backed by a small
Go + PostgreSQL server. Pages are published under a vanity `@handle` and edited
via a secret capability link (Notion/Google-Doc style — no accounts).

## Prerequisites

A reachable PostgreSQL database. Set `DATABASE_URL` (default in `serve.sh` is
`postgres://postgres:postgres@localhost:5432/playlists?sslmode=disable`).

Quick local Postgres via Docker:

```sh
docker run --name playlist-pg -e POSTGRES_PASSWORD=postgres \
  -e POSTGRES_DB=playlists -p 5432:5432 -d postgres:16
```

Or a throwaway cluster without Docker:

```sh
export PGDATA=/tmp/playlist-pg
initdb -D "$PGDATA" -U postgres --auth=trust
pg_ctl -D "$PGDATA" -o "-p 5432" -l /tmp/playlist-pg.log -w start
createdb -h 127.0.0.1 -p 5432 -U postgres playlists
```

The server creates its tables (and the `pgcrypto`/`citext` extensions) on first
start via an embedded idempotent `schema.sql`.

## Build

```sh
./build.sh        # compiles cmd/app -> main.wasm
```

## Run

```sh
./serve.sh        # builds + runs cmd/server (needs DATABASE_URL + a built main.wasm)
```

Then open `http://127.0.0.1:8081/`.

## Flow

1. `/` — discovery feed for public playlist pages.
2. `/create` — creation form. Pick a `@handle` and publish.
3. `/created` — shows the Public URL and the private Edit URL (save it; shown once).
4. `/@handle` — public read-only page. Opening a player records a metric event.
5. `/@handle/edit/<editToken>` — prefilled editor that autosaves to the server.

If a requested handle is taken it is auto-suffixed (`@foo` → `@foo-2`), so
publishing never fails.

## Layout

- `cmd/app` — WASM client (`main.go`, `router.go`, `api.go`).
- `cmd/server` — HTTP entrypoint (config, pool, schema, graceful shutdown).
- `internal/state` — the document model shared by client and server.
- `internal/store` — PostgreSQL access + `schema.sql`.
- `internal/{handle,token,config,api}` — handle rules, edit tokens, config, routes.

## Configuration

| Env var        | Default                           | Purpose                               |
| -------------- | --------------------------------- | ------------------------------------- |
| `DATABASE_URL` | (required)                        | Postgres DSN                          |
| `ADDR`         | `0.0.0.0:8081`                    | listen address                        |
| `STATIC_DIR`   | `.`                               | dir with index.html etc.              |
| `SITE_URL`     | `https://sharemusicwith.live`     | Canonical/social/sitemap public origin |
