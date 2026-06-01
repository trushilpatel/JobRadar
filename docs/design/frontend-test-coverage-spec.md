# Frontend Test Coverage Backfill — Edge-Case Spec

**Goal:** raise frontend statement coverage to ≥90% global (per-file ≥90% target per `frontend/CLAUDE.md`).

**Date:** 2026-05-31

## Baseline & method

- All 60 test files / 714 tests pass. The "20 failing files" reported initially were a stale Vite resolver cache from the `ui/console → frontend` rename — cleared with `rm -rf node_modules/.vite`, not a real failure.
- The coverage **denominator** was polluted: vitest v8 (`all` on) instrumented `dist/` build bundles, `plopfile.js`, and the `__tests__` test files themselves (the latter at 100%, inflating the headline number). Fixed in `frontend/vite.config.ts`.

### What is excluded from coverage (and why)

| Exclusion                                                                                                                                         | Rationale                                                                                                                         |
| ------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------- |
| `dist/**`, `plopfile.js`, `**/*.config.*`, `**/*.d.ts`                                                                                            | Build output / tooling / generated — never source under test                                                                      |
| `**/__tests__/**`, `**/*.test.{ts,tsx}`                                                                                                           | Test code must not count toward source coverage                                                                                   |
| `src/types/**`, `src/lib/commands/types.ts`                                                                                                       | Type-only modules — no executable statements                                                                                      |
| `src/main.tsx`, `src/App.tsx`, `src/app/{router,providers,auth-bootstrap}.tsx`, `src/lib/core/queryClient.ts`                                     | App wiring / entry points — frontend analogue of backend `main.go` / `wire.go`                                                    |
| `src/features/resumes/**` + its page surfaces (`ResumeLibraryPage`, `ResumeDetailPage`, `ResumeEditorPage`, `ApplyQueuePage`, `JobResumeSection`) | Mock-data WIP: no real `api/` layer, hooks read `mockData.ts`. Excluded until wired to the backend, then re-include and backfill. |

**Baseline after clean denominator: 86.11% statements, 36 source files < 90%.**

## Per-file edge-case inventory

Each file lists the untested behavior that drives its coverage gap and the planned tests.
Conventions: mock at the API-module level; each test file builds its own `QueryClient`
(`retry:0, gcTime:0`); pages use `MemoryRouter`; cover loading / populated / empty / error per panel.

### Bucket A — components/ui + shared + brand + app/Sidebar

- **kpi-card.tsx** — non-interactive div vs button (`onClick` / `clickable` without handler), active styling + title, custom `valueClassName`, string value.
- **toggle.tsx** — off-by-default, click toggles + `onPressedChange`, controlled `pressed`, disabled, outline+sm variant classes, `toggleVariants` helper.
- **toggle-group.tsx** — selected-value reflection, `onValueChange`, variant/size from group context, item-level fallback when context unset (`context.variant || variant`).
- **resize-handle.tsx** — separator render, drag within bounds, min/max clamp, listener detachment on mouseup.
- **status-pill.tsx** — every CONFIG variant (ok/error/running/queued/pending), custom `label` override, pulse only on running, className merge.
- **multi-select-dropdown.tsx** — placeholder/empty, single + multi count label, open/select/deselect, search filter + "No matches", outside-click close, addable add (button + Enter, lowercasing, empty/duplicate ignored), leading icon + forwarded aria-label.
- **data-table.tsx** (+ `DataTableColumnHeader`) — populated/empty/loading(`aria-busy`)/error+retry, internal vs controlled sort, `onRowClick`, ArrowUp/Down keyboard nav, selected-row highlight, `getRowClassName`/`getRowProps`, server pagination, header sort aria states.
- **button.tsx** — variant/size classes, `asChild` slot render.
- **Logo.tsx** — LogoMark size/a11y/className, LogoSweepingMark scan label + animated sweep, LogoLockup md/sm/lg + wrapper className.
- **ErrorBoundary.tsx** — children render, default + custom fallback on throw, `componentDidCatch` logging.
- **Sidebar.tsx** — non-admin vs admin nav (7 admin links), active-route highlight, email + sign-out, collapse/expand, collapsed avatar initial + "?" fallback.

### Bucket B — auth slice

- **adminUsers.api.ts** — `fetchAdminUsers` populated/empty parse, default page/limit + filter omission, status/email filter inclusion, empty-status skip, schema-violation reject, error propagation; `setUserRoles`/`setUserStatus`/`deleteAdminUser` endpoint + payload + error.
- **me.api.ts** — `fetchMe` valid/empty-roles parse, invalid-status + missing-field reject, error propagation; `updateMe`/`deleteMe` endpoint + payload + error.
- **adminAudit.api.ts** — `fetchAuditLog` populated/empty parse, nullable actor/target + unknown payload, default + included filters, schema-violation reject, error propagation.
- **auth.store.ts** — `signInWithEmail` OTP shape + error throw; `signOut` clears session/identity/isAdmin/sessionRef; `refreshIdentity` admin/non-admin derivation + fetchMe-reject catch; bootstrap already-initialized no-op, cleanup unsubscribe, session-cleared-mid-fetch + null-session listener branches.
- **LoginPage.tsx** — disabled submit initial, redirect-home when session present, stays on form while loading, enable-on-email + sent confirmation, resend returns to form, Error vs generic error display.
- **AuthCallback.tsx** — code-exchange empty-message fallback, hash-token success / getSession-error / no-session, no-code/no-hash existing-session, missing-authorization-code error.

### Bucket C — jobs slice

- **useJob.ts** — success transform + `fetchJob(id)`, query disabled when id undefined / empty string, error propagation.
- **useJobs.ts** — `handleSortingChange([])` clear, `goToNextPage` boundary-fetch / already-fetched-immediate / no-op-when-no-next, `goToPrevPage` from later page, `itemsBeforePage` accumulation. (Throttled nuqs writes → page-transition `waitFor`s use a 5s timeout to stay deterministic under full-suite load.)
- **JobFilters.tsx** — desktop pill toggles (exp/date/location on→off→all/any), `5+ yrs` label, search change, reset hidden vs shown+fires, `aria-pressed`, ref forwarding; mobile branch (search, open-filters trigger, active-count badge, drawer + onReset).
- **MobileJobDetailRoute.tsx** — loading skeleton, found→panel, 404/403/generic/non-Error error states, deep-link back vs in-app `navigate(-1)`, status change, toggle-save, dismiss + stripe (with/without hideTarget) + undo, close.
- **JobsPage.tsx** — row-click open/close, keyboard `a`/`d`/`mod+enter`/`Escape`/`/`, tab switch, command-palette `mod+k` open + select-job.

### Bucket D — companies slice

- **companies.api.ts** — `fetchCompanies` query serialization (no-filter/full/`active=undefined` omission) + parse + reject, `createCompany`/`updateCompany`/`deleteCompany` (+URL-encoding), `triggerScanAll`/`setAllCompaniesActive`, `fetchQueue` (populated/empty/invalid-status), `removeFromQueue` — payload + error each.
- **RetryStage.tsx** — zero-total muted vs non-zero amber styling + tier summation, multi-word label split, tooltip retry fires mutation, disabled empty tiers, all-disabled while pending, per-tier counts in portalled tooltip.
- **companies-columns.tsx** — name cell + external-link stop-propagation, adapter badge, all 5 `last_scan_status` branches, history cell (padded/running-override/none), relative-time helper (null/now/m/h/d), active toggle labels + handler, action buttons (scan/edit/delete + scanning disabled).
- **PipelineStrip.tsx** — loading skeleton, error, missing-data-without-error, Total stage shown when total≠active vs hidden when equal, named stages.
- **CompanyDetailDrawer.tsx** — header actions (scan/edit/close), URL host parse + invalid fallback, delete-confirm (confirm/cancel), scan-stats loading + time/avg-duration (s/ms), logs (skeletons/"No runs yet"/OK-run/error expand-collapse), hiring activity (error/empty/populated chart, up/flat/down velocity). _(One uncovered region: recharts `HiringTooltip` custom content, fires only on hover — unreachable in jsdom; file ≥93%.)_
- **CompaniesHeader.tsx** — title/count + filter and action controls.
- **CompaniesPage.tsx** — header+rows, drawer open on row click, View-jobs nav, row-action delete dialog confirm/cancel, drawer-delete flow, both empty-message branches.

### Bucket E — dashboard + lib + misc

- **api-client.ts** — `{data}` + paged envelope unwrap, raw/empty/text/primitive bodies; URL building (`/api` prefix for leading-slash + bare, absolute passthrough); Authorization header presence/omission; Content-Type only with body; all verbs; `ApiValidationError` on 422 + status fallback + network-error wrap; 429 toast (with/without Retry-After); 401 → refresh+retry, refresh-fail → signOut+redirect, refresh-throws, double-401, no-redirect-on-`/login`, concurrent-refresh dedup; timeout timer, already-aborted signal, caller-abort.
- **userConfig.ts** — valid parse, missing field → default `false`, malformed parse error, error propagation (get + patch).
- **pipeline.ts** — full normalization, server `null` → zeros + empty tiers, absent/missing tier keys → 0, error propagation; `triggerRetry` filter/`1m` + enricher/`dlq` + missing-`drained` parse error + error.
- **dashboard.api.ts** — every fetch fn populated + server-`null` fallback (progress/market/heatmap/interviews/velocity/techTrends), velocity default/explicit limit, techTrends missing `total_jobs` default, `updateApplicationStage` PATCH + error.
- **MarketPanel.tsx** — error state, nav branches (click/Enter/Space → `/jobs?since=24h`, unrelated key ignored).
- **AccountPage.tsx** — email render, profile save success (`updateMe`→`refreshIdentity`→toast) + failure toast, account delete success (`deleteMe`→signOut) + failure toast. _(Line 43 — defensive empty-keys dash branch — unreachable without mocking the command registry; file ≥99%.)_
- **routes.ts** — all static builders, `jobs` query composition (undefined/empty drop), `jobOpen` merge, id builders, `applyQueue` filters. (Resume route builders live in the excluded resumes surface.)

## Status table

| Bucket                      | Files                                                                                                                               | Final coverage | Status  |
| --------------------------- | ----------------------------------------------------------------------------------------------------------------------------------- | -------------- | ------- |
| A — ui/shared/brand/Sidebar | kpi-card, toggle, toggle-group, resize-handle, status-pill, multi-select-dropdown, data-table, button, Logo, ErrorBoundary, Sidebar | all 98–100%    | ✅ Done |
| B — auth                    | LoginPage, AuthCallback, adminUsers.api, me.api, adminAudit.api, auth.store                                                         | all 100%       | ✅ Done |
| C — jobs                    | MobileJobDetailRoute, useJob, JobFilters, useJobs, JobsPage                                                                         | 94–100%        | ✅ Done |
| D — companies               | RetryStage, companies-columns, PipelineStrip, CompanyDetailDrawer, companies.api, CompaniesHeader, CompaniesPage                    | 93–100%        | ✅ Done |
| E — dashboard/lib/misc      | dashboard.api, MarketPanel, userConfig, api-client, pipeline, AccountPage, routes                                                   | 99–100%        | ✅ Done |

## Result

- **Before:** 60 test files / 714 tests; true source coverage 80.5% (polluted gate reported 85.6%); 36 source files < 90%.
- **After:** 91 test files / 1090 tests (+31 files, +376 tests); **97.78% statements**, **0 source files < 90%**; the `statements: 90` gate now passes against a clean denominator.

## Follow-up (when the resumes feature is wired to a real backend)

Remove the `src/features/resumes/**` + resume-page exclusions from `vite.config.ts` and backfill: ResumeLibraryPage, ResumeDetailPage, ResumeEditorPage, ApplyQueuePage, JobResumeSection, and the `features/resumes/{components,hooks}` slice (ApplyQueueTable, SavedJobsList, ContextPanel, ApproachPicker, ResumeCard, ContentPane, MetadataPanel, StateBadge, StorageMeter, VersionList, useApplyQueue, useResume).
