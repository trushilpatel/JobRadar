# e2e/ — Playwright end-to-end suite

Full-flow browser tests against a live stack (Vite :5173 + Go API :8080). Gated in
CI on every dev→main PR; run locally via `npm run test:e2e` any time.

## Running locally

1. Make sure `make dev` is already running (this suite does not manage it).
2. `npm run test:e2e` — full suite, Chromium only, both roles.
3. `npm run test:e2e:ui` — Playwright's interactive UI mode.
4. `npm run test:e2e:report` — open the last HTML report.

Everything runs through `doppler run --config dev --` so `TEST_ROLE_*` credentials and
`CLERK_SECRET_KEY` are present in `process.env`.

## Running in CI

The `e2e` job in `.github/workflows/ci.yml` runs on every PR/push to `main`: it runs
migrations, starts the API and Vite dev server as background processes against the
same shared dev Supabase DB and Clerk instance local runs use, waits for both to
respond, installs Chromium, then runs `npm run test:e2e`. The job's `concurrency`
group has no `cancel-in-progress` — a second run queues behind the first instead of
being cancelled, so two PRs' E2E runs can never write to the shared `TEST_ROLE_*`
accounts at the same time. Playwright's HTML report is uploaded as an artifact on
every run (`if: always()`) for debugging a CI-only failure.

**Required one-time setup (not something an agent can do — needs a human in the
Doppler + GitHub UI):** the job needs a `DOPPLER_TOKEN` repo secret to fetch real
credentials non-interactively.

1. In the Doppler dashboard: `job-radar` project → `dev` config → Access → Service
   Tokens → create a new **read-only** token scoped to that one config.
2. Add it as a GitHub Actions secret: `gh secret set DOPPLER_TOKEN` (paste the token
   when prompted), or via the repo's Settings → Secrets and variables → Actions UI.

Without that secret, the `e2e` job's Doppler-dependent steps fail immediately with
an auth error — the other CI jobs (lint/test) don't need it and are unaffected.

**Dry-run the CI job locally before trusting it in Actions:**
`scripts/ci-e2e-local.sh` runs the exact same sequence the `e2e` job does (migrate →
start API → start Vite → wait for both → install Chromium → `npm run test:e2e`),
using your own `doppler login` session instead of `DOPPLER_TOKEN`. It kills and
restarts whatever's on :8080/:5173 first — `make dev` again afterward to get your
normal dev stack back.

## Structure

- `playwright.config.ts` — two authenticated projects, `chromium-user` (testDir
  `specs/user`) and `chromium-admin` (testDir `specs/admin`), each depending on the
  `setup` project (`fixtures/auth.setup.ts`) which logs in via Clerk's real embedded
  UI and saves `storageState` to `.auth/user.json` / `.auth/admin.json` (gitignored).
- `pages/` — Page Object Model, one class per page (`pages/admin/` for `/admin/*`).
- `specs/user/`, `specs/admin/` — spec files, matching whichever project owns that route.
- `fixtures/disposable-clerk-user.ts` — creates/deletes throwaway Clerk identities
  (via Clerk's Backend API) for tests that need to exercise a genuinely destructive
  flow (self-delete account, admin-delete user) without ever touching the shared
  `TEST_ROLE_USER_EMAIL` / `TEST_ROLE_ADMIN_EMAIL` accounts every other spec depends on.

## Conventions

- **Auth is already handled.** Specs run pre-authenticated via the project's
  `storageState` — never write sign-in code for the project's own role, just
  `page.goto('/whatever')`.
- **Fresh context ≠ signed out.** A project's `use.storageState` is silently applied
  as the default for _any_ `browser.newContext()` call inside a test belonging to
  that project — not just the framework's own `page` fixture. A disposable-identity
  test MUST pass `{ storageState: undefined }` explicitly, or it silently inherits
  the project's real session (see `account.spec.ts`, `admin-users.spec.ts`).
- **Locators: accessible name first.** Only `JobsPage` has real `data-testid` hooks
  (`jobs-table-sentinel`, `dismiss-stripe`, `job-tab-count`, ...). Every other page
  has none — use `getByRole` / `getByLabel` / `getByText` / `getByPlaceholder` /
  `getByTitle` against real aria-labels, button text, and `title` attributes read
  from the actual component source. Do not add `data-testid` to app source for a
  test's convenience.
- **Data safety.** Never perform an irreversible action (delete, permanently remove)
  against real pre-existing dev data or the shared `TEST_ROLE_*` accounts. Every
  destructive action must act only on a record the test itself created (a company it
  added, a resume it uploaded, a disposable Clerk user it created). Reversible
  actions (save/unsave, status change, suspend/reinstate) may act on live dev data,
  restoring original state at the end of the test.
- **Debounced filters need a real wait, not a row-count proxy.** A `rows().count() >
0` poll right after `.fill()` on a debounced filter input is trivially satisfied by
  the _pre-filter_ list (which already has rows > 0) before the debounce + refetch
  lands — the race passes intermittently on stale data. Poll on the actual narrowed
  content instead (e.g. every visible cell already matches the filter value); see
  `admin-audit.spec.ts`.
- **Mutating a live pre-existing row needs `try`/`finally`, not sequential revert
  statements.** If an assertion between the mutating action and its revert throws,
  a bare sequential revert never runs and the real row is left changed. Wrap the
  mutate + assert in `try`, the revert + its assertion in `finally`, and make the
  revert idempotent (re-read current state; only act if it still differs from
  original) — see `jobs.spec.ts`.

## Operational constraint — never run two invocations concurrently

`account.spec.ts`'s "edit display name" test mutates a genuinely shared, singleton
field (`TEST_ROLE_USER`'s `display_name`) with no per-run isolation — there's only
one shared account, so there's nothing to seed a disposable copy of. A single clean
`npm run test:e2e` invocation is reliable (verified across multiple full-suite runs).
But launching a second invocation before the first has fully exited — e.g. running
it locally while a CI job is also running it, or firing off back-to-back manual runs
without waiting for full process teardown — lets both write a different random temp
value to the same field and read back whichever write landed last, producing a
confusing "expected X, got Y" failure that looks like a timing flake but is actually
a concurrent-invocation collision. This is an operational constraint, not a bug to
fix in the test itself: never run this suite twice concurrently against the same
dev environment.

## Known, accepted gaps

- **`/jobs/:id` (mobile detail route) is out of scope.** The browser matrix is
  desktop Chromium only (a deliberate scope decision); this route only renders on
  mobile viewports.
- **Admin Pipeline retry-tier click path only runs when a tier's count happens to be
  0** on the day the suite runs (never fires against a real nonzero-count tier, by
  design). If every tier has a nonzero count, `admin-pipeline.spec.ts` only asserts
  button presence/enabled-state and annotates the skip — not a failure.

## Real bugs this suite found — all fixed

- **Admin Audit Log was broken for every admin, on real dev data, 100% of the time:**
  `actor_email` in `frontend/src/features/auth/api/adminAudit.api.ts` was
  `z.string().nullable()`, but the Go model tags it `json:"actor_email,omitempty"` —
  a nil pointer omits the key entirely rather than emitting `null`, which Zod's
  `.nullable()` rejects. Changed to `.nullish()`; covered by a new case in
  `adminAudit.api.test.ts`.
- **Undo was non-functional for every dismiss/status-change** — two independent bugs
  stacked:
  1. `frontend/src/features/jobs/hooks/useJobActions.ts`'s `lookupStatus`/`lookupSaved`
     scanned the query cache for a `{ pages: [...] }` shape; the real jobs-list cache
     (`useJobs.ts`) stores a flat `{ items, next_cursor }` with no `.pages` field, so
     the lookup always fell back to `status: 'new'`. Fixed to scan the real shape.
  2. `JobValidationService.SetStatus` (backend) rejected `status: 'new'` outright,
     even though `UserJobStateRepositoryService.Upsert` already correctly implemented
     "new clears the workflow status, leaves saved_at alone" — a stale validation
     rule left over from before that repository logic existed. Removed the rejection;
     `'new'` was already in `validJobStatuses`.
     `jobs.spec.ts` test 5 now asserts Undo genuinely restores the job (previously it
     documented the "Undo failed" toast as the real, live-verified outcome).
- **Admin-delete-user (and self-service account deletion) weren't idempotent against
  an already-deleted Clerk account:** if the Clerk-side account for a `users` row was
  already gone (e.g. a prior delete attempt's Clerk call succeeded but the DB delete
  step afterward failed), retrying the delete returned `500 INTERNAL` instead of
  proceeding to remove the local DB row, permanently orphaning it with no way to
  remove it via the UI. `clerkadmin.ErrUserNotFound` already existed for exactly this
  case ("callers usually treat as success") but wasn't wired into either
  `UserAdminImplService.AdminDelete` or `UserImplService.Delete` — both now tolerate
  it. Found while cleaning up two orphaned disposable users left over from this
  suite's own development.

## Real bug found — documented, not fixed (out of scope here)

- **The Jobs detail panel doesn't refresh when a status change moves the open job
  out of the currently-viewed tab's filter.** `JobDetailPanel` receives its `job` as
  a prop looked up by ID from the currently-active list query's cache, not fetched
  independently. Changing status while viewing a tab that would no longer include
  the job after the change (e.g. New → Applied while viewing the New tab, or
  Applied/Interviewing → New while viewing Tracking) leaves the still-open panel
  showing stale data — confirmed live: the `PATCH` returns `200`, but the radio's
  `aria-checked` never updates, indefinitely. `jobs.spec.ts` avoids this by
  navigating to whichever tab still includes the job (or reloading) before reading
  status, rather than trusting an already-open panel instance across a
  filter-crossing transition — same technique a real user would need (switch tabs,
  or refresh, to see the change reflected). Worth a real fix later: either give the
  panel its own `useQuery(['job', id])`, or have the parent close/reopen the panel
  when its underlying job drops out of the active list.

## Fragile-ambient-data lesson — self-seed instead of depending on live Tracking data

`jobs.spec.ts` originally read whatever the live dev account's Tracking tab already
contained (status applied/interviewing/offer/rejected). That ambient data drained to
zero during real usage — 623 accumulated dismissed jobs, 0 left in any workflow
status — which silently broke both the status-change and dismiss/undo tests (no
data to act on). Both now self-seed by promoting a New-tab job to Applied first
(same convention as `resumes.spec.ts`/`apply-queue.spec.ts`'s own disposable-record
seeding) and fully revert it to New when done — self-contained, no dependency on
what the shared account happens to already have tracked.
