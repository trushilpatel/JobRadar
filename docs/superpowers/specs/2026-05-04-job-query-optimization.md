# Job Query Optimization

**Date:** 2026-05-04

## Problem

Status-tab pagination was broken. `JobImplService.List` fetched a page of 20 global
jobs, overlaid per-user state in Go, then discarded non-matching rows. For sparse
tabs (saved/applied/dismissed), this produced pages of 0–3 results even when more
matching jobs existed deeper in the feed. Pagination was effectively broken for every
tab except "new".

Two missing DB indexes meant keyset cursor pagination and company filtering fell back
to sequential scans as the jobs table grows. Two stale indexes from the multi-user
refactor (referencing dropped `status` and `saved` columns) were adding overhead to
every write.

## What changed

### Migration 048

- `DROP INDEX idx_job_feed_status_added` — referenced `jobs.status`, dropped in 044
- `DROP INDEX idx_job_feed_saved` — referenced `jobs.saved`, dropped in 044
- `CREATE INDEX idx_jobs_added_at_id ON jobs (added_at DESC, id DESC)` — B-tree for
  keyset cursor; BRIN cannot do ordered point lookups
- `CREATE INDEX idx_jobs_company_id ON jobs (company_id)` — company filter +
  companies dropdown; old `idx_job_company` was on the pre-rename table, never recreated
- `CREATE INDEX idx_user_job_state_user_status ON user_job_state (user_id, status)` —
  for the new status-tab query path

### New query path for non-"new" tabs

`UserJobStateRepository.GetJobsByStatus` drives from `user_job_state`
(INNER JOIN `jobs` + Company association) filtered by `(user_id, status)`.
Orders by `(jobs.added_at DESC, jobs.id DESC)` — same as the global feed —
so the existing cursor encoding is reused unchanged. Returns `[]jobmodel.Job`;
`listByUserStatus` sets `UserState.Status` from the filter (no GetMany needed).

`JobImplService.List` routes to `listByUserStatus` before the cache path when
`filter.Status != "" && filter.Status != "new"`. The "new" tab and unfiltered
feed stay on the existing shared-cache + GetMany path.

### What was NOT changed

- Shared page cache (30s) — still correct for "new" tab + unfiltered feed
- GetMany overlay pattern — still correct for the shared-cache path
- Cursor encoding format — unchanged, frontend needs no changes
- GetCompanies caching — frontend 5-min staleTime is sufficient at current scale
- JobView by-value embedding — not a concern at current scale
