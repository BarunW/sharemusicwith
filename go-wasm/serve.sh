#!/usr/bin/env sh
set -eu

export GOCACHE="${GOCACHE:-/tmp/connect-playlist-go-build-cache}"
# DATABASE_URL is optional: if you don't export it, cmd/server uses the DSN
# hardcoded in main.go. Override for a different DB, e.g.:
#   DATABASE_URL="postgresql://dbarun:mypassword@localhost:5432/playlists?sslmode=disable" ./serve.sh

# METRICS_SALT salts the visitor/IP hashes used for view/click dedup. Without it,
# events are stored but NOT counted toward rankings (the discovery feed stays
# empty). This dev default is fine locally; in production set a long random secret
# and TRUSTED_PROXIES to your nginx address(es) so X-Forwarded-For is trusted.
export METRICS_SALT="${METRICS_SALT:-dev-local-salt-change-in-prod}"

# YOUTUBE_API_KEY (optional) enables GET /api/youtube/playlists/{id}/tracks,
# which lists a YouTube/YouTube Music playlist's songs via the Data API v3.
# Get a key: console.cloud.google.com -> enable "YouTube Data API v3" -> API key.
# Without it the endpoint responds 503 and everything else works as before.

go run ./cmd/server
