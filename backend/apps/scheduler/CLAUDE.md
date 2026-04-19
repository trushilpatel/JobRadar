# apps/scheduler/ — Scan scheduler

Checks every 1 minute for companies whose `last_scan_at` is NULL or older than `SCAN_INTERVAL_MINUTES`. Inserts due companies into `company_scan_queue`. Also resets queue rows stuck in `processing` state (crawler crash recovery).

## Key files

| File             | Role                                                     |
| ---------------- | -------------------------------------------------------- |
| `main.go`        | Tick loop — resets stuck rows, enqueues due companies    |
| `main_test.go`   | Unit tests for `runCycle`                                |

## Rules

- Only enqueues and resets stuck rows — never runs scans directly
- `POST /companies/:id/scan` bypasses the cooldown and always enqueues immediately (handled by the API, not the scheduler)
- Duplicate-safe: `company_scan_queue` has a unique partial index that prevents double-enqueue while a scan is pending/in-progress
- Dashboard materialized view refresh is owned by the API, not the scheduler
