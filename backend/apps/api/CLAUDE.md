# apps/api/ — REST API server

HTTP server on `:8080`. Mounts all service routers, handles graceful shutdown.

Every `RestRouter`/`RestService` pair in `service/*/services/` registers through
[Huma](https://github.com/danielgtaylor/huma) (`humachi` adapter on top of the
existing chi router) — see `docs/superpowers/plans/2026-06-13-huma-migration-plan.md`
for the completed migration and its per-slice findings. This is now the standard
pattern for every route in this codebase (public and admin); see "Adding a new
route" below.

## Key files

| File                 | Role                                                                                                                                                                                                              |
| -------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `main.go`            | Entry point — wire, build router, serve, shutdown                                                                                                                                                                 |
| `wire.go`            | Wire provider declarations (build tag: `wireinject`) — documents intended composition; see note below                                                                                                             |
| `wire_gen.go`        | **Hand-maintained**, despite the `!wireinject` build tag and Wire's usual convention — see note below                                                                                                             |
| `wire_multiuser.go`  | `buildMultiUser` — constructs the /me and /admin/\* decorator chains + route groups; also owns `adminRouteGroup`, which mounts every admin router under one `/admin` chi subrouter with the full middleware chain |
| `multiuser_setup.go` | `multiUser` struct + accessors (`APIHandlers()`, `AdminHandlers()`, middleware getters) — construction lives in `wire_multiuser.go`, not here                                                                     |
| `app.go`             | `App` struct — holds all route group handlers                                                                                                                                                                     |
| `router.go`          | Mounts all sub-routers, adds middleware — Huma-agnostic: it only calls `RouteGroup.Register`, unaware that the group registers via Huma internally                                                                |
| `genspec.go`         | `--print-spec` CLI mode — regenerates `openapi.yaml` from the exact same wiring `main()` uses; the stale-spec CI guard                                                                                            |

## Sub-packages

| Package       | Role                                                                                                                                                                                                                                                                                                                                                           |
| ------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `apihuma/`    | Huma migration foundation — `NewZoneConfig`/`NewSharedRegistry` (one config per middleware zone, one schema registry shared across zones), `SingleOutput`/`PagedOutput`/`CursorPageOutput`/`AdminCursorPageOutput` (output wrappers reproducing legacy envelope shapes byte-for-byte), `MapError`/`InstallLegacyErrorModel` (legacy error envelope)            |
| `apispec/`    | `APIZoneConfig()`/`AdminZoneConfig()` — the two zone configs every RestRouter shares; `Build()` merges both zones' operations into the combined served spec                                                                                                                                                                                                    |
| `handler/`    | `Respond[T]`, `RespondPaged[T]`, `RespondError`, `HandleError` helpers — legacy pre-Huma helpers, still used by non-migrated code (`service/crawlernode/`, `apps/orchestrator/`) and by middleware that runs before Huma routing (auth/rate-limit/role failures); never add a new Huma-registered handler using these — use the `apihuma` output types instead |
| `middleware/` | See **middleware** section below                                                                                                                                                                                                                                                                                                                               |
| `request/`    | Pagination parsing, query param helpers — `ParsePage`/`ParseLimit` are also called directly from Huma input `Resolve()` methods to reproduce the same fallback/clamp semantics                                                                                                                                                                                 |
| `response/`   | Shared response envelope types — reused directly by `apihuma`'s output wrappers                                                                                                                                                                                                                                                                                |

## Middleware

| Name                                                     | Role                                                                           |
| -------------------------------------------------------- | ------------------------------------------------------------------------------ |
| Request logger / metrics / body limit / security headers | Always-on chain                                                                |
| `Auth`                                                   | JWT verify + dev-bypass; exports `VerifyJWT(r)` helper                         |
| `RequireIdentity`                                        | DB-backed enrichment + cache-aside + lazy provisioning + status check          |
| `RequireRole(role)`                                      | Role gate for admin routes                                                     |
| `RateLimit(limiter)`                                     | Per-user, method-derived scope; fail-open on Redis outage; 429 + `Retry-After` |
| `AuditAdminMutation(svc)`                                | Appends `admin_audit_log` on 2xx mutating responses                            |

## Route structure

- `/api/*` — all user-facing routes; `RequireIdentity` + `RateLimit` applied globally via `NewRouter`
- `/admin/*` — admin-only routes; `RequireIdentity` + `RequireRole("admin")` + `AuditAdminMutation` applied per-route group
- `/health`, `/ready`, `/metrics` — operational endpoints; no auth; served on ops listener (`:9090`)

## `wire_gen.go` cannot actually be regenerated by Wire — it is hand-maintained

`go generate ./apps/api/` (equivalently, running `wire` in this directory) **fails** with "multiple bindings" for the chain interface in every single domain (`company` both chains, `scan`, `job`, `application`, `resume`, `applyqueue`, ...). Wire's DI graph has no built-in way to auto-resolve a linear chain of decorators that all produce the same interface (`RepositoryService → ImplService → ValidationService → RestService`, all returning e.g. `CompanyService`) — from Wire's point of view that's just N ambiguous providers for one type, and it refuses to guess an order. Every domain's `wire.go` `ProviderSet` therefore documents the _intended_ composition only; it is never actually fed through Wire's codegen. `wire_gen.go` is edited by hand whenever the wiring changes, same as any other Go file — the `!wireinject` build tag and the file's own "generated" header are historical, not currently accurate.

**Practical fix for a linear decorator chain would be:** one hand-written composing function per domain (e.g. `func ProvideCompanyService(db *gorm.DB, c cache.Cache) CompanyService { return NewValidationService(NewCacheService(NewImplService(NewRepositoryService(db)), c, ttl)) }`) so Wire only ever sees one provider per interface — the standard pattern for decorator chains with Wire. Nobody has done this refactor yet; see `docs/tech-debt/2026-07-05-backend-service-boilerplate-debt.md` for the tracked finding. Do not attempt a partial fix (e.g. "just this one domain") — it would leave the codebase with two different composition strategies and no clear rule for which one a new domain should follow.

## Rules

- Every new operation is a Huma operation: `huma.Register(api, huma.Operation{...}, handlerFunc)` in the domain's `RestRouter.Register`, with `handlerFunc(ctx context.Context, in *someInput) (*someOutput, error)` in `RestService` — never raw `http.HandlerFunc`, never raw `json.NewEncoder`
- `RestRouter.Register` takes the shared zone `huma.Config` (`apispec.APIZoneConfig()` for `/api`, `apispec.AdminZoneConfig()` for `/admin`) via its constructor and calls `humachi.New(r, zone)` once at the top of `Register` — every operation in that router registers onto the same `api` handle
- Output types reuse `apihuma.SingleOutput[T]` / `PagedOutput[T]` / `CursorPageOutput[T]` / `AdminCursorPageOutput[T]` wherever the legacy envelope shape matches one of them; only hand-roll a `Body func(huma.Context)` output for genuinely non-JSON or conditional-status responses (binary downloads, multipart uploads, one operation with two different success shapes) — see `service/resume/`, `service/applyqueue/` for examples
- Errors return `apihuma.MapError(err)`, never a raw error or `handler.HandleError`
- Body struct fields need `required:"false"` whenever the pre-Huma handler tolerated an absent key (i.e. almost every optional-in-practice field) — Huma marks Body fields required by default, unlike `encoding/json`'s zero-value-on-absence behavior
- **Never name a local Huma output-wrapper type generically** (`idResponse`, `response`, `result`, ...) — Huma's schema registry names by bare Go type name, not package-qualified, and `apispec.sharedRegistry` is shared across every zone; a same-named type in another domain's package collides at spec-build time (a real bug found in Slice 11, see the migration plan's findings). Prefix with the domain (`companyIDResponse`, `userIDResponse`)
- `router.go` only mounts route groups — it is Huma-agnostic and never calls `huma.Register` itself
- 5-second graceful shutdown timeout on SIGINT/SIGTERM

## Adding a new route

1. Add the operation to the domain's `RestRouter.Register` (`huma.Operation{OperationID, Method, Path, ...}` + handler function) and implement the handler in `RestService` per the Rules above
2. Add input/output types (Body structs, query/path param structs, output wrappers) in the same `RestService.go` file, or reuse existing `dto/` types when the shape already matches
3. Hand-wire the constructor into `apps/api/wire_gen.go` directly (see the note above — there is no working `go generate` step); public/`/api`-zone routers thread `apiZoneCfg`, admin/`/admin`-zone routers thread `adminZoneCfg` (built once in `main.go`/`genspec.go` via `apispec.APIZoneConfig()`/`AdminZoneConfig()`)
4. Mount in `apps/api/router.go` (public `/api` groups) or `apps/api/wire_multiuser.go`'s `adminGroup.handlers` (admin `/admin` groups) — `multiuser_setup.go` only holds the `multiUser` struct/accessors, not construction
5. Add a contract test (`{Domain}RestRouter_contract_test.go`) asserting the operation appears in the generated spec with the right method/path/OperationID, plus edge-case tests per the usual per-slice quality gate
6. Regenerate `apps/api/openapi.yaml` via `go run ./apps/api --print-spec` and commit it alongside the code change — CI's stale-spec guard fails the build if the committed spec drifts from what `--print-spec` produces
