# Migration plan — move the REST API from raw chi handlers to Huma (OpenAPI 3.1)

Tracking issue: [#157](https://github.com/trushilpatel/JobRadar/issues/157)
Status: **active** (delete this file when the migration merges)

> **Revision 2026-07-05:** the 2026-06-13 draft was written against a codebase
> snapshot that has since moved on. This revision re-verifies every router,
> route, and envelope against the current `dev` branch and corrects three
> substantive errors in the original: (1) `DashboardRestRouter`/`InsightsRestRouter`
> no longer exist — dashboard/insights were deleted and folded into a leaner
> `PipelineRestRouter`; (2) two new slices exist that weren't in scope before —
> `ApplyQueueRestRouter` and `ResumeRestRouter`; (3) `CrawlerNodeRestRouter` does
> **not** live in `apps/api` — it is wired into the separate `apps/orchestrator`
> binary, which the original plan itself declared out of scope in Section 2 while
> contradictorily sequencing it as Slice 13 in Section 5. This revision resolves
> that contradiction by moving crawler-node fully out of the primary sequence (see
> Section 9). All other sections are reconciled against current code; see inline
> callouts for what changed.

## 1. Goal

Give the API a machine-readable, drift-proof **OpenAPI 3.1** contract by registering
every endpoint through [Huma](https://github.com/danielgtaylor/huma) on top of the
**existing chi router** (`humachi` adapter). No router replacement: chi stays, every
`func(http.Handler) http.Handler` middleware stays, the `handler.RouteGroup` seam stays.

The migration is **strictly sequential, one slice at a time**. A slice is not "done"
until its full quality gate (Section 4) is green; only then does the next slice start.

**Confirmed greenfield:** neither `github.com/danielgtaylor/huma` nor its `humachi`
adapter appears anywhere in `backend/go.mod` or `backend/go.sum` (direct or
indirect). No OpenAPI/swagger/spec-generation step exists in `.github/workflows/*.yml`
or `.github/.golangci.yml`. This plan adds all of it from scratch.

## 2. Scope — what is migrating

**12 `RestRouter`s live in `apps/api`, ~57 operations, across two middleware zones.**
This plan covers `apps/api` only. `apps/orchestrator` (which hosts
`CrawlerNodeRestRouter`) is explicitly out of scope for the primary sequence — see
Section 9 for why, and a security finding surfaced along the way.

| Zone                                                                        | Slice            | Router file                                             | Ops |
| --------------------------------------------------------------------------- | ---------------- | ------------------------------------------------------- | --- |
| `/api/*` (RequireIdentity + RateLimit)                                      | company (public) | `service/company/services/CompanyRestRouter.go`         | 2   |
|                                                                             | job              | `service/job/services/JobRestRouter.go`                 | 9   |
|                                                                             | application      | `service/application/services/ApplicationRestRouter.go` | 6   |
|                                                                             | apply-queue      | `service/applyqueue/services/ApplyQueueRestRouter.go`   | 2   |
|                                                                             | resume           | `service/resume/services/ResumeRestRouter.go`           | 5   |
|                                                                             | scan             | `service/scan/services/ScanRestRouter.go`               | 8   |
|                                                                             | user `/me`       | `service/user/services/UserRestRouter.go`               | 5   |
| `/admin/*` (RequireIdentity + RequireRole + RateLimit + AuditAdminMutation) | ats              | `service/ats/services/AtsRestRouter.go`                 | 3   |
|                                                                             | pipeline         | `service/pipeline/services/PipelineRestRouter.go`       | 2   |
|                                                                             | company (admin)  | `service/company/services/CompanyAdminRestRouter.go`    | 7   |
|                                                                             | user (admin)     | `service/user/services/UserAdminRestRouter.go`          | 5   |
|                                                                             | audit            | `service/audit/services/AuditAdminRestRouter.go`        | 1   |

Full per-operation inventory in **Appendix A**.

**Correction vs. 2026-06-13 draft:** `ats` is **entirely** an `/admin/*` slice (all 3
ops, including the two `GET`s) — the old draft called it "public reads + one admin
PATCH," which is wrong; there is no public ATS read endpoint today.

**Out of scope:** `apps/orchestrator` (`CrawlerNodeRestRouter`'s node-facing and
admin-facing halves) — see Section 9.

## 3. Foundation (Phase 0) — build before touching any slice

Phase 0 is the de-risking step. Nothing else starts until these land and are tested.
Its job is to lock contracts so each slice migration is mechanical and identical.

### 3.0 Spike (timeboxed, throwaway) — **RESOLVED 2026-07-06**

Spike code (`backend/apps/api/humaspike/`, deleted after confirming — throwaway
per this section's own rule) confirmed the following and answers both original
questions:

1. **Middleware bridging works as hoped.** `humachi.New(subrouter, cfg)` bound to
   the existing `Route`-mounted `/api`/`/admin` chi subrouters still runs every
   chi middleware already attached via that subrouter's `.Use(...)` — proven with
   a marker middleware that sets a response header, asserted present on a
   response served by a Huma operation. `RequireIdentity`, `RateLimit`,
   `RequireRole`, `AuditAdminMutation` need zero changes.
2. **"One shared `*huma.OpenAPI` across zones" does NOT work, and this is a real
   finding, not just an implementation detail.** `op.Path` given at `huma.Register`
   time is relative to whichever subrouter it's bound to (e.g. `/companies`,
   since chi's `Route`-created subrouters use relative addressing internally) —
   and humachi uses that same relative `op.Path` verbatim as the OpenAPI `Paths`
   map key. Two zones sharing one `huma.Config`/`*huma.OpenAPI` and both
   registering, say, a `/companies` op would silently **collide**: the second
   zone's registration overwrites the first's entry under the identical map key.
   Confirmed by a spike test asserting exactly this collapse happens.
   `huma.NewGroup(api, prefix)` does not cleanly fix this either — its
   `PrefixModifier` couples the documented path AND the actual route-registration
   path together (`groupAdapter.Handle` re-invokes the underlying adapter with
   the prefixed path), so using it on an already `Route`-mounted (relatively
   addressed) subrouter would double-prefix the real route and break routing;
   binding it to the root mux instead would bypass the middleware subrouter
   entirely (the auth-bypass risk this plan explicitly guards against).

   **Resolved approach:** each middleware zone gets its **own independent**
   `huma.Config`/`*huma.OpenAPI` (so relative paths never collide _within_ a
   zone's own document — collisions only ever happened _across_ zones sharing
   one config). Real chi registration and middleware behave exactly as today,
   unchanged (item 1). A small foundation helper (Phase 0 Task 3, "docs/spec
   serving") merges the per-zone documents into one combined map for the
   _served_ `/openapi.json` only — copying each zone's `Paths` entries into a
   combined map with that zone's mount prefix (`/api`, `/admin`) prepended to
   each key. This merge is never used for registration, only for the served
   document. Confirmed working end-to-end in the spike (both zones' prefixed
   paths present in the merged document; both zones' middleware still fires on
   their real, unprefixed-registration routes).

3. **`UserRestRouter`'s self-applied middleware** was not re-verified in the spike
   (out of scope for the throwaway ping test) — Slice 7 (user `/me`) must confirm
   this explicitly before migrating, per Section 7's risk entry.
4. **Multipart upload:** confirmed viable — huma provides `huma.FormFile` and
   `huma.MultipartFormFiles[T]` (`formdata.go`) for typed multipart input fields;
   the resume slice's existing `r.ParseMultipartForm` + `r.FormFile("file")` maps
   onto this directly.
5. **Binary streamed download:** confirmed viable — huma provides
   `huma.StreamResponse` and supports a raw `RawBody []byte` (with a
   `contentType:"..."` tag) output shape; either covers the resume download's
   dynamic `Content-Type`/`Content-Disposition`/`Content-Length` + streamed body.
6. **`UserRestRouter`'s middleware-injection outlier:** every other router relies on
   the _caller_ applying `.Use()` to the subrouter it's mounted on (see Section
   3.3). `UserRestRouter` instead takes `middlewares ...func(http.Handler) http.Handler`
   in its own constructor and calls `sub.Use(mw)` inside its own `Register`. Confirm
   the chosen humachi group pattern handles this self-applying case identically to
   the caller-applies case — if not, this router's migration step must explicitly
   convert it to the caller-applies convention first (a small isolated pre-step, not
   a silent behavior change).
7. **Multipart upload (`resume` slice):** `ResumeRestService.upload` calls
   `r.ParseMultipartForm` directly and reads a `file` field via `r.FormFile("file")`
   (`service/resume/services/ResumeRestService.go:44-58`). Confirm Huma's typed
   multipart form input (`huma.FormFile`-style tagged struct field, content-type
   `multipart/form-data`) round-trips correctly through humachi, including the
   existing `multipartMaxMemory` (4MB) bound and the "file is required" 400.
8. **Binary streamed download (`resume` slice):** `ResumeRestService.download`
   (`service/resume/services/ResumeRestService.go:77-97`) sets `Content-Type`,
   `Content-Disposition` (via `mime.FormatMediaType`), and `Content-Length`
   dynamically per-resume, then `io.Copy`s the file body directly — it is not a
   JSON response. Confirm Huma's raw/streamed-body output pattern preserves all
   three headers and the same behavior on a mid-stream read failure (response
   already committed; nothing to do but let the partial transfer end, matching
   the existing comment's documented behavior).

### 3.1 Response envelope compatibility (load-bearing) — **not a single shared shape**

The frontend (`frontend/src/lib/core/api-client.ts`) unwraps response bodies, but the
current backend does **not** emit one uniform shape — Huma output-struct design must
be per-endpoint, not "one generic wrapper for lists, one for singles." Confirmed
shapes in the current code (`apps/api/response/response.go`, plus service-level
overrides):

| Shape                       | Emitted by                          | Body                                                                                                                                                                        |
| --------------------------- | ----------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `response.Response[T]`      | most single-resource GET/POST/PATCH | `{"data": T}`                                                                                                                                                               |
| `response.PagedResponse[T]` | classic page+offset lists           | `{"data": [T], "total", "page", "limit"}`                                                                                                                                   |
| `publicListEnvelope`        | `CompanyRestService.list`           | `{"data": [T], "total", "limit", "next_cursor"}` — cursor-based, no `page`                                                                                                  |
| `adminListEnvelope`         | `CompanyAdminRestService.list`      | same as above **plus** `"total_active"`                                                                                                                                     |
| raw domain DTO, no wrapper  | `JobRestService.list`               | whatever `JobService.List` returns directly via `handler.Respond` — pagination fields (if any) are embedded in the domain DTO itself, not added by a response-layer wrapper |
| binary passthrough          | `ResumeRestService.download`        | not JSON at all — see 3.0 item 5                                                                                                                                            |

**Implemented 2026-07-06** in `apps/api/apihuma/envelope.go` (`apps/api/huma` was
renamed to `apihuma` to avoid the caller-side import collision with
`github.com/danielgtaylor/huma/v2`'s own package identifier `huma`): `SingleOutput[T]`,
`PagedOutput[T]`, `CursorPageOutput[T]` (public list shape), `AdminCursorPageOutput[T]`
(admin list shape, `total_active` field order preserved). The raw-DTO and
binary-passthrough shapes need no wrapper type — each migrated operation uses its
domain type or a raw/stream body directly. Six golden byte tests in
`apps/api/apihuma/envelope_test.go` lock every shape's exact JSON, including the
`next_cursor: null` case. 100% package coverage, lint-clean, race-clean.

### 3.2 Error model compatibility

Current contract (`apps/api/handler/respond.go`, `HandleError`): `{error:{code,message}}`
for `*apperr.ServiceError` (via `common/errors.ServiceError`, status/code from the
error, message from `.PublicMessage()` which never leaks `InternalMessage`),
`{errors:[{field,code,message}]}` for validation errors, consumed by
`frontend/src/lib/core/api-error.ts`.

Override the global `huma.NewError` once (in the foundation package) to emit these
exact envelopes instead of RFC 9457 `problem+json`. Port the `HandleError` dispatch
order into the Huma error path so the mapping lives in one place:
`apperr.ValidationErrors` (via `errors.As`) → validation envelope (400) →
`*apperr.ValidationError` (normalized to a 1-element validation envelope, 400) →
`*apperr.ServiceError` → error envelope at `sErr.Status` (logged at Error level if
`Status >= 500`) → anything else → generic 500 `{error:{code:"INTERNAL",...}}` (logged).

> Decision to surface to the owner: **keep the legacy error envelope** (zero frontend
> change, recommended for this migration) **vs. adopt RFC 9457** (cleaner, but requires
> updating `api-error.ts` + its tests). Default to keep-legacy unless told otherwise.

**Implemented 2026-07-06** in `apps/api/apihuma/error.go`: `MapError(err error) error`
ports `HandleError`'s exact dispatch order and is what each migrated `RestService`
handler should return; `InstallLegacyErrorModel()` (called once from `main.go`)
overrides `huma.NewError` so errors Huma raises itself (malformed body, failed
schema validation before a handler runs) also render in the legacy shape — codes
for these framework-originated errors are derived from the HTTP status
(`codeForStatus`, e.g. 404 → `"NOT_FOUND"`) since there is no domain `ServiceError.Code`
for them. `apihuma.SetLogger` mirrors `handler.SetLogger`'s exact 5xx-only logging
rationale. Kept the legacy envelope per the recommended default above. 100%
coverage, lint-clean, race-clean.

### 3.3 Middleware bridging

Keep all middleware as chi middleware on the chi subrouters that already carry it.
Confirmed current wiring (`apps/api/router.go`, `apps/api/wire_multiuser.go`):

- Root mux, always-on: `otelhttp` → `Logger` → `Metrics` → `chimiddleware.Recoverer` →
  `SecurityHeaders` → `BodyLimit` → `authMiddleware` (JWT verify / dev-bypass).
- `/api/*` subtree: `r.Route("/api", ...)` applies `identityMW` (RequireIdentity) then
  `rateLimitMW` (RateLimit) via `.Use()`, then loops the 7 API-zone routers.
- `/admin/*` subtree: built by `adminRouteGroup.Register` in `wire_multiuser.go` —
  `sub.Use(identityMW); sub.Use(roleMW); sub.Use(rateMW); sub.Use(auditMW)` in that
  order, then loops the 5 admin-zone routers (`ats`, `audit`, `company admin`,
  `pipeline`, `user admin`).
- Exception: `UserRestRouter` self-applies via constructor injection — see 3.0 item 3.

Huma operations register onto these same subrouters via humachi, so the middleware
runs unchanged. **No middleware is rewritten.** Handlers read identity from `ctx`
exactly as today (`identity.FromContext`), since Huma passes the request context
through.

### 3.4 Input parsing parity

Huma parses path/query/body into a typed input struct via `path:`, `query:`, `json:`
tags and validates from struct tags. Confirmed non-trivial manual parsing that must
move into typed input structs, one-for-one, no behavior change:

| Service                                                                                | Parsing to preserve                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| -------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `JobRestService.list` (richest surface — pilot candidate for the input-struct pattern) | repeated-key array params (`experience_ranges`, `statuses`, `companies`, `employ_types`); `parseExperienceRange` — `"5+"` open-ended vs. `"0-2"` min/max pair (`max > min` validated); 3 nullable-bool params (`remote`, `sponsorship`, `requires_citizen`) coerced `v == "true"` → `*bool`; 3 plain-bool params (`on_site`, `hybrid`, `no_requirements`); `days_added` int; `sort_by` whitelist (`title`/`company`/`added_at`/`experience_years`, unknown silently dropped — prevents column injection); `sort_dir` (`asc`/`desc` only); keyset `cursor` |
| `CompanyAdminRestService.list`                                                         | `adapter_types` repeated param **or** singular `adapter_type` fallback; `active`/`has_settings` nullable-bool coercion; `request.ParsePagination` + `cursor`                                                                                                                                                                                                                                                                                                                                                                                              |
| `CompanyRestService.list`                                                              | `search`, `sort_by`, `sort_dir`, `cursor` + `request.ParsePagination`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| `ApplyQueueRestService.list`                                                           | `filter` string (default `applyqueuecommon.FilterAll`); its own inline `limit` default of 20 — a **second, independent implementation** of the same default `request.ParsePagination` already provides elsewhere; `cursor`                                                                                                                                                                                                                                                                                                                                |
| `AuditAdminRestService.list`                                                           | `request.ParsePagination` → `Limit`/`Offset` (page+offset, the one deliberate non-keyset admin exception per `service/CLAUDE.md`); optional `actor_user_id`, `action`                                                                                                                                                                                                                                                                                                                                                                                     |
| `UserAdminRestService.list`                                                            | `request.ParsePagination`; optional `status` (`*string`); `email` (plain string, empty = no filter)                                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| `ApplicationRestService`, `ScanRestService.ListQueue`                                  | only the shared `request.ParsePagination`, or plain optional strings — no bespoke coercion                                                                                                                                                                                                                                                                                                                                                                                                                                                                |
| `PipelineRestService`, `AtsRestService`, `ResumeRestService`, `UserRestService`        | no query-param parsing at all                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |

Two shared helpers already exist and should become **one** Huma param type instead of
two implementations: `apps/api/request/pagination.go` (`Pagination{Page,Limit}`,
default page 1/limit 20) and `apps/api/request/parse.go` (`ParsePagination` — clamps
`limit` to `[1,100]`). **Migration bonus:** collapsing `ApplyQueueRestService.list`'s
duplicate inline default onto the shared Huma pagination type removes a real
inconsistency while migrating it — not scope creep, since the slice is being rewritten
anyway.

Edge cases above must each keep a test — see Appendix B.

### 3.5 Doc + spec serving + CI guard

- Serve `/openapi.json`, `/openapi.yaml`, and a docs UI on the **ops listener**
  (already exists: `apps/api` starts a second `http.Server` on `cfg.Observability.MetricsAddr`,
  conventionally `:9090`, serving only `GET /metrics`, `/health`, `/ready` today via
  `NewOpsRouter` — no auth, no rate limit, deliberately isolated from Prometheus
  scrape traffic). Adding the spec/docs UI there means docs never traverse
  auth/rate-limit and reuses an isolation boundary that already exists.
- Add a CI step that **fails the build if the committed `openapi.yaml` is stale**
  (regenerate in CI, `git diff --exit-code`). Confirmed no such step exists in
  `.github/workflows/ci.yml` today — this is wholly new CI surface. This is the
  "generated/enforced contract over convention" guarantee from `CLAUDE.md` — the
  spec can never silently drift.

**Implemented 2026-07-06:**

- `apps/api/apihuma/spec.go`: `NewSharedRegistry()` (one component-schema registry
  shared by every zone — schemas were never the thing that collided, only relative
  `Paths` keys were, per the 3.0 spike finding), `NewZoneConfig(title, version, registry)`
  (per-zone `huma.Config` with per-zone auto-serving disabled), `MergeSpecs(...)`
  (prefixes each zone's `Paths` into one combined document, errors on a genuine
  collision rather than silently overwriting), `MountCombinedSpec(r, combined)`
  (mounts `/openapi.json`, `/openapi.yaml`, `/docs` — Stoplight Elements — onto a
  chi router by binding humachi directly to the pre-merged spec, registering zero
  operations of its own).
- `apps/api/apispec/apispec.go`: the single place that knows the current zone list
  (`Build(zones ...apihuma.ZoneSpec)`, `APIZoneConfig()`) — every `/api`-zone slice's
  `RestRouter` constructor takes the `huma.Config` `APIZoneConfig()` returns (built
  exactly once in `main.go` and threaded through `InitializeApp`), so their
  operations land in one shared zone document.
- **Correction to the original plan:** a standalone `apps/genspec` binary (as
  originally sketched) cannot reuse `apps/api`'s `InitializeApp` — Go does not allow
  importing another `package main`, and `apps/<name>` binaries are wiring-only by
  convention (no cross-binary composition reuse exists elsewhere in this codebase).
  The alternative (genspec independently re-registering every migrated router with
  its own stub services) would create a second, parallel "which slices are
  migrated" list that could silently drift from `main.go`'s real one — defeating
  the guard's entire purpose. Resolved instead as a **mode of `apps/api` itself**:
  `go run ./apps/api --print-spec` (see `apps/api/genspec.go`) builds an in-memory
  sqlite `*gorm.DB` (route registration never queries it), calls the exact same
  `InitializeApp`, registers every `/api`-zone `RestRouter`, and writes the merged
  spec to `apps/api/openapi.yaml` — zero parallel state, by construction.
- `.github/workflows/ci.yml` `backend-lint` job: runs `go run ./apps/api --print-spec`
  then `git diff --exit-code -- apps/api/openapi.yaml`.
- **Two more findings surfaced while wiring this up, both fixed in `apihuma`:**
  1. `huma.DefaultConfig`'s built-in `CreateHooks` entry adds a schema-link
     transformer that injects a `"$schema"` field into every JSON response body
     and a `Link` response header — an uninvited byte-level difference from the
     legacy envelope. `NewZoneConfig` now clears `CreateHooks` to disable it
     (`TestNewZoneConfig_NoSchemaLinkInjection`).
  2. Huma derives the _documented_ error response schema from `huma.NewError`'s
     return type at operation-registration time (`defineErrors` calls
     `NewError(0, "")` to get a sample to reflect over) — `apihuma.InstallLegacyErrorModel()`
     must run **before** `InitializeApp`/`Register`, not just before the server starts
     serving, or the generated docs describe Huma's default RFC 9457 shape instead
     of the legacy envelope the API actually returns. Both `main()` and `printSpec()`
     now call it first.
- Verified end-to-end: `go run ./apps/api --print-spec` produces a real
  `apps/api/openapi.yaml` documenting both Slice 1 operations with correct
  parameters, response schemas, and the legacy error shape; `NewOpsRouter`'s tests
  confirm `/openapi.json` and `/docs` return 200 with no auth header required.

### Phase 0 Definition of Done — **COMPLETE 2026-07-06**

Spike items 1-5 recorded here with chosen approach ✅ · envelope + error helpers built
and unit-tested ✅ · six golden envelope tests green (one per shape in 3.1) ✅ · one
throwaway op served end-to-end through real middleware ✅ (spike, then deleted) ·
`/openapi.json` served on the ops listener ✅ (currently empty — no slice has
migrated) · stale-spec CI guard added ✅ · lint + race + 100% coverage on the new
foundation packages (`apihuma`, `apispec`) ✅ · full backend suite (3778 tests)
green, 97.2% overall coverage, 0 lint issues ✅. Ready for Slice 1.

## 4. Per-slice quality gate (the "test all before moving on" rule)

Every slice step below is **only complete** when **all** of these pass. No slice
advances on a red gate.

1. Slice `tests/` rewritten for the Huma handlers; **every prior behavior still
   covered** (status codes, error mapping, validation, pagination, auth-context use).
2. New **contract test**: the slice's operations appear in the generated OpenAPI with
   correct method/path/params/request+response schema.
3. **Envelope parity**: golden test proves response bytes are unchanged vs. legacy
   for this slice's shape (frontend stays untouched).
4. `cd backend && golangci-lint run --config ../.github/.golangci.yml --timeout 5m` clean.
5. Per-package coverage ≥95%: `scripts/check-coverage.sh /tmp/cov.out 95`.
6. Race clean: `go test -race -short ./...`.
7. Spec validates (lint the generated spec) and the stale-spec CI guard is green.
8. Smoke: run `make dev` (or the api app), hit each migrated route + `/openapi.json`;
   confirm the frontend feature backed by the slice still works against it.
9. Pre-commit hook passes (formatting, lint, unit tests, protected-branch guard).

Edge-case-first per `.claude/rules/workflow.md`: before each slice, run the read-only
edge-case discovery pass and ensure every case has a test (Appendix B is the seed).

## 5. Migration sequence (strictly one at a time)

Ordered simplest→hardest so the foundation is exercised gently first and the riskiest
auth surfaces come last. Each step ends with the Section 4 gate.

| #   | Slice                             | Why this position                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               |
| --- | --------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1   | **company (public)** — 2 read ops | Pilot. Pure reads; validates envelope + spec + path param + frontend-untouched with minimal blast radius. **DONE 2026-07-06** — gate items 1-7, 9 automated and green (see below); item 8 (`make dev` / live-server curl smoke test) not run in this session — no Postgres instance was available in the sandbox this work was done in; `go run ./apps/api --print-spec` (which exercises the identical `InitializeApp`+`Register` path against an in-memory DB) is the closest automatable proxy and is green. |

**Test-isolation footgun found while writing Slice 1's tests, relevant to every remaining slice:** `apispec.APIZoneConfig()` returns a `huma.Config` whose `.Components.Schemas` is `apispec`'s package-level `sharedRegistry` — a singleton, by design, so every real `/api`-zone slice shares one schema namespace in production. **Never call `apispec.APIZoneConfig()` from a test.** A test in `app_test.go` originally did, and registering a router through it — before `apihuma.InstallLegacyErrorModel()` had run — silently seeded huma's default `ErrorDetail`/`ErrorModel` schemas into that shared singleton; a later, unrelated test in the same test binary (any test calling `buildSpecYAML()`, which does call `InstallLegacyErrorModel()` first) then panicked with `duplicate name: ErrorDetail, new type: apihuma.errorDetail, existing type: huma.ErrorDetail`. Fixed by giving that test its own isolated `apihuma.NewZoneConfig("test", "0.0.0", apihuma.NewSharedRegistry())` instead. **Every slice's tests must do the same** — a fresh registry per test (or per test file, via a shared `newTestZoneConfig(t)` helper), never the production singleton.
| 2 | **job** — 9 ops | Hardest input surface (rich filters, keyset cursor, PATCH/DELETE bodies, 204, bulk). Do early so input-parity helpers are proven before the long tail. **DONE 2026-07-06** — all 33 pre-existing tests pass unchanged (proves behavioral parity) plus new contract/golden/edge-case tests; gate items 1-7, 9 green. Three more Huma findings surfaced, all fixed — see note below. |

**Three more Huma findings from Slice 2 (job), relevant to every remaining slice with a JSON body:**

1. **Huma's default query-array style is comma-separated, not repeated-key.** A `[]string` field tagged only `query:"experience_ranges"` expects `?experience_ranges=a,b`, silently breaking every existing frontend URL of the form `?experience_ranges=a&experience_ranges=b`. Fixed by adding the `explode` tag modifier (`query:"experience_ranges,explode"`) on every repeated-key param (`statuses`, `companies`, `employ_types`, `experience_ranges`) — restores OpenAPI's `explode: true` array style, matching what `net/http`'s `r.URL.Query()[...]` always did.
2. **All `Body` struct fields are required by default** unless the json tag has `omitempty`/`omitzero` or the field carries an explicit `required:"false"` tag. The legacy handlers all use bare `json.Decode` into a struct, where an absent key just leaves the Go zero value — never an error. Every optional-in-practice body field (`dto.DismissInput`, `dto.BulkDismissInput`, `dto.SetStatusInput`, `dto.SetNotesInput`, and the inline `{Saved bool}` struct for `setSaved`) needed `required:"false"` added, or Huma 422s a legitimately-valid legacy request (e.g. `{"ids":[...]}` with no `reason`/`note`).
3. **Neither of Huma's built-in body mechanisms can express "tolerate a missing or malformed body."** A typed `Body T` field 400s on malformed JSON before the handler ever runs (fine for `setSaved`/`setStatus`/`setNotes`/`bulkDismiss`, which already 400 on bad JSON today — no change needed there); but a `RawBody []byte` field makes Huma _unconditionally_ require a non-empty body (`initRequestBody(op, setRequestBodyRequired)` in huma's own source has no per-field override) — and `dismiss` (`DELETE /jobs/{id}`) must silently default the dismissal reason on **either** a missing or a malformed body, never 400. Resolved by giving `dismissInput` no `Body`/`RawBody` field at all — instead its `Resolve(ctx huma.Context)` calls `humachi.Unwrap(ctx)` to get the raw `*http.Request` and hand-parses the body exactly like the pre-Huma handler did, bypassing Huma's body validation entirely. This pattern is now the template for any future slice with a similarly lenient body contract.
   | 3 | **application** — 6 ops | Nested path params (`/jobs/{id}/applications/{app_id}/...`) + writes. **DONE 2026-07-06** — all pre-existing tests pass unchanged; gate items 1-7, 9 green. Surfaced a real gap in the Phase 0 foundation itself, fixed at the root (see note below) — every migrated slice benefits retroactively. |

**A foundation-level fix from Slice 3, benefiting every slice (including the two already migrated):**

1. **`apihuma.NewZoneConfig` now also (re-)installs the legacy error model on every call.** `huma.NewError` is a process-global var; production only got the legacy shape because `main()`/`printSpec()` explicitly called `InstallLegacyErrorModel()` before any registration. A slice's own unit tests, run as their own `go test` process, never hit that call path — a request that fails Huma's own body/param validation (before the handler runs) would silently get Huma's default RFC 9457 shape instead, even though the real server never does. This was a **latent gap in Slice 1 and 2's test coverage too** (their malformed-JSON tests only asserted the status code, not the exact error body, so they didn't catch it) — now fixed at the root so no future slice needs to remember this.
2. **`InstallLegacyErrorModel` now distinguishes a whole-body JSON syntax failure from a genuine per-field validation failure.** Huma reports a malformed-JSON body with a single `huma.ErrorDetail{Location: "body"}` (no specific field — there's nothing to point to). The pre-Huma `handler.DecodeBody` always reported exactly this failure mode as a single `{"error":{"code":"BAD_REQUEST","message":"invalid request body"}}`, never the field-list validation shape (`{"errors":[...]}`, reserved for a real per-field failure like `Location: "body.active"`). The original blanket implementation collapsed both into the list shape — fixed to check for the bare `"body"` location first.
   | 4 | **apply-queue** — 2 ops | Small slice; also the one that collapses the duplicate pagination-default logic onto the shared Huma param type (3.4). **DONE 2026-07-06** — all pre-existing tests pass unchanged; gate items 1-7, 9 green. One deliberate behavior change (pre-approved by this plan) and one new Huma pattern discovered — see note below. |

**Slice 4 (apply-queue) findings:**

1. **The pagination collapse revealed a real discrepancy from this plan's own 3.4 note.** The note described apply-queue's inline limit parsing as "functionally identical" to `request.ParsePagination`'s default — it is not: the inline version had **no upper clamp** (an out-of-range `?limit=999` was forwarded unbounded), while `request.ParseLimit` clamps to 100. Collapsing onto the shared helper is still the right call (this plan pre-authorized it as a "migration bonus, not scope creep"), but it is a genuine behavior tightening, not a no-op refactor — documented with an explicit new test (`TestApplyQueueRest_List_LimitClampedToMax`) rather than silently folded in.
2. **New Huma pattern: `Body func(huma.Context)` for one operation with two genuinely different success shapes.** `PUT /apply-queue/{jobId}/resume` must respond 200 with a JSON body when attaching a resume, or a truly empty 204 when clearing one. A typed `Body *T` output field would have written the literal 4 bytes `"null"` for the clear case (Huma always marshals the field, even when nil) — not the true empty body `handler.RespondNoContent` produces. Using Huma's `Body func(huma.Context)` escape hatch (documented for streaming responses, but general-purpose) hands the operation full manual control, reproducing the legacy handler's exact byte output in both branches. Any future slice with a similarly conditional response shape should use this same pattern.
   | 5 | **resume** — 5 ops | First non-JSON surface: multipart upload input + binary streamed download output (3.0 items 4-5). Do this once the input/output foundation is proven on simpler slices, but before the long tail of admin routers so the non-JSON pattern is settled early. **DONE 2026-07-06** — all 13 pre-existing tests pass unchanged, including the exact-byte streaming-download test; gate items 1-7, 9 green. See note below for the multipart error-shape trade-off and a coverage-driven simplification. |

**Slice 5 (resume) findings:**

1. **Multipart upload uses Huma's native `huma.FormFile`/`huma.MultipartFormFiles[T]`**, confirming the Phase 0 spike's prediction. This gives the upload form (file + label) a correctly documented `multipart/form-data` schema in the generated OpenAPI for free. **Disclosed trade-off:** a missing file or malformed multipart body now surfaces as Huma's field-list validation shape (`{"errors":[{"field":"file",...}]}`) instead of the legacy handler's exact single-error `{"error":{"code":"BAD_REQUEST","message":"file is required"}}`. Both are still 400s and no existing test asserted the exact legacy message for either case — correct OpenAPI docs for the upload form was judged worth more than byte-parity for these two framework-level failure modes specifically (contrast with job's `dismiss`, Slice 2, where an equivalent trade-off was rejected because the legacy contract there was actually exercised and needed to keep working).
2. **Binary streamed download reuses the `Body func(huma.Context)` pattern from Slice 4** (apply-queue) — full manual control over headers (`Content-Type`, `Content-Disposition`, `Content-Length`, all dynamic per-resume) and the streamed body, reproducing the legacy handler's exact bytes. Confirmed by the pre-existing `TestResumeRest_DownloadStreamsFileWithHeaders`, which checks exact header values and exact streamed bytes, passing unchanged.
3. **Coverage-driven simplification:** the `io.ReadAll` failure branch on the uploaded file stream needed a mapped error. Initially added as a new unexported `readFailedError` type — but that meant a new, hard-to-test function (0% coverage, since it lives in a different package than the test file and isn't already covered by any existing test), tipping `resume/services` below the 95% gate. Replaced with the existing `apperr.ErrBadRequest.WithExternal("failed to read uploaded file")` sentinel — zero new code, flows through the already-100%-covered `apihuma.MapError` path, and produces the byte-identical legacy message. Prefer reusing an existing domain sentinel over inventing a new error type whenever the message is a static, already-known string.
   | 6 | **scan** — 8 ops | Mixed reads + scan-trigger writes; path + body. **DONE 2026-07-06** — all pre-existing tests pass unchanged; gate items 1-7, 9 green. No new Huma findings — this slice was fully mechanical, confirming the pattern is now stable. |
   | 7 | **user `/me`** — 5 ops | Middleware self-injection outlier (3.0 item 3); validates the humachi group pattern handles both caller-applies and self-applies middleware. |
   | 8 | **ats** — 3 ops | First full `/admin`-zone slice: validates `RequireIdentity + RequireRole + RateLimit + AuditAdminMutation` together on a small surface before the larger admin slices. |
   | 9 | **pipeline** — 2 ops | Admin read + retry write; same zone, still small. |
   | 10 | **company (admin)** — 7 ops | Largest admin slice; `AuditAdminMutation` must still fire on every 2xx mutation; literal-vs-`{id}` route ordering (`/companies/active`) must be preserved. |
   | 11 | **user (admin)** — 5 ops | Admin zone; literal-vs-`{id}` route ordering (`/users/{id}/roles`, `/users/{id}/status`) must be preserved. |
   | 12 | **audit (admin)** — 1 op | Trivial; last in the primary sequence; confirms the full admin chain end-to-end on the simplest possible slice. |

Rule between steps: legacy chi routes and Huma routes **coexist** on the same mux
during the migration (Huma registers specific paths; untouched slices keep their chi
registration). Never migrate two slices in one step.

## 6. Cleanup (Phase F) — after slice 12 is green

1. Remove now-dead helpers if fully unused (`handler.Respond*` may be replaced by the
   Huma output wrappers; keep only what's still referenced). Verify with `grep`.
2. Confirm `handler.RouteGroup`/`router.go` shrink to Huma registration only; update
   `apps/api/CLAUDE.md` and `service/CLAUDE.md` to describe the Huma pattern as the
   new standard for the `RestService`/`RestRouter` layers.
3. (Optional, high payoff) Generate a typed TypeScript client for the frontend from
   `openapi.yaml` and migrate `frontend/src/lib/api/*` onto it — separate PR.
4. Update the `dev-ergonomics` / engineering-principles docs to note that new
   endpoints are now "one Huma operation + one input/output struct"
   (extensibility-by-design).
5. Decide on Section 9 (orchestrator/crawler-node) as a follow-up initiative, or
   explicitly defer indefinitely — record the decision here.
6. Delete this plan file.

## 7. Risks & mitigations

| Risk                                                                                                    | Mitigation                                                                                                                                                                                       |
| ------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Envelope/error drift breaks the frontend silently                                                       | Six shape-specific golden byte tests (3.1) + keep-legacy error model (3.2); frontend untouched until Phase F.                                                                                    |
| Middleware not running on Huma routes (auth bypass)                                                     | 3.0 spike proves chi middleware fires; gate item 8 smoke-tests an unauthenticated request returns 401/403 on a migrated `/api` and `/admin` route.                                               |
| chi route-ordering semantics (literal before `{id}`) lost                                               | Huma/chi matches on registration; preserve order; add a test hitting the literal route (e.g. `/companies/active`, `/users/{id}/roles`).                                                          |
| `UserRestRouter`'s self-applied middleware silently stops firing under the chosen humachi group pattern | Explicit spike item (3.0.3) before Slice 7 starts; if unsupported, convert to caller-applies convention as an isolated pre-step with its own test, not folded silently into the slice migration. |
| Multipart upload / binary download don't fit Huma's typed-body model cleanly                            | Explicit spike items (3.0.4, 3.0.5) before Slice 5 starts.                                                                                                                                       |
| Spec drift over time                                                                                    | Stale-spec CI guard (3.5).                                                                                                                                                                       |
| Coverage dips during rewrite                                                                            | Per-slice 95% gate blocks the step.                                                                                                                                                              |
| Big-bang temptation                                                                                     | Sequence is enforced one-slice-per-step; coexistence makes partial migration safe.                                                                                                               |
| Non-uniform envelopes get flattened into one generic shape, silently breaking a frontend consumer       | 3.1 explicitly enumerates all six current shapes; each keeps its own golden test — no shape is assumed "close enough" to another.                                                                |

## 8. Rollback

Each slice is an independent PR onto its feature branch. Because legacy and Huma
routing coexist, reverting a single slice's commit restores its chi handlers without
touching other slices. The foundation package is additive until Slice 1 adopts it.

## 9. Out of scope: `apps/orchestrator` / crawler-node — and a finding worth flagging separately

`CrawlerNodeRestRouter` (`service/crawlernode/services/CrawlerNodeRestRouter.go`) is
**not mounted in `apps/api`**. It is wired into the standalone `apps/orchestrator`
binary (`:8082`), which builds its own bare `chi.NewRouter()` with a lighter
middleware stack (`Recoverer`, `RequestID`, `Logger`, `Metrics` only — no `otelhttp`,
no `SecurityHeaders`, no `BodyLimit`) and mounts `r.Route("/node", nodeRouter.Register)`
and `r.Route("/admin/crawler-nodes", adminRouter.Register)` directly, bypassing the
`handler.RouteGroup`/`apiGroups`/`rootGroups` convention this plan builds on entirely.

Two consequences:

1. **Architecturally**, migrating it would mean standing up a second, independent
   Huma/OpenAPI surface on a different binary/process/port with a different
   middleware baseline — not a 13th slice in the same sequence. If/when this is
   pursued, it should be its own plan, not an appendage to this one (this corrects
   the 2026-06-13 draft, which listed crawler-node as Slice 13 in Section 5 while
   also declaring the orchestrator out of scope in Section 2 — an internal
   contradiction).
2. **Security finding, surfaced but not fixed here:** `/admin/crawler-nodes/*` on the
   orchestrator has **no `RequireRole` and no `AuditAdminMutation`** — confirmed by
   reading the orchestrator's route registration; only node-token auth
   (`RequireNodeToken`) exists, and only on `/node/*`, not `/admin/crawler-nodes/*`.
   This is a pre-existing gap, not something this plan introduces, but it is worth
   fixing (adding the same `RequireRole("admin")` + `AuditAdminMutation` gating that
   every other `/admin/*` route already has) as its **own small PR, independent of
   Huma**, and ideally _before_ any future decision to document this surface in a
   public-facing OpenAPI spec — publishing an ungated admin endpoint in machine-readable
   docs makes the gap easier to find, not harder.

---

## Appendix A — operation inventory (12 in-scope routers, current as of 2026-07-05)

**company (public)** — `GET /companies`, `GET /companies/{id}`
**job** — `POST /jobs/dismiss/bulk`, `GET /jobs`, `GET /jobs/companies`,
`GET /jobs/counts`, `GET /jobs/{id}`, `DELETE /jobs/{id}`, `PATCH /jobs/{id}/save`,
`PATCH /jobs/{id}/status`, `PATCH /jobs/{id}/notes`
**application** — `GET /jobs/{id}/applications`, `POST /jobs/{id}/applications`,
`PATCH /jobs/{id}/applications/{app_id}/stage`,
`PATCH /jobs/{id}/applications/{app_id}/note`, `GET /applications`,
`PATCH /applications/{id}/stage`
**apply-queue** — `GET /apply-queue`, `PUT /apply-queue/{jobId}/resume`
**resume** — `GET /resumes`, `POST /resumes`, `GET /resumes/{id}/download`,
`PATCH /resumes/{id}`, `DELETE /resumes/{id}`
**scan** — `GET /companies/queue`, `DELETE /companies/queue/{entry_id}`,
`POST /companies/scan-all`, `GET /companies/{id}/scan-stats`,
`POST /companies/{id}/scan`, `GET /companies/{id}/analytics`,
`GET /companies/{id}/scan-config`, `PATCH /companies/{id}/scan-interval-override`
**user /me** — `GET /me`, `PATCH /me`, `DELETE /me`, `GET /me/config`, `PATCH /me/config`
**ats** — `GET /ats`, `GET /ats/{name}`, `PATCH /ats/{name}` (all `/admin/*`)
**pipeline** — `GET /pipeline/stats`, `POST /pipeline/retry`
**company (admin)** — `GET /companies`, `POST /companies`, `PATCH /companies/active`,
`GET /companies/{id}`, `PATCH /companies/{id}`,
`PATCH /companies/{id}/scan-settings`, `DELETE /companies/{id}`
**user (admin)** — `GET /users`, `GET /users/{id}`, `PATCH /users/{id}/roles`,
`PATCH /users/{id}/status`, `DELETE /users/{id}`
**audit (admin)** — `GET /audit-log`

**Out of scope (Section 9)** — crawler-node (admin): `GET /`, `POST /`, `PATCH /{id}`,
`DELETE /{id}` (under `/admin/crawler-nodes`, orchestrator binary); crawler-node
(node): `GET /work`, `POST /results`, `POST /error` (under `/node`, orchestrator
binary, `RequireNodeToken`).

## Appendix B — edge cases that must each keep a test

- Job list: experience-range parse (`"0-2"`, `"5+"`, invalid → dropped), `sort_by`
  whitelist rejects unknown columns, bool query coercion (`remote`/`sponsorship`/
  `requires_citizen`/`on_site`/`hybrid`/`no_requirements`), `days_added` > 0, default
  limit 20 / clamp 1–100, keyset `cursor` round-trip + `next_cursor` echo.
- DELETE `/jobs/{id}`: empty body → default `DismissalReasonUserDismissed`; explicit
  reason honored; 204 No Content.
- Bulk dismiss: empty `reason` → default `DismissalReasonNotRelevant`; count echoed.
- Apply-queue list: `filter` defaults to `FilterAll` when omitted; migrated `limit`
  default (20) and clamp must match the shared `request.ParsePagination` behavior
  exactly, since this slice collapses a previously-duplicated implementation (3.4).
- Resume upload: missing `file` field → 400 "file is required"; oversized body beyond
  `multipartMaxMemory` (4MB) → 400 "invalid multipart form"; content-type/size on the
  stored resume match the uploaded file.
- Resume download: `Content-Type`, `Content-Disposition` (correct filename escaping
  via `mime.FormatMediaType`), and `Content-Length` all match the stored resume
  metadata; a read failure mid-stream does not change the already-committed 200
  status (matches existing documented behavior, not a regression).
- Invalid JSON body → 400 `BAD_REQUEST` (every PATCH/POST).
- Service errors map to correct status/code (`ServiceError` → its Status;
  `ValidationErrors` → 400 field list; unmapped → opaque 500 + logged).
- Auth: unauthenticated `/api/*` → 401; non-admin `/admin/*` → 403.
- Admin mutation on 2xx still appends `admin_audit_log` (AuditAdminMutation fires) —
  applies to `ats` PATCH, `pipeline` retry, `company admin` writes, `user admin`
  writes.
- Route ordering: literal segments (`/companies/active`, `/companies/queue`,
  `/companies/scan-all`, `/users/{id}/roles`, `/users/{id}/status`) resolve before
  `/{id}`.
- Pagination envelope shape unchanged per-shape (3.1): keyset (`next_cursor`),
  legacy page+offset (`{data,total,page,limit}`), `publicListEnvelope`,
  `adminListEnvelope` (with `total_active`), and job list's raw-DTO shape are each
  independently verified — no shape is assumed interchangeable with another.
- `UserRestRouter` `/me` routes: confirm the migrated slice still enforces
  RequireIdentity + RateLimit even though this router applies its own middleware
  rather than relying on the caller's `.Use()` (3.0 item 3).
