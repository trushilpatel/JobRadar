# job/ — Job feed: list/get/dismiss/save/status/notes (user-facing) + pipeline write path (worker-facing)

## Multiple repository types

The job slice is the most complex — it has five distinct repository/store implementations beyond the canonical single one, all declared in `JobService.go`:

| Interface                                                                               | Implementation file                | Backing table         |
| --------------------------------------------------------------------------------------- | ---------------------------------- | --------------------- |
| `JobStore` (`JobReader` + `JobWriter`)                                                  | `JobRepositoryService.go`          | `jobs`                |
| `UserJobStateRepository`                                                                | `UserJobStateRepositoryService.go` | `user_job_state`      |
| `SeenLedgerStore` (`SeenLedgerReader` + `SeenLedgerWriter` + `SeenLedgerCompanyReader`) | `JobSeenLedgerRepository.go`       | `job_pipeline_ledger` |
| `SnapshotWriter`                                                                        | `JobSnapshotRepository.go`         | `job_analytics`       |
| `TechTagWriter`                                                                         | `JobTechTagRepository.go`          | `job_tech_tags`       |

The user-facing `JobService` chain (`ImplService` terminal) uses `JobStore` + `UserJobStateRepository`. The pipeline-facing `PipelineService` and `LedgerService` (worker contracts) are separate implementations that share the same repository types.

## Interfaces (all declared in JobService.go)

| Interface                                                                               | Consumed by                                                        |
| --------------------------------------------------------------------------------------- | ------------------------------------------------------------------ |
| `JobService`                                                                            | API handler chain                                                  |
| `LedgerService`                                                                         | Filter worker (dedup)                                              |
| `PipelineService`                                                                       | Analyzer worker (write enriched jobs)                              |
| `JobReader` / `JobWriter` / `JobStore`                                                  | `ImplService`, `PipelineService`, dashboard                        |
| `UserJobStateRepository`                                                                | `ImplService`                                                      |
| `SeenLedgerReader` / `SeenLedgerWriter` / `SeenLedgerCompanyReader` / `SeenLedgerStore` | Filter worker, analyzer worker                                     |
| `SnapshotWriter`                                                                        | Analyzer worker                                                    |
| `TechTagWriter`                                                                         | Analyzer worker                                                    |
| `ApplicationWriter`                                                                     | `ImplService` (auto-inserts application row when status → applied) |

## Non-canonical files

| File                                 | Purpose                                                                               |
| ------------------------------------ | ------------------------------------------------------------------------------------- |
| `UserJobCounterRepositoryService.go` | Counter machinery for `GetCounts` — separated from `UserJobStateRepositoryService.go` |
| `JobPipelineImplService.go`          | Implements `PipelineService` (analyzer path)                                          |
| `location/classifier.go`             | Pure USA location classifier (`IsNonUSA`) — no DB, no context, testable in isolation  |
| `services/cursor_internal_test.go`   | Tests unexported cursor encode/decode helpers                                         |
| `services/handler_internal_test.go`  | Tests unexported handler parsing helpers                                              |
| `services/sqlprobe_internal_test.go` | Tests unexported SQL probe helpers                                                    |

## Key invariants

- `job_post_id` is UNIQUE (`uq_jobs_job_post_id`, migration 071). `InsertJob` is idempotent via `ON CONFLICT DO NOTHING` and surfaces `ErrJobAlreadyExists` when the caller needs to know.
- `SeenLedgerWriter` UPDATE methods check `RowsAffected` and return `ErrLedgerRowMissing` on 0/partial rows — never return nil for a write that silently did nothing. Workers act on this: filter Naks for redelivery, analyzer treats it as non-fatal.
- `MarkSeenBatch` (via `LedgerService` or `SeenLedgerStore`) must be called **before** publishing to NATS (ledger-before-publish invariant). The crawler-node writes the `fetch` ledger row before publish so the row exists before the filter consumer can receive the message.
- `user_job_state` drives all non-`new` tab queries (saved, applied, dismissed) via `GetJobsByStatus` — it INNER JOINs to `jobs` so every page returns a full `limit`-sized result. The `new` tab uses a global `jobs` feed with a per-user overlay via `GetMany`.
- `ApplicationWriter.InsertAppliedApplication` is called by `ImplService.SetStatus` when status → `"applied"`. This is the only cross-domain write in the job slice; it must remain a narrow interface, never a direct import of `application/services`.
- `SeenLedgerWriter.UpdateRetryBatchWithPayload` preserves the `RawJob` payload for the sweeper retry path (LLM exhaustion). `FetchDueRetryJobsWithPayload` filters to rows where `payload IS NOT NULL AND adapter IS NOT NULL` to skip pre-payload-persistence rows.
- `EscalateStuckRetryJobs` promotes `reason='retry'` rows stuck past the 4-hour threshold to `reason='error'` — called by the scheduler, not the filter worker.
- `ReclaimStuckFetchRows` **DELETEs** `reason='fetch'` rows older than 1h (bounded by a per-call LIMIT, regardless of `removed_at`) — called by the scheduler each cycle. It must DELETE, not escalate: `HasSeenBatch` matches `job_id` with **no `reason`/`removed_at` filter**, so a stranded `fetch` row (left when a job entered the filter but never reached a terminal verdict — e.g. a NATS restart mid-flight) permanently blocks re-processing. DELETE clears the dedup block so the next crawl re-drives a still-live job. As a count-based sweep it returns `RowsAffected` and intentionally does **not** return `ErrLedgerRowMissing` on 0 rows (unlike the targeted UPDATE methods).
