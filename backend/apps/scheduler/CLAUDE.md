# apps/scheduler/ — Scan scheduler

Checks every 1 minute for companies whose `last_scan_at` is NULL or due for a scan. Inserts due companies into `company_scan_queue`. Also resets queue rows stuck in `running` state (crawler crash recovery) and reclaims two classes of stranded ledger rows each cycle:

- `ReclaimStuckFetchRows` — DELETEs `reason='fetch'` rows older than 1h (regardless of `removed_at`) so a job that never reached a terminal filter verdict is re-driven on the next crawl instead of being silently deduped out.
- `ReclaimStuckFilteredRows` — DELETEs `reason='filtered'` rows older than 1h (regardless of `removed_at`) so a job that passed the filter but whose enricher NATS message expired is re-driven rather than permanently blocked.

The effective scan interval per company is resolved from a 3-level cascade inside `EnqueueDue`:
`company_scan_settings.scan_interval_override → adapter_configs.scan_interval_minutes → SCAN_INTERVAL_MINUTES env var`
All three are in minutes. `last_scan_at` lives on `company_scan_settings` (not `companies`).

## Key files

| File                | Role                                                                                   |
| ------------------- | -------------------------------------------------------------------------------------- |
| `main.go`           | Wiring only — sets up config, initialises SchedulerService, calls `Run()`              |
| `scheduler.go`      | Tick loop (`Run`, `runCycle`), jitter, graceful shutdown                               |
| `interfaces.go`     | Local `SchedulerService` interface (mirrors `scansvc.SchedulerService` for decoupling) |
| `main_test.go`      | Startup and config tests                                                               |
| `scheduler_test.go` | Tick loop tests (`Run`, `runCycle`, concurrency, jitter)                               |

## Rules

- Only enqueues and resets stuck rows — never runs scans directly
- `POST /companies/:id/scan` bypasses the cooldown and always enqueues immediately (handled by the API, not the scheduler)
- Duplicate-safe: `company_scan_queue` has a unique partial index that prevents double-enqueue while a scan is pending/in-progress
- Dashboard materialized view refresh is owned by the API, not the scheduler
- Each operation in `runCycle` continues on error — `runCycle` never returns early on a single failure

## Adding a new scheduled task

Six touch-points, in order:

1. `backend/apps/scheduler/interfaces.go` — add the method to the local `SchedulerService` interface
2. `backend/service/scan/services/ScanService.go` — add the method to both the `SchedulerService` interface and the `Service` interface
3. `backend/service/scan/services/ScanImplService.go` — implement on `schedulerService` (the concrete struct used by the scheduler binary)
4. `backend/service/scan/services/ScanRepositoryService.go` — implement on `scanQueueStore` (if DB access is needed)
5. `backend/apps/scheduler/scheduler.go` — add a call inside `runCycle`; each operation continues on error, never returns early
6. `backend/common/metrics/metrics.go` — add a counter following the existing pattern (e.g. `SchedulerCompaniesEnqueuedTotal`)
