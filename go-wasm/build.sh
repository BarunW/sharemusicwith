#!/usr/bin/env sh
set -eu

# Build the WASM client with TinyGo (~0.5 MB gzip vs ~3 MB from the standard Go
# compiler). Needs TinyGo >= 0.41 (for Go 1.25 support; older TinyGo rejects the
# go.mod go directive). Production builds the same way via the Dockerfile.
#
# Standard-compiler fallback (bigger, but no TinyGo needed) — note it requires
# the standard Go wasm_exec.js instead:
#   GOOS=js GOARCH=wasm go build -o main.wasm ./cmd/app
#   cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" ./wasm_exec.js

export GOCACHE="${GOCACHE:-/tmp/connect-playlist-go-build-cache}"
tinygo build -o main.wasm -target wasm ./cmd/app
# Ship TinyGo's matching wasm_exec.js (not interchangeable with the standard one).
cp "$(tinygo env TINYGOROOT)/targets/wasm_exec.js" ./wasm_exec.js
