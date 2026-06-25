# apps/api/middleware/ — HTTP middleware chain

HTTP middleware for the REST API server. Applied in a defined chain order; each middleware has a single responsibility.

## Key files

| File                  | Role                                                                                                          |
| --------------------- | ------------------------------------------------------------------------------------------------------------- |
| `auth.go`             | JWT verification + dev-bypass; exports `VerifyJWT(r)` helper used by downstream middleware                    |
| `identity.go`         | `RequireIdentity` — DB lookup + cache-aside + lazy user provisioning + active status check                    |
| `role.go`             | `RequireRole(role)` — reads `identity.Roles` set by `RequireIdentity`; no DB lookup                           |
| `ratelimit.go`        | `RateLimit(limiter)` — per-user, method-derived scope; fail-open on Redis outage; returns 429 + `Retry-After` |
| `audit.go`            | `AuditAdminMutation(svc)` — appends a row to `admin_audit_log` on 2xx mutating (non-GET) responses            |
| `logger.go`           | Request logging middleware (always-on)                                                                        |
| `metrics.go`          | Prometheus request metrics (always-on)                                                                        |
| `body_limit.go`       | Request body size cap (always-on)                                                                             |
| `security_headers.go` | Security response headers (always-on)                                                                         |

## Chain order

```
Auth → RequireIdentity → RateLimit → (admin routes only) RequireRole + AuditAdminMutation
```

Always-on middleware (logger, metrics, body limit, security headers) wraps the entire router before the auth chain.

## Dev bypass

`Auth` reads the `X-Dev-Token` request header. When `cfg.API.DevToken` is set **and** the header matches, JWT verification is skipped and a synthetic identity is injected. This path is never active in production — `DevToken` is empty in the production config.

## Clock skew

JWT expiry (`exp`) validation applies a `jwtClockSkew = 60s` tolerance to account for small time differences between issuer and server clocks.

## Identity caching

`RequireIdentity` uses a cache-aside pattern: it checks an in-process (or Redis-backed) cache keyed by Clerk user ID before hitting the DB. Cache TTL is `CACHE_TTL_USER_SECONDS`. On a cache miss the user row is loaded from the DB and lazy-provisioned if absent (first-ever request for a new Clerk user).

## Role checks

`RequireRole` reads `identity.Roles` from context — set earlier by `RequireIdentity` from the Clerk JWT `publicMetadata.role` claim. There is no DB lookup at the role gate. Source of truth for roles is the Clerk dashboard (`publicMetadata = {"role":"admin"}`).
