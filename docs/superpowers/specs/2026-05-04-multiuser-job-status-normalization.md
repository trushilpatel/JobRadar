# Multi-User Job Status Normalization Fix

**Date:** 2026-05-04

## Overview

Fixed a crash on the jobs page when clicking any job row. The multi-user refactor moved per-user job state out of the `jobs` table into `user_job_state`, which caused the backend to return status/notes/saved nested under `user_state` in the API response. The frontend still read `job.status` at the top level, so it was `undefined` — crashing `StatusSelect` when it tried to access `STATUS_CONFIG[undefined].classes`.

---

## Root Cause

`backend/service/job/dto/JobView.go` — introduced as part of the multi-user refactor, `JobView` embeds `jobmodel.Job` (which has no `status`, `saved`, or `notes` fields) and adds `UserState UserJobStateView` for the per-user overlay. The JSON response shape became:

```
{ ...job fields..., "user_state": { "status": "new", "notes": null, ... } }
```

The frontend `Job` type in `frontend/src/types/index.ts` still declared `status: JobStatus` at the top level. `job.status` was always `undefined`, and `STATUS_CONFIG[undefined]` returned `undefined`, crashing `.classes` access in `StatusSelect`.

---

## Fix

### `frontend/src/types/index.ts`

- Added `UserJobState` interface matching `UserJobStateView` from the backend
- Added `user_state: UserJobState` to `Job`
- Kept `status`, `saved`, `notes` at the top level (flattened by the API layer for backward compatibility with all existing components)

### `frontend/src/api/jobs.ts`

- Added `normalizeJob()` — flattens `user_state.status` → `status` (default `'new'`), `user_state.notes` → `notes`, `user_state.saved_at != null` → `saved`, and populates the full `user_state` object
- Applied to `fetchJobs` (maps over `page.items`) and `fetchJob`

---

## Multi-User State Semantics

All job states (save, apply, dismiss, bulk dismiss, status changes) are **per-user**. They write to `user_job_state` keyed by `(user_id, job_id)`. No state change affects what other users see.

Dismiss also writes `seen_reason = 'user_dismissed'` to `job_pipeline_ledger` as a pipeline analytics signal, but this does not filter jobs globally — the crawler's `GetActiveJobIDsByCompany` query filters only on `removed_at IS NULL`.
