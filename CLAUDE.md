# JobRadar

Single-user automated job tracker. Monitors careers pages, filters via LLM, surfaces relevant jobs.

Full design spec: `Designs/job-tracker-final-architecture.md`

## Pipeline

```
company_scan_scheduler → company_scan_queue (DB) → job_listing_crawler → job_listings.raw (Kafka) → job_listing_relevance_filter → job_listings.filtered (Kafka) → job_post_analyzer → job_feed (DB) → API
```

## Directory map

| Path | What lives there |
|---|---|
| `cmd/` | Binary entry points — one per worker + api |
| `pipeline/` | The entire data pipeline |
| `pipeline/ats/` | Careers page fetchers (Greenhouse, Lever, Ashby, Google) |
| `pipeline/crawler/` | Polls scan queue, fetches careers pages, publishes raw jobs |
| `pipeline/filter/` | Scores batches via LLM, routes relevant jobs |
| `pipeline/analyzer/` | Fetches full job page, LLM enriches, writes to job_feed |
| `pipeline/llm/` | LLM scorer interface + Gemini implementation |
| `api/` | REST API router, handlers, middleware |
| `model/` | DB structs + all typed constants |
| `store/` | All DB query logic — one file per table |
| `infra/` | Infrastructure plumbing (db, kafka, logger, metrics) |
| `migrations/` | goose SQL migration files |
| `deploy/` | Docker / Kafka entrypoint scripts |

## Workers

| Binary | Role |
|---|---|
| `cmd/company_scan_scheduler` | Queries DB for companies due for a scan, inserts into `company_scan_queue` |
| `cmd/job_listing_crawler` | Polls `company_scan_queue` (DB), fetches careers pages, publishes to `job_listings.raw` |
| `cmd/job_listing_relevance_filter` | Scores batches via LLM, routes relevant to `job_listings.filtered` |
| `cmd/job_post_analyzer` | Fetches full job page (or uses pre-fetched FullContent), LLM enriches, writes to `job_feed` table |
| `cmd/api` | REST API on `:8080` |

## Database tables

| Table | Role |
|---|---|
| `companies` | Tracked companies |
| `company_scan_queue` | Durable work queue for pending/in-progress company scans; deduped by unique partial index |
| `job_pipeline_ledger` | Dedup + pipeline blocking; `removed_at` tracks when a job disappears from a careers page |
| `company_scan_log` | Per-run scan summary (fetched, new, duration, error) |
| `job_feed` | Operational feed — only `new`, `saved`, `applied`, `dismissed` rows live here |
| `job_analytics` | Append-only analytics record written by job_post_analyzer; survives job dismissal |
| `job_application_tracker` | Application stage tracking |
| `job_dismissal_log` | Dismiss log with reason and note |

## Job status flow

```
new → saved → applied    (user advances)
new → dismissed          (user dismisses — job row kept for FK integrity, hidden from feed)
```

`dismissed` jobs still exist in `job_feed` but are excluded from feed queries (`WHERE status = 'new'`). Analytics come from `job_analytics`, not `job_feed`.

## Scan scheduling

- `SCAN_INTERVAL_MINUTES` (default `1440` = 24h) controls how often each company is scanned
- Scheduler checks every 1 min; only enqueues companies where `last_scan_at` is NULL or older than the interval
- Manual trigger (`POST /companies/:id/scan`) enforces the same interval — returns 429 if scanned too recently

## Dashboard stats fields

`GET /dashboard/stats` returns:

| Field | Description |
|---|---|
| `jobs_scanned_today` | Total job pages fetched today across all companies |
| `jobs_new` | Jobs in feed with status `new` |
| `jobs_saved` | Jobs with status `saved` |
| `jobs_applied` | Jobs with status `applied` |
| `pipeline_pending` | Jobs in `job_pipeline_ledger` with `reason='fetch'` but not yet in `job_feed` table |
| `pipeline_errors` | Jobs in `job_pipeline_ledger` with `reason='error'` |

## Key rules (detail in subdirectory CLAUDE.md files)

- Workers depend on **interfaces**, never concrete types
- All DB access goes through `store/` — no raw GORM elsewhere
- All typed values (status, stage, reason) use constants from `model/constants.go`
- Insights query `job_analytics`, not `job_feed` — snapshot rows are never deleted
- job_listing_relevance_filter batch failures mark jobs as `error` in `job_pipeline_ledger` — never re-queue to Kafka
- All logging goes through `infra/logger.Logger` — never use stdlib `log` package directly
- Field keys must use constants from `infra/logger/fields.go` — never raw strings
