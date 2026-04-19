# apps/analyzer/ — Job post analyzer (enricher)

Consumes `job_listings.filtered` (Kafka), fetches full job page content, runs LLM enrichment, writes final rows to `job_feed` and `job_analytics`.

## Key files

| File           | Role                                                                  |
| -------------- | --------------------------------------------------------------------- |
| `main.go`      | Wiring — Kafka consumer, LLM enricher, pipeline service               |
| `worker.go`    | Per-job enrichment pipeline: fetch → LLM → drop check → save         |
| `techtag/`     | Normalizes and deduplicates raw `tech_stack` strings from LLM output  |

## Enrichment flow

```
RawJob.FullContent != nil  →  fullContentToResponse()  →  Enrich()  →  dropJob()  →  SaveEnrichedJob()
RawJob.FullContent == nil  →  fetchContent()            →  Enrich()  →  dropJob()  →  SaveEnrichedJob()
```

`FullContent` skips the HTTP fetch but enrichment always runs — it extracts sponsorship flags, tech stack, salary etc.

## dropJob — eligibility rules (Go, not LLM)

| Condition              | Drop if                                  |
| ---------------------- | ---------------------------------------- |
| No sponsorship         | `Sponsorship == false`                   |
| Citizenship required   | `RequiresCitizen == true`                |
| Security clearance     | `SecurityClearance == true`              |
| Over experience limit  | `ExperienceYears > DISCARD_JOB_WITH_EXP_GT` (0 = disabled) |

## Rules

- Errors are **per-job** — one bad job never stops the worker
- Snapshot write (`job_analytics`) is non-fatal — if it fails, job row is already saved
- Depends on `service/llm/services.JobPostEnricher` — never on `*geminiScorer` directly
