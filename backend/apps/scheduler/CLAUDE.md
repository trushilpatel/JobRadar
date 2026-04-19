# apps/scheduler/ — Scan scheduler

Checks every 1 minute for companies whose `last_scan_at` is NULL or older than `SCAN_INTERVAL_MINUTES`. Inserts due companies into `company_scan_queue`.

## Key files

| File        | Role                                      |
| ----------- | ----------------------------------------- |
| `main.go`   | Tick loop — queries DB, enqueues, sleeps  |

## Rules

- Only enqueues — never runs scans directly
- `POST /companies/:id/scan` bypasses the cooldown and always enqueues immediately (handled by the API, not the scheduler)
- Duplicate-safe: `company_scan_queue` has a unique partial index that prevents double-enqueue while a scan is pending/in-progress
