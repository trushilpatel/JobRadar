# apps/ — Runnable binaries

Each subdirectory is a self-contained app: one `main.go` entry point, its worker logic, and any adapters it owns exclusively.

## Apps

| App          | Binary name | Role                                              |
| ------------ | ----------- | ------------------------------------------------- |
| `crawler/`   | `crawler`   | Fetches careers pages, publishes raw jobs         |
| `scheduler/` | `scheduler` | Enqueues companies due for a scan; resets stuck queue rows |
| `filter/`    | `filter`    | LLM batch scoring, routes relevant jobs           |
| `analyzer/`  | `enricher`  | LLM enrichment, writes final rows to `job_feed`   |
| `api/`       | `api`       | REST API on `:8080`                               |
| `migrate/`   | `migrate`   | One-shot migration runner (goose up)              |

## Rules

- No business logic in `main.go` — wiring only (open DB, build deps, start worker)
- Workers depend on **service interfaces**, never on concrete store types or GORM
- Each app owns its pipeline logic inline (`package main`) — no separate pipeline packages
- Shared domain logic lives in `service/` — apps import from there, never the reverse
