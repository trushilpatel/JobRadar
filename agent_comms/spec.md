# Jobs UI Redesign — Spec

Date: 2026-04-15

## Goal

Implement the jobs listing UI redesign to exactly match `Designs/UI/jobs-listing.html`, component-based. Two new job statuses (`interviewing`, `offer`) are added end-to-end.

---

## Backend Changes

### 1. Migration `023_job_status_interviewing_offer.sql`

```sql
-- +goose Up
ALTER TABLE job_feed
  DROP CONSTRAINT IF EXISTS job_status_check,
  ADD CONSTRAINT job_status_check
    CHECK (status IN ('new', 'saved', 'applied', 'interviewing', 'offer', 'dismissed'));

-- +goose Down
ALTER TABLE job_feed
  DROP CONSTRAINT IF EXISTS job_status_check,
  ADD CONSTRAINT job_status_check
    CHECK (status IN ('new', 'saved', 'applied', 'dismissed'));
```

NOTE: Check the actual constraint name in the DB schema/migrations first.

### 2. `model/constants.go`

```go
JobStatusInterviewing = "interviewing"
JobStatusOffer        = "offer"
```

### 3. `service/job/validator.go`

Add `JobStatusInterviewing` and `JobStatusOffer` to `validJobStatuses`. Update error messages.

### 4. New `GET /jobs/counts` endpoint

Response: `{ "data": { "new": N, "saved": N, "applied": N, "interviewing": N, "offer": N } }`
SQL: `SELECT status, COUNT(*) FROM job_feed WHERE status IN ('new','saved','applied','interviewing','offer') GROUP BY status`
Mount BEFORE `GET /jobs/{id}` to avoid route conflict.
Add `GetCounts()` to Service interface, store, handler.

### 5. Backend API: support `on_site=true` query param

When `on_site=true`, filter `remote = false`. Add to `JobFilter` struct and store query.

### 6. Update `service/job/validator.go` for `on_site` param (no-op, just don't reject it)

---

## Frontend Changes

### A. `src/types/index.ts`

```ts
export type JobStatus = 'new' | 'saved' | 'applied' | 'interviewing' | 'offer'
```

### B. `src/api/jobs.ts`

- `JobFilters.status`: add `'interviewing' | 'offer'`
- Remove from JobFilters: `employTypes`, `sponsorship`, `noRequirements`, `requiresCitizen`
- Add to JobFilters: `onSite?: boolean`
- Update `fetchJobs` to send `on_site=true` when `onSite` is set
- Add `fetchJobCounts()`:

```ts
export interface JobCounts {
  new: number
  saved: number
  applied: number
  interviewing: number
  offer: number
}
export async function fetchJobCounts(): Promise<JobCounts> {
  const res = await fetch(`${BASE}/jobs/counts`)
  if (!res.ok) throw new Error('Failed to fetch job counts')
  return res.json().then((r: { data: JobCounts }) => r.data)
}
```

### C. `src/features/jobs/hooks/useJobs.ts`

- `TabKey`: add `'interviewing' | 'offer'`
- Remove state: `employTypes`, `sponsorshipFilter`, `noSponsorshipFilter`, `noRequirementsFilter`, `citizenshipReqFilter`
- Add state: `onSiteFilter` (boolean, default false)
- Update `filters` memo to remove those 4 and add `onSite: onSiteFilter || undefined`
- Add `useQuery` for `fetchJobCounts()` — key `['jobs', 'counts']`; expose as `counts: JobCounts`
- Update `resetFilters()` to reset `onSiteFilter`
- Expose `onSiteFilter`, `setOnSiteFilter`, `counts`

### D. `src/features/jobs/components/JobStatusTabs.tsx`

5 tabs: New, Saved, Applied, Interviewing, Offer
Each tab shows its count badge from `counts[tab]` (not just active tab total).
Props: `counts: Partial<Record<TabKey, number>>` — pass the full counts object.

### E. `src/features/jobs/components/JobFilters.tsx`

**Remove props and pills:** `employTypes`/`onEmployTypesChange`, `sponsorshipFilter`/`onSponsorshipChange`, `noSponsorshipFilter`/`onNoSponsorshipChange`, `noRequirementsFilter`/`onNoRequirementsChange`, `citizenshipReqFilter`/`onCitizenshipReqChange`

**Add props:** `onSiteFilter: boolean`, `onOnSiteChange: (v: boolean) => void`, `onReset: () => void`

**New filter bar order:**
Search | Companies | `<Divider>` | 0–2 yrs / 3–5 yrs / 5+ yrs | `<Divider>` | Any / 7d / 30d | `<Divider>` | Remote / On-site | `<Divider>` | Reset (text button)

- Date label for `'all'` → **"Any"** (not "All")
- `On-site` pill: `FilterPill` with `active={onSiteFilter}` color `violet`
- `Reset`: plain text button, `className="text-[11px] text-slate-500 hover:text-slate-300 px-2"`, calls `onReset`

### F. `src/features/jobs/hooks/useJobActions.ts`

Add:

```ts
const markInterviewing = useMutation({
  mutationFn: (id: string) => setJobStatus(id, 'interviewing'),
  onSuccess: () => {
    toast.success('Marked as interviewing')
    invalidateJobs()
  },
  onError: () => toast.error('Failed to update job'),
})
const markOffer = useMutation({
  mutationFn: (id: string) => setJobStatus(id, 'offer'),
  onSuccess: () => {
    toast.success('Marked as offer')
    invalidateJobs()
  },
  onError: () => toast.error('Failed to update job'),
})
// return { save, apply, markInterviewing, markOffer, dismiss, bulkDismiss }
```

### G. `src/features/jobs/components/JobDetailPanel.tsx` — Layout Redesign

**Header section restructure:**

```
[top-right: Trash2 dismiss btn] [top-right: X close btn]
[Avatar] [Title]
         [Company · Location]
[Remote badge] [Salary badge] [Employ type badge] [Exp badge]
[Open posting] [Save/Saved] [Mark applied / Mark Interviewing / Mark Offer]
```

- Dismiss + Close: both in top-right, absolute or flex, dismiss is `Trash2` icon, styled `hover:text-red-400`
- Subtitle: `{companyName} · {job.location ?? 'Location unknown'}`
- Badges moved from body to here (Remote, Salary, EmployType, Experience)
- For `new`/`saved`: show Save + Mark applied buttons
- For `applied`: show Mark Interviewing button (replaces Mark applied)
- For `interviewing`: show Mark Offer button
- For `offer`: no advancement buttons

**Body section — remove:**

- The badges block (location, remote, salary, employ_type, experience) — moved to header
- The sponsorship/citizenship/clearance flags block
- The notes section

**Body section — keep in order:**

- "Added X ago" timestamp
- Summary (`role_brief ?? llm_summary`)
- Responsibilities list
- Tech stack chips

### H. `src/pages/JobsPage.tsx`

**Header:**

```tsx
<div className="flex items-center justify-between px-6 py-3 border-b border-white/[0.07]">
  <div className="flex items-baseline gap-3">
    <h1 className="text-lg font-semibold text-white">Jobs</h1>
    <span className="text-xs text-slate-400">
      {displayTotal} job{displayTotal !== 1 ? 's' : ''}
      {activeFilterCount > 0 && (
        <> · <span className="text-violet-300">{activeFilterCount} selected filter{activeFilterCount !== 1 ? 's' : ''}</span></>
      )}
    </span>
  </div>
  <button onClick={() => jobs.refetch()} ...>
    <RefreshCw size={14} ... />
  </button>
</div>
```

**`activeFilterCount`:** update to only count the filters that still exist:

```ts
const activeFilterCount = [
  jobs.searchQuery.length > 0,
  jobs.selectedCompanies.length > 0,
  jobs.remoteFilter,
  jobs.onSiteFilter,
  JSON.stringify(jobs.expFilter) !== JSON.stringify(['0-2', '3-5']),
  jobs.dateFilter !== 'all',
].filter(Boolean).length
```

**Wire JobStatusTabs:** pass `counts={jobs.counts}`

**Wire JobFilters:** remove deleted props, add `onSiteFilter`, `onOnSiteChange`, `onReset`

**Wire JobDetailPanel:** add `onMarkInterviewing` and `onMarkOffer` handlers + pass to panel

**Add handlers:**

```ts
function handleMarkInterviewing(job: Job) {
  actions.markInterviewing.mutate(job.id)
  if (activeJob?.id === job.id) setActiveJob(null)
}
function handleMarkOffer(job: Job) {
  actions.markOffer.mutate(job.id)
  if (activeJob?.id === job.id) setActiveJob(null)
}
```

---

## Communication Protocol

- Dev agent writes progress to: `agent_comms/dev_log.md`
- QA agent writes feedback to: `agent_comms/qa_feedback.md`
- Check each other's files before starting each new section
- QA marks issues as [OPEN] or [RESOLVED]
- Dev marks addressed items as [ADDRESSED] in dev_log

## Key Rules

- All DB access through `store/` interfaces — no raw GORM in handlers
- All typed values use constants from `model/constants.go`
- No auto-commit (never run `git commit`)
- New migration: `backend/migrations/023_job_status_interviewing_offer.sql`
- Read every file before editing it
