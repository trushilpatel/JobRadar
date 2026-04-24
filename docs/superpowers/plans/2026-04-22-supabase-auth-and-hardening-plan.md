# Implementation Plan — Supabase Auth + Hardening

Source spec: `docs/superpowers/specs/2026-04-22-supabase-auth-and-hardening-design.md`

## Partitioning principle

Three independent buckets in **Wave 1** — no file overlap. Then main thread does **Wave 2** (docker-compose.yml + Dockerfiles) because those files would be the hot spot for conflicts. **Wave 3** is QA.

## File ownership matrix (strict — no overlap)

### Bucket A — Backend (agent A)

Creates:
- `backend/apps/api/middleware/auth.go`
- `backend/apps/api/middleware/auth_test.go`
- `backend/apps/api/middleware/security_headers.go`
- `backend/apps/api/middleware/security_headers_test.go`
- `backend/apps/api/middleware/body_limit.go`
- `backend/apps/api/middleware/body_limit_test.go`
- `backend/apps/api/handler/respond_redaction_test.go` (new test for split error shape)

Modifies:
- `backend/apps/api/main.go` — second http.Server for metrics on `METRICS_ADDR` (default `127.0.0.1:9100`); timeouts on both servers
- `backend/apps/api/router.go` — mount security_headers + body_limit + Auth; remove `/metrics`, `/health`, `/ready` from this router
- `backend/apps/api/handler/respond.go` — split `ServiceError` into internal/external message
- `backend/common/config/config.go` — new struct `AuthConfig` + `ObservabilityConfig.MetricsAddr`; load from env; boot-time validation refusing both/neither of JWT secret and JWKS URL
- `backend/go.mod` / `go.sum` — add `github.com/golang-jwt/jwt/v5`, `github.com/MicahParks/keyfunc/v3`

Tests must pass: `cd backend && go test ./... && golangci-lint run --config ../.github/.golangci.yml --timeout 5m`

### Bucket B — Frontend (agent B)

Creates:
- `ui/console/src/lib/supabase.ts`
- `ui/console/src/lib/api-client.ts` — central fetch wrapper with Supabase bearer interceptor + single in-flight refresh + 401→refresh→retry→logout
- `ui/console/src/features/auth/store.ts`
- `ui/console/src/features/auth/LoginPage.tsx`
- `ui/console/src/features/auth/AuthCallback.tsx`
- `ui/console/src/features/auth/RequireAuth.tsx`
- `ui/console/src/features/auth/index.ts` — barrel
- `ui/console/public/robots.txt` — `User-agent: *` / `Disallow: /`
- Vitest specs under `ui/console/src/features/auth/__tests__/`

Modifies:
- `ui/console/src/App.tsx` — wrap protected routes in `<RequireAuth>`, add `/login` and `/auth/callback` public routes
- `ui/console/src/api/*.ts` — replace direct `fetch(...)` with `apiClient.get/post/...` from new wrapper (keeps existing call sites minimal)
- `ui/console/package.json` — add `@supabase/supabase-js`, `vitest`, `@testing-library/react`, `@testing-library/jest-dom`, `jsdom`
- `ui/console/vite.config.ts` — `build.sourcemap: false`, add vitest config
- `ui/console/.env.example` — `VITE_SUPABASE_URL`, `VITE_SUPABASE_ANON_KEY`, `VITE_APP_DOMAIN` (placeholders)
- `ui/console/tsconfig.app.json` — include vitest globals if needed

Tests must pass: `cd ui/console && npm run lint && npx vitest run`

### Bucket C — Infra files non-compose (agent C)

Creates:
- `infra/caddy/Dockerfile` — xcaddy build with `caddy-ratelimit` and `caddy-crowdsec-bouncer` plugins, pinned versions
- `infra/caddy/Caddyfile` — fully env-driven via `{$DOMAIN}`, `{$SUPABASE_PROJECT_HOST}`, `{$RATE_LIMIT_API_PER_MIN}`, `{$RATE_LIMIT_CALLBACK_PER_MIN}`, etc.
- `infra/caddy/README.md` — env vars table
- `infra/host/bootstrap.sh` — idempotent, env-driven (`ADMIN_USER`, `ADMIN_IP_ALLOWLIST`, etc.), single entry: `./bootstrap.sh`
- `infra/host/README.md` — run order, OCI prerequisites, DNS records, rollback
- `infra/host/backup.sh` — restic wrapper driven by `BACKUP_REPO`, `BACKUP_PASSWORD_FILE`, `BACKUP_PATHS`, `BACKUP_RETENTION`
- `infra/crowdsec/scenarios/jobradar-api-401-abuse.yaml`
- `infra/crowdsec/scenarios/jobradar-grafana-login-abuse.yaml`
- `infra/crowdsec/acquis.yaml` — Caddy log source
- `infra/crowdsec/profiles.yaml` — ban duration env-driven
- `infra/promtail/config.yaml` — scrape Caddy + CrowdSec logs, env-driven Loki URL
- `infra/loki/config.yaml` — retention env-driven (if not already there, augment)
- `infra/observability/supabase-audit-sync/` — a small Go binary or shell+psql that copies `auth.audit_log_entries` into Loki; env-driven (`AUDIT_SYNC_INTERVAL_SECONDS`, Supabase DB URL, Loki URL)
- `infra/grafana/dashboards/auth-events.json`
- `infra/grafana/dashboards/security-posture.json`
- `infra/grafana/dashboards/tls-health.json`
- `.env.example` (repository root) — consolidated example of ALL env vars (backend + frontend + infra) with comments

Does NOT touch: `docker-compose.yml`, any `Dockerfile` outside `infra/caddy/`. Agent must not modify these — bucket D does.

### Bucket D — docker-compose + Dockerfiles (main thread, Wave 2)

Main thread does this after Waves 1 complete so it can see all produced files and wire them correctly.

Modifies:
- `docker-compose.yml` — rewrite per spec §2.2 (port rebinds, security_opt, cap_drop, read_only, pids_limit, caddy service, loki, promtail, supabase-audit-sync, top-level `secrets:`, Grafana config)
- `infra/Dockerfile` — add `USER 65532:65532`
- `ui/console/Dockerfile` — switch to `nginxinc/nginx-unprivileged:alpine`; ensure build copies `public/robots.txt`
- `backend/.env.example` — append new auth vars (if not already placed by Bucket C in root `.env.example`, keep backend/.env.example in sync)

Runs:
- `docker compose config` (lint)
- `cd backend && golangci-lint run ... && go test ./...`
- `cd ui/console && npm run lint && npx vitest run`
- `docker compose build`
- `docker compose up -d` + curl smoke

## Wave 3 — QA (single reviewer agent)

Reviews all changes against spec. Checks:
- Every spec item present
- Env-first: no hardcoded domain / email / Supabase URL / threshold anywhere
- No secrets committed
- Tests cover the critical-gap items from the earlier security reviews (alg-confusion, wrong iss, fail-closed on JWKS outage, body limit, allowlist timing-safe)
- Lint + tests green

Reports discrepancies; main thread fixes them.

## Env-first constraints (applies to every bucket)

- Every configurable value must be read from env.
- No literals for domain, email, Supabase URL/key, Google client IDs, rate limit numbers, session TTLs, log retention, metrics addr, ports, CrowdSec ban duration, backup repo, admin IP allowlist.
- Defaults live in code / compose-file defaults ONLY as fallbacks, with the sentinel value documented in `.env.example`.
- Boot-time validation (backend & scripts) refuses to start if required env missing.

## Do NOT

- Do not commit. User will review.
- Do not push. Do not create PR.
- Do not touch Oracle Cloud itself (no `oci` CLI calls, no SSH anywhere).
- Do not introduce features beyond the spec.
- Do not add comments explaining "what" — only "why" when non-obvious.
- Do not hardcode credentials — use placeholders in `.env.example`.
