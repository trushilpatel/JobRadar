# QA Agent Feedback

Status: FINAL REVIEW COMPLETE

Last updated: 2026-04-15

---

## Final Review — All 37 Original Issues

### SECTION 1 — Backend

#### B1-1 [RESOLVED] Migration file

`backend/migrations/023_job_status_interviewing_offer.sql` exists with correct goose Up/Down format.
The constraint name `job_status_check` is correct: migration 010 explicitly named it `job_status_check` on
the `job` table, and migration 012 renamed the table to `job_feed`. Postgres preserves constraint names
through `ALTER TABLE ... RENAME`, so `DROP CONSTRAINT IF EXISTS job_status_check` on `job_feed` is correct.

#### B1-2 [RESOLVED] `model/constants.go` — new constants

`JobStatusInterviewing = "interviewing"` and `JobStatusOffer = "offer"` are present in the `// JobStatus`
block at lines 45–46.

#### B1-3 [RESOLVED] `service/job/validator.go` — new statuses in `validJobStatuses`

Both `model.JobStatusInterviewing` and `model.JobStatusOffer` are in `validJobStatuses` (lines 28–29).
Error messages in both `ValidateList` and `ValidateSetStatus` now say
`"status must be 'new', 'saved', 'applied', 'interviewing', or 'offer'"`.

#### B1-4 [RESOLVED] `GET /jobs/counts` endpoint

All four layers are implemented:

- **store**: `GetJobCounts()` on `JobReader` interface returns `*JobCounts`. Correctly returns a struct,
  not positional returns. Uses GROUP BY query with all 5 statuses; missing statuses default to 0 via
  the switch loop over rows (lines 248–260).
- **service**: `GetCounts() (*JobCounts, error)` on `Service` interface, implemented in service.go.
- **handler**: `GetCounts` handler at lines 91–98. Registered as
  `r.Get("/jobs/counts", h.GetCounts)` at line 28 — critically, this appears BEFORE
  `r.Get("/jobs/{id}", h.Get)` at line 29, so there is no route shadowing.
- **response shape**: uses `handler.Respond(w, http.StatusOK, counts)` which wraps in `{ "data": ... }`.

#### B1-5 [RESOLVED] `on_site` filter

`OnSite bool` is in `JobFilter` struct (types.go line 9). Handler parses `on_site=true`
(handler.go line 68). Store applies `WHERE remote = false` when `filter.OnSite` is true (store.go line 133).
The `bool` type (not `*bool`) is safe here: the zero value `false` simply skips the filter clause.

#### B1-6 [RESOLVED] Tests updated

`validator_test.go`: Tests for `interviewing` and `offer` being accepted by `ValidateList` (lines 22–23)
and `ValidateSetStatus` (lines 71–76). `dismissed` is explicitly tested as rejected in both
`ValidateList` (line 24, `wantErr: true`) and `ValidateSetStatus` (lines 77–79).

---

### SECTION 2 — Frontend Types & API

#### F2-1 [RESOLVED] `src/types/index.ts` — `JobStatus`

`export type JobStatus = 'new' | 'saved' | 'applied' | 'interviewing' | 'offer'` at line 50.

#### F2-2 [RESOLVED] `src/api/jobs.ts` — `JobFilters` updated

- `status` field includes `'interviewing' | 'offer'` (line 13)
- Removed: `employTypes`, `sponsorship`, `noRequirements`, `requiresCitizen`
- Added: `onSite?: boolean` (line 17)
- `fetchJobs` sends `on_site=true` when `filters.onSite` is set (line 42)
- Old params (`employ_types`, `sponsorship`, etc.) are not present

#### F2-3 [RESOLVED] `src/api/jobs.ts` — `fetchJobCounts` function

`JobCounts` interface and `fetchJobCounts()` function present at lines 21–28 and 89–93.
Response unwrapping correctly reads `r.data`.

#### F2-4 [RESOLVED] `src/api/jobs.ts` — `setJobStatus` type

`setJobStatus` signature at line 69 accepts `'new' | 'saved' | 'applied' | 'interviewing' | 'offer'`.

---

### SECTION 3 — Frontend Hooks

#### F3-1 [RESOLVED] `useJobs.ts` — `TabKey`

`export type TabKey = 'new' | 'saved' | 'applied' | 'interviewing' | 'offer'` at line 5.

#### F3-2 [RESOLVED] `useJobs.ts` — old filter state removed

`employTypes`, `sponsorshipFilter`, `noSponsorshipFilter`, `noRequirementsFilter`, `citizenshipReqFilter`
and their setters are absent from the file.

#### F3-3 [RESOLVED] `useJobs.ts` — `onSiteFilter` state

`const [onSiteFilter, setOnSiteFilter] = useState(false)` at line 15.
`onSite: onSiteFilter || undefined` in the `filters` memo at line 31.

#### F3-4 [RESOLVED] `useJobs.ts` — `counts` query

`countsQuery` via `useQuery` with key `['jobs', 'counts']` and `queryFn: fetchJobCounts` at lines 46–49.
`counts` is typed as `Partial<Record<TabKey, number>>` with all 5 keys set from the response,
falling back to `{}` when data is undefined (lines 53–61). This means badges show once data loads.

#### F3-5 [RESOLVED] `useJobs.ts` — `resetFilters` updated

`resetFilters()` at lines 71–79: resets `searchQuery`, `selectedCompanies`, `remoteFilter`,
`onSiteFilter`, `expFilter`, `dateFilter`, `page`. No references to deleted filter states.

#### F3-6 [RESOLVED] `useJobs.ts` — return object updated

Return object exposes `onSiteFilter`, `setOnSiteFilter`, `counts`. No deleted filter states in return.

#### F3-7 [RESOLVED] `useJobActions.ts` — `markInterviewing` and `markOffer`

Both mutations present at lines 37–47. Both call `invalidateJobs()` on success. Both returned at line 59.

---

### SECTION 4 — Frontend Components

#### F4-1 [RESOLVED] `JobStatusTabs.tsx` — 5 tabs

`TABS` array has all 5 entries: new, saved, applied, interviewing, offer (lines 4–10).

#### F4-2 [RESOLVED] `JobStatusTabs.tsx` — count badge rendering

Condition at line 33: `{counts[t.key] != null && (` — renders the badge whenever the key exists in the
`counts` object, which includes 0 counts. When all 5 keys are present (after the countsQuery resolves),
all tabs show their badge including zeros. This matches spec intent.

#### F4-3 [RESOLVED] `JobStatusTabs.tsx` — uses `counts` prop only

The component no longer has a `total` prop. It uses only `counts: Partial<Record<TabKey, number>>`.
All tab badges read from `counts[t.key]` uniformly.

#### F4-4 [RESOLVED] `JobFilters.tsx` — old filter props removed

Interface contains only the required props. No `sponsorshipFilter`, `noSponsorshipFilter`,
`noRequirementsFilter`, `citizenshipReqFilter`, `employTypes`, or their callbacks.
`EMPLOY_TYPE_OPTIONS` constant is absent.

#### F4-5 [RESOLVED] `JobFilters.tsx` — `On-site` pill present

`<FilterPill color="violet" active={onSiteFilter} onClick={() => onOnSiteChange(!onSiteFilter)}>On-site</FilterPill>`
at lines 121–123. `onSiteFilter: boolean` and `onOnSiteChange: (v: boolean) => void` are in the interface.

#### F4-6 [RESOLVED] `JobFilters.tsx` — `Reset` button present

`<button onClick={onReset} className="text-[11px] text-slate-500 hover:text-slate-300 px-2">Reset</button>`
at lines 127–131. `onReset: () => void` is in the interface.

#### F4-7 [RESOLVED] `JobFilters.tsx` — date "Any" label

`DATE_OPTIONS` at lines 58–62 uses `{ value: 'all', label: 'Any' }`. The old `'All'` is gone.

#### F4-8 [RESOLVED] `JobFilters.tsx` — filter bar order matches mockup

Order: Search | Companies | Divider | Exp pills | Divider | Date pills | Divider | Remote | On-site |
Divider | Reset. Matches the spec and HTML mockup exactly.

#### F4-9 [RESOLVED] `JobDetailPanel.tsx` — dismiss and close in top-right

Both `Trash2` (dismiss) and `X` (close) buttons are grouped in a `flex items-center justify-end` div
at the top of the header (lines 35–51), before the avatar/title row. This is functionally equivalent
to `absolute top-3 right-3` from the mockup's intent.

#### F4-10 [RESOLVED] `JobDetailPanel.tsx` — subtitle includes location

Subtitle at line 59: `{companyName} · {job.location ?? 'Location unknown'}`.

#### F4-11 [RESOLVED] `JobDetailPanel.tsx` — badges in header

Badge block (Remote, Salary, EmployType, Experience) is at lines 65–86, inside the header `div`,
before the action buttons row. Body section contains none of these badges.

#### F4-12 [RESOLVED] `JobDetailPanel.tsx` — action button logic for all statuses

- `new`/`saved`: Save button + "Mark applied" button (lines 100–126)
- `applied`: "Mark interviewing" button only (lines 127–134)
- `interviewing`: "Mark offer" button only (lines 135–143)
- `offer`: no advancement buttons (correctly falls through all conditions)
  `onMarkInterviewing` and `onMarkOffer` are in `JobDetailPanelProps` (lines 12–13).

#### F4-13 [RESOLVED] `JobDetailPanel.tsx` — sponsorship/citizenship/clearance removed from body

Body section (lines 147–181) contains only: timestamp, summary, responsibilities, tech stack.
No flag indicators block is present.

#### F4-14 [RESOLVED] `JobDetailPanel.tsx` — notes section removed from body

No `{job.notes && ...}` block exists in the body.

---

### SECTION 5 — JobsPage Wiring

#### F5-1 [RESOLVED] `JobsPage.tsx` — header layout

`py-3` at line 78. Title and count are inline via `flex items-baseline gap-3` at line 79.
Count span at lines 81–85 says "selected filters" (not "active filters").

#### F5-2 [RESOLVED] `JobsPage.tsx` — `activeFilterCount`

Computed at lines 53–60 with exactly the 6 conditions from the spec: searchQuery, selectedCompanies,
remoteFilter, onSiteFilter, expFilter deviation, dateFilter deviation. No deleted filter states.

#### F5-3 [RESOLVED] `JobsPage.tsx` — `JobStatusTabs` receives full counts

`counts={jobs.counts}` at line 100. No `total` prop is passed (and the component no longer accepts one).

#### F5-4 [RESOLVED] `JobsPage.tsx` — `JobFilters` props

All deleted props are absent. New props passed: `onSiteFilter={jobs.onSiteFilter}`,
`onOnSiteChange={jobs.setOnSiteFilter}`, `onReset={jobs.resetFilters}` (lines 111–117).

#### F5-5 [RESOLVED] `JobsPage.tsx` — `handleMarkInterviewing` and `handleMarkOffer`

Both handlers defined at lines 29–37, matching the spec exactly.

#### F5-6 [RESOLVED] `JobsPage.tsx` — `JobDetailPanel` new props wired

`onMarkInterviewing={handleMarkInterviewing}` and `onMarkOffer={handleMarkOffer}` are passed at lines 179–180.

---

## New Issues Found

### [NEW-OPEN] N1 — Dashboard `JobCountReader` interface signature mismatch with `JobCounts` struct

**Location:** `backend/service/dashboard/service.go` lines 23–25 and `backend/cmd/api/wire.go` lines 41–46

The `dashboard.JobCountReader` interface still returns three positional values:

```go
GetJobCounts() (newCount, savedCount, appliedCount int64, err error)
```

The `jobCountReaderAdapter` in `wire.go` (and duplicated in `wire_gen.go`) bridges this by pulling
`.New`, `.Saved`, `.Applied` from `*job.JobCounts` and dropping `Interviewing` and `Offer`.

This is **not a bug that breaks existing functionality** — the dashboard stats view only shows new/saved/applied
counts, and the adapter correctly provides those three. However, it is an **architectural inconsistency**:
the `job.GetJobCounts()` store method now returns a richer struct with 5 counts, but the adapter silently
discards `Interviewing` and `Offer` before handing them to the dashboard. If a future dashboard widget
wants to show interviewing/offer counts it will require refactoring the `JobCountReader` interface.

Severity: **Suggestion** — does not block shipping, but the interface should ideally be updated to
pass the full `*JobCounts` struct to the dashboard service rather than the three-positional-return pattern.

### [NEW-OPEN] N2 — `OnSite bool` in `JobFilter` is a non-pointer, keeping legacy filter fields

**Location:** `backend/service/job/types.go` lines 9–14

The `JobFilter` struct retains `EmployTypes`, `Sponsorship`, `NoRequirements`, and `RequiresCitizen`
fields (lines 11–14) even though the spec removed these from the frontend API. The handler.go also still
parses `sponsorship`, `no_requirements`, and `requires_citizen` query parameters (lines 56–66).

This is not a bug — these filters remain functional on the backend — but it means the backend accepts
parameters that the frontend no longer sends, creating dead code that could confuse future maintainers.

Severity: **Suggestion** — does not block shipping. The backend keeping these params is harmless and
may be intentional for API forward-compatibility.

### [NEW-OPEN] N3 — `JobDetailPanel` dismiss button hidden for `applied`, `interviewing`, `offer` statuses

**Location:** `frontend/src/features/jobs/components/JobDetailPanel.tsx` lines 36–43

The dismiss button (`Trash2`) renders only when
`job.status !== 'applied' && job.status !== 'interviewing' && job.status !== 'offer'`.
This means jobs that have been applied-to, are in interviewing, or have an offer cannot be dismissed
from the detail panel.

The spec (`agent_comms/spec.md`) does not explicitly state whether dismiss should be available for
these statuses. The HTML mockup shows the dismiss button unconditionally in the top-right corner.
However, from a product perspective, allowing dismiss on `applied`/`interviewing`/`offer` jobs may be
intentional or unintentional. This should be confirmed with the product owner.

Severity: **Important** — the mockup shows an unconditional dismiss button. If the intent is to
always show it, lines 36 and 43 (`{job.status !== 'applied' && ... && (` and `)}`) should be removed.

### [NEW-OPEN] N4 — `counts` type mismatch: `Partial<Record<TabKey, number>>` vs backend `int64`

**Location:** `frontend/src/api/jobs.ts` line 22 and `frontend/src/features/jobs/hooks/useJobs.ts` line 53

`JobCounts` in `api/jobs.ts` declares all fields as `number`. The backend `JobCounts` struct uses `int64`
fields which serialize to JSON integers. JavaScript `number` can represent all `int64` values that fit
within `Number.MAX_SAFE_INTEGER` (2^53 - 1). For job counts this is not a practical concern, but the
type is technically correct: `int64` serializes to JSON number, TypeScript `number` reads it fine.

Severity: **No issue** — documented for completeness. No fix required.

### [NEW-OPEN] N5 — `useJobs.ts`: `counts` fallback is `{}` not a zero-filled object; tabs show no badge until counts load

**Location:** `frontend/src/features/jobs/hooks/useJobs.ts` lines 53–61

When `countsQuery.data` is `undefined` (during the initial load), `counts` falls back to `{}`.
`JobStatusTabs` renders `counts[t.key] != null` — since no keys exist in `{}`, no badge spans
are rendered at all until the counts query resolves. This is a momentary flash (tabs show without
badges on first load), not a persistent issue.

The spec says `counts: Partial<Record<TabKey, number>>` which makes `{}` a valid value. However, the
initial QA feedback (F3-4) suggested defaulting to
`{ new: 0, saved: 0, applied: 0, interviewing: 0, offer: 0 }` so badges show immediately as "0".
The dev chose not to default-fill zeros, opting instead for "no badge until loaded."

Severity: **Suggestion** — minor UX difference from the spec's suggested default. Does not block shipping.

---

## Summary

| Section                          | Original Issues | Resolved | Still Open |
| -------------------------------- | --------------- | -------- | ---------- |
| Backend (Section 1)              | 6               | 6        | 0          |
| Frontend Types & API (Section 2) | 4               | 4        | 0          |
| Frontend Hooks (Section 3)       | 7               | 7        | 0          |
| Frontend Components (Section 4)  | 14              | 14       | 0          |
| JobsPage Wiring (Section 5)      | 6               | 6        | 0          |
| **Original Total**               | **37**          | **37**   | **0**      |
| New Issues                       | 5               | —        | 5          |

### New Issue Severities

| ID  | Severity   | Description                                                                       |
| --- | ---------- | --------------------------------------------------------------------------------- |
| N1  | Suggestion | Dashboard JobCountReader drops interviewing/offer counts                          |
| N2  | Suggestion | Backend retains legacy filter params not used by frontend                         |
| N3  | Important  | Dismiss button hidden for applied/interviewing/offer — conflicts with HTML mockup |
| N4  | No issue   | int64/number type note — no fix needed                                            |
| N5  | Suggestion | Counts badge doesn't show on first load (empty fallback vs zero-filled)           |

---

## Final Verdict

**QA BLOCKED** — one issue requires clarification before shipping.

**Blocking issue:**

**N3** — The HTML mockup shows the dismiss (`Trash2`) button unconditionally in the top-right corner
of the detail panel for all job statuses. The implementation hides it for `applied`, `interviewing`,
and `offer`. This is a direct deviation from the mockup. If the intent is to prevent dismissal of
advanced-stage jobs, this needs to be explicitly documented in the spec. If the mockup is authoritative,
the conditional must be removed from `JobDetailPanel.tsx` lines 36–43.

**Resolution options:**

1. Remove the conditional: allow dismiss for all statuses (matches mockup).
2. Update the spec to explicitly state that `applied`, `interviewing`, and `offer` jobs cannot be dismissed
   from the detail panel, then mark N3 as a known, intentional deviation.

All 37 original issues are resolved. The implementation is otherwise high-quality, well-structured,
and consistent with the spec and project conventions.
