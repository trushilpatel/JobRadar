---
name: run
description: Launch and verify the JobRadar local dev stack (API + console). Use before checking UI changes, verifying fixes, or confirming a feature works end-to-end.
---

# Running JobRadar locally

## Stack at a glance

| Service                 | Port  | Command            |
| ----------------------- | ----- | ------------------ |
| Go API (air hot-reload) | :8080 | `make dev-backend` |
| Vite dev server (HMR)   | :5173 | `make dev-console` |
| Ops/health endpoint     | :9100 | (started by API)   |

Secrets come from Doppler (`job-radar / dev`). No `.env` files, no Docker, no k3d needed for UI+API work.

## Start everything

```bash
make dev          # kills stale processes, migrates, then starts API + Vite in parallel
```

Or independently:

```bash
make dev-backend  # terminal 1 — Go API with air (auto-reloads on .go save)
make dev-console  # terminal 2 — Vite HMR (frontend updates instantly on .tsx/.ts save)
```

## Check services are up

```bash
make dev-check
# → API  :8080  UP
# → Vite :5173  UP
```

Or via curl:

```bash
curl -sf http://localhost:9100/health && echo ok   # API liveness
curl -sf http://localhost:5173/ > /dev/null && echo ok  # Vite up
```

## Authenticate API calls (dev bypass)

All `/api/*` endpoints require auth. Use the dev bypass header — no real JWT needed:

```bash
DEV_TOKEN=$(doppler secrets get DEV_TOKEN --project job-radar --config dev --plain)
curl -s -H "X-Dev-Token: $DEV_TOKEN" http://localhost:8080/api/dashboard/market
```

Route rule: user API routes live at `/api/*` on the Go server. Admin routes live at `/admin/*`.
Vite proxy at :5173 preserves the `/api` prefix for user routes and strips it for `/api/admin/*`.

## Open in browser

```bash
make dev-open     # opens http://localhost:5173/
open http://localhost:5173/
```

The app requires Supabase login. Log in once via the browser UI — the session persists in localStorage.

## Visual verification (screenshots not available in CLI)

`screencapture` does not work inside Claude Code CLI (no display). To verify UI changes:

- Direct the user to reload `http://localhost:5173/` in Chrome
- Use curl to smoke-test API responses and validate JSON shape
- Use `make dev-check` to confirm services are live

## Stop everything

```bash
make dev-stop     # kills processes on :8080 and :5173
```

## Key endpoints to smoke-test

```bash
DEV_TOKEN=$(doppler secrets get DEV_TOKEN --project job-radar --config dev --plain)
H="X-Dev-Token: $DEV_TOKEN"

curl -s -H "$H" http://localhost:8080/api/dashboard/market | python3 -m json.tool
curl -s -H "$H" http://localhost:8080/api/dashboard/tech-trends | python3 -m json.tool
curl -s -H "$H" "http://localhost:8080/api/jobs?limit=5" | python3 -m json.tool
```

## After a backend code change

`air` watches all `.go` files and recompiles automatically (~2-8s). Watch the terminal running `make dev-backend` — it prints `building...` then `running...` when ready. No manual restart needed.

## After a frontend code change

Vite HMR applies the change instantly in the browser. No reload needed for component/style changes. Full page reload only needed for router or provider changes.

## Deeper verification: Playwright E2E suite

For a real end-to-end check (not just a curl smoke-test) — full browser flows, both
a regular user and an admin — once the stack above is up:

```bash
cd frontend && npm run test:e2e
```

See `frontend/e2e/CLAUDE.md` for conventions and what's covered. Not part of this
skill's baseline check; run it when verifying a change to a flow it covers (Jobs,
Account, Resumes, Apply Queue, auth/admin-gating, or any admin page).
