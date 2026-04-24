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
- Automatic scan scheduling via `SCAN_INTERVAL_MINUTES` (default: 8h)

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
| Reverse proxy | Caddy (prod) |
| Containerisation | Docker Compose · Kubernetes (k3d) |

---

## Services

| Container | Role |
|---|---|
| `jobradar_kafka` | Message broker between pipeline stages |
| `jobradar_redis` | LLM rate-limit + cooldown state |
| `jobradar_scheduler` | Enqueues companies due for a scan |
| `jobradar_crawler` | Fetches careers pages, publishes raw job listings |
| `jobradar_filter` | LLM batch-scores titles, routes relevant jobs |
| `jobradar_enricher` | Fetches full job pages, LLM enriches, writes to DB |
| `jobradar_api` | REST API on `:8080` |
| `jobradar_console` | Management console (React UI) on `:3000` |
| `jobradar_grafana` | Metrics dashboards on `:3001` |
| `jobradar_prometheus` | Metrics scraper on `:9090` |
| `jobradar_jaeger` | Distributed tracing on `:16686` |
| `jobradar_loki` | Log aggregation |
| `jobradar_promtail` | Log shipping |
| `jobradar_caddy` | TLS reverse proxy (prod only) |

---

## Configuration

Two env files:

- **`backend/.env`** — backend service config (DB, Kafka, Redis, LLM, Auth)
- **`.env`** (repo root) — Docker Compose level (frontend build args, domain, observability ports)

Copy from examples:
```bash
cp backend/example.env backend/.env
cp .env.example .env
```

### Key variables (`backend/.env`)

| Variable | Description |
|---|---|
| `DB_HOST` | Supabase pooler host |
| `DB_USER` | Supabase pooler user (`postgres.<project-ref>`) |
| `DB_PASSWORD` | Supabase DB password |
| `DB_SSLMODE` | Always `require` |
| `SUPABASE_PROJECT_URL` | `https://<project-ref>.supabase.co` |
| `SUPABASE_JWKS_URL` | Auto-derived from `SUPABASE_PROJECT_URL` if blank |
| `ALLOWED_EMAIL` | Single email allowed through auth |
| `TARGET_ROLES` | Comma-separated target role titles |
| `YEARS_EXP` | Your years of experience |
| `DISCARD_JOB_WITH_EXP_GT` | Drop jobs requiring more than N years (0 = disabled) |
| `LLM_SLOTS_CONFIG` | Path to `llm-slots.yaml` (Gemini API keys + models) |
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

## Running

### Active development (hot reload)
```bash
make dev          # Kafka + Redis in Docker; API + Vite on host at :8080 / :5173
```

### Full local stack — Docker Compose
```bash
make local        # API :8080, console :3000, Grafana :3001
make local-down   # stop
```

### Full local stack — Kubernetes (k3d)

#### Prerequisites
- [k3d](https://k3d.io) + [kubectl](https://kubernetes.io/docs/tasks/tools/) installed
- k3d cluster created:
  ```bash
  k3d cluster create jobradar \
    --port "30080:30080@loadbalancer" \
    --port "30300:30300@loadbalancer"
  ```

#### First-time setup
Create one secret per service (copy from the `secret.yaml.example` in each `k8s/<service>/` directory and apply):
```bash
# Example — repeat for kafka, crawler, filter, analyzer, api, migrate
cp k8s/scheduler/secret.yaml.example k8s/scheduler/secret.yaml
# edit with real values, then:
kubectl apply -f k8s/scheduler/secret.yaml
```

#### Build images and deploy
```bash
make k8s-build    # build jobradar-backend:local + jobradar-crawler:local and import into k3d
make k8s-up       # deploy infra (Kafka, Redis) → migrate → workers → API
```

The API is available at **`http://localhost:30080`** after `k8s-up` completes.

#### Day-to-day commands
```bash
make k8s-status           # show all pod statuses
make k8s-forward          # port-forward console → localhost:3000, api → localhost:8080
make k8s-unforward        # stop all port-forwards
make k8s-logs             # tail all service logs (prefixed by pod name)
make k8s-logs-api         # tail a specific service  (also: -crawler, -filter, -analyzer, -scheduler, -console)
make k8s-restart          # rolling restart all deployments
make k8s-restart-api      # rolling restart one service
make k8s-migrate          # re-run migrations (idempotent)
make k8s-down             # tear down workloads (secrets preserved)
```

> **Note:** If the cluster was created without `--port` mappings, use `make k8s-forward` to access services. Recreating the cluster with the port flags above makes NodePorts directly accessible on `localhost:30080` (API) and `localhost:30300` (console) without port-forwarding.

#### After a code change
```bash
make k8s-build-backend    # rebuild + reimport backend image  (scheduler/filter/analyzer/api/migrate)
make k8s-build-crawler    # rebuild + reimport crawler image
make k8s-restart          # roll all deployments onto the new image
```

### Production (Docker Compose + Caddy + TLS)
```bash
make prod         # requires DOMAIN + ACME_EMAIL in root .env
make prod-down    # stop
```

### Useful commands
```bash
make migrate          # run pending DB migrations (Docker / local Go)
make db-truncate      # truncate all data except companies (requires DB_URL env var)
make test             # run all Go tests
make lint             # ESLint on console
make lint-go          # golangci-lint on backend
```

---

## Auth

Login uses **Supabase magic link (email OTP)**. No password, no Google OAuth needed.

Add allowed redirect URLs in Supabase → Authentication → URL Configuration:
- Local dev: `http://localhost:5173/auth/callback` (make dev)
- Local Docker: `http://localhost:3000/auth/callback` (make local)
- Production: `https://<your-domain>/auth/callback`
