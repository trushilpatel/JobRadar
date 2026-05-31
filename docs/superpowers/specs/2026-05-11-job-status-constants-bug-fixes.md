# Job Status Constants & Bug Fixes

**Date:** 2026-05-11

## What changed

### New files

- `frontend/src/constants/jobStatus.ts` — Single source of truth for all job status values. Exports `JOB_STATUS` constant object, `JobStatus` type (derived from the object), `PATCHABLE_STATUSES` / `PatchableStatus` (statuses writable via `PATCH /jobs/:id/status`), `TAB_KEYS` / `TabKey` (tab navigation), and `SELECT_STATUSES` / `SelectStatus` (status dropdown options).

### Modified files

- `frontend/src/types/index.ts` — `JobStatus` type definition replaced with a re-export from `@/constants/jobStatus`. All existing consumers continue to import from `@/types` without change.
- `frontend/src/api/jobs.ts` — `setJobStatus` parameter narrowed to `PatchableStatus`; raw `'new'` literals in `normalizeJob` replaced with `JOB_STATUS.NEW`; `JobFilters.status` typed as `JobStatus`.
- `frontend/src/features/jobs/hooks/useJobs.ts` — Imports `JOB_STATUS` and `TabKey` from constants; re-exports `TabKey` for downstream consumers; initial tab state uses `JOB_STATUS.NEW`.
- `frontend/src/features/jobs/hooks/useJobActions.ts` — `setStatus` mutation routes `JOB_STATUS.NEW` to `setJobSaved(id, false)` (deletes the `user_job_state` row) instead of the invalid `setJobStatus(id, 'new')`; `KEY_TO_STATUS` entries use `JOB_STATUS` constants.
- `frontend/src/features/jobs/hooks/useJobShortcuts.ts` — `KEY_TO_STATUS` entries use `JOB_STATUS` constants.
- `frontend/src/features/jobs/components/StatusSelect.tsx` — `ALL_STATUSES` spread from `SELECT_STATUSES`; `SettableStatus` aliased to `SelectStatus`.
- `frontend/src/features/jobs/components/JobStatusTabs.tsx` — `TABS` array derived from `TAB_KEYS.map()` with labels keyed by `JOB_STATUS` constants.

## Bugs fixed

- **Saved → New transition** — Selecting "New" in the status dropdown sent `PATCH /jobs/:id/status` with `{ status: 'new' }`. The backend explicitly rejects this because `'new'` is synthetic (it means no row in `user_job_state`). The fix routes the action through `setJobSaved(id, false)` which issues `PATCH /jobs/:id/save` with `{ saved: false }`, deleting the row and returning the job to the implicit "new" state.
- **Type gap on `setJobStatus`** — The function accepted `'new'` as a valid status string. Narrowing to `PatchableStatus` makes passing `'new'` or `'dismissed'` a compile-time error.

## Architecture rules enforced

- No raw job status string literals — all status values come from `JOB_STATUS` constants.
- `'new'` is never written to the API as a status value — the semantic is "delete the row", handled in `useJobActions`.
- `PatchableStatus` type is the gatekeeper at the API boundary; the mutation hook accepts `Exclude<JobStatus, 'dismissed'>` (the broader UI surface) and resolves the correct call internally.
