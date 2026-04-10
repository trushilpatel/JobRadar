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
        ↓  (Postgres: job_feed)
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
Jobs are automatically discarded (never shown) if the posting:
- Explicitly denies H-1B / visa sponsorship
- Requires US Citizenship or Green Card
- Requires or asks candidate to obtain any security clearance
- Requires more experience than your configured `DISCARD_JOB_WITH_EXP_GT` limit

### Applications Tracker
- Track every job you've applied to through the pipeline stages
- Stages: Phone Screen → Technical Interview → Final Round → Offer → Rejected
- Notes and timestamps per stage

### Company Management
- Add companies to monitor by careers URL
- Manual scan trigger per company (rate-limited to avoid abuse)
- Automatic scan scheduling via `SCAN_INTERVAL_MINUTES` (default: 24h)

### Dashboard
Live stats at a glance:
- Jobs scanned today
- New / Saved / Applied counts
- Pipeline pending (fetched but not yet enriched)
- Pipeline errors

### Insights
Analytics over all enriched jobs:
- **Pipeline funnel** — enriched → saved → applied → dismissed
- **Role category distribution** — backend / frontend / fullstack / devops / ml / data
- **Company hiring velocity** — this week vs last week per company
- **Location breakdown** — by country and US state
- **Sponsorship trend** — daily total vs sponsorship-offering jobs (30-day chart)
- **Tech stack trends** — top technologies across all enriched jobs
- **Company activity** — all-time jobs enriched per company

---

## Supported ATS Platforms

| Platform | Adapter |
|---|---|
| Greenhouse | `internal/ats_fetcher/greenhouse` |
| Lever | `internal/ats_fetcher/lever` |
| Ashby | `internal/ats_fetcher/ashby` |
| Workable | `internal/ats_fetcher/workable` |
| Workday | `internal/ats_fetcher/workday` |
| Generic (JS-rendered) | `internal/ats_fetcher/chromedp` |

---

## Stack

| Layer | Technology |
|---|---|
| Backend | Go |
| Frontend | React + TypeScript + Tailwind |
| Database | PostgreSQL 16 |
| Message queue | Kafka (Confluent) |
| LLM | Gemini (via Google Generative AI SDK) |
| Migrations | goose |
| Containerization | Docker Compose |

---

## Services

| Container | Role |
|---|---|
| `jobradar_postgres` | Primary database |
| `jobradar_kafka` | Message broker between pipeline stages |
| `jobradar_scheduler` | Enqueues companies due for a scan |
| `jobradar_crawler` | Fetches careers pages, publishes raw job listings |
| `jobradar_filter` | LLM batch-scores titles, routes relevant jobs |
| `jobradar_enricher` | Fetches full job pages, LLM enriches, writes to DB |
| `jobradar_api` | REST API on `:8080` |
| `jobradar_frontend` | React UI |

---

## Configuration

All config is via environment variables (or `.env` file in `backend/`):

| Variable | Default | Description |
|---|---|---|
| `TARGET_ROLES` | `Software Engineer` | Comma-separated list of target role titles |
| `YEARS_EXP` | `0` | Your years of experience (used in LLM scoring prompt) |
| `DISCARD_JOB_WITH_EXP_GT` | `0` | Drop jobs requiring more than N years (0 = disabled) |
| `LLM_PROVIDER` | `gemini` | LLM provider |
| `LLM_API_KEY` | — | API key for the LLM provider |
| `LLM_MODEL` | `gemini-3.1-flash-lite-preview` | Model name |
| `LLM_RPM` | `15` | Rate limit: requests per minute |
| `LLM_RPD` | `500` | Rate limit: requests per day |
| `LLM_TPM` | `250000` | Rate limit: input tokens per minute |
| `SCAN_INTERVAL_MINUTES` | `1440` | How often each company is re-scanned (24h default) |
| `SCAN_COMPANY_DELAY_SECONDS` | `60` | Delay between company fetches within a scan cycle |
| `SCORER_BATCH_SIZE` | `50` | Job titles per LLM batch-score call |
| `DB_HOST` | `localhost` | Postgres host |
| `DB_PORT` | `5432` | Postgres port |
| `DB_USER` | `postgres` | Postgres user |
| `DB_PASSWORD` | `postgres` | Postgres password |
| `DB_NAME` | `jobradar` | Postgres database name |
| `KAFKA_BROKERS` | `localhost:9092` | Kafka broker address |

---

## Running

**Full stack (Docker):**
```bash
make up        # build + start everything
make logs      # tail all logs
make down      # stop
```

**Local dev (Go + Vite):**
```bash
make docker-up    # start Postgres + Kafka only
make migrate      # run DB migrations
make dev          # start API + frontend dev server
```

**Useful commands:**
```bash
make migrate      # run pending migrations
make db-truncate  # truncate all data except companies
make test         # run all Go tests
make build        # build frontend + backend
```
