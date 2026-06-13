# Migration plan — move the REST API from raw chi handlers to Huma (OpenAPI 3.1)

Tracking issue: [#157](https://github.com/trushilpatel/JobRadar/issues/157)
Status: **active** (delete this file when the migration merges)

## 1. Goal

Give the API a machine-readable, drift-proof **OpenAPI 3.1** contract by registering
every endpoint through [Huma](https://github.com/danielgtaylor/huma) on top of the
**existing chi router** (`humachi` adapter). No router replacement: chi stays, every
`func(http.Handler) http.Handler` middleware stays, the `handler.RouteGroup` seam stays.

The migration is **strictly sequential, one slice at a time**. A slice is not "done"
until its full quality gate (Section 4) is green; only then does the next slice start.

## 2. Scope — what is migrating

13 `RestRouter`s, ~70 operations, across two middleware zones.

| Zone | Slice | Router file | Ops |
| --- | --- | --- | --- |
| `/api/*` (RequireIdentity + RateLimit) | company (public) | `service/company/services/CompanyRestRouter.go` | 2 |
| | job | `service/job/services/JobRestRouter.go` | 9 |
| | application | `service/application/services/ApplicationRestRouter.go` | 6 |
| | dashboard | `service/dashboard/services/DashboardRestRouter.go` | 8 |
| | insights | `service/insights/services/InsightsRestRouter.go` | 7 |
| | scan | `service/scan/services/ScanRestRouter.go` | 8 |
| | ats | `service/ats/services/AtsRestRouter.go` | 3 |
| | user `/me` | `service/user/services/UserRestRouter.go` | 5 |
| `/admin/*` (RequireRole + AuditAdminMutation) | company (admin) | `service/company/services/CompanyAdminRestRouter.go` | 7 |
| | user (admin) | `service/user/services/UserAdminRestRouter.go` | 5 |
| | audit | `service/audit/services/AuditAdminRestRouter.go` | 1 |
| | dashboard pipeline | `service/dashboard/services/DashboardPipelineRestRouter.go` | 2 |
| | crawler-node (admin) | `service/crawlernode/services/CrawlerNodeRestRouter.go` | 4 |
| `/node/*` (RequireNodeToken) | crawler-node (node-facing) | same file | 3 |

Full per-operation inventory in **Appendix A**.

**Out of scope:** the orchestrator HTTP server (`apps/orchestrator/`) — it is an
internal node API, not the user-facing REST surface. It can adopt Huma later as a
follow-up; this plan covers `apps/api/` only.

## 3. Foundation (Phase 0) — build before touching any slice

Phase 0 is the de-risking step. Nothing else starts until these land and are tested.
Its job is to lock four contracts so each slice migration is mechanical and identical.

### 3.0 Spike (timeboxed, throwaway)

Stand up one Huma operation on a throwaway route to confirm, in *this* codebase:
- `humachi.New` registering onto an existing chi subrouter still runs the chi
  middleware already attached via `.Use(...)` (so `RequireIdentity`, `RateLimit`,
  `RequireRole`, `AuditAdminMutation` are reused unchanged — no rewrite to Huma
  middleware).
- Multiple humachi adapters (one per zone) can write into **one shared
  `*huma.OpenAPI`** so a single `/openapi.json` is produced. If sharing one doc
  across adapters proves awkward, fall back to `huma.NewGroup(api, "/api")` +
  `group.UseMiddleware(...)` with net/http middleware wrapped via a small adapter.
  **Pick one approach here and record it in this doc before Step 1.**

### 3.1 Response envelope compatibility (load-bearing)

The frontend (`frontend/src/lib/core/api-client.ts`) unwraps `{data}` and
`{data,total,page,limit}`. Huma serializes the output struct's `Body` directly, so
to keep the frontend **untouched**, Huma outputs must reuse the existing envelopes
from `apps/api/response/response.go`:

- single resource → `Body response.Response[T]`
- paged list → `Body response.PagedResponse[T]`

Add a generic helper package (e.g. `apps/api/huma/respond.go`) exposing typed output
wrappers so every slice produces byte-identical envelopes. A **golden test** asserts
the migrated bytes equal the pre-migration bytes for at least one op per shape.

### 3.2 Error model compatibility

Current contract (`apps/api/handler/respond.go`): `{error:{code,message}}` for
service errors, `{errors:[{field,code,message}]}` for validation, consumed by
`frontend/src/lib/core/api-error.ts`.

Override the global `huma.NewError` once (in the foundation package) to emit these
exact envelopes instead of RFC 9457 `problem+json`. Provide a single
`huma`-side adapter that maps `apperr.ValidationErrors`, `*apperr.ValidationError`,
and `*apperr.ServiceError` to the right status/body — i.e. port the `HandleError`
switch into the Huma error path so the mapping lives in one place.

> Decision to surface to the owner: **keep the legacy error envelope** (zero frontend
> change, recommended for this migration) **vs. adopt RFC 9457** (cleaner, but requires
> updating `api-error.ts` + its tests). Default to keep-legacy unless told otherwise.

### 3.3 Middleware bridging

Keep all middleware as chi middleware on the chi subrouters that already carry them
(`NewRouter` in `apps/api/router.go` already builds the `/api` subtree with
identity+ratelimit; `/admin` groups add role+audit). Huma operations register onto
those same subrouters via humachi, so the middleware runs unchanged. **No middleware
is rewritten.** Handlers read identity from `ctx` exactly as today
(`identity.FromContext`), since Huma passes the request context through.

### 3.4 Input parsing parity

Huma parses path/query/body into a typed input struct via `path:`, `query:`, `json:`
tags and validates from struct tags. Per slice, the manual parsing in each
`RestService` (e.g. the rich `dto.JobFilter` query parsing in
`service/job/services/JobRestService.go`) moves into a Huma input struct. Edge cases
(experience-range parsing `"0-2"`/`"5+"`, `sort_by` whitelist, bool query coercion,
empty-body DELETE defaults) must each keep a test — see Appendix B.

### 3.5 Doc + spec serving + CI guard

- Serve `/openapi.json`, `/openapi.yaml`, and a docs UI on the **ops listener**
  (`:9090`, `NewOpsRouter`) so docs never traverse auth/rate-limit. Confirm no auth
  leak (the spec must not expose admin-only schemas to anonymous users if that is a
  concern — gate behind ops listener which is already internal).
- Add a CI step that **fails the build if the committed `openapi.yaml` is stale**
  (regenerate in CI, `git diff --exit-code`). This is the "generated/enforced contract
  over convention" guarantee from `CLAUDE.md` — the spec can never silently drift.

### Phase 0 Definition of Done
Spike approach recorded here · envelope + error helpers built and unit-tested ·
golden envelope test green · one throwaway op served end-to-end through real
middleware · `/openapi.json` served · stale-spec CI guard added · lint + race + 95%
coverage on the new foundation package.

## 4. Per-slice quality gate (the "test all before moving on" rule)

Every slice step below is **only complete** when **all** of these pass. This is the
gate the request asks for — no slice advances on a red gate.

1. Slice `tests/` rewritten for the Huma handlers; **every prior behavior still
   covered** (status codes, error mapping, validation, pagination, auth-context use).
2. New **contract test**: the slice's operations appear in the generated OpenAPI with
   correct method/path/params/request+response schema.
3. **Envelope parity**: golden test proves response bytes are unchanged vs. legacy
   (frontend stays untouched).
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

| # | Slice | Why this position |
| --- | --- | --- |
| 1 | **company (public)** — 2 read ops | Pilot. Pure reads; validates envelope + spec + path param + frontend-untouched with minimal blast radius. |
| 2 | **job** — 9 ops | Hardest input surface (rich filters, keyset cursor, PATCH/DELETE bodies, 204, bulk). Do early so input-parity helpers are proven before the long tail. |
| 3 | **application** — 6 ops | Nested path params (`/jobs/{id}/applications/{app_id}/...`) + writes. |
| 4 | **dashboard (public)** — 8 read ops | Bulk of simple GETs; fast once the pattern is set. |
| 5 | **insights** — 7 read ops | Same shape as dashboard. |
| 6 | **scan** — 8 ops | Mixed reads + scan-trigger writes; path + body. |
| 7 | **ats** — 3 ops | Public reads + one admin PATCH; first taste of mixed gating in one slice. |
| 8 | **user `/me`** — 5 ops | Sub-route middleware nuance (`UserRestRouter` applies its own `.Use` on `/me`); validate group middleware bridging. |
| 9 | **company (admin)** — 7 ops | First full `/admin` zone slice: RequireRole + AuditAdminMutation must still fire on 2xx mutations. |
| 10 | **user (admin)** — 5 ops | Admin zone; literal-vs-`{id}` route ordering must be preserved. |
| 11 | **audit (admin)** — 1 op | Trivial; confirms admin reads. |
| 12 | **dashboard pipeline (admin)** — 2 ops | Admin read + retry write. |
| 13 | **crawler-node** — 4 admin + 3 node ops | **Last.** Special `RequireNodeToken` auth on `/node/*` and bespoke router-held middleware; highest auth risk. |

Rule between steps: legacy chi routes and Huma routes **coexist** on the same mux
during the migration (Huma registers specific paths; untouched slices keep their chi
registration). Never migrate two slices in one step.

## 6. Cleanup (Phase F) — after slice 13 is green

1. Remove now-dead helpers if fully unused (`handler.Respond*` may be replaced by the
   Huma output wrappers; keep only what's still referenced). Verify with `grep`.
2. Confirm `handler.RouteGroup`/`router.go` shrink to Huma registration only; update
   `apps/api/CLAUDE.md` and `service/CLAUDE.md` to describe the Huma pattern as the
   new standard for the `RestService`/`RestRouter` layers.
3. (Optional, high payoff) Generate a typed TypeScript client for the frontend from
   `openapi.yaml` and migrate `frontend/src/lib/api/*` onto it — separate PR.
4. Update the `dev-ergonomics` / engineering-principles docs note that new endpoints
   are now "one Huma operation + one input/output struct" (extensibility-by-design).
5. Delete this plan file.

## 7. Risks & mitigations

| Risk | Mitigation |
| --- | --- |
| Envelope/error drift breaks the frontend silently | Golden byte tests (3.1) + keep-legacy error model (3.2); frontend untouched until Phase F. |
| Middleware not running on Huma routes (auth bypass) | 3.0 spike proves chi middleware fires; gate item 8 smoke-tests an unauthenticated request returns 401/403 on a migrated `/api` and `/admin` route. |
| chi route-ordering semantics (literal before `{id}`) lost | Huma/chi matches on registration; preserve order; add a test hitting the literal route (e.g. `/companies/active`, `/users/{id}/roles`). |
| Spec drift over time | Stale-spec CI guard (3.5). |
| Coverage dips during rewrite | Per-slice 95% gate blocks the step. |
| Big-bang temptation | Sequence is enforced one-slice-per-step; coexistence makes partial migration safe. |

## 8. Rollback

Each slice is an independent PR onto its feature branch. Because legacy and Huma
routing coexist, reverting a single slice's commit restores its chi handlers without
touching other slices. The foundation package is additive until slice 1 adopts it.

---

## Appendix A — operation inventory

**company (public)** — `GET /companies`, `GET /companies/{id}`
**job** — `POST /jobs/dismiss/bulk`, `GET /jobs`, `GET /jobs/companies`,
`GET /jobs/counts`, `GET /jobs/{id}`, `DELETE /jobs/{id}`, `PATCH /jobs/{id}/save`,
`PATCH /jobs/{id}/status`, `PATCH /jobs/{id}/notes`
**application** — `GET /jobs/{id}/applications`, `POST /jobs/{id}/applications`,
`PATCH /jobs/{id}/applications/{app_id}/stage`,
`PATCH /jobs/{id}/applications/{app_id}/note`, `GET /applications`,
`PATCH /applications/{id}/stage`
**dashboard** — `GET /dashboard/{stats,activity,progress,market,heatmap,interviews,velocity,tech-trends}`
**insights** — `GET /insights/{tech-stack,company-activity,funnel,location,sponsorship,velocity,role-categories}`
**scan** — `GET /companies/queue`, `DELETE /companies/queue/{entry_id}`,
`POST /companies/scan-all`, `GET /companies/{id}/scan-stats`,
`POST /companies/{id}/scan`, `GET /companies/{id}/analytics`,
`GET /companies/{id}/scan-config`, `PATCH /companies/{id}/scan-interval-override`
**ats** — `GET /ats`, `GET /ats/{name}`, `PATCH /ats/{name}`
**user /me** — `GET /me`, `PATCH /me`, `DELETE /me`, `GET /me/config`, `PATCH /me/config`
**company (admin)** — `GET /companies`, `POST /companies`, `PATCH /companies/active`,
`GET /companies/{id}`, `PATCH /companies/{id}`,
`PATCH /companies/{id}/scan-settings`, `DELETE /companies/{id}`
**user (admin)** — `GET /users`, `GET /users/{id}`, `PATCH /users/{id}/roles`,
`PATCH /users/{id}/status`, `DELETE /users/{id}`
**audit (admin)** — `GET /audit-log`
**dashboard pipeline (admin)** — `GET /dashboard/pipeline`, `POST /dashboard/pipeline/retry`
**crawler-node (admin)** — `GET /` , `POST /`, `PATCH /{id}`, `DELETE /{id}` (under `/admin/crawler-nodes`)
**crawler-node (node)** — `GET /work`, `POST /results`, `POST /error` (under `/node`, RequireNodeToken)

## Appendix B — edge cases that must each keep a test

- Job list: experience-range parse (`"0-2"`, `"5+"`, invalid → dropped), `sort_by`
  whitelist rejects unknown columns, bool query coercion (`remote`/`sponsorship`/
  `requires_citizen`/`on_site`/`hybrid`/`no_requirements`), `days_added` > 0, default
  limit 20 / clamp 1–100, keyset `cursor` round-trip + `next_cursor` echo.
- DELETE `/jobs/{id}`: empty body → default `DismissalReasonUserDismissed`; explicit
  reason honored; 204 No Content.
- Bulk dismiss: empty `reason` → default `DismissalReasonNotRelevant`; count echoed.
- Invalid JSON body → 400 `BAD_REQUEST` (every PATCH/POST).
- Service errors map to correct status/code (`ServiceError` → its Status;
  `ValidationErrors` → 400 field list; unmapped → opaque 500 + logged).
- Auth: unauthenticated `/api/*` → 401; non-admin `/admin/*` → 403; node routes
  reject missing/invalid token.
- Admin mutation on 2xx still appends `admin_audit_log` (AuditAdminMutation fires).
- Route ordering: literal segments (`/companies/active`, `/companies/queue`,
  `/companies/scan-all`, `/users/{id}/roles`, `/users/{id}/status`) resolve before
  `/{id}`.
- Pagination envelope shape unchanged for both keyset (`next_cursor`) and legacy
  paged (`{data,total,page,limit}`) endpoints.
