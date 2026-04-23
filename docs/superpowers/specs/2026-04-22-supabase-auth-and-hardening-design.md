# Supabase Auth + End-to-End Hardening — Design

Single-user JobRadar, deployed to one Oracle Cloud VM. Adds authentication across frontend, API, and Grafana; locks the public attack surface to three ports; ships centralised logs for incident response.

## Goals

- Every public endpoint requires proof of ownership (Google OAuth via Supabase for app, Google OAuth direct for Grafana).
- Only `:22`, `:80`, `:443` reachable from the internet.
- Defence in depth: network isolation, rate limiting, intrusion prevention, CSP, host hardening.
- OSS-first — minimum new code, maximum config.
- **Env-first configurability**: every tunable (domain, email allowlist, Supabase URL/keys, Google client IDs, rate-limit thresholds, JWT TTLs, log retention, metrics addr, ports, CrowdSec scenarios thresholds, alert emails) lives in `.env` / compose `environment:` — no values hardcoded in Go, Caddyfile, docker-compose, or scripts. Default values documented in a single `.env.example`.

## Non-goals

- WAF (ModSecurity / Coraza).
- mTLS between browser and Caddy.
- HSTS preload list submission.
- SIEM / SOAR tooling.
- Multi-tenant / multi-user support.

## Decisions

| # | Decision | Value |
|---|---|---|
| 1 | Identity provider | Supabase Auth with Google provider |
| 2 | Authorization model | Single email allowlist (`ALLOWED_EMAIL` env) |
| 3 | Reverse proxy | Caddy with auto-HTTPS (Let's Encrypt) |
| 4 | Intrusion prevention | CrowdSec + Caddy bouncer |
| 5 | Domain layout | `app.<domain>` (console + API same origin), `grafana.<domain>` |
| 6 | JWT algorithm | ES256 via JWKS (fallback to HS256 if project still on shared-secret) — single algo selected at boot |
| 7 | Session storage | Supabase default (localStorage) + strict CSP |
| 8 | Session scope | Access token 1h, refresh 30d, `signOut({ scope: 'global' })` |
| 9 | Depth | Tier-2 "good hygiene" |

## Architecture

```
Internet → Caddy (:443) ──┬── app.<domain>/         → console (nginx, static)
                          ├── app.<domain>/api/*    → api:8080 (Supabase JWT + allowlist)
                          └── grafana.<domain>/     → grafana:3000 (Google OAuth)

127.0.0.1-only (SSH tunnel): prometheus, jaeger, redis, kafka, api-metrics,
                             worker-metrics, grafana (direct), loki
```

Caddy is the only container that binds a public port. All other host-published ports use `127.0.0.1:PORT:PORT`.

## Request flow

```
Browser → signInWithOAuth({ provider: 'google' })
       → Supabase hosted flow → Google → Supabase mints JWT, sets localStorage
       → Browser stores session; subsequent requests send Authorization: Bearer <jwt>
       → Caddy: TLS termination, security headers, rate limit, CrowdSec check
       → API middleware.Auth: verify JWT (iss, aud, alg, exp, signature) + email allowlist
       → Handler → service → store
```

On Supabase/JWKS outage: middleware **fails closed** — returns `503 Retry-After`, never bypasses.

## Components

### Backend

**New: `backend/apps/api/middleware/auth.go`**
- `Auth(cfg) func(http.Handler) http.Handler`
- Verifies: signature (ES256 via JWKS, or HS256 if configured — exactly one at boot), `iss == https://<project-ref>.supabase.co/auth/v1`, `aud == authenticated`, `exp` with 60s skew
- Uses `jwt.WithValidMethods` to prevent algorithm confusion
- Compares `email` claim to `cfg.AllowedEmail` via `subtle.ConstantTimeCompare`
- Status codes: 401 for missing/expired/invalid JWT, **403** for email mismatch (frontend must not trigger refresh loop on 403), 503 on JWKS fetch failure
- Stores `{UserID, Email}` in request context via typed context key
- Skip list: none (metrics endpoint is moved off this listener — see below)

**New: `backend/apps/api/middleware/security_headers.go`**
- Sets `X-Content-Type-Options`, `Referrer-Policy`, `Permissions-Policy` at the API layer as defence-in-depth. HSTS + CSP set by Caddy.

**New: `backend/apps/api/middleware/body_limit.go`**
- `http.MaxBytesReader` wrapper, 1 MiB limit.

**Modified: `backend/apps/api/router.go`**
- Middleware chain becomes: otelhttp → Logger → Metrics → Recoverer → SecurityHeaders → BodyLimit → Auth → routeGroups
- `/metrics` endpoint is **removed** from this router.

**Modified: `backend/apps/api/main.go`**
- Second `http.Server` on `127.0.0.1:9100` serving only `/metrics` (Prometheus) and `/health`, `/ready`. Public API server keeps `:8080` (internal to compose network now).
- Add `ReadHeaderTimeout: 5s`, `ReadTimeout: 15s`, `WriteTimeout: 30s`, `IdleTimeout: 60s` to both servers.

**Modified: `backend/apps/api/handler/respond.go`**
- `ServiceError` split into `externalMessage` (returned to client) and `internalMessage` (logged only). `HandleError` never reflects `internalMessage` into the response body.

**Modified: `backend/common/config/config.go`**
- New fields: `SupabaseProjectURL`, `SupabaseJWKSURL` (derived), `SupabaseJWTSecret` (fallback), `AllowedEmail`, `MetricsAddr`.
- Refuse to boot if both JWT secret and JWKS URL are set, or neither.

**Tests: `backend/apps/api/middleware/auth_test.go`** (table-driven)
- Valid token, expired token, wrong issuer, wrong audience, wrong email, missing header, malformed, `alg=none`, alg-confusion (ES256 public key as HS256 secret), wrong project JWKS, 60s skew boundary, fail-closed on JWKS 503.

### Frontend

**New files in `ui/console/src/`:**
- `lib/supabase.ts` — `createClient(url, anonKey)` singleton
- `features/auth/store.ts` — Zustand store, subscribed to `supabase.auth.onAuthStateChange`
- `features/auth/LoginPage.tsx` — one button calling `signInWithOAuth({ provider: 'google', options: { redirectTo: '<app>/auth/callback' }})`
- `features/auth/AuthCallback.tsx` — calls `supabase.auth.exchangeCodeForSession`, then `history.replaceState` to strip URL fragment before render
- `features/auth/RequireAuth.tsx` — route guard; redirects to `/login` when no session

**Modified:**
- HTTP client (wherever it lives under `ui/console/src/api/`): interceptor attaches `Authorization: Bearer <session.access_token>`; on 401 funnels through a single in-flight `refreshSession()` promise and only logs out if refresh itself fails
- `App.tsx` — wraps routes in `<RequireAuth>` except `/login` and `/auth/callback`
- `ui/console/vite.config.ts` — `build.sourcemap: false`
- `ui/console/public/robots.txt` — `Disallow: /`
- `.env` — `VITE_SUPABASE_URL`, `VITE_SUPABASE_ANON_KEY`

**Tests:** vitest — interceptor (attaches/refreshes/logs-out), RequireAuth (renders/redirects), AuthCallback (handles bad hash).

### Supabase dashboard configuration (no code)

- Authentication → Providers → Google → enable, paste client ID/secret from `backend/.env`
- Redirect URL pinned exactly to `https://app.<domain>/auth/callback` (no wildcards, no localhost in prod)
- `auth.users` trigger: reject inserts where `email != <ALLOWED_EMAIL>` (defence-in-depth)
- Enable auth.audit_log_entries retention (managed by Supabase)

### Grafana

**Modified `docker-compose.yml` grafana block:**
- Drop `GF_AUTH_ANONYMOUS_ENABLED`, `GF_AUTH_ANONYMOUS_ORG_ROLE`
- Add: `GF_SERVER_ROOT_URL=https://grafana.<domain>`, `GF_SERVER_DOMAIN=grafana.<domain>`
- Add: `GF_AUTH_GOOGLE_ENABLED=true`, `GF_AUTH_GOOGLE_CLIENT_ID`, `GF_AUTH_GOOGLE_CLIENT_SECRET`, `GF_AUTH_GOOGLE_ALLOWED_DOMAINS` or `GF_AUTH_GOOGLE_ALLOWED_EMAILS=<allowed>`
- Add: `GF_AUTH_DISABLE_LOGIN_FORM=true` (Google only)
- Admin password via `GF_SECURITY_ADMIN_PASSWORD__FILE` → docker secret
- Bind: `127.0.0.1:3001:3000` (direct access via SSH tunnel for bootstrap; Caddy reaches it via compose network)

Redirect URI at Google Cloud Console: `https://grafana.<domain>/login/generic_oauth`.

### Reverse proxy — Caddy

**New files:**
- `infra/caddy/Dockerfile` — `xcaddy` build including `caddy-ratelimit` and `caddy-crowdsec-bouncer` plugins
- `infra/caddy/Caddyfile` — snippets for security_headers; virtual hosts for `app.<domain>`, `grafana.<domain>`; rate_limit zones (`/api/*` 30/min/IP, `/auth/callback` 5/min/IP); CrowdSec bouncer directive; JSON access logs to `/var/log/caddy/`
- Volume: `caddy_data` (ACME account key + certs — persistent)
- Compose service `caddy` binds `:80:80` and `:443:443`, depends on api/console/grafana

**Security headers set by Caddy:**
- `Strict-Transport-Security "max-age=31536000; includeSubDomains"` (no `preload` until stable for 2 weeks)
- `Content-Security-Policy "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:; connect-src 'self' https://<project>.supabase.co https://accounts.google.com; frame-ancestors 'none'; base-uri 'self'; form-action 'self' https://accounts.google.com"`
- `X-Content-Type-Options "nosniff"`
- `Referrer-Policy "strict-origin-when-cross-origin"`
- `Permissions-Policy "geolocation=(), camera=(), microphone=(), interest-cohort=()"`
- Remove `Server` header

### docker-compose hardening (all services)

| Change | Applies to |
|---|---|
| Remove public `ports:` | api, console, grafana (reached via Caddy) |
| Rebind to `127.0.0.1:PORT:PORT` | prometheus, jaeger, redis, kafka, worker metrics (9101–9104), grafana (direct), api metrics (9100), loki |
| `security_opt: ["no-new-privileges:true"]` | all Go services, console, caddy |
| `cap_drop: ["ALL"]` | all Go services, console, caddy |
| `read_only: true` + `tmpfs: ["/tmp"]` | all Go services, console, caddy |
| `pids_limit: 200` | all Go services, console |
| `restart: "no"` | migrate |
| Drop `user: "0"` | kafka (redpanda) |
| `infra/Dockerfile`: `USER 65532:65532` | Go image |
| `ui/console/Dockerfile`: switch to `nginxinc/nginx-unprivileged:alpine` | console |

Top-level `secrets:` block for `supabase_jwt_secret`, `grafana_admin_password`, `crowdsec_bouncer_key`. Services reference via `secrets:` instead of `env_file` where possible.

### Host bootstrap

**New file: `infra/host/bootstrap.sh`** — idempotent one-shot for fresh Oracle Linux VM.

Installs / configures:
- `firewalld` — open 22/80/443 only
- `sshd_config.d/10-hardening.conf` — key-only, no root, `AllowUsers`, `MaxAuthTries 3`, `ClientAliveInterval 300`
- `dnf-automatic` — daily security updates, `apply_updates = yes`
- `chronyd` — enable, verify drift
- `/etc/docker/daemon.json` — log rotation (50m × 5), `live-restore`, `userland-proxy: false`, `no-new-privileges`, `icc: false`
- `iptables -I DOCKER-USER -d 169.254.169.254 -j DROP` — block OCI metadata from containers; persist via iptables-save
- CrowdSec: install, register `sshd`, `linux`, `caddy`, `http-cve` collections, install `crowdsec-firewall-bouncer-iptables`, register Caddy bouncer LAPI key
- `.env` permissions: `chmod 600`, `chown $USER:$USER`

**New file: `infra/host/README.md`** — run order, prerequisites, OCI Security List setup, DNS records.

### CrowdSec

Community collections handle SSH, generic HTTP probing, CVEs. Two custom scenarios:

**New files in `infra/crowdsec/scenarios/`:**
- `jobradar-api-401-abuse.yaml` — ≥10× 401 from `/api/*` per IP in 1 minute → 4h ban
- `jobradar-grafana-login-abuse.yaml` — ≥5× failed generic_oauth per IP in 1 minute → 4h ban

**Break-glass:** script in `infra/host/bootstrap.sh` adds an allow decision for the owner's IP range via `cscli decisions add --type allow --duration 8760h`. Documented recovery: SSH in, `cscli decisions delete --all`.

### Observability

Wire Loki into compose (currently orphaned in `infra/loki/`):
- Compose service `loki`, bound `127.0.0.1:3100`
- Compose service `promtail` (or Alloy) scraping Caddy logs + `/var/log/crowdsec.log`
- Docker `loki` log driver plugin for container stdout
- **New file** `infra/observability/supabase-audit-sync.go` or shell cron — every 15 min pulls `auth.audit_log_entries` from Supabase into Loki (Supabase dashboard only retains ~7d on free tier)
- Loki compactor enabled, retention: 30d access, 90d auth/audit

**New Grafana dashboards** under `infra/grafana/dashboards/`:
- `auth-events-24h.json` — logins, failures, source IPs
- `security-posture.json` — active CrowdSec bans, top scenarios, rate-limit hits
- `tls-health.json` — ACME renewal countdown, alert at T-14d

### Backups

**New file: `infra/host/backup.sh`** — nightly cron, `restic` to OCI Object Storage (encrypted repo).

Backs up:
- `/var/lib/docker/volumes/jobradar_grafanadata/_data`
- `/var/lib/docker/volumes/jobradar_prometheusdata/_data`
- `/var/lib/docker/volumes/jobradar_caddy_data/_data`
- `/opt/jobradar/.env` (encrypted)

Supabase manages DB backups. Document restore procedure and test once post-deploy.

## Data flow (what crosses what)

| Boundary | Payload | Protection |
|---|---|---|
| Browser ↔ Caddy :443 | HTTPS, Bearer JWT | TLS 1.2+, CSP, HSTS, rate limit |
| Caddy ↔ api:8080 | Plaintext HTTP inside compose network | Compose network isolation; `icc: false` at docker daemon means services don't talk unless on explicit user-defined network |
| api ↔ Supabase | HTTPS (outbound) | System trust store; JWKS cached 10min, one refetch on `kid` miss |
| API ↔ Postgres (Supabase) | TLS (already configured) | Service-role key NOT loaded by API; anon key only on frontend |

## Config matrix

New env vars (all backend):
- `SUPABASE_PROJECT_URL` — e.g. `https://<ref>.supabase.co`
- `SUPABASE_JWKS_URL` — derived if empty
- `SUPABASE_JWT_SECRET` — fallback only; refuse-to-boot if both this and JWKS_URL set
- `ALLOWED_EMAIL`
- `METRICS_ADDR` — default `127.0.0.1:9100`

Frontend (Vite):
- `VITE_SUPABASE_URL`
- `VITE_SUPABASE_ANON_KEY`

Secrets (docker-compose `secrets:`):
- `supabase_jwt_secret` (or JWKS signature via network only)
- `grafana_admin_password`
- `crowdsec_bouncer_key`
- `restic_password` (for backups)

## Testing strategy

**Unit:**
- Backend: middleware table tests (see auth_test.go spec above), respond-helper redaction, body-limit enforcement, security-headers-set, fail-closed on JWKS outage
- Frontend: interceptor, RequireAuth, AuthCallback

**Integration (local compose):**
- `curl -i https://localhost/api/*` without token → 401
- With wrong-email JWT → 403
- With valid token → 200
- Rate limit: 31 reqs in 60s → 429

**E2E smoke (post-deploy, manual):**
- Log in via Google, hit `/api/companies`, hit Grafana dashboard
- Revoke session in Supabase dashboard, confirm frontend logs out on next request
- Break-glass: from a CrowdSec-banned test IP, confirm SSH recovery works

**Lint gates (must pass before done):**
- `cd backend && golangci-lint run --config ../.github/.golangci.yml --timeout 5m`
- `cd ui/console && npm run lint`

## Rollout order

1. Backend auth middleware + all tests (green locally)
2. Frontend auth + tests (green locally, tested against real Supabase from localhost)
3. Move `/metrics` to `127.0.0.1:9100` listener in api
4. `docker-compose` port rebinds + `security_opt` + `USER` changes; Dockerfile updates
5. Caddy service + Caddyfile locally (use LE staging CA first to avoid rate limits)
6. Host bootstrap script tested on a throwaway VM
7. CrowdSec install + scenarios; **allowlist owner IP before enabling bouncer**
8. Wire Loki + Promtail + dashboards
9. Deploy to Oracle Cloud VM, E2E smoke
10. Backups enabled, test restore
11. After 14 days stable: add HSTS `includeSubDomains` (never `preload`)

## Open questions (deferred to implementation)

- Whether Supabase project is already on ES256 or still HS256 — discovered at implementation start, drives which verifier path is implemented first
- Exact domain name — placeholder `<domain>` in configs, substituted at deploy
- OCI Security List IDs and compartment OCID — collected at deploy time

## Risks and mitigations

| Risk | Mitigation |
|---|---|
| CrowdSec bans owner via VPN/mobile | Allowlist owner IP range before enabling bouncer; documented `cscli decisions delete --all` recovery via SSH |
| ACME renewal fails silently (firewall closes :80) | Grafana alert on cert expiry T-14d; fallback to DNS-01 challenge if registrar has API |
| Supabase outage | API fails closed with 503, not bypass; frontend displays retry UI |
| Let's Encrypt rate limit after repeated builds | Use LE staging CA in `infra/caddy/Dockerfile` during dev; persist `caddy_data` volume in prod |
| XSS steals localStorage token | Strict CSP (no `unsafe-inline` scripts); regular dep audits; known single-user-app trade-off |
| VM boot volume fills (46.6GB default) | Docker log rotation 50m×5; Prometheus retention 15d; nightly `restic` prune |
| OCI metadata service accessible from containers | `iptables -I DOCKER-USER -d 169.254.169.254 -j DROP` in bootstrap |
