# syntax=docker/dockerfile:1

# ---- build stage: compile the WASM client and the static server binary ----
FROM golang:1.25 AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# WASM client + the wasm_exec.js that matches THIS Go version (avoids drift
# between the bundled bridge file and the runtime).
RUN GOOS=js GOARCH=wasm go build -o main.wasm ./cmd/app \
 && cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" ./wasm_exec.js

# pure-Go server (pgx needs no cgo) -> static binary suitable for distroless.
RUN CGO_ENABLED=0 GOOS=linux go build -o server ./cmd/server

# ---- runtime stage: minimal, non-root ----
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /src/server /app/server
COPY --from=build /src/index.html /src/privacy.html /src/styles.css /src/main.wasm /src/wasm_exec.js /app/
COPY --from=build /src/assets /app/assets
ENV ADDR=0.0.0.0:8081 STATIC_DIR=/app
EXPOSE 8081
USER nonroot:nonroot
CMD ["/app/server"]
