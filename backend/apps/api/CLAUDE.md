# apps/api/ — REST API server

HTTP server on `:8080`. Mounts all service routers, handles graceful shutdown.

## Key files

| File          | Role                                                         |
| ------------- | ------------------------------------------------------------ |
| `main.go`     | Entry point — wire, build router, serve, shutdown            |
| `wire.go`     | Wire provider declarations (build tag: `wireinject`)         |
| `wire_gen.go` | Generated wire code (build tag: `!wireinject`) — do not edit |
| `app.go`      | `App` struct — holds all route group handlers                |
| `router.go`   | Mounts all sub-routers, adds middleware                      |

## Sub-packages

| Package       | Role                                                                   |
| ------------- | ---------------------------------------------------------------------- |
| `handler/`    | `Respond[T]`, `RespondPaged[T]`, `RespondError`, `HandleError` helpers |
| `middleware/` | See **middleware** section below                                       |
| `request/`    | Pagination parsing, query param helpers                                |
| `response/`   | Shared response envelope types                                         |

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

## Rules

- All handlers use `handler.Respond`, `handler.RespondPaged`, or `handler.HandleError` — never raw `json.NewEncoder`
- Route registration belongs in each service's `RestRouter` — `router.go` only mounts them
- `wire_gen.go` is manually maintained (Wire codegen not used); update it when adding or removing providers in `wire.go`
- 5-second graceful shutdown timeout on SIGINT/SIGTERM
- `main.go` owns dashboard materialized view refresh via a background goroutine (`runRefresher`): pipeline stats every 3 min, heatmap every 5 min
