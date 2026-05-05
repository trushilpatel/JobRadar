# Job Query Optimization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix broken status-tab pagination and add missing DB indexes — no unnecessary changes for 5 users.

**Architecture:** Status tabs other than "new" will drive queries from `user_job_state` (INNER JOIN to `jobs`) so every page returns exactly `limit` results. The "new" tab and global feed stay on the existing shared-cache path. The cursor format (`added_at|id`, base64) is unchanged so the frontend needs no changes. A single migration drops two stale indexes left over from multi-user refactor, adds a B-tree on `(added_at DESC, id DESC)` for keyset cursor support, adds `company_id` index, and adds a `(user_id, status)` index for the new query path.

**Tech Stack:** Go 1.23, GORM v2, PostgreSQL 15, goose migrations, React + TanStack Query

---

## File Map

| File | Change |
|---|---|
| `backend/migrations/048_job_query_optimizations.sql` | New — drop 2 stale indexes, add 3 new indexes |
| `backend/service/job/services/JobService.go` | Add `GetJobsByStatus` to `UserJobStateRepository` interface + `BaseService` delegation |
| `backend/service/job/services/UserJobStateRepositoryService.go` | Implement `GetJobsByStatus` |
| `backend/service/job/services/JobImplService.go` | Add `listByUserStatus`, route non-"new" tabs in `List` |
| `backend/service/job/tests/JobImplService_test.go` | Add `GetJobsByStatus` to `fakeUserState`, add routing tests |
| `docs/superpowers/specs/2026-05-04-job-query-optimization.md` | New design doc |

---

## Task 1: Design doc

**Files:**
- Create: `docs/superpowers/specs/2026-05-04-job-query-optimization.md`

- [ ] **Step 1: Create the spec file**

```markdown
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
- `CREATE INDEX idx_jobs_company_id ON jobs (company_id)` — company filter + companies
  dropdown query; old idx_job_company was on the pre-rename table and was never
  recreated
- `CREATE INDEX idx_user_job_state_user_status ON user_job_state (user_id, status)` —
  for the new status-tab query path

### New query path for non-"new" tabs

`UserJobStateRepository.GetJobsByStatus` drives from `user_job_state` (INNER JOIN to
`jobs` + Company association) filtered by `(user_id, status)`. Orders by
`(jobs.added_at DESC, jobs.id DESC)` — same as global feed — so the existing cursor
encoding is reused unchanged. Returns `[]jobmodel.Job`; the calling `listByUserStatus`
sets `UserState.Status` from the filter (no GetMany needed for list view).

`JobImplService.List` routes to `listByUserStatus` before the cache path when
`filter.Status != "" && filter.Status != "new"`. The "new" tab and unfiltered feed
stay on the existing shared-cache + GetMany path.

### What was NOT changed

- Shared page cache (30s) — still correct for "new" tab + unfiltered feed
- GetMany overlay pattern — still correct for the cache path
- Cursor encoding format — unchanged, frontend needs no changes
- GetCompanies caching — frontend 5-min staleTime is sufficient at current scale
- JobView by-value embedding — not a concern at current scale
```

- [ ] **Step 2: Verify file written**
```bash
head -5 docs/superpowers/specs/2026-05-04-job-query-optimization.md
```
Expected: `# Job Query Optimization`

---

## Task 2: Migration 048

**Files:**
- Create: `backend/migrations/048_job_query_optimizations.sql`

- [ ] **Step 1: Write the migration**

```sql
-- +goose Up

-- Drop stale indexes from migration 044 multi-user refactor.
-- Both reference columns (status, saved) that were dropped from jobs in 044.
DROP INDEX IF EXISTS idx_job_feed_status_added;
DROP INDEX IF EXISTS idx_job_feed_saved;

-- B-tree for keyset cursor on the global feed.
-- BRIN (the existing idx_job_feed_added_brin) cannot do ordered point lookups
-- required by: WHERE (added_at, id) < (?, ?) ORDER BY added_at DESC, id DESC
CREATE INDEX IF NOT EXISTS idx_jobs_added_at_id
    ON jobs (added_at DESC, id DESC);

-- Company filter + GetDistinctCompanyNames.
-- idx_job_company from migration 004 was on the pre-rename table; never recreated.
CREATE INDEX IF NOT EXISTS idx_jobs_company_id
    ON jobs (company_id);

-- Status-tab query path: drives from user_job_state filtered by (user_id, status).
CREATE INDEX IF NOT EXISTS idx_user_job_state_user_status
    ON user_job_state (user_id, status);

-- +goose Down
DROP INDEX IF EXISTS idx_user_job_state_user_status;
DROP INDEX IF EXISTS idx_jobs_company_id;
DROP INDEX IF EXISTS idx_jobs_added_at_id;
-- Note: stale indexes are not restored on Down — they were invalid writes.
```

- [ ] **Step 2: Verify file written**
```bash
head -3 backend/migrations/048_job_query_optimizations.sql
```
Expected: `-- +goose Up`

---

## Task 3: Interface + fakeUserState stub

**Files:**
- Modify: `backend/service/job/services/JobService.go`
- Modify: `backend/service/job/tests/JobImplService_test.go`

- [ ] **Step 1: Add `GetJobsByStatus` to `UserJobStateRepository` interface in `JobService.go`**

Find the `UserJobStateRepository` interface (around line 134). Add the new method:

```go
type UserJobStateRepository interface {
	Get(ctx context.Context, userID, jobID string) (*jobmodel.UserJobState, error)
	GetMany(ctx context.Context, userID string, jobIDs []string) (map[string]*jobmodel.UserJobState, error)
	// GetJobsByStatus returns up to limit jobs for userID with the given status,
	// ordered by (jobs.added_at DESC, jobs.id DESC). Cursor format matches GetJobs.
	GetJobsByStatus(ctx context.Context, userID, status, cursor string, limit int) ([]jobmodel.Job, string, error)
	Upsert(ctx context.Context, userID, jobID string, state dto.UpsertState) error
	UpsertBulkDismiss(ctx context.Context, userID string, jobIDs []string, reason, note string) error
	Delete(ctx context.Context, userID, jobID string) error
	CountsByUser(ctx context.Context, userID string) (dto.Counts, error)
}
```

- [ ] **Step 2: Add BaseService delegation in `JobService.go`**

`BaseService` does not delegate `UserJobStateRepository` methods — those go directly on the repository, not through the decorator chain. No change needed to `BaseService`.

- [ ] **Step 3: Add `GetJobsByStatus` stub to `fakeUserState` in `JobImplService_test.go`**

Add fields and method to `fakeUserState`:

```go
// in the fakeUserState struct, add:
statusJobs       []jobmodel.Job
statusNext       string
statusErr        error
statusCalls      int
lastStatusStatus string
```

```go
// add method:
func (f *fakeUserState) GetJobsByStatus(_ context.Context, userID, status, _ string, _ int) ([]jobmodel.Job, string, error) {
	if userID == "" {
		return nil, "", apperr.ErrInternal.WithMessage("user id required")
	}
	f.statusCalls++
	f.lastStatusStatus = status
	return f.statusJobs, f.statusNext, f.statusErr
}
```

- [ ] **Step 4: Verify it compiles**
```bash
cd backend && go build ./service/job/...
```
Expected: no output (clean build). If `userJobStateRepository` doesn't satisfy the interface, the error will say which method is missing — that gets implemented in Task 4.

---

## Task 4: Write failing tests

**Files:**
- Modify: `backend/service/job/tests/JobImplService_test.go`

- [ ] **Step 1: Write test — non-"new" tab routes to GetJobsByStatus**

Add after the existing List tests:

```go
func TestList_SavedTab_RoutesToGetJobsByStatus(t *testing.T) {
	reader := &fakeJobReader{}
	state := &fakeUserState{
		statusJobs: []jobmodel.Job{
			{ID: "j1", Title: "Engineer", AddedAt: time.Now()},
		},
	}
	svc := jobsvc.NewImplService(jobsvc.NewRepositoryService(nil), reader, state, nil, nil, nil, nil)
	ctx := identity.WithUserID(context.Background(), "u1")

	result, err := svc.List(ctx, jobdto.JobFilter{Status: "saved"}, "", 20)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state.statusCalls != 1 {
		t.Errorf("expected GetJobsByStatus called once, got %d", state.statusCalls)
	}
	if reader.calls != 0 {
		t.Errorf("expected GetJobs not called for saved tab, got %d calls", reader.calls)
	}
	if state.lastStatusStatus != "saved" {
		t.Errorf("expected status=saved, got %q", state.lastStatusStatus)
	}
	if len(result.Items) != 1 || result.Items[0].UserState.Status != "saved" {
		t.Errorf("expected 1 item with status=saved, got %+v", result.Items)
	}
}

func TestList_NewTab_UsesGlobalFeed(t *testing.T) {
	reader := &fakeJobReader{
		jobs: []jobmodel.Job{{ID: "j1", Title: "Engineer", AddedAt: time.Now()}},
	}
	state := &fakeUserState{}
	svc := jobsvc.NewImplService(jobsvc.NewRepositoryService(nil), reader, state, nil, nil, nil, nil)
	ctx := identity.WithUserID(context.Background(), "u1")

	_, err := svc.List(ctx, jobdto.JobFilter{Status: "new"}, "", 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state.statusCalls != 0 {
		t.Errorf("expected GetJobsByStatus not called for new tab, got %d", state.statusCalls)
	}
	if reader.calls != 1 {
		t.Errorf("expected GetJobs called once for new tab, got %d", reader.calls)
	}
}

func TestList_AppliedTab_SetsStatusOnView(t *testing.T) {
	state := &fakeUserState{
		statusJobs: []jobmodel.Job{
			{ID: "j1", Title: "SWE", AddedAt: time.Now()},
			{ID: "j2", Title: "SRE", AddedAt: time.Now().Add(-time.Hour)},
		},
	}
	svc := jobsvc.NewImplService(jobsvc.NewRepositoryService(nil), &fakeJobReader{}, state, nil, nil, nil, nil)
	ctx := identity.WithUserID(context.Background(), "u1")

	result, err := svc.List(ctx, jobdto.JobFilter{Status: "applied"}, "", 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, item := range result.Items {
		if item.UserState.Status != "applied" {
			t.Errorf("expected status=applied on all views, got %q on item %s", item.UserState.Status, item.Job.ID)
		}
	}
}
```

- [ ] **Step 2: Run tests — expect compilation failure or wrong-constructor error**
```bash
cd backend && go test ./service/job/... 2>&1 | head -30
```
Expected: fails because `listByUserStatus` doesn't exist yet, or `GetJobsByStatus` not implemented on `userJobStateRepository`.

---

## Task 5: Implement GetJobsByStatus + listByUserStatus + List routing

**Files:**
- Modify: `backend/service/job/services/UserJobStateRepositoryService.go`
- Modify: `backend/service/job/services/JobImplService.go`

- [ ] **Step 1: Implement `GetJobsByStatus` in `UserJobStateRepositoryService.go`**

Add at the end of the file (after `CountsByUser`):

```go
// GetJobsByStatus returns up to limit jobs for userID filtered by status,
// ordered by (jobs.added_at DESC, jobs.id DESC) — same as the global feed.
// Drives from user_job_state so every page is a full limit-sized result set.
// Cursor format is identical to GetJobs (base64-encoded added_at|id).
func (r *userJobStateRepository) GetJobsByStatus(ctx context.Context, userID, status, cursor string, limit int) (_ []jobmodel.Job, _ string, err error) {
	ctx, span := tracing.Start(ctx, "job.userstate.GetJobsByStatus")
	defer tracing.End(span, &err)

	if userID == "" {
		return nil, "", errNoUser()
	}

	cursorAt, cursorID, err := decodeCursor(cursor)
	if err != nil {
		return nil, "", err
	}

	query := r.db.WithContext(ctx).
		Model(&jobmodel.Job{}).
		Joins("Company").
		Joins("INNER JOIN user_job_state ujs ON ujs.job_id = jobs.id AND ujs.user_id = ? AND ujs.status = ?", userID, status)

	if !cursorAt.IsZero() {
		query = query.Where("(jobs.added_at, jobs.id) < (?, ?)", cursorAt, cursorID)
	}

	var jobs []jobmodel.Job
	if err = query.Order("jobs.added_at DESC, jobs.id DESC").Limit(limit + 1).Find(&jobs).Error; err != nil {
		return nil, "", err
	}

	var next string
	if len(jobs) > limit {
		last := jobs[limit-1]
		next = encodeCursor(last.AddedAt, last.ID)
		jobs = jobs[:limit]
	}
	return jobs, next, nil
}
```

- [ ] **Step 2: Add `listByUserStatus` to `JobImplService.go`**

Add after the existing `List` method (around line 169):

```go
// listByUserStatus serves status tabs other than "new". Drives from
// user_job_state so every page is a full limit-sized result set — the
// previous in-memory status filter on the global feed produced sparse pages.
func (s *implService) listByUserStatus(ctx context.Context, filter dto.JobFilter, cursor string, limit int) (_ dto.PageResult, err error) {
	ctx, span := tracing.Start(ctx, "job.impl.listByUserStatus")
	defer tracing.End(span, &err)

	userID := identity.UserIDFrom(ctx)
	jobs, next, err := s.userState.GetJobsByStatus(ctx, userID, filter.Status, cursor, limit)
	if err != nil {
		return dto.PageResult{}, apperr.ErrInternal.WithMessage("failed to fetch jobs by status")
	}

	views := make([]dto.JobView, len(jobs))
	for i, j := range jobs {
		views[i] = dto.JobView{
			Job:       j,
			UserState: dto.UserJobStateView{Status: filter.Status},
		}
	}
	return dto.PageResult{Items: views, NextCursor: next}, nil
}
```

- [ ] **Step 3: Add routing to `List` in `JobImplService.go`**

At the very top of the `List` method body, before the cache logic, add:

```go
func (s *implService) List(ctx context.Context, filter dto.JobFilter, cursor string, limit int) (_ dto.PageResult, err error) {
	ctx, span := tracing.Start(ctx, "job.impl.List")
	defer tracing.End(span, &err)

	// Non-"new" status tabs drive from user_job_state — the global feed cache
	// is user-agnostic and in-memory status filtering produced sparse pages.
	if filter.Status != "" && filter.Status != "new" {
		return s.listByUserStatus(ctx, filter, cursor, limit)
	}

	userID := identity.UserIDFrom(ctx)
	// ... rest of existing method unchanged
```

- [ ] **Step 4: Verify it compiles**
```bash
cd backend && go build ./service/job/...
```
Expected: clean build.

---

## Task 6: Run tests and lint

- [ ] **Step 1: Run job service tests**
```bash
cd backend && go test ./service/job/... -v 2>&1 | tail -40
```
Expected: all tests pass including the three new ones:
- `TestList_SavedTab_RoutesToGetJobsByStatus` — PASS
- `TestList_NewTab_UsesGlobalFeed` — PASS
- `TestList_AppliedTab_SetsStatusOnView` — PASS

- [ ] **Step 2: Run full backend tests**
```bash
cd backend && go test ./... 2>&1 | grep -E "FAIL|ok" | head -30
```
Expected: all packages `ok`.

- [ ] **Step 3: Run backend linter**
```bash
cd backend && golangci-lint run --config ../.github/.golangci.yml --timeout 5m 2>&1 | tail -20
```
Expected: no issues.

- [ ] **Step 4: Run frontend lint** (no frontend changes but confirm baseline)
```bash
cd ui/console && npm run lint 2>&1 | tail -5
```
Expected: clean.

---

## Task 7: Git commit

- [ ] **Step 1: Stage and commit**
```bash
git add \
  backend/migrations/048_job_query_optimizations.sql \
  backend/service/job/services/JobService.go \
  backend/service/job/services/UserJobStateRepositoryService.go \
  backend/service/job/services/JobImplService.go \
  backend/service/job/tests/JobImplService_test.go \
  docs/superpowers/specs/2026-05-04-job-query-optimization.md \
  docs/superpowers/plans/2026-05-04-job-query-optimization.md
git commit -m "$(cat <<'EOF'
fix(jobs): fix status-tab pagination and add missing DB indexes

Status tabs (saved/applied/dismissed/etc) were fetching global pages and
filtering in-memory — producing sparse or empty pages even when matching
jobs existed. Non-"new" tabs now drive from user_job_state (INNER JOIN
jobs) so every page is a full limit-sized result set.

Migration 048: drops two stale indexes left from the multi-user refactor
(referencing dropped status/saved columns), adds B-tree on
(added_at DESC, id DESC) for keyset cursor support, adds company_id and
user_job_state(user_id, status) indexes.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 8: Redeploy to k3d

- [ ] **Step 1: Verify kubectl context**
```bash
kubectl config current-context
```
Expected: `k3d-jobradar`

- [ ] **Step 2: Build and import backend image**
```bash
make k8s-build-backend
```
Expected: image built and imported into k3d.

- [ ] **Step 3: Run migrations**
```bash
make k8s-migrate
```
Expected: migration 048 runs successfully. Confirm with:
```bash
kubectl logs -n jobradar -l app=migrate --tail=20
```
Look for: `OK   048_job_query_optimizations.sql`

- [ ] **Step 4: Rolling restart API**
```bash
make k8s-restart-api
```

- [ ] **Step 5: Verify API is healthy**
```bash
kubectl get pods -n jobradar -l app=api
```
Expected: pod in `Running` state, `READY 1/1`.

- [ ] **Step 6: Smoke test**
```bash
curl -s http://localhost:30080/api/jobs?status=saved | jq '.data | length'
```
Expected: returns valid JSON (auth required in real usage — confirm no 500).
