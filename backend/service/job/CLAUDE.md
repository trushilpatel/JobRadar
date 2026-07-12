# job/ — Job feed: list/get/save/status/notes (user-facing) + pipeline write path (worker-facing)

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

| File                                    | Purpose                                                                                                                                   |
| --------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------- |
| `UserJobCounterRepositoryService.go`    | Counter machinery for `GetCounts` — separated from `UserJobStateRepositoryService.go`                                                     |
| `JobPipelineImplService.go`             | Implements `PipelineService` (analyzer path)                                                                                              |
| `location/classifier.go`                | Pure USA location classifier (`IsNonUSA`) — no DB, no context, testable in isolation                                                      |
| `services/cursor_internal_test.go`      | Tests unexported cursor encode/decode helpers                                                                                             |
| `services/handler_internal_test.go`     | Tests unexported handler parsing helpers                                                                                                  |
| `services/sqlprobe_internal_test.go`    | Tests unexported SQL probe helpers                                                                                                        |
| `tests/JobImplService_test.go`          | Shared fixtures (fakeJobReader, fakeUserState, `newSvc`, ...) + read-path (List/Get) tests                                                |
| `tests/JobImplServiceMutations_test.go` | SetSaved/SetStatus/SetNotes/Dismiss/BulkDismiss behavioral tests (split out of JobImplService_test.go 2026-07-05 — it had hit 1113 lines) |
| `tests/JobImplServiceCache_test.go`     | Page cache + per-user counts cache tests, incl. invalidation on every mutating method (split out 2026-07-05)                              |

## Caching deviates from the `CacheService` decorator pattern — intentionally

`JobImplService.go` caches inline (`s.cache cache.Cache` field, `pageCacheKey`/`userCountsKey`, `cache.GetOrLoad` calls inside `List` and `GetCounts`) instead of extracting a separate `JobCacheService.go` decorator like `user/` and `ats/` do. This is not an oversight:

- `implService` is the terminal `JobService` — it composes five narrow repo interfaces (`JobReader`, `UserJobStateRepository`, `SnapshotWriter`, `ApplicationWriter`, `SeenLedgerWriter`) directly; there is no `next JobService` to wrap the canonical way.
- More importantly, `List`'s cached entry (`pageCacheEntry`) holds only the **global, pre-overlay** page — `s.jobs.GetJobs(...)`'s raw result — before the per-user `user_job_state` overlay is merged in. A canonical outer `CacheService` decorator sitting in front of `List` would instead cache `List`'s full return value, which already has one caller's overlay merged in — that cached entry would then leak into a **different user's** cache hit on the same (cursor, filter) key. Do not "fix" this deviation by extracting an outer decorator; it would introduce a cross-user data leak. If this ever needs to change, the overlay merge must stay outside whatever layer is cached.

## `ImplService` doesn't embed `BaseService` — unlike every other slice's Impl layer

`JobImplService.go`'s `implService` implements all 11 `JobService` methods directly with zero `BaseService` embedding, unlike the `{Domain}ImplService` in every other slice (which embeds `BaseService` and overrides only the methods it owns). This isn't an oversight either, and it isn't safe to "fix" by adding the embed: `BaseService` exists to delegate to a `next {Domain}Service` — but `implService` here has no `next JobService`, it terminates directly against five narrow repo interfaces (`JobReader`, `UserJobStateRepository`, `SnapshotWriter`, `ApplicationWriter`, `SeenLedgerWriter`), the same reason it can't use the `CacheService` decorator pattern (see above). There is nothing to delegate to, so there is nothing for an embedded `BaseService` to wrap.

## Key invariants

- `job_post_id` is UNIQUE (`uq_jobs_job_post_id`, migration 071). `InsertJob` is idempotent via `ON CONFLICT DO NOTHING` and surfaces `ErrJobAlreadyExists` when the caller needs to know.
- `SeenLedgerWriter` UPDATE methods check `RowsAffected` and return `ErrLedgerRowMissing` on 0/partial rows — never return nil for a write that silently did nothing. Workers act on this: filter Naks for redelivery, analyzer treats it as non-fatal.
- `MarkSeenBatch` (via `LedgerService` or `SeenLedgerStore`) must be called **before** publishing to NATS (ledger-before-publish invariant). The crawler-node writes the `fetch` ledger row before publish so the row exists before the filter consumer can receive the message.
- `user_job_state` drives all non-`new` tab queries (saved, applied, interviewing, offer, rejected) via `GetJobsByStatus` — it INNER JOINs to `jobs` so every page returns a full `limit`-sized result. The `new` tab uses a global `jobs` feed with a per-user overlay via `GetMany`. `dismissed` is no longer a queryable tab/filter (feature removed, migration 091) — `validJobStatuses` in `JobValidationService.go` rejects it, but historical rows keep that status forever (see root `CLAUDE.md`'s Job status flow section).
- `ApplicationWriter.InsertAppliedApplication` is called by `ImplService.SetStatus` when status → `"applied"`. This is the only cross-domain write in the job slice; it must remain a narrow interface, never a direct import of `application/services`.
- `SeenLedgerWriter.UpdateRetryBatchWithPayload` preserves the `RawJob` payload for the sweeper retry path (LLM exhaustion). `FetchDueRetryJobsWithPayload` filters to rows where `payload IS NOT NULL AND adapter IS NOT NULL` to skip pre-payload-persistence rows.
- `EscalateStuckRetryJobs` promotes `reason='retry'` rows stuck past the 4-hour threshold to `reason='error'` — called by the scheduler, not the filter worker.
- `ReclaimStuckFetchRows` **DELETEs** `reason='fetch'` rows older than 1h (bounded by a per-call LIMIT, regardless of `removed_at`) — called by the scheduler each cycle. It must DELETE, not escalate: `HasSeenBatch` matches `job_id` with **no `reason`/`removed_at` filter**, so a stranded `fetch` row (left when a job entered the filter but never reached a terminal verdict — e.g. a NATS restart mid-flight) permanently blocks re-processing. DELETE clears the dedup block so the next crawl re-drives a still-live job. As a count-based sweep it returns `RowsAffected` and intentionally does **not** return `ErrLedgerRowMissing` on 0 rows (unlike the targeted UPDATE methods).
