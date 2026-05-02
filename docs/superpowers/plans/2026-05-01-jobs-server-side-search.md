# Jobs Server-Side Search Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move the jobs search filter from client-side in-memory filtering to fully server-side SQL, matching on title, location, and company name across all pages, not just the current page.

**Architecture:** Add a `Search` field to the `JobFilter` DTO; the handler reads `?search=` and passes it through the decorator chain; the store applies `(title ILIKE ? OR location ILIKE ? OR companies.name ILIKE ?)` with a conditional LEFT JOIN; the frontend replaces the `searchQuery` state with a debounced value that is included in the TanStack Query key and sent as a query param.

**Tech Stack:** Go/GORM (backend), React/TypeScript/TanStack Query (frontend)

---

## File Map

| File | Change |
|------|--------|
| `backend/service/job/dto/JobFilter.go` | Add `Search string` field |
| `backend/service/job/services/JobRestService.go` | Parse `?search=` query param, set `filter.Search` |
| `backend/service/job/services/JobValidationService.go` | Validate `Search` max length (200 chars) |
| `backend/service/job/services/JobFeedRepository.go` | Add LEFT JOIN companies + `(title ILIKE ? OR location ILIKE ? OR companies.name ILIKE ?)` WHERE clause; qualify ambiguous columns with table prefix |
| `backend/service/job/tests/JobValidationService_test.go` | Add test cases for search validation |
| `backend/service/job/tests/JobRestService_test.go` | Add test that `?search=` is parsed and forwarded |
| `ui/console/src/api/jobs.ts` | Add `search?: string` to `JobFilters`; include in URLSearchParams |
| `ui/console/src/features/jobs/hooks/useJobs.ts` | Replace `searchQuery` + `setSearchQuery` with debounced pattern; include `debouncedSearch` in `filters` useMemo and its dependency array |
| `ui/console/src/pages/JobsPage.tsx` | Remove `displayJobs`/`displayTotal` client-side filter; use `jobs.jobs` and `jobs.total` directly |

---

## Task 1: Add `Search` to `JobFilter` DTO

**Files:**
- Modify: `backend/service/job/dto/JobFilter.go`

- [ ] **Step 1: Add the `Search` field**

Replace the contents of `backend/service/job/dto/JobFilter.go` with:

```go
package dto

// JobFilter holds all filter criteria for listing jobs.
// Every field is optional — zero value means "no filter on this field".
type JobFilter struct {
	Status           string            `json:"status"`    // "new" | "saved" | "applied" | "interviewing" | "offer"
	Companies        []string          `json:"companies"` // filter by company_id (exact match, OR across values)
	Search           string            // ILIKE match on title, location, or company name
	Remote           *bool             // remote = true/false
	OnSite           bool              // remote = false (on-site only)
	EmployTypes      []string          // employ_type IN (...) — OR across values
	Sponsorship      *bool             // sponsorship = true/false
	NoRequirements   bool              // requires_citizen = false AND security_clearance = false
	RequiresCitizen  *bool             // requires_citizen = true/false
	ExperienceRanges []ExperienceRange // numeric range bands — OR across values
	DaysAdded        *int              // added_at >= NOW() - N days
	SortBy           string            // "title" | "company" | "added_at" — defaults to "added_at"
	SortDir          string            // "asc" | "desc" — defaults to "desc"
}
```

- [ ] **Step 2: Verify compilation**

```bash
cd /Users/trushil/Projects/JobRadar/backend && go build ./service/job/...
```

Expected: no output (clean build).

- [ ] **Step 3: Commit**

```bash
cd /Users/trushil/Projects/JobRadar
git add backend/service/job/dto/JobFilter.go
git commit -m "feat(job): add Search field to JobFilter DTO"
```

---

## Task 2: Parse `search` query param in handler

**Files:**
- Modify: `backend/service/job/services/JobRestService.go`

- [ ] **Step 1: Add search param parsing to the `List` handler**

In `JobRestService.go`, inside `func (h *RestService) List(...)`, add the following block immediately after the `sort_dir` block (after line 78, before line 81):

```go
	if v := q.Get("search"); v != "" {
		filter.Search = v
	}
```

The updated handler block from the `sort_by` parse through the service call should look like:

```go
	if v := q.Get("sort_by"); v != "" {
		filter.SortBy = v
	}
	if v := q.Get("sort_dir"); v != "" {
		filter.SortDir = v
	}
	if v := q.Get("search"); v != "" {
		filter.Search = v
	}

	jobs, total, err := h.next.List(r.Context(), filter, p.Page, p.Limit)
```

- [ ] **Step 2: Build to confirm no errors**

```bash
cd /Users/trushil/Projects/JobRadar/backend && go build ./service/job/...
```

Expected: no output.

- [ ] **Step 3: Write the failing test**

In `backend/service/job/tests/JobRestService_test.go`, add the following test after `TestJobHandlerList_ServiceError`:

```go
func TestJobHandlerList_SearchParamForwarded(t *testing.T) {
	var capturedFilter jobdto.JobFilter
	fake := &captureFilterSvc{}
	r := newJobTestRouter(fake)

	req := httptest.NewRequest(http.MethodGet, "/jobs?search=backend+engineer", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want 200", w.Code)
	}
	capturedFilter = fake.lastFilter
	if capturedFilter.Search != "backend engineer" {
		t.Errorf("Search = %q, want %q", capturedFilter.Search, "backend engineer")
	}
}
```

Also add the `captureFilterSvc` test double in the same file:

```go
// captureFilterSvc records the filter passed to List so tests can assert on it.
type captureFilterSvc struct {
	lastFilter jobdto.JobFilter
}

func (f *captureFilterSvc) List(_ context.Context, filter jobdto.JobFilter, _, _ int) ([]jobmodel.Job, int, error) {
	f.lastFilter = filter
	return nil, 0, nil
}
func (f *captureFilterSvc) Get(_ context.Context, _ string) (*jobmodel.Job, error) {
	return nil, nil
}
func (f *captureFilterSvc) GetCounts(_ context.Context) (*jobdto.JobCounts, error) {
	return &jobdto.JobCounts{}, nil
}
func (f *captureFilterSvc) Dismiss(_ context.Context, _ string, _ jobdto.DismissInput) error {
	return nil
}
func (f *captureFilterSvc) SetSaved(_ context.Context, _ string, _ bool) error { return nil }
func (f *captureFilterSvc) SetStatus(_ context.Context, _, _ string) error     { return nil }
func (f *captureFilterSvc) SetNotes(_ context.Context, _, _ string) error      { return nil }
func (f *captureFilterSvc) MarkApplied(_ context.Context, _ string) error      { return nil }

var _ jobsvc.Service = (*captureFilterSvc)(nil)
```

- [ ] **Step 4: Run the test to confirm it fails**

```bash
cd /Users/trushil/Projects/JobRadar/backend && go test ./service/job/tests/... -run TestJobHandlerList_SearchParamForwarded -v
```

Expected: `FAIL` — `Search` field is empty because the handler doesn't parse it yet (it does after Task 2 Step 1, so this confirms the test + implementation work together).

- [ ] **Step 5: Run full REST handler tests to confirm pass**

```bash
cd /Users/trushil/Projects/JobRadar/backend && go test ./service/job/tests/... -run TestJobHandler -v
```

Expected: all `TestJobHandler*` tests PASS.

- [ ] **Step 6: Commit**

```bash
cd /Users/trushil/Projects/JobRadar
git add backend/service/job/services/JobRestService.go backend/service/job/tests/JobRestService_test.go
git commit -m "feat(job): parse ?search= query param and forward to service"
```

---

## Task 3: Validate `search` in ValidationService

**Files:**
- Modify: `backend/service/job/services/JobValidationService.go`
- Modify: `backend/service/job/tests/JobValidationService_test.go`

- [ ] **Step 1: Write failing tests first**

In `backend/service/job/tests/JobValidationService_test.go`, add these test cases to the `tests` slice inside `TestValidateList`:

```go
{name: "search valid", filter: jobdto.JobFilter{Search: "golang engineer"}, page: 1, limit: 20},
{name: "search empty valid", filter: jobdto.JobFilter{Search: ""}, page: 1, limit: 20},
{name: "search exactly 200 chars valid", filter: jobdto.JobFilter{Search: strings.Repeat("a", 200)}, page: 1, limit: 20},
{name: "search 201 chars invalid", filter: jobdto.JobFilter{Search: strings.Repeat("a", 201)}, page: 1, limit: 20, wantErr: true},
```

Add the `strings` import at the top of the test file (it's in the same package, add to imports):

```go
import (
    "context"
    "strings"
    "testing"
    // ... existing imports
)
```

- [ ] **Step 2: Run failing tests to confirm they fail**

```bash
cd /Users/trushil/Projects/JobRadar/backend && go test ./service/job/tests/... -run TestValidateList/search_201 -v
```

Expected: `FAIL` — validator does not yet reject long search strings.

- [ ] **Step 3: Add search validation to the `List` method**

In `JobValidationService.go`, inside `func (v *validationService) List(...)`, add the following block after the `ExperienceRanges` loop (after the closing `}` of the for loop, before the `page < 1` check):

```go
	if len(filter.Search) > 200 {
		errs = append(errs, errors.ValidationError{
			Field:   "search",
			Code:    "INVALID_VALUE",
			Message: "search must be 200 characters or fewer",
		})
	}
```

- [ ] **Step 4: Run all validation tests to confirm they pass**

```bash
cd /Users/trushil/Projects/JobRadar/backend && go test ./service/job/tests/... -run TestValidateList -v
```

Expected: all `TestValidateList/*` cases PASS.

- [ ] **Step 5: Run full job test suite**

```bash
cd /Users/trushil/Projects/JobRadar/backend && go test ./service/job/... -v
```

Expected: all tests PASS.

- [ ] **Step 6: Commit**

```bash
cd /Users/trushil/Projects/JobRadar
git add backend/service/job/services/JobValidationService.go backend/service/job/tests/JobValidationService_test.go
git commit -m "feat(job): validate search length in List filter"
```

---

## Task 4: Apply search WHERE clause in store

**Files:**
- Modify: `backend/service/job/services/JobFeedRepository.go`

- [ ] **Step 1: Replace the `GetJobs` method**

Replace the entire `GetJobs` function (lines 46–143) in `JobFeedRepository.go` with the following. The changes are: (1) add conditional LEFT JOIN companies when search or company sort is used; (2) add search WHERE clause; (3) qualify `job_feed.` prefix on columns that would be ambiguous when the join is active; (4) remove the separate `dataQuery`/`needsCompanyJoin` block at the end — the join is now on the base query.

```go
func (s *jobStore) GetJobs(ctx context.Context, filter dto.JobFilter, page, limit int) (_ []jobmodel.Job, _ int64, err error) {
	ctx, span := tracing.Start(ctx, "job.store.GetJobs")
	defer tracing.End(span, &err)
	query := s.db.WithContext(ctx).Model(&jobmodel.Job{})

	// Join companies when needed for search (company name match) or sorting by company.
	// The FK job_feed.company_id → companies.id guarantees at most one match per row.
	if filter.Search != "" || filter.SortBy == "company" {
		query = query.Joins("LEFT JOIN companies ON companies.id = job_feed.company_id")
	}

	// Status — "saved" is a boolean flag, not a status value
	if filter.Status == jobcommon.JobStatusSaved {
		query = query.Where("saved = ?", true)
	} else if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}

	// Search: case-insensitive match on title, location, or company name
	if filter.Search != "" {
		term := "%" + filter.Search + "%"
		query = query.Where(
			"(job_feed.title ILIKE ? OR job_feed.location ILIKE ? OR companies.name ILIKE ?)",
			term, term, term,
		)
	}

	if len(filter.Companies) > 0 {
		query = query.Where("job_feed.company_id IN ?", filter.Companies)
	}

	if filter.Remote != nil {
		query = query.Where("job_feed.remote = ?", *filter.Remote)
	}

	if filter.OnSite {
		query = query.Where("job_feed.remote = ?", false)
	}

	if len(filter.EmployTypes) > 0 {
		query = query.Where("employ_type IN ?", filter.EmployTypes)
	}

	if filter.Sponsorship != nil {
		query = query.Where("sponsorship = ?", *filter.Sponsorship)
	}

	if filter.NoRequirements {
		query = query.Where("requires_citizen = ? AND security_clearance = ?", false, false)
	}

	if filter.RequiresCitizen != nil {
		query = query.Where("requires_citizen = ?", *filter.RequiresCitizen)
	}

	if len(filter.ExperienceRanges) > 0 {
		var clauses []string
		var args []any
		for _, r := range filter.ExperienceRanges {
			if r.Max != nil {
				if r.Min == 0 {
					clauses = append(clauses, "(experience_years <= ? OR experience_years IS NULL)")
					args = append(args, *r.Max)
				} else {
					clauses = append(clauses, "(experience_years > ? AND experience_years <= ?)")
					args = append(args, r.Min, *r.Max)
				}
			} else {
				clauses = append(clauses, "experience_years > ?")
				args = append(args, r.Min)
			}
		}
		if len(clauses) > 0 {
			query = query.Where("("+strings.Join(clauses, " OR ")+")", args...)
		}
	}

	if filter.DaysAdded != nil {
		query = query.Where("added_at >= NOW() - ? * INTERVAL '1 day'", *filter.DaysAdded)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Whitelist sort columns to prevent SQL injection.
	var col string
	switch filter.SortBy {
	case "title":
		col = "job_feed.title"
	case "company":
		col = "companies.name"
	default:
		col = "job_feed.added_at"
	}
	dir := "DESC"
	if filter.SortDir == "asc" {
		dir = "ASC"
	}

	var jobs []jobmodel.Job
	offset := (page - 1) * limit
	err = query.Order(col + " " + dir).Offset(offset).Limit(limit).Preload("Company").Find(&jobs).Error
	return jobs, total, err
}
```

- [ ] **Step 2: Build to confirm no errors**

```bash
cd /Users/trushil/Projects/JobRadar/backend && go build ./service/job/...
```

Expected: no output (clean build).

- [ ] **Step 3: Run full job test suite**

```bash
cd /Users/trushil/Projects/JobRadar/backend && go test ./service/job/... -v
```

Expected: all tests PASS.

- [ ] **Step 4: Run full backend lint**

```bash
cd /Users/trushil/Projects/JobRadar/backend && golangci-lint run --config ../.github/.golangci.yml --timeout 5m ./service/job/...
```

Expected: no lint errors.

- [ ] **Step 5: Commit**

```bash
cd /Users/trushil/Projects/JobRadar
git add backend/service/job/services/JobFeedRepository.go
git commit -m "feat(job): add server-side search WHERE clause — title, location, company name ILIKE"
```

---

## Task 5: Frontend — Add `search` to API function

**Files:**
- Modify: `ui/console/src/api/jobs.ts`

- [ ] **Step 1: Add `search` to the `JobFilters` interface and `fetchJobs` URLSearchParams**

In `ui/console/src/api/jobs.ts`:

1. Update the `JobFilters` interface — add `search?: string` after the `status` line:

```typescript
export interface JobFilters {
  status: 'new' | 'saved' | 'applied' | 'interviewing' | 'offer'
  search?: string
  companies?: string[]
  remote?: boolean
  onSite?: boolean
  experienceRanges?: string[]
  daysAdded?: number
  sortBy?: string
  sortDir?: 'asc' | 'desc'
}
```

2. Inside `fetchJobs`, add `search` to the URLSearchParams — add after `p.set('limit', String(limit))`:

```typescript
  if (filters.search) p.set('search', filters.search)
  filters.companies?.forEach((c) => p.append('companies', c))
```

The full `fetchJobs` function should now look like:

```typescript
export async function fetchJobs(filters: JobFilters, page = 1, limit = 20): Promise<JobsPage> {
  const p = new URLSearchParams()
  p.set('status', filters.status)
  p.set('page', String(page))
  p.set('limit', String(limit))

  if (filters.search) p.set('search', filters.search)
  filters.companies?.forEach((c) => p.append('companies', c))
  if (filters.remote) p.set('remote', 'true')
  if (filters.onSite) p.set('on_site', 'true')
  filters.experienceRanges?.forEach((r) => p.append('experience_ranges', r))
  if (filters.daysAdded) p.set('days_added', String(filters.daysAdded))
  if (filters.sortBy) p.set('sort_by', filters.sortBy)
  if (filters.sortDir) p.set('sort_dir', filters.sortDir)

  return apiClient.get<JobsPage>(`/jobs?${p}`)
}
```

- [ ] **Step 2: Check TypeScript compiles**

```bash
cd /Users/trushil/Projects/JobRadar/ui/console && npx tsc --noEmit
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
cd /Users/trushil/Projects/JobRadar
git add ui/console/src/api/jobs.ts
git commit -m "feat(jobs): add search param to JobFilters interface and fetchJobs"
```

---

## Task 6: Frontend — Debounce search in `useJobs` hook

**Files:**
- Modify: `ui/console/src/features/jobs/hooks/useJobs.ts`

- [ ] **Step 1: Replace the hook with the debounced version**

Replace the full contents of `ui/console/src/features/jobs/hooks/useJobs.ts` with:

```typescript
import { useState, useMemo, useEffect, useRef } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useLocation } from 'react-router-dom'
import { fetchJobs, fetchJobCounts, type JobFilters } from '@/api/jobs'
import type { SortingState } from '@/components/ui/data-table'

export type TabKey = 'new' | 'saved' | 'applied' | 'interviewing' | 'offer'
export type DateFilter = 'all' | '24h' | '7d' | '30d'

export function useJobs() {
  const location = useLocation()
  const initialDateFilter: DateFilter =
    (location.state as { dateFilter?: DateFilter } | null)?.dateFilter ?? 'all'

  // Tab
  const [tab, setTab] = useState<TabKey>('new')

  // Filter state
  const [selectedCompanies, setSelectedCompanies] = useState<string[]>([])
  const [remoteFilter, setRemoteFilter] = useState(false)
  const [onSiteFilter, setOnSiteFilter] = useState(false)
  const [expFilter, setExpFilter] = useState<string[]>([])
  const [dateFilter, setDateFilter] = useState<DateFilter>(initialDateFilter)

  // Search: raw value drives the input; debouncedSearch drives the API query
  const [searchQuery, setSearchQueryRaw] = useState('')
  const [debouncedSearch, setDebouncedSearch] = useState('')
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  function setSearchQuery(value: string) {
    setSearchQueryRaw(value)
    if (debounceRef.current) clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(() => setDebouncedSearch(value), 300)
  }

  useEffect(() => {
    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current)
    }
  }, [])

  // Sorting
  const [sorting, setSorting] = useState<SortingState>([])

  // Pagination
  const [page, setPage] = useState(1)
  const [limit, setLimit] = useState(20)

  const sortBy = sorting[0]?.id
  const sortDir = sorting[0] ? (sorting[0].desc ? 'desc' : 'asc') : undefined

  // Stable filter object — debouncedSearch prevents a query per keystroke
  const filters = useMemo<JobFilters>(
    () => ({
      status: tab,
      search: debouncedSearch || undefined,
      companies: selectedCompanies.length > 0 ? selectedCompanies : undefined,
      remote: remoteFilter || undefined,
      onSite: onSiteFilter || undefined,
      experienceRanges: expFilter.length > 0 ? expFilter : undefined,
      daysAdded:
        dateFilter === '24h' ? 1 : dateFilter === '7d' ? 7 : dateFilter === '30d' ? 30 : undefined,
      sortBy,
      sortDir,
    }),
    [tab, debouncedSearch, selectedCompanies, remoteFilter, onSiteFilter, expFilter, dateFilter, sortBy, sortDir]
  )

  // Reset to page 1 when filters change
  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setPage(1)
  }, [filters])

  const query = useQuery({
    queryKey: ['jobs', filters, page, limit],
    queryFn: () => fetchJobs(filters, page, limit),
  })

  const countsQuery = useQuery({
    queryKey: ['jobs', 'counts'],
    queryFn: fetchJobCounts,
  })

  const jobs = query.data?.data ?? []
  const total = query.data?.total ?? 0
  const counts: Partial<Record<TabKey, number>> = countsQuery.data
    ? {
        new: countsQuery.data.new,
        saved: countsQuery.data.saved,
        applied: countsQuery.data.applied,
        interviewing: countsQuery.data.interviewing,
        offer: countsQuery.data.offer,
      }
    : {}

  // Companies for the multi-select dropdown — derived from current page results
  const companies = useMemo(() => {
    const names = Array.from(
      new Set(jobs.map((j) => j.company?.name ?? j.company_id).filter(Boolean))
    )
    return (names as string[]).sort()
  }, [jobs])

  function resetFilters() {
    setSearchQueryRaw('')
    setDebouncedSearch('')
    if (debounceRef.current) clearTimeout(debounceRef.current)
    setSelectedCompanies([])
    setRemoteFilter(false)
    setOnSiteFilter(false)
    setExpFilter([])
    setDateFilter('all')
    setSorting([])
    setPage(1)
  }

  function handleSortingChange(newSorting: SortingState) {
    setSorting(newSorting)
    setPage(1)
  }

  return {
    // Query state
    jobs,
    total,
    companies,
    isLoading: query.isLoading,
    refetch: query.refetch,
    counts,

    // Tab
    tab,
    setTab,

    // Search — searchQuery is the raw input value; debouncedSearch drives the API
    searchQuery,
    setSearchQuery,

    // Filters
    selectedCompanies,
    setSelectedCompanies,
    remoteFilter,
    setRemoteFilter,
    onSiteFilter,
    setOnSiteFilter,
    expFilter,
    setExpFilter,
    dateFilter,
    setDateFilter,
    resetFilters,

    // Sorting
    sorting,
    handleSortingChange,

    // Pagination
    page,
    setPage,
    limit,
    setLimit,
  }
}
```

- [ ] **Step 2: Check TypeScript compiles**

```bash
cd /Users/trushil/Projects/JobRadar/ui/console && npx tsc --noEmit
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
cd /Users/trushil/Projects/JobRadar
git add ui/console/src/features/jobs/hooks/useJobs.ts
git commit -m "feat(jobs): debounce search and include in server-side filters"
```

---

## Task 7: Frontend — Remove client-side filtering from `JobsPage`

**Files:**
- Modify: `ui/console/src/pages/JobsPage.tsx`

- [ ] **Step 1: Remove the client-side filter and update dependent references**

In `JobsPage.tsx` make the following changes:

1. Remove lines 52–63 (the `displayJobs` and `displayTotal` computed values):

```typescript
// DELETE these lines:
const displayJobs = jobs.searchQuery.trim()
  ? jobs.jobs.filter((j) => {
      const q = jobs.searchQuery.toLowerCase()
      return (
        j.title.toLowerCase().includes(q) ||
        (j.company?.name ?? '').toLowerCase().includes(q) ||
        (j.location ?? '').toLowerCase().includes(q)
      )
    })
  : jobs.jobs

const displayTotal = jobs.searchQuery.trim() ? displayJobs.length : jobs.total
```

2. Replace every reference to `displayJobs` with `jobs.jobs` and `displayTotal` with `jobs.total`.

3. The `activeFilterCount` on lines 43–50 currently checks `jobs.searchQuery.length > 0`. Keep this — it correctly reflects whether the search input has text, which is what the filter badge should show.

The updated file sections should look like:

```typescript
  const activeFilterCount = [
    jobs.searchQuery.length > 0,
    jobs.selectedCompanies.length > 0,
    jobs.remoteFilter,
    jobs.onSiteFilter,
    jobs.expFilter.length > 0,
    jobs.dateFilter !== 'all',
  ].filter(Boolean).length

  // No displayJobs/displayTotal — server handles all filtering
```

In the JSX, replace `displayJobs` → `jobs.jobs` and `displayTotal` → `jobs.total` in these four locations:

```tsx
// Header count span:
{jobs.total} job{jobs.total !== 1 ? 's' : ''}

// JobsTable props:
jobs={jobs.jobs}
total={jobs.total}

// onSelectAll handler:
setSelectedIds(checked ? new Set(jobs.jobs.map((j) => j.id)) : new Set())
```

- [ ] **Step 2: Check TypeScript compiles**

```bash
cd /Users/trushil/Projects/JobRadar/ui/console && npx tsc --noEmit
```

Expected: no errors.

- [ ] **Step 3: Run lint**

```bash
cd /Users/trushil/Projects/JobRadar/ui/console && npm run lint
```

Expected: no errors.

- [ ] **Step 4: Commit**

```bash
cd /Users/trushil/Projects/JobRadar
git add ui/console/src/pages/JobsPage.tsx
git commit -m "feat(jobs): remove client-side search filter — all filtering is now server-side"
```

---

## Task 8: Final verification

- [ ] **Step 1: Run full backend test suite**

```bash
cd /Users/trushil/Projects/JobRadar/backend && go test ./... -v 2>&1 | tail -30
```

Expected: `ok` for all packages, no `FAIL`.

- [ ] **Step 2: Run full backend lint**

```bash
cd /Users/trushil/Projects/JobRadar/backend && golangci-lint run --config ../.github/.golangci.yml --timeout 5m
```

Expected: no errors.

- [ ] **Step 3: Run frontend TypeScript check**

```bash
cd /Users/trushil/Projects/JobRadar/ui/console && npx tsc --noEmit
```

Expected: no errors.

- [ ] **Step 4: Run frontend lint**

```bash
cd /Users/trushil/Projects/JobRadar/ui/console && npm run lint
```

Expected: no errors.

- [ ] **Step 5: Confirm search flows end-to-end**

Verify manually in the browser (or via curl) that:
- Typing in the search box triggers a network request after 300ms (not on every keystroke)
- The request URL contains `?search=<term>`
- Results update to show only matching jobs across all pages (not just the current page's data)
- Clearing the search box removes the `search` param from the next request
- The filter badge count reflects whether search is active

---

## Self-Review Checklist

- [x] DTO change covered (Task 1)
- [x] Handler param parsing covered (Task 2)
- [x] Validation covered (Task 3)
- [x] Store WHERE clause + JOIN covered (Task 4)
- [x] Frontend API interface + URLSearchParams covered (Task 5)
- [x] Frontend debounce + filters useMemo covered (Task 6)
- [x] Frontend client-side filter removal covered (Task 7)
- [x] Tests added for validation and handler parsing
- [x] No placeholders — all code is complete and runnable
- [x] Type names consistent: `JobFilter` (Go DTO), `JobFilters` (TS interface), `filter.Search` (Go), `filters.search` (TS), `debouncedSearch` (hook state) — all used consistently across tasks
- [x] `resetFilters` clears both `searchQueryRaw` and `debouncedSearch` and cancels the debounce timer
