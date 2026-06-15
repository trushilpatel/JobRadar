# apps/ — Runnable binaries

Each subdirectory is a self-contained app: one `main.go` entry point, its worker logic, and any adapters it owns exclusively.

## Apps

| Binary         | Role                                                                                                    |
| -------------- | ------------------------------------------------------------------------------------------------------- |
| `scheduler`    | Enqueues companies into `company_scan_queue` on schedule                                                |
| `orchestrator` | HTTP server — assigns `company_scan_queue` rows to crawler-nodes; receives results; publishes to NATS   |
| `crawler-node` | Stateless HTTP worker — polls orchestrator for work, runs crawler-v2 ATS adapters, submits results back |
| `crawler-v2`   | ATS adapter library (not a binary) — listing + detail fetch; used by crawler-node and the analyzer      |
| `filter`       | Consumes `jobs.raw.*` from NATS; runs LLM relevance filter; publishes to `jobs.filtered.*`              |
| `analyzer`     | Consumes `jobs.filtered.*`; enriches + writes to `jobs` DB                                              |
| `api`          | REST API server                                                                                         |
| `migrate`      | One-shot DB migration runner                                                                            |

## Rules

- No business logic in `main.go` — wiring only (open DB, build deps, start worker)
- Workers depend on **service interfaces**, never on concrete store types or GORM
- Each app owns its pipeline logic inline (`package main`) — no separate pipeline packages
- Shared domain logic lives in `service/` — apps import from there, never the reverse
- Crawler-node count is a Helm replica field — never hardcode concurrency in application code
