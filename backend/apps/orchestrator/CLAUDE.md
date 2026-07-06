# apps/orchestrator/ — Crawler orchestrator HTTP server

HTTP server on `:8082`. Assigns `company_scan_queue` rows to crawler-nodes, receives crawl results, and publishes raw job listings to NATS JetStream.

Every route registers through [Huma](https://github.com/danielgtaylor/huma) (`humachi` adapter), like `apps/api`'s slices — but this is its **own, independent** OpenAPI document (`apps/orchestrator/openapi.yaml`), not merged into `apps/api`'s spec. See `docs/superpowers/plans/2026-06-13-huma-migration-plan.md` Section 9 for why this binary is a separate migration, not a 13th slice.

## Key files

| File                 | Role                                                                                                                                              |
| -------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------- |
| `main.go`            | Wiring + HTTP server startup + `runResetStuck` background ticker + graceful shutdown; also handles `--print-spec`                                 |
| `wire_app.go`        | `wireApp` — composes the crawlernode decorator chain, both REST routers, and the admin auth gate; shared by `main()` and `genspec.go`             |
| `wire_spec.go`       | `nodeZoneConfig()`/`adminZoneConfig()` — the two zone configs `NodeRestRouter`/`AdminRestRouter` share; `buildOrchestratorSpec` merges them       |
| `wire_admin_gate.go` | `wireAdminGate` — mounts the admin-facing crawlernode router behind `RequireIdentity` -> `RequireRole("admin")` -> `AuditAdminMutation`           |
| `genspec.go`         | `--print-spec` CLI mode — regenerates `openapi.yaml` from the exact same `wireApp` wiring `main()` uses, against an in-memory DB + noop publisher |
| `middleware/`        | Node token authentication — verifies bearer tokens against `crawler_nodes.token_hash` (HMAC-SHA256)                                               |
| `nodeidentity/`      | Context helpers — injects and reads the authenticated crawler-node identity from `context.Context`                                                |

## Route structure

| Group                    | Purpose                                                                                                                      |
| ------------------------ | ---------------------------------------------------------------------------------------------------------------------------- |
| `/node/*`                | Crawler-node work polling (`GET /node/work`) and result submission (`POST /node/results`) — gated by `RequireNodeToken`      |
| `/admin/crawler-nodes/*` | Node management — register, list, and decommission crawler-node records — gated by `wireAdminGate` (JWT admin auth, audited) |
| `/openapi.json`, `/docs` | Combined OpenAPI spec + docs UI, served unauthenticated on the main router (no separate ops listener in this binary)         |

## Admin auth gate

`/admin/crawler-nodes/*` is gated by the same admin-auth stack `apps/api` uses for every `/admin/*` route (`RequireIdentity` -> `RequireRole("admin")` -> `AuditAdminMutation`), wired in `wire_admin_gate.go` against the same `users`/`admin_audit_log` tables. This closed a real, pre-existing gap: the surface previously had zero admin authentication.

JWKS init failure (`apimiddleware.Auth` at boot) is **not** fatal here, unlike `apps/api`'s identical call — `/node/*` (the core crawl-assignment pipeline) has never depended on Clerk and must keep serving crawler nodes even if Clerk is unreachable. A failed init instead degrades only `/admin/crawler-nodes/*` to an unconditional 503.

## Background ticker — runResetStuck

Runs every **5 minutes**. Calls `ResetStuckForNodes(ctx, HeartbeatStaleDuration)` to reclaim `company_scan_queue` rows whose heartbeat has expired (i.e., the crawler-node that claimed them crashed or went offline). Reclaimed rows return to `pending` state and are reassigned on the next poll.

## NATS publishing

After receiving a successful crawl result via `POST /node/results`, the orchestrator publishes each raw job listing to `jobs.raw.<adapter>` on the `JOBS_RAW` JetStream stream. The filter worker consumes from this stream.

## Graceful shutdown

Listens for SIGINT/SIGTERM. On signal: stops accepting new requests and waits up to **10 seconds** for in-flight requests to complete before exiting.

## How to add a new route

1. Add the Huma operation to `NodeRestRouter`/`AdminRestRouter.Register` (`service/crawlernode/services/CrawlerNodeRestRouter.go`) and implement the handler in the corresponding `RestService` — same pattern as `apps/api` (see its CLAUDE.md's "Adding a new route" for the full Huma checklist: input/output structs, `apihuma.MapError`, `required:"false"` on optional Body fields)
2. If the new route needs a new dependency, wire it into `wireApp` (`wire_app.go`) so both `main()` and `genspec.go` stay in sync automatically
3. `/node/*` routes are gated by `RequireNodeToken`; `/admin/crawler-nodes/*` routes are gated by `wireAdminGate` — never add a route to either group without its existing gate, and never invent a third auth mechanism
4. Add a contract test asserting the operation appears in the generated spec, plus edge-case tests per the usual per-slice quality gate
5. Regenerate `apps/orchestrator/openapi.yaml` via `go run ./apps/orchestrator --print-spec` and commit it — CI's stale-spec guard fails the build if the committed spec drifts from what `--print-spec` produces
