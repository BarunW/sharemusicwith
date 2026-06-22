# syntax=docker/dockerfile:1

# ---- client stage: compile the WASM client with TinyGo ----
# TinyGo produces a ~0.5 MB-gzipped wasm vs ~3 MB from the standard Go compiler.
# 0.41.x bundles Go 1.25, matching go.mod. cmd/app imports no server deps (pgx),
# so the rest of the module graph is irrelevant here.
FROM tinygo/tinygo:0.41.1 AS wasm
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Build the client and ship TinyGo's matching wasm_exec.js (it differs from the
# standard Go one — they are not interchangeable). Then content-hash the wasm
# filename and rewrite index.html to point at it, so the CDN can edge-cache
# main.<hash>.wasm as immutable and a new build busts the cache with a fresh URL.
RUN tinygo build -o main.wasm -target wasm ./cmd/app \
 && cp "$(tinygo env TINYGOROOT)/targets/wasm_exec.js" ./wasm_exec.js \
 && HASH="$(sha256sum main.wasm | cut -c1-12)" \
 && mv main.wasm "main.$HASH.wasm" \
 && sed -i "s#/main\.wasm#/main.$HASH.wasm#g" index.html

# ---- server stage: pure-Go static server (pgx needs Go 1.25, no cgo) ----
FROM golang:1.25 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o server ./cmd/server

# ---- runtime stage: minimal, non-root ----
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /src/server /app/server
# Background/theme images are served from a CDN (jsDelivr) — see styles.css — so
# they're intentionally NOT baked into the image.
# index.html comes from the wasm stage — that's where it was rewritten to
# reference the hashed wasm. main.*.wasm matches the single content-hashed file.
COPY --from=build /src/privacy.html /src/styles.css /app/
COPY --from=wasm  /src/index.html /src/main.*.wasm /src/wasm_exec.js /app/
ENV ADDR=0.0.0.0:8081 STATIC_DIR=/app
EXPOSE 8081
USER nonroot:nonroot
CMD ["/app/server"]
