# audit/ — Append-only admin audit log: `Append` (middleware) + `List` (admin REST)

## Deviation from canonical layout

The chain is intentionally shallow — no `ValidationService`, no `CacheService`:

```
RepositoryService → ImplService
```

The admin REST surface (`GET /admin/audit-log`) is wired separately via `AuditAdminRestService` + `AuditAdminRestRouter` but these are **not** part of the decorator chain — they depend directly on `Service`.

## Interface

`Service` (`AuditService.go`) — `Append(ctx, entry)` and `List(ctx, filter)`. All layers implement this.

## Key invariants

- `Append` is consumed by the `AuditAdminMutation` middleware (in `apps/api/`), not by any user-facing handler. No user REST surface for audit writes exists.
- `ListFilter` uses page+offset pagination — this is an admin-only endpoint on a bounded result set; the keyset-cursor exception documented in `service/CLAUDE.md` applies.
- `admin_audit_log` is append-only: no update or delete methods exist on `Service` or `RepositoryService`. Never add them without a retention-policy design.
- The `ProviderSet` in `wire.go` exposes only `NewRepositoryService` + `NewImplService`. The admin REST pair (`NewAdminRestRouter`, `NewAdminRestService`) are wired directly in `apps/api/wire.go` — they do not live in `ProviderSet`.
