# Staging deployment

A single small Hetzner VM running **host nginx** (TLS via certbot) in front of a
Docker Compose stack: the **app** (Go server) → **Postgres**. The image is built
and pushed by CI; the VM only pulls it.

```
 GitHub push ─▶ Actions (build image) ─▶ GHCR
                                          │  docker compose pull
 Internet ─443▶ host nginx (TLS, certbot) ─▶ 127.0.0.1:8081 (app) ─▶ db (pgdata volume)
```

nginx terminates TLS and forwards `X-Forwarded-For`; the app reads it
(`TRUSTED_PROXIES`) so view/click dedup sees the real client IP. A real HTTPS
domain is required — Spotify/Apple Music embeds only load in a secure context.

> **Why not Kubernetes?** One stateless binary + one Postgres, no horizontal
> scaling. Running >1 app replica would split the in-process 60s rankings ticker
> and in-memory rate-limit/dedup state. Compose on one VM is the right scale.

## 1. CI: build + push the image

`.github/workflows/deploy.yml` builds `go-wasm/Dockerfile` (compiles WASM client
+ server, embeds static assets) and pushes to **GHCR** on push to `main`/`staging`
and on `v*` tags. Nothing else is needed — `GITHUB_TOKEN` authorizes the push.

- Find the image under the repo's **Packages**. For the VM to pull it, either make
  the package **public** (simplest for staging) or create a read-only PAT
  (`read:packages`) and `docker login ghcr.io -u <user>` once on the VM.
- Set `APP_IMAGE` in `.env` to the pushed ref, e.g. `ghcr.io/owner/repo:staging`.

## 2. Provision the Hetzner VM (one-time)

- **Instance:** CX22 (2 vCPU / 4 GB) is comfortable; CPX11 (2 GB) also works since
  CI builds the image (no source build on the box). Ubuntu **24.04 LTS**.
- **DNS first:** add an **A record** `staging.example.com → <VM IPv4>` (and AAAA
  if you enable IPv6). Verify before certbot: `dig +short staging.example.com`.
- **Hetzner Cloud Firewall** (the real boundary — Docker's iptables bypass `ufw`):
  inbound allow **22, 80, 443** only. Never expose 5432 or 8081. SSH-key auth only.
- **Install:** Docker Engine + Compose plugin (Docker's apt repo, not snap), then
  `sudo apt install nginx certbot python3-certbot-nginx`.

## 3. First deploy

```sh
git clone <repo> && cd <repo>/go-wasm
cp .env.example .env
# edit .env:
#   APP_IMAGE=ghcr.io/owner/repo:staging
#   POSTGRES_PASSWORD=$(openssl rand -hex 32)
#   METRICS_SALT=$(openssl rand -hex 32)
#   YOUTUBE_API_KEY=<Data API v3 key>   # optional: enables playlist tracklists
docker compose up -d            # pulls the image; starts app + db (Caddy is off)
```

The app applies the DB schema (idempotent) and starts the rankings ticker (once,
then every 60 s) on boot.

Then put nginx in front and get a cert:

```sh
sudo cp deploy/nginx/staging.conf /etc/nginx/sites-available/sharemusicwith
# edit server_name to your domain
sudo ln -s /etc/nginx/sites-available/sharemusicwith /etc/nginx/sites-enabled/
sudo rm -f /etc/nginx/sites-enabled/default     # drop the default vhost
sudo nginx -t && sudo systemctl reload nginx
sudo certbot --nginx -d staging.example.com     # issues cert, adds 443 + redirect,
                                                 # installs an auto-renew timer
```

## 4. Verify

```sh
dig +short staging.example.com                       # → VM IP (before certbot)
curl -sf https://$DOMAIN/healthz                     # {"ok":true}  (also pings the DB)
curl -sI  http://$DOMAIN/                            # 301 → https
curl -sI  https://$DOMAIN/main.wasm                  # 200, Content-Type: application/wasm,
                                                     #      Content-Encoding: gzip
docker compose logs app | grep -i metrics_salt       # must NOT show "is unset"
```

- Open `https://$DOMAIN/` → discovery home renders (feed fills once pages get views).
- **Real-IP check:** hit the site from an external IP, then `docker compose logs app
  | tail` — the client IP must be your **real public IP**, not `127.0.0.1` or a
  `172.x` docker-gateway address (a `172.x` here means `TRUSTED_PROXIES` is wrong).
- **Dedup check:** publish a page; open it from **two different devices/networks**;
  after ≤60 s, `GET https://$DOMAIN/api/discover?section=popular` shows
  `unique_views` incrementing **per visitor** (a same-device reload does not). This
  confirms the `X-Forwarded-For` → `TRUSTED_PROXIES` → salt → ranking chain.
- Confirm Spotify/Apple embeds load on a published page (secure HTTPS context).

## 5. Update

```sh
git pull                              # picks up compose/nginx changes, if any
docker compose pull                   # newest CI-built image
docker compose up -d                  # recreate app; Postgres data persists,
                                      # nginx/TLS untouched
```

## Operations

- Logs: `docker compose logs -f app` (or `db`).
- Restart app only: `docker compose restart app` (schema re-applies cleanly).
- **Backups** — data lives in the `pgdata` Docker volume. Nightly cron, copied
  off-box (Hetzner Storage Box / object storage):
  ```sh
  docker compose exec -T db pg_dump -U postgres playlists \
    | gzip > /var/backups/playlists-$(date +\%F).sql.gz
  ```
- Tunables (in `.env` / compose): `RATE_LIMIT_RPS`, `RATE_LIMIT_BURST`.
- **Local source build (no CI):** layer the build override —
  `docker compose -f docker-compose.yml -f docker-compose.build.yml up -d --build`.
- **Zero-config alternative:** `docker compose --profile caddy up -d` runs Caddy
  (auto-HTTPS) instead of host nginx — handy for a throwaway box.

## Production path (later)

- Promote by image tag (`:vX.Y.Z`) per environment; separate domain + `.env`.
- Off-box backup retention + **Hetzner Volume snapshots** of the data disk.
- Least-privilege app DB role (extensions pre-installed) instead of the superuser
  (the app currently uses it so `CREATE EXTENSION pgcrypto/citext` succeeds on
  first boot).
- Optional CI auto-deploy: a second workflow job that SSHes to the VM and runs
  `docker compose pull && docker compose up -d` (needs `SSH_HOST`/`SSH_USER`/
  `SSH_KEY` repo secrets).
- Optional perf: serve the static dir from nginx with `gzip_static` +
  `Cache-Control: immutable` and a content-hashed `main.wasm` filename, proxying
  only `/api/` + `/healthz` (kills the per-visit ~3 MB wasm re-download the app's
  `no-store` headers currently cause). Requires extracting assets from the image
  on deploy and replicating the SPA fallback.
