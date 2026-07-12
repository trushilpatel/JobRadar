# 2026-07-06 — chi → Huma migration (OpenAPI 3.1)

Every REST router in `apps/api` and `apps/orchestrator` now registers through
[Huma](https://github.com/danielgtaylor/huma) instead of raw chi handlers, giving both
binaries a generated, drift-guarded OpenAPI 3.1 contract. Full plan and per-slice
findings: `docs/superpowers/plans/2026-06-13-huma-migration-plan.md`.

## What was built

**Foundation (`backend/apps/api/apihuma/`, `backend/apps/api/apispec/`)**

- `apihuma/spec.go` — `NewSharedRegistry`/`NewZoneConfig` (one schema registry shared
  across zones, one independent `huma.Config`/document per middleware zone —
  relative-path collisions are a real per-zone-document constraint, not a shared one),
  `MergeSpecs`/`MountCombinedSpec` (prefix-merge per-zone docs into one served spec)
- `apihuma/envelope.go` — `SingleOutput`/`PagedOutput`/`CursorPageOutput`/
  `AdminCursorPageOutput` output wrappers reproducing every pre-Huma envelope shape
  byte-for-byte (`AdminCursorPageOutput` wraps in an extra `response.Response` layer —
  a real Phase 0 bug, found and fixed in Slice 10, see below)
- `apihuma/error.go` — `InstallLegacyErrorModel`/`MapError`, preserving the legacy
  `{"error":{"code","message"}}` and `{"errors":[...]}` shapes for both domain errors
  and Huma's own framework-level validation failures
- `apispec/apispec.go` — `APIZoneConfig()`/`AdminZoneConfig()`, the two zone configs
  every `apps/api` slice's `RestRouter` constructor takes
- `apps/api/genspec.go` — `--print-spec` CLI mode regenerating `apps/api/openapi.yaml`
  from the same wiring `main()` uses; CI (`.github/workflows/ci.yml`) regenerates and
  diffs it on every run

**All 12 `apps/api` slices migrated** (one commit each; `service/{domain}/services/
{Domain}RestRouter.go` + `{Domain}RestService.go` rewritten, `dto/` structs gained
`required:"false"` tags wherever the pre-Huma handler tolerated an absent JSON key):
company (public + admin), job, application, apply-queue, resume (multipart upload +
binary streamed download via `Body func(huma.Context)`), scan, user (`/me` + admin),
ats (first `/admin`-zone slice), pipeline, audit.

**`apps/orchestrator` — security fix + separate Huma migration**

- `apps/orchestrator/wire_admin_gate.go` — closed a real, pre-existing gap:
  `/admin/crawler-nodes/*` had zero admin authentication (only node-token auth on
  `/node/*`). Now gated by `RequireIdentity` → `RequireRole("admin")` →
  `AuditAdminMutation`, the same stack every `apps/api` `/admin/*` route uses. JWKS
  init failure is deliberately non-fatal at boot (unlike `apps/api`'s identical call)
  so `/node/*` (the core crawl pipeline) never depends on Clerk availability
- `apps/orchestrator/wire_app.go`, `wire_spec.go`, `genspec.go` — `NodeRestRouter`/
  `AdminRestRouter` (`service/crawlernode/services/`) migrated to Huma with their own
  independent OpenAPI document (`apps/orchestrator/openapi.yaml`) — a separate
  binary/process/port, not merged into `apps/api`'s spec

**Frontend (`frontend/src/lib/api/schema.gen.ts`)**

- `openapi-typescript` generates TS types from `apps/api/openapi.yaml`
  (`npm run gen:api-types`), CI-guarded against drift the same way
- Existing zod-schema-per-endpoint pattern kept for response parsing (the runtime
  validation layer); generated types applied to a few request bodies via `satisfies`
  as the adoption pattern (`features/auth/api/adminUsers.api.ts`, `lib/api/pipeline.ts`)

## Coverage achieved

- Backend: 3859 tests, race-detector clean, every package ≥95% (97.1% overall)
- Frontend: 1274 tests passing, typecheck + lint clean

## Key invariants / findings worth remembering

- `apihuma.NewZoneConfig` must stay safe under concurrent calls — a `sync.Once`-guarded
  install of the legacy error model fixed a genuine data race (`t.Parallel()` tests
  each building their own zone config), caught by the full suite, not any one slice
- Huma's schema registry names by bare Go type name, not package-qualified — two
  domains defining the same local output-wrapper type name (e.g. `idResponse`)
  collide the moment both routers share `apispec.sharedRegistry`; always prefix
  locally-defined Huma output types with the domain
- `apispec.APIZoneConfig()`/`AdminZoneConfig()` (and orchestrator's equivalents) are
  production singletons — never call them from a test; use
  `apihuma.NewZoneConfig("test", "0.0.0", apihuma.NewSharedRegistry())` instead
- Admin-zone routers register **relative** paths on whatever router they're given —
  the `/admin` prefix comes only from the caller's mount point and the zone-merge at
  spec-build time, never from the router itself
