# Bulk Dismiss API — Design Spec

Date: 2026-05-01

## Problem

The current "mark irrelevant" bulk action fires one HTTP request per job via `Promise.allSettled`. For large selections this is slow and wasteful. A single bulk endpoint with batch DB writes solves it.

## API Contract

```
POST /jobs/dismiss/bulk
Body:    { "ids": ["uuid1", "uuid2", ...] }
Response 200: { "dismissed": 12 }
Response 400: validation error (empty list, exceeds 100)
Response 500: internal error
```

Reason is hardcoded to `user_not_relevant` on the backend — no reason field on the wire.

## New DTO

`backend/service/job/dto/BulkDismissInput.go`

- `IDs []string` — job_feed row IDs
- `Reason string` — injected by handler, not from client

## Service Interface Change

`backend/service/job/services/JobService.go`

- Add `BulkDismiss(ctx context.Context, input dto.BulkDismissInput) (int, error)` to the `Service` interface
- Add auto-delegation in `BaseService`

## Decorator Chain

### RestService

- Route: `POST /jobs/dismiss/bulk`
- Parses `{ "ids": [...] }` from body
- Injects `Reason: jobcommon.DiscardReasonUserNotRelevant`
- Calls `h.next.BulkDismiss`
- Responds with `{ "dismissed": N }`

### ValidationService

- Validates `len(ids) >= 1` and `len(ids) <= 100`
- Returns `ValidationError` on failure

### ImplService

Orchestrates 4 batch writes in order:

1. `discard.InsertDiscardBatch(ctx, ids, reason)` — bulk INSERT into `job_dismissal_log`
2. `jobStore.GetJobPostIDs(ctx, ids)` — resolve job_feed IDs to job_post_ids for ledger
3. `ledger.UpdateSeenReasonBatch(ctx, jobPostIDs, jobcommon.SeenReasonUserDismissed)` — already exists
4. `s.next.BulkDismiss(ctx, input)` — updates `job_feed` in batch
5. Non-fatal: `snapshot.UpdateSnapshotActionBatch(ctx, ids, jobcommon.JobStatusDismissed)`

### RepositoryService

- Terminal: `UPDATE job_feed SET status='dismissed' WHERE id = ANY(?)`
- Returns count of affected rows

## New Store Methods

### JobStore (interface + implementation)

| Method                                                                  | Query                                                |
| ----------------------------------------------------------------------- | ---------------------------------------------------- |
| `GetJobPostIDs(ctx, ids []string) ([]string, error)`                    | `SELECT job_post_id FROM job_feed WHERE id = ANY(?)` |
| `UpdateJobStatusBatch(ctx, ids []string, status string) (int64, error)` | `UPDATE job_feed SET status=? WHERE id = ANY(?)`     |

### DiscardStore

| Method                                                       | Query                                                                                                   |
| ------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------- |
| `InsertDiscardBatch(ctx, ids []string, reason string) error` | Multi-row `INSERT INTO job_dismissal_log (job_result_id, reason, discarded_at) VALUES (?,?,NOW()), ...` |

### SnapshotStore

| Method                                                              | Query                                                            |
| ------------------------------------------------------------------- | ---------------------------------------------------------------- |
| `UpdateSnapshotActionBatch(ctx, ids []string, action string) error` | `UPDATE job_analytics SET action=? WHERE job_result_id = ANY(?)` |

## Route Registration

`backend/service/job/services/JobRestRouter.go`

- Add `router.Post("/jobs/dismiss/bulk", h.handler.BulkDismiss)`

## Frontend Change

`frontend/src/features/jobs/hooks/useJobActions.ts`

- Replace `Promise.allSettled(ids.map(id => dismissJob(id, ...)))` with a single `POST /jobs/dismiss/bulk` call
- Add `bulkDismiss` function in `frontend/src/api/jobs.ts`

## Files Touched

| File                                                | Change                                                          |
| --------------------------------------------------- | --------------------------------------------------------------- |
| `service/job/dto/BulkDismissInput.go`               | New DTO                                                         |
| `service/job/services/JobService.go`                | Add method to interface + BaseService                           |
| `service/job/services/JobRestRouter.go`             | Register new route                                              |
| `service/job/services/JobRestService.go`            | Add handler                                                     |
| `service/job/services/JobValidationService.go`      | Add validation                                                  |
| `service/job/services/JobImplService.go`            | Add orchestration                                               |
| `service/job/services/JobFeedRepository.go`         | Add terminal BulkDismiss + GetJobPostIDs + UpdateJobStatusBatch |
| `service/job/services/JobDiscardRepository.go`      | Add InsertDiscardBatch                                          |
| `service/job/services/JobSnapshotRepository.go`     | Add UpdateSnapshotActionBatch                                   |
| `frontend/src/api/jobs.ts`                          | Add bulkDismissJobs API function                                |
| `frontend/src/features/jobs/hooks/useJobActions.ts` | Replace N-requests with single bulk call                        |
