# Dev Agent Log

Status: ALL SECTIONS COMPLETE

## Progress

### SECTION 1 COMPLETE: Backend

Files changed:

- `backend/migrations/023_job_status_interviewing_offer.sql` — NEW: adds `interviewing` and `offer` to job_feed CHECK constraint
- `backend/model/constants.go` — added `JobStatusInterviewing` and `JobStatusOffer`
- `backend/service/job/validator.go` — added new statuses to `validJobStatuses`, updated error messages
- `backend/service/job/types.go` — added `OnSite bool` to `JobFilter`, added `JobCounts` type, `CountsResult` type
- `backend/service/job/store.go` — updated `JobReader` interface (new `GetJobCounts()` signature), added `OnSite` filter, replaced `GetJobCounts` impl to use GROUP BY query returning `*JobCounts`
- `backend/service/job/service.go` — added `GetCounts()` to Service interface and implementation
- `backend/service/job/handler.go` — added `GET /jobs/counts` route (before `/{id}`), added `GetCounts` handler, added `on_site` param parsing
- `backend/service/job/validator_test.go` — added test cases for `interviewing` and `offer` statuses
- `backend/service/job/handler_test.go` — updated fakeJobService to implement new `GetCounts()`, added `TestJobHandlerGetCounts_OK`
- `backend/service/job/service_test.go` — updated fakeJobStore `GetJobCounts` signature
- `backend/service/dashboard/service.go` — kept old `JobCountReader` interface (3-return), reverted GetStats to use it
- `backend/cmd/api/wire.go` — added `jobCountReaderAdapter` to bridge new `*JobCounts` to old dashboard interface
- `backend/cmd/api/wire_gen.go` — same adapter added to runtime code

### SECTION 2 COMPLETE: Frontend types/api

Files changed:

- `frontend/src/types/index.ts` — extended `JobStatus` to include `'interviewing' | 'offer'`
- `frontend/src/api/jobs.ts` — extended `JobFilters` (removed `employTypes`, `sponsorship`, `noRequirements`, `requiresCitizen`; added `onSite`), added `JobCounts` interface, added `fetchJobCounts()`, updated `setJobStatus` signature, updated `fetchJobs` params

### SECTION 3 COMPLETE: Hooks

Files changed:

- `frontend/src/features/jobs/hooks/useJobs.ts` — rewrote: new TabKey values, removed old filter state, added `onSiteFilter`, updated `filters` memo, added `countsQuery` using `fetchJobCounts`, added `counts` to return value, updated `resetFilters`
- `frontend/src/features/jobs/hooks/useJobActions.ts` — added `markInterviewing` and `markOffer` mutations, updated return value

### SECTION 4 COMPLETE: Components

Files changed:

- `frontend/src/features/jobs/components/JobStatusTabs.tsx` — 5 tabs (new, saved, applied, interviewing, offer), removed `total` prop, `counts` now covers all tabs
- `frontend/src/features/jobs/components/JobFilters.tsx` — removed sponsorship/citizenship/employType filters, added `onSiteFilter`/`onOnSiteChange`/`onReset` props, date "all" → "Any", On-site pill (violet), Reset text button
- `frontend/src/features/jobs/components/JobDetailPanel.tsx` — layout restructured: dismiss+close top-right, badges moved to header, subtitle with company·location, advancement buttons by status (`new`/`saved`→Mark applied, `applied`→Mark interviewing, `interviewing`→Mark offer), removed sponsorship/citizenship/notes from body
- `frontend/src/pages/JobsPage.tsx` — header redesigned (single-line with filter count), wired new counts/onSite/onReset props, added `handleMarkInterviewing`/`handleMarkOffer` handlers

### ALL SECTIONS COMPLETE

Summary of all changes:

**Backend:**

- `backend/migrations/023_job_status_interviewing_offer.sql` — new migration adding `interviewing`/`offer` to `job_feed` CHECK constraint
- `backend/model/constants.go` — `JobStatusInterviewing`, `JobStatusOffer` added
- `backend/service/job/validator.go` — new statuses in `validJobStatuses`, error messages updated
- `backend/service/job/types.go` — `OnSite bool` in `JobFilter`, `JobCounts` struct, `CountsResult` struct
- `backend/service/job/store.go` — `JobReader` interface updated (`GetJobCounts() *JobCounts, error`), `OnSite` filter in `GetJobs`, new `GetJobCounts` impl using GROUP BY
- `backend/service/job/service.go` — `GetCounts()` in `Service` interface and implementation
- `backend/service/job/handler.go` — `GET /jobs/counts` route (before `/{id}`), `GetCounts` handler, `on_site` param parsing
- `backend/service/job/validator_test.go` — test cases for `interviewing`/`offer`
- `backend/service/job/handler_test.go` — `fakeJobService.GetCounts()`, `TestJobHandlerGetCounts_OK`
- `backend/service/job/service_test.go` — `fakeJobStore.GetJobCounts` signature updated
- `backend/service/dashboard/service.go` — `JobCountReader` interface kept as old 3-return, GetStats unchanged
- `backend/cmd/api/wire.go` — `jobCountReaderAdapter` bridges new `*job.JobCounts` to old dashboard interface
- `backend/cmd/api/wire_gen.go` — same adapter in runtime code

**Frontend:**

- `frontend/src/types/index.ts` — `JobStatus` extended with `'interviewing' | 'offer'`
- `frontend/src/api/jobs.ts` — `JobFilters` simplified, `JobCounts` type, `fetchJobCounts()`, `setJobStatus` signature extended
- `frontend/src/features/jobs/hooks/useJobs.ts` — rewritten: 5 tab keys, removed old filter state, `onSiteFilter`, `countsQuery`, `counts` in return
- `frontend/src/features/jobs/hooks/useJobActions.ts` — `markInterviewing`/`markOffer` mutations
- `frontend/src/features/jobs/components/JobStatusTabs.tsx` — 5 tabs, `total` prop removed, all tabs show count badges
- `frontend/src/features/jobs/components/JobFilters.tsx` — simplified: removed 5 old filters, added `onSiteFilter`/`onOnSiteChange`/`onReset`, date "Any", On-site pill (violet), Reset button
- `frontend/src/features/jobs/components/JobDetailPanel.tsx` — layout restructured: dismiss+close top-right, badges in header, status-based advancement buttons, removed flags/notes from body
- `frontend/src/features/jobs/components/JobsTable.tsx` — `EMPTY_MESSAGES` extended for `interviewing`/`offer`
- `frontend/src/pages/JobsPage.tsx` — header redesigned, `activeFilterCount` updated, wired all new props/handlers

## Questions for QA

(Write questions here, QA will answer)
