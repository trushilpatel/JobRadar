# scan/ — Scan domain: queue management, scheduler, stats, and crawler result recording

## Multiple impl services

The scan slice has three implementation files, each covering a different concern:

| File                        | Interface implemented         | Purpose                                                                                                                                                                                                                                                                                |
| --------------------------- | ----------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `ScanImplService.go`        | `Service` (via `BaseService`) | Business logic: enqueue, trigger, stats, queue management, retry                                                                                                                                                                                                                       |
| `ScanCrawlerImplService.go` | `CrawlerService`              | Crawler result recording (`RecordScanResult` → `company_scan_log` + scan state)                                                                                                                                                                                                        |
| `ScanSchedulerService.go`   | `SchedulerService`            | Standalone ~470-line service with its own state (`queue`, `ledger`, `publisher`, `auditor`, `nodeReader`, `settingsReader`) — not a thin wrapper over `Service`. Owns the retry sweeper, stuck-row reclaimers, auto-pause, and gauge-metric refresh logic used by the scheduler binary |

Queue claiming for crawler-node polls is **not** in this slice — it lives in `service/crawlernode/services/CrawlerNodeAssignmentRepositoryService.go` (`AssignmentStore.ClaimNextForNode`), which is the only live claim path. The `ScanQueueStore.ClaimNext` + `CrawlerQueueService` wrapper that used to live here (`ScanCrawlerQueueImplService.go`) were dead code from the removed v1 monolith crawler — deleted; do not re-add a second claim path in this package.

## Interfaces

| Interface          | Consumed by                                   |
| ------------------ | --------------------------------------------- |
| `Service`          | API handler chain (scan REST endpoints)       |
| `SchedulerService` | Scheduler binary worker loop                  |
| `CrawlerService`   | Orchestrator / crawler result submission path |

`Service` and `SchedulerService` are **fully independent interfaces** — `Service` does not embed `SchedulerService` (fixed 2026-07-05; it used to, which forced `ScanRepositoryService.go` to fake-implement 6 `SchedulerService` methods as hardcoded no-ops purely to satisfy the embed, none of which were ever called through the REST decorator chain — an ISP violation). `ScanSchedulerService.go`'s `schedulerService` struct holds its own `ScanQueueStore`/`SeenLedgerStore`/etc. and is the _only_ implementation of `SchedulerService`; it is never wrapped around a `Service` implementation. If a REST-facing method and a `SchedulerService` method happen to share a name (there are none left after the split), that is coincidence, not a shared contract.

## Separate stores

| File                               | Purpose                                                                                |
| ---------------------------------- | -------------------------------------------------------------------------------------- |
| `ScanRepositoryService.go`         | Terminal `Service` — scan queue + stats DB operations                                  |
| `ScanLogRepositoryService.go`      | Terminal `ScanLogWriter` — writes to `company_scan_log` and updates company scan state |
| `QueueStore` (via `NewQueueStore`) | Scan queue (company_scan_queue) operations used by the orchestrator                    |

## Key invariants

- `EnqueueDue` deduplicates via the unique partial index on `company_scan_queue` — idempotent inserts. `TriggerScan` bypasses the cooldown and always enqueues immediately.
- `ResetStuck` resets rows in `running` state whose `heartbeat_at` has expired (crawler crash recovery). The heartbeat threshold is passed by the caller, not hardcoded.
- `PurgeInactive` removes queued (not running) entries for companies whose `company_scan_settings.active = false`. It never touches running rows.
- `RetryDueJobs` is called by the scheduler tick; it reads `reason='retry'` rows with `next_attempt_at <= now()` from `job_pipeline_ledger`, re-publishes to `jobs.raw.{adapter}` via NATS, and escalates exhausted rows to `reason='error'`. Implementation delegates the ledger operations to `job/services.SeenLedgerWriter`.
- `ScanCrawlerImplService` writes `company_scan_log` and updates `company_scan_settings` scan state in a **single transaction** — never write the log without updating the state or vice versa.
- `ScanImplService` depends on `companysvc.ScanSettingsRepository` for per-company adapter config reads and `companysvc.CompanyAdminService` only for human-readable names in queue listings.
