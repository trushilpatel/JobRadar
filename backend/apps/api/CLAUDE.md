# apps/api/ — REST API server

HTTP server on `:8080`. Mounts all service routers, handles graceful shutdown.

## Key files

| File           | Role                                                              |
| -------------- | ----------------------------------------------------------------- |
| `main.go`      | Entry point — wire, build router, serve, shutdown                 |
| `wire.go`      | Wire provider declarations (build tag: `wireinject`)              |
| `wire_gen.go`  | Generated wire code (build tag: `!wireinject`) — do not edit      |
| `app.go`       | `App` struct — holds all route group handlers                     |
| `router.go`    | Mounts all sub-routers, adds middleware                           |

## Sub-packages

| Package      | Role                                                                        |
| ------------ | --------------------------------------------------------------------------- |
| `handler/`   | `Respond[T]`, `RespondPaged[T]`, `RespondError`, `HandleError` helpers      |
| `middleware/`| Request logger, metrics, body limit, security headers, `Auth` / `RequireAuth` (legacy email allowlist via `ALLOWED_EMAIL`), exported `VerifyJWT(r)` helper, plus Phase 1: `RequireIdentity` (DB-backed enrichment + cache-aside + lazy provisioning + status check + `ADMIN_EMAILS` auto-grant), `RequireRole(role)`, `RateLimit(limiter)` (per-user, method-derived scope, fail-open on Redis outage, 429 + `Retry-After`), `AuditAdminMutation(svc)` (appends `admin_audit_log` on 2xx mutating responses) |
| `request/`   | Pagination parsing, query param helpers                                     |
| `response/`  | Shared response envelope types                                              |

## Rules

- All handlers use `handler.Respond`, `handler.RespondPaged`, or `handler.HandleError` — never raw `json.NewEncoder`
- Route registration belongs in each service's `RestRouter` — `router.go` only mounts them
- `wire_gen.go` is auto-generated — regenerate with `go generate ./apps/api/` after changing `wire.go`
- 5-second graceful shutdown timeout on SIGINT/SIGTERM
- `main.go` owns dashboard materialized view refresh via a background goroutine (`runRefresher`): pipeline stats every 3 min, heatmap every 5 min
