# JobRadar

An automated job tracking system for software engineers. Monitors company careers pages continuously, filters listings via LLM, enriches them with structured data, and surfaces only the relevant ones in a clean feed.

Built for job seekers who need visa sponsorship — jobs that explicitly deny sponsorship, require citizenship, or require security clearance are automatically discarded before they ever reach your feed.

---

## How It Works

```
company_scan_scheduler
        ↓  (DB queue)
job_listing_crawler       ← fetches careers pages via ATS adapters
        ↓  (Kafka: job_listings.raw)
job_listing_relevance_filter  ← LLM batch-scores job titles
        ↓  (Kafka: job_listings.filtered)
job_post_analyzer         ← fetches full page, LLM enriches, eligibility check
        ↓  (Supabase: job_feed)
REST API + React UI
```

---

## Features

### Job Feed
- Auto-populated from monitored company careers pages
- LLM-filtered by title relevance against your target roles and experience level
- Each job enriched with: tech stack, salary range, remote status, location, sponsorship flag, experience requirement, role category, responsibilities
- Status tracking: **New → Saved → Applied** (or Dismissed)
- Notes per job

### Eligibility Filtering
Jobs are automatically discarded if the posting:
- Explicitly denies H-1B / visa sponsorship
- Requires US Citizenship or Green Card
- Requires or asks candidate to obtain any security clearance
- Requires more experience than your configured `DISCARD_JOB_WITH_EXP_GT` limit

### Applications Tracker
- Track every job through pipeline stages
- Stages: Phone Screen → Technical Interview → Final Round → Offer → Rejected

### Company Management
- Add companies to monitor by careers URL
- Manual scan trigger per company
- Automatic scan scheduling via `SCAN_INTERVAL_MINUTES` (default: 24h)

### Dashboard
Live stats: jobs scanned today, new/saved/applied counts, pipeline pending, pipeline errors.

### Insights
Analytics over all enriched jobs: funnel, role categories, company hiring velocity, location breakdown, sponsorship trend, tech stack trends.

---

## Supported ATS Platforms

| Platform | Adapter |
|---|---|
| Greenhouse | `backend/apps/crawler/ats/greenhouse` |
| Lever | `backend/apps/crawler/ats/lever` |
| Ashby | `backend/apps/crawler/ats/ashby` |
| Apple | `backend/apps/crawler/ats/apple` |
| Meta | `backend/apps/crawler/ats/meta` |
| Microsoft | `backend/apps/crawler/ats/microsoft` |
| Google (JS-rendered) | `backend/apps/crawler/ats/google` |

---

## Stack

| Layer | Technology |
|---|---|
| Backend | Go |
| Frontend | React + TypeScript + Tailwind |
| Database | Supabase (PostgreSQL) |
| Message queue | Kafka |
| LLM | Gemini (via Google Generative AI SDK) |
| Auth | Supabase (magic link / email OTP) |
| Migrations | goose |
| Container orchestration | Kubernetes (k3d local) via Helm |
| Reverse proxy | Caddy (prod) |

---

## Running — Kubernetes (primary)

Kubernetes with Helm is the primary way to run JobRadar locally. Everything runs in a k3d cluster.

### Prerequisites

- [k3d](https://k3d.io) + [kubectl](https://kubernetes.io/docs/tasks/tools/) + [Helm 3](https://helm.sh/docs/intro/install/)
- Docker Desktop running

### 1. Create the cluster

```bash
k3d cluster create jobradar \
  --port "30080:30080@loadbalancer" \
  --port "30300:30300@loadbalancer" \
  --port "30001:30001@loadbalancer" \
  --port "30090:30090@loadbalancer" \
  --port "30686:30686@loadbalancer"
```

This wires NodePorts directly to localhost — no port-forwarding needed for day-to-day use.

### 2. Create secrets

Each service needs a Kubernetes Secret with its DB credentials. Examples are in `k8s/helm/jobradar/examples/secrets/`.

```bash
# Copy and fill in real values for each service
for svc in api scheduler crawler filter enricher migrate; do
  cp k8s/helm/jobradar/examples/secrets/${svc}-secret.yaml.example /tmp/${svc}-secret.yaml
  # edit /tmp/${svc}-secret.yaml with real DB credentials
  kubectl apply -f /tmp/${svc}-secret.yaml -n jobradar-local
done
```

For the observability audit sync (optional — ships Supabase auth logs to Loki):
```bash
kubectl create secret generic supabase-audit-sync-secret \
  --from-literal=SUPABASE_DB_URL="postgres://postgres:PASSWORD@HOST.supabase.co:5432/postgres" \
  -n jobradar-local
```

### 3. Build and import images

```bash
make k8s-build          # builds backend + crawler + console images and imports into k3d
```

For the Supabase audit sync image:
```bash
docker build -t jobradar-supabase-audit-sync:local ./infra/observability/supabase-audit-sync/
k3d image import jobradar-supabase-audit-sync:local -c jobradar
```

### 4. Deploy the app

```bash
make k8s-up             # helm install jobradar chart (namespace → kafka → redis → migrate → workers → api → console)
```

### 5. Deploy observability (optional)

```bash
make k8s-obs-up         # helm install obs chart (Prometheus + Grafana + Loki + Jaeger + audit-sync)
```

### Access

| Service | URL | Notes |
|---|---|---|
| Console (UI) | http://localhost:30300 | Direct via NodePort |
| API | http://localhost:30080 | Direct via NodePort |
| Grafana | http://localhost:30001 | Login: `admin` / `admin` — change on first use |
| Jaeger (tracing) | http://localhost:30686 | Direct via NodePort |
| Prometheus | http://localhost:30090 | Direct via NodePort |

> If the cluster was created **without** the `--port` flags above, use `make k8s-forward` and `make k8s-obs-forward` instead.

### Day-to-day commands

```bash
make k8s-status              # show all pod statuses
make k8s-logs                # tail logs for all app services
make k8s-logs-api            # tail a specific service (also: -crawler, -filter, -enricher, -scheduler, -console)
make k8s-restart             # rolling restart all app deployments
make k8s-restart-api         # rolling restart one service
make k8s-migrate             # re-run DB migrations
make k8s-forward             # port-forward console → :3000, api → :8080 (if no NodePort mappings)
make k8s-obs-forward         # port-forward Grafana :3001, Jaeger :16686, Prometheus :9090
make k8s-down                # uninstall app chart (secrets and PVCs preserved)
make k8s-obs-down            # uninstall observability chart
```

### After a code change

```bash
make k8s-build-backend       # rebuild + reimport backend image (api/scheduler/filter/enricher/migrate)
make k8s-build-crawler       # rebuild + reimport crawler image
make k8s-build-console       # rebuild + reimport console image
make k8s-restart             # roll all deployments onto the new image
```

---

## Running — Active Development (hot reload)

For iterating quickly on the API or frontend without rebuilding containers:

```bash
make dev          # Kafka + Redis in Docker; API on host :8080, Vite dev server :5173
```

---

## Running — Docker Compose (legacy)

Docker Compose is kept for reference but Kubernetes is preferred.

```bash
make local        # full stack — API :8080, console :3000, Grafana :3001
make local-down   # stop
```

---

## Configuration

Two env files:

- **`.env`** (repo root) — Docker Compose + infra vars; also symlinked as `ui/console/.env` for the Vite dev server
- **`backend/.env`** — backend service config (DB, Kafka, Redis, LLM, Auth)

Copy from examples:
```bash
cp .env.example .env
cp backend/.env.example backend/.env
```

### Key variables (`backend/.env`)

| Variable | Description |
|---|---|
| `DB_HOST` | Supabase pooler host |
| `DB_USER` | Supabase pooler user (`postgres.<project-ref>`) |
| `DB_PASSWORD` | Supabase DB password |
| `DB_SSLMODE` | Always `require` |
| `SUPABASE_PROJECT_URL` | `https://<project-ref>.supabase.co` |
| `ALLOWED_EMAIL` | Single email allowed through auth |
| `TARGET_ROLES` | Comma-separated target role titles |
| `YEARS_EXP` | Your years of experience |
| `DISCARD_JOB_WITH_EXP_GT` | Drop jobs requiring more than N years (0 = disabled) |
| `LLM_SLOTS_CONFIG` | Path to `llm-slots.yaml` |
| `SCAN_INTERVAL_MINUTES` | How often each company is re-scanned |

### LLM slots (`backend/common/config/llm-slots.yaml`)

```yaml
filter:
  slots:
    - key_id: filter_1
      api_key: ${GEMINI_KEY}
      model: gemini-2.0-flash-lite
      rpm: 12
      rpd: 300
      tpm_in: 1000000

enricher:
  slots:
    - key_id: enricher_1
      api_key: ${GEMINI_KEY}
      model: gemini-2.0-flash
      rpm: 12
      rpd: 1500
      tpm_in: 1000000
```

---

## Kubernetes Structure

```
k8s/helm/
  jobradar/             ← app chart (api, console, workers, kafka, redis, migrate)
    Chart.yaml
    values.yaml         ← all config defaults
    templates/          ← one directory per component
    examples/secrets/   ← secret templates (fill in and apply manually)
  observability/        ← observability chart (Prometheus, Grafana, Loki, Jaeger, audit-sync)
    Chart.yaml          ← depends on kube-prometheus-stack, loki-stack, jaeger
    values.yaml
    templates/
    dashboards/         ← Grafana dashboard JSON files
```

Helm releases in namespace `jobradar-local`:

| Release | Chart | Manages |
|---|---|---|
| `jobradar` | `k8s/helm/jobradar` | api, console, scheduler, crawler, filter, enricher, kafka, redis, migrate |
| `obs` | `k8s/helm/observability` | Grafana, Prometheus, Loki, Promtail, Jaeger, supabase-audit-sync |

---

## Auth

Login uses **Supabase magic link (email OTP)**. No password required.

Add allowed redirect URLs in Supabase → Authentication → URL Configuration:
- Local k8s: `http://localhost:30300/auth/callback`
- Local dev (Vite): `http://localhost:5173/auth/callback`
- Production: `https://<your-domain>/auth/callback`

### Multi-user (Phase 1)

Phase 1 of the multi-user rollout has shipped — see [`Designs/multi-user-rollout-plan.md`](Designs/multi-user-rollout-plan.md):

- New tables: `users`, `roles`, `user_roles`, `admin_audit_log` (migration `043`)
- On first login, the user is provisioned in `users` and granted `user`. If their email is in `ADMIN_EMAILS`, they also get `admin`.
- Identity flows via `context.Context` — middleware enriches every request with the caller's `Identity`.
- `GET /api/me`, `PATCH /api/me`, `DELETE /api/me` are live.
- Per-user rate limit (Redis token bucket, fail-open) — defaults: 30 rpm read, 20 rpm write.

Phase 2 (per-user data model, `/api` vs `/admin` route split) and Phase 3 (frontend admin UI) are **planned**.

---

## Useful commands

```bash
make migrate          # run pending DB migrations (local Go binary)
make db-truncate      # truncate all data except companies (requires DB_URL env var)
make test             # run all Go tests
make lint             # ESLint on console
make lint-go          # golangci-lint on backend
make fmt              # gofmt all Go files
```
