# Issues Fix Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix three independent issues: add 'rejected' job status, fix Greenhouse career page URL, and add keyboard navigation to DataTable.

**Architecture:** Three fully independent change groups. Issue 1 touches DB → backend constants → validation → DTO → frontend types → UI. Issue 2 is a one-line backend fix + test update. Issue 3 is a frontend-only DataTable enhancement.

**Tech Stack:** Go (goose migrations, GORM), React + TypeScript, TanStack Table, Radix UI, Tailwind CSS

---

## Issue 1 — "Rejected" Status

### Task 1: DB migration — expand user_job_state.status constraint

**Files:**
- Create: `backend/migrations/045_add_interviewing_offer_rejected_statuses.sql`

- [ ] **Step 1: Write the migration**

```sql
-- +goose Up
-- Expand the user_job_state.status CHECK constraint to include
-- 'interviewing', 'offer', and 'rejected'. The constraint in migration 044
-- only allowed ('saved', 'applied', 'dismissed'); 'interviewing' and 'offer'
-- were in the frontend but missing from the DB, making those status writes fail.
ALTER TABLE user_job_state
    DROP CONSTRAINT IF EXISTS user_job_state_status_check;

ALTER TABLE user_job_state
    ADD CONSTRAINT user_job_state_status_check
    CHECK (status IN ('saved', 'applied', 'dismissed', 'interviewing', 'offer', 'rejected'));

-- +goose Down
ALTER TABLE user_job_state
    DROP CONSTRAINT IF EXISTS user_job_state_status_check;

ALTER TABLE user_job_state
    ADD CONSTRAINT user_job_state_status_check
    CHECK (status IN ('saved', 'applied', 'dismissed'));
```

- [ ] **Step 2: Run migration**

```bash
cd backend && go run ./apps/migrate
```

Expected: `goose: successfully migrated database to version: 45`

- [ ] **Step 3: Commit**

```bash
git add backend/migrations/045_add_interviewing_offer_rejected_statuses.sql
git commit -m "feat(db): expand user_job_state.status to include interviewing, offer, rejected"
```

---

### Task 2: Backend — constants and model updates

**Files:**
- Modify: `backend/service/job/models/UserJobState.go`
- Modify: `backend/service/job/common/JobServiceConstants.go`

- [ ] **Step 1: Add model constants**

In `backend/service/job/models/UserJobState.go`, replace the `const` block:

```go
// Per-user job status values — mirror the user_job_state.status CHECK
// constraint. Absence of a row implies "new"; "new" is intentionally NOT a
// valid stored status.
const (
	StatusSaved        = "saved"
	StatusApplied      = "applied"
	StatusDismissed    = "dismissed"
	StatusInterviewing = "interviewing"
	StatusOffer        = "offer"
	StatusRejected     = "rejected"
)
```

- [ ] **Step 2: Add common constants**

In `backend/service/job/common/JobServiceConstants.go`, replace the first `const` block:

```go
const (
	JobStatusNew         = "new" // synthetic — never written, only returned via overlay
	JobStatusSaved       = "saved"
	JobStatusApplied     = "applied"
	JobStatusDismissed   = "dismissed"
	JobStatusInterviewing = "interviewing"
	JobStatusOffer       = "offer"
	JobStatusRejected    = "rejected"
)
```

- [ ] **Step 3: Build to confirm no compile errors**

```bash
cd backend && go build ./...
```

Expected: exits 0 with no output.

- [ ] **Step 4: Commit**

```bash
git add backend/service/job/models/UserJobState.go backend/service/job/common/JobServiceConstants.go
git commit -m "feat(job): add interviewing, offer, rejected status constants"
```

---

### Task 3: Backend — validation service update

**Files:**
- Modify: `backend/service/job/services/JobValidationService.go`
- Modify: `backend/service/job/tests/JobValidationService_test.go`

- [ ] **Step 1: Update validJobStatuses map**

In `backend/service/job/services/JobValidationService.go`, replace the `validJobStatuses` var:

```go
var validJobStatuses = map[string]bool{
	jobcommon.JobStatusNew:          true,
	jobcommon.JobStatusSaved:        true,
	jobcommon.JobStatusApplied:      true,
	jobcommon.JobStatusDismissed:    true,
	jobcommon.JobStatusInterviewing: true,
	jobcommon.JobStatusOffer:        true,
	jobcommon.JobStatusRejected:     true,
}
```

- [ ] **Step 2: Update validation error messages**

In `backend/service/job/services/JobValidationService.go`, update the two error `Message` strings that list valid statuses:

In `List` (around line 46):
```go
Message: "status must be one of: new, saved, applied, interviewing, offer, dismissed, rejected",
```

In `SetStatus` (around line 104):
```go
Message: "status must be one of: saved, applied, interviewing, offer, dismissed, rejected",
```

- [ ] **Step 3: Update the validation test**

In `backend/service/job/tests/JobValidationService_test.go`, replace the `"invalid status"` test case (line 51):

```go
{name: "invalid status", filter: jobdto.JobFilter{Status: "pending"}, limit: 20, wantErr: true},
```

Add a new valid status test case after `"valid saved status"`:

```go
{name: "valid interviewing status", filter: jobdto.JobFilter{Status: jobcommon.JobStatusInterviewing}, limit: 20},
{name: "valid offer status", filter: jobdto.JobFilter{Status: jobcommon.JobStatusOffer}, limit: 20},
{name: "valid rejected status", filter: jobdto.JobFilter{Status: jobcommon.JobStatusRejected}, limit: 20},
```

- [ ] **Step 4: Run tests**

```bash
cd backend && go test ./service/job/tests/... -run TestValidate -v
```

Expected: All `TestValidate_*` tests PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/service/job/services/JobValidationService.go backend/service/job/tests/JobValidationService_test.go
git commit -m "feat(job): allow interviewing, offer, rejected in status validation"
```

---

### Task 4: Backend — Counts DTO and CountsByUser

**Files:**
- Modify: `backend/service/job/dto/Counts.go`
- Modify: `backend/service/job/services/UserJobStateRepositoryService.go`

- [ ] **Step 1: Expand Counts DTO**

Replace `backend/service/job/dto/Counts.go`:

```go
package dto

// Counts holds the per-user count of jobs in each user_job_state status
// bucket. "New" is computed elsewhere (it is the absence of a row), so it
// is not a stored count — callers wanting "new" derive it from total jobs
// minus the sum of these stored statuses.
type Counts struct {
	Saved        int64 `json:"saved"`
	Applied      int64 `json:"applied"`
	Dismissed    int64 `json:"dismissed"`
	Interviewing int64 `json:"interviewing"`
	Offer        int64 `json:"offer"`
	Rejected     int64 `json:"rejected"`
}

// CountsRow is the raw GROUP BY status row returned by the per-user count
// query — internal to the repository.
type CountsRow struct {
	Status string
	Count  int64
}
```

- [ ] **Step 2: Update CountsByUser switch**

In `backend/service/job/services/UserJobStateRepositoryService.go`, update the `CountsByUser` switch (around line 215):

```go
for _, r := range rows {
    switch r.Status {
    case jobmodel.StatusSaved:
        out.Saved = r.Count
    case jobmodel.StatusApplied:
        out.Applied = r.Count
    case jobmodel.StatusDismissed:
        out.Dismissed = r.Count
    case jobmodel.StatusInterviewing:
        out.Interviewing = r.Count
    case jobmodel.StatusOffer:
        out.Offer = r.Count
    case jobmodel.StatusRejected:
        out.Rejected = r.Count
    }
}
```

- [ ] **Step 3: Build and run all job tests**

```bash
cd backend && go build ./... && go test ./service/job/... -v 2>&1 | tail -30
```

Expected: all tests PASS, no compile errors.

- [ ] **Step 4: Commit**

```bash
git add backend/service/job/dto/Counts.go backend/service/job/services/UserJobStateRepositoryService.go
git commit -m "feat(job): add interviewing/offer/rejected to Counts DTO and CountsByUser"
```

---

### Task 5: Frontend — type and API updates

**Files:**
- Modify: `ui/console/src/types/index.ts`
- Modify: `ui/console/src/api/jobs.ts`

- [ ] **Step 1: Expand JobStatus type**

In `ui/console/src/types/index.ts`, replace line 66:

```typescript
export type JobStatus = 'new' | 'saved' | 'applied' | 'interviewing' | 'offer' | 'rejected'
```

- [ ] **Step 2: Expand JobFilters.status and JobCounts**

In `ui/console/src/api/jobs.ts`, replace the `JobFilters.status` field (line 10):

```typescript
  status: 'new' | 'saved' | 'applied' | 'interviewing' | 'offer' | 'rejected'
```

Replace the `JobCounts` interface (lines 21-27):

```typescript
export interface JobCounts {
  new: number
  saved: number
  applied: number
  interviewing: number
  offer: number
  rejected: number
}
```

- [ ] **Step 3: Lint check**

```bash
cd ui/console && npm run lint 2>&1 | tail -20
```

Expected: no errors from the modified files.

- [ ] **Step 4: Commit**

```bash
git add ui/console/src/types/index.ts ui/console/src/api/jobs.ts
git commit -m "feat(frontend): add rejected to JobStatus type and JobCounts"
```

---

### Task 6: Frontend — StatusSelect and JobStatusTabs

**Files:**
- Modify: `ui/console/src/features/jobs/components/StatusSelect.tsx`
- Modify: `ui/console/src/features/jobs/components/JobStatusTabs.tsx`
- Modify: `ui/console/src/features/jobs/hooks/useJobs.ts`

- [ ] **Step 1: Add rejected to StatusSelect**

In `ui/console/src/features/jobs/components/StatusSelect.tsx`:

Add `rejected` to `STATUS_CONFIG` (after `offer`):

```typescript
  rejected: {
    label: 'Rejected',
    classes: 'text-red-300 bg-red-500/10 border-red-500/20',
  },
```

Replace `ALL_STATUSES` (line 29):

```typescript
const ALL_STATUSES: JobStatus[] = ['new', 'saved', 'applied', 'interviewing', 'offer', 'rejected']
```

- [ ] **Step 2: Add Rejected tab to JobStatusTabs**

In `ui/console/src/features/jobs/components/JobStatusTabs.tsx`, replace the `TABS` array:

```typescript
const TABS: { key: TabKey; label: string }[] = [
  { key: 'new', label: 'New' },
  { key: 'saved', label: 'Saved' },
  { key: 'applied', label: 'Applied' },
  { key: 'interviewing', label: 'Interviewing' },
  { key: 'offer', label: 'Offer' },
  { key: 'rejected', label: 'Rejected' },
]
```

- [ ] **Step 3: Expand TabKey and counts in useJobs**

In `ui/console/src/features/jobs/hooks/useJobs.ts`, replace line 7:

```typescript
export type TabKey = 'new' | 'saved' | 'applied' | 'interviewing' | 'offer' | 'rejected'
```

In the `counts` object (lines 96-104), add `rejected`:

```typescript
  const counts: Partial<Record<TabKey, number>> = countsQuery.data
    ? {
        new: countsQuery.data.new,
        saved: countsQuery.data.saved,
        applied: countsQuery.data.applied,
        interviewing: countsQuery.data.interviewing,
        offer: countsQuery.data.offer,
        rejected: countsQuery.data.rejected,
      }
    : {}
```

- [ ] **Step 4: Lint check**

```bash
cd ui/console && npm run lint 2>&1 | tail -20
```

Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add ui/console/src/features/jobs/components/StatusSelect.tsx \
        ui/console/src/features/jobs/components/JobStatusTabs.tsx \
        ui/console/src/features/jobs/hooks/useJobs.ts
git commit -m "feat(frontend): add Rejected tab and status option to job status UI"
```

---

## Issue 2 — Greenhouse Career Page URL

### Task 7: Fix CareersURL and update test

**Files:**
- Modify: `backend/apps/crawler/ats/greenhouse/greenhouse.go`
- Modify: `backend/apps/crawler/ats/greenhouse/greenhouse_test.go`

- [ ] **Step 1: Fix CareersURL method**

In `backend/apps/crawler/ats/greenhouse/greenhouse.go`, replace lines 40-43:

```go
// CareersURL returns the public Greenhouse job board URL for a given company slug.
// This is the human-readable page shown to users, not the internal API endpoint.
func (g *Greenhouse) CareersURL(id string) string {
	return "https://job-boards.greenhouse.io/" + id
}
```

- [ ] **Step 2: Update the test**

In `backend/apps/crawler/ats/greenhouse/greenhouse_test.go`, replace lines 120-127:

```go
func TestGreenhouse_CareersURL(t *testing.T) {
	g := &Greenhouse{}
	got := g.CareersURL("stripe")
	want := "https://job-boards.greenhouse.io/stripe"
	if got != want {
		t.Errorf("CareersURL(%q) = %q, want %q", "stripe", got, want)
	}
}
```

- [ ] **Step 3: Run test**

```bash
cd backend && go test ./apps/crawler/ats/greenhouse/... -run TestGreenhouse_CareersURL -v
```

Expected: `PASS`

- [ ] **Step 4: Run all greenhouse tests**

```bash
cd backend && go test ./apps/crawler/ats/greenhouse/... -v 2>&1 | tail -20
```

Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/apps/crawler/ats/greenhouse/greenhouse.go backend/apps/crawler/ats/greenhouse/greenhouse_test.go
git commit -m "fix(greenhouse): CareersURL returns public job board page instead of API endpoint"
```

---

## Issue 3 — Keyboard Navigation for DataTable

### Task 8: Add keyboard navigation to DataTable

**Files:**
- Modify: `ui/console/src/components/ui/data-table.tsx`

- [ ] **Step 1: Add useRef and useEffect imports**

In `ui/console/src/components/ui/data-table.tsx`, change the React import on line 1:

```typescript
import { useState, useRef, useEffect } from 'react'
```

- [ ] **Step 2: Add rowRefs, scroll effect, and keyDown handler inside the DataTable component**

Inside the `DataTable` component function, after the `table` variable is declared (after line 97), add:

```typescript
  // Refs for each rendered row — used to scroll the active row into view.
  const rowRefs = useRef<(HTMLTableRowElement | null)[]>([])

  // Scroll the active row into view whenever selectedId changes (keyboard nav).
  useEffect(() => {
    if (!selectedId || !getRowId) return
    const rows = table.getRowModel().rows
    const idx = rows.findIndex((r) => getRowId(r.original) === selectedId)
    if (idx >= 0) rowRefs.current[idx]?.scrollIntoView({ block: 'nearest' })
  }, [selectedId]) // eslint-disable-line react-hooks/exhaustive-deps

  const handleKeyDown = (e: React.KeyboardEvent<HTMLDivElement>) => {
    if (!onRowClick) return
    const rows = table.getRowModel().rows
    if (rows.length === 0) return

    const currentIdx =
      selectedId && getRowId
        ? rows.findIndex((r) => getRowId(r.original) === selectedId)
        : -1

    if (e.key === 'ArrowDown') {
      e.preventDefault()
      const next = Math.min(currentIdx + 1, rows.length - 1)
      onRowClick(rows[next].original)
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      const prev = Math.max(currentIdx - 1, 0)
      onRowClick(rows[prev].original)
    }
  }
```

- [ ] **Step 3: Make the container div focusable with keyboard handler**

In the return statement, replace the outer `<div className="flex flex-col">` (line 112):

```typescript
  return (
    <div
      className="flex flex-col outline-none"
      tabIndex={onRowClick ? 0 : undefined}
      onKeyDown={handleKeyDown}
    >
```

- [ ] **Step 4: Attach row refs to TableRow**

In the `table.getRowModel().rows.map()` callback, change the map signature to include `index` and add a `ref` to TableRow:

Replace the `.map((row) => {` (line 131) with `.map((row, index) => {`

And replace the `<TableRow` opening tag (lines 139-142) with:

```typescript
              <TableRow
                key={row.id}
                ref={(el) => {
                  rowRefs.current[index] = el
                }}
                onClick={() => onRowClick?.(row.original)}
                className={cn(onRowClick && 'cursor-pointer', rowBg)}
              >
```

- [ ] **Step 5: Lint check**

```bash
cd ui/console && npm run lint 2>&1 | tail -20
```

Expected: no errors.

- [ ] **Step 6: Commit**

```bash
git add ui/console/src/components/ui/data-table.tsx
git commit -m "feat(ui): add arrow-key keyboard navigation to DataTable"
```

---

## Final verification

- [ ] **Run all backend tests**

```bash
cd backend && go test ./... 2>&1 | grep -E "FAIL|ok" | tail -20
```

Expected: all packages show `ok`, no `FAIL`.

- [ ] **Run frontend lint**

```bash
cd ui/console && npm run lint 2>&1 | tail -10
```

Expected: no errors.
