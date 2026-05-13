# Jobs API audit (2026-05-12)

Full production-grade audit of `service/job/` after a user report that the
search endpoint was returning no data. The audit added edge-case tests across
all four layers of the slice and uncovered a Postgres aliasing bug that broke
search, company filtering, and sort-by-company for every authenticated user.

## Bug fixed

GORM's `.Joins("Company")` aliases the joined `companies` table as `"Company"`
in the emitted SQL. Every WHERE/ORDER reference written as `companies.name`
(plain) caused Postgres to error with `invalid reference to FROM-clause entry
for table "companies"`, which the impl service mapped to `ErrInternal` (500).
The frontend rendered this as "no data" on the affected paths.

Affected paths (all in the jobs slice):

- Search filter — `(jobs.title ILIKE … OR companies.name ILIKE …)`
- Companies filter — `companies.name IN (…)`
- Sort by company — `ORDER BY companies.name …`
- Cursor predicate for company sort — second-page boundary

All references switched to `"Company".name` to match the alias GORM produces.

## Files touched (fix)

- `backend/service/job/services/JobRepositoryService.go` — search clause,
  companies filter, sort clause (`sortOrderClause`), cursor predicate
  (`applyCursorPredicate` company branch).
- `backend/service/job/services/UserJobStateRepositoryService.go` — search
  clause and companies filter in `GetJobsByStatus` (drives saved/applied/
  dismissed/interviewing/offer/rejected tabs).

`GetDistinctCompanyNames` was already correct — it uses an explicit
`Joins("JOIN companies ON …")` that doesn't alias.

## Files touched (tests)

- `backend/service/job/services/handler_internal_test.go` (new) — internal
  test for `parseExperienceRange`, covering `+`, closed ranges, malformed,
  negative bounds, whitespace.
- `backend/service/job/services/cursor_internal_test.go` (extended) —
  round-trip per sort column, `buildCursor` for each sort, NULL
  experience_years anchor, `sortOrderClause` whitelist and default direction.
- `backend/service/job/services/sqlprobe_internal_test.go` (new) — regression
  guards for the alias bug. Uses GORM DryRun + a capturing logger to inspect
  the SQL emitted by `GetJobs`, `GetJobsByStatus`, and the company-sort
  cursor predicate; fails if any `companies.name` reference returns.
- `backend/service/job/tests/JobRestService_test.go` (extended) — full
  query-param parsing across every filter, default values, limit boundary,
  sort whitelist, body-decoding edge cases for save/status/notes/dismiss,
  validation/notfound/service-error mapping.
- `backend/service/job/tests/JobValidationService_test.go` (extended) —
  limit and search-length boundaries, experience-range invariants, all
  writable status values, dismissal reason whitelist, bulk-dismiss size
  boundaries.
- `backend/service/job/tests/JobImplService_test.go` (extended) — status
  routing across all status values, sparse-page behaviour on the new tab,
  cache-key scoping (status stripped; shared across users; refetch on
  filter change; status tabs bypass cache), mutation cache invalidation
  per method, missing-user-id rejection on every mutating method, SetNotes
  status preservation, GetCounts derivation and underflow guard,
  BulkDismiss empty/boundary, Dismiss not-found, anonymous reads.

## Test-only dependency

`gorm.io/driver/sqlite` and its sqlite3 driver are now direct test
dependencies in `backend/go.mod` (used only by the SQL alias regression
checks, never linked into a production binary).

## Out of scope

No frontend changes — the contract is unchanged. No integration tests against
real Postgres were added; the alias bug was reproduced once in a throwaway
Docker container during the audit, then locked in via the DryRun regression
tests. A future migration to `dockertest` / `testcontainers-go` would let us
catch this class of bug without a manual probe.
