# JobRadar

Single-user automated job tracker. Monitors careers pages, filters via LLM, surfaces relevant jobs.

Full design spec: `Designs/job-tracker-final-architecture.md`

## Pipeline

```
company_scan_scheduler → company.scan_request (Kafka) → job_listing_crawler → job_listings.raw (Kafka) → job_listing_relevance_filter → job_listings.filtered (Kafka) → job_post_analyzer → job (DB) → API
```

## Directory map

| Path | What lives there |
|---|---|
| `cmd/` | Binary entry points — one per worker + api |
| `internal/ats_fetcher/` | Careers page fetchers (Greenhouse, Lever, etc.) |
| `internal/api/` | REST API router, handlers, middleware |
| `internal/db/` | GORM connection setup |
| `internal/kafka/` | Producer, consumer, admin, topic names |
| `internal/llm/` | LLM scorer interface + Anthropic implementation |
| `internal/logger/` | Logger interface + zerolog implementation + noop for tests |
| `internal/model/` | DB structs + all typed constants |
| `internal/store/` | All DB query logic — one file per table |
| `internal/worker/` | The three pipeline workers |
| `migrations/` | goose SQL migration files |
| `deploy/` | Docker / Kafka entrypoint scripts |

## Workers

| Binary | Role |
|---|---|
| `cmd/company_scan_scheduler` | Queries DB for companies due for a scan, publishes to `company.scan_request` |
| `cmd/job_listing_crawler` | Consumes `company.scan_request`, fetches careers pages, publishes to `job_listings.raw` |
| `cmd/job_listing_relevance_filter` | Scores batches via LLM, routes relevant to `job_listings.filtered` |
| `cmd/job_post_analyzer` | Fetches full job page, LLM enriches, writes to `job` table |
| `cmd/api` | REST API on `:8080` |

## Database tables

| Table | Role |
|---|---|
| `companies` | Tracked companies |
| `job_seen_ledger` | Dedup + pipeline blocking; `removed_at` tracks when a job disappears from a careers page |
| `company_scan_log` | Per-run scan summary (fetched, new, duration, error) |
| `job` | Operational feed — only `new`, `saved`, `applied`, `dismissed` rows live here |
| `job_snapshot` | Append-only analytics record written by job_post_analyzer; survives job dismissal |
| `job_application` | Application stage tracking |
| `job_discarded` | Dismiss log with reason and note |

## Job status flow

```
new → saved → applied    (user advances)
new → dismissed          (user dismisses — job row kept for FK integrity, hidden from feed)
```

`dismissed` jobs still exist in `job` but are excluded from feed queries (`WHERE status = 'new'`). Analytics come from `job_snapshot`, not `job`.

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
| `pipeline_pending` | Jobs in `job_seen_ledger` with `reason='fetch'` but not yet in `job` table |
| `pipeline_errors` | Jobs in `job_seen_ledger` with `reason='error'` |

## Key rules (detail in subdirectory CLAUDE.md files)

- Workers depend on **interfaces**, never concrete types
- All DB access goes through `internal/store/` — no raw GORM elsewhere
- All typed values (status, stage, reason) use constants from `internal/model/constants.go`
- Insights query `job_snapshot`, not `job` — snapshot rows are never deleted
- job_listing_relevance_filter batch failures mark jobs as `error` in `job_seen_ledger` — never re-queue to Kafka
- All logging goes through `internal/logger.Logger` — never use stdlib `log` package directly
- Field keys must use constants from `internal/logger/fields.go` — never raw strings
