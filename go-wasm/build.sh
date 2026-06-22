#!/usr/bin/env sh
set -eu

export GOCACHE="${GOCACHE:-/tmp/connect-playlist-go-build-cache}"
GOOS=js GOARCH=wasm go build -o main.wasm ./cmd/app
