# JobRadar

Single-user automated job tracker. Monitors careers pages, filters via LLM, surfaces relevant jobs.

Full design spec: `Designs/job-tracker-final-architecture.md`

## Pipeline

```
Scan Engine → jobs.raw (Kafka) → LLM Scorer → jobs.raw_detail (Kafka) → Enricher → job (DB) → API
```

## Directory map

| Path | What lives there |
|---|---|
| `cmd/` | Binary entry points — one per worker + api |
| `internal/adapter/` | Careers page fetchers (Greenhouse, Lever, etc.) |
| `internal/api/` | REST API router, handlers, middleware |
| `internal/db/` | GORM connection setup |
| `internal/kafka/` | Producer, consumer, admin, topic names |
| `internal/llm/` | LLM scorer interface + Anthropic implementation |
| `internal/model/` | DB structs + all typed constants |
| `internal/store/` | All DB query logic — one file per table |
| `internal/worker/` | The three pipeline workers |
| `migrations/` | goose SQL migration files |
| `deploy/` | Docker / Kafka entrypoint scripts |

## Workers

| Binary | Role |
|---|---|
| `cmd/scan` | Fetches jobs from careers pages, publishes to `jobs.raw` |
| `cmd/scorer` | Scores batches via LLM, routes relevant to `jobs.raw_detail` |
| `cmd/enricher` | Fetches full job page, LLM enriches, writes to `job` table |
| `cmd/api` | REST API on `:8080` |

## Database tables

`companies` · `job_seen_ledger` · `job_scan_log` · `job` · `job_application` · `job_discarded`

## Key rules (detail in subdirectory CLAUDE.md files)

- Workers depend on **interfaces**, never concrete types
- All DB access goes through `internal/store/` — no raw GORM elsewhere
- All typed values (status, stage, reason) use constants from `internal/model/constants.go`
