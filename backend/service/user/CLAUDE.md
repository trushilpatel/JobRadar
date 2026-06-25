# user/ — User profile and admin user management: dual self-service/admin surfaces with cache decorator

## Dual chain architecture

Two independent decorator chains against two interfaces:

```
Self-service (/api/me):
  RepositoryService → ImplService → CacheService → ValidationService → RestService → RestRouter

Admin (/admin/users):
  AdminRepositoryService → AdminImplService → AdminValidationService → AdminRestService → AdminRestRouter
```

`CacheService` is the only cache decorator in this slice and sits between `ImplService` and `ValidationService` (backed by `platform/cache.Cache` — Redis in prod, noop in tests).

## Interfaces

| Interface      | Declared in           | Purpose                                                                                                        |
| -------------- | --------------------- | -------------------------------------------------------------------------------------------------------------- |
| `Service`      | `UserService.go`      | Self-service contract: `Get`, `GetByEmail`, `Provision`, `UpdateProfile`, `Delete`, `GetConfig`, `PatchConfig` |
| `AdminService` | `UserAdminService.go` | Admin contract: `List`, `GetWithRoles`, `SetRoles`, `SetStatus`, `AdminDelete`                                 |

`AdminService` also declares helper types in `UserAdminService.go`: `AdminUserFilter`, `UserWithRoles`, `AdminListResult`.

## Key invariants

- `Provision` is idempotent — called by auth middleware on every first login. It must tolerate a race between two concurrent first-login requests (upsert semantics at the DB layer).
- `Delete` (self-service) calls the Supabase admin API **before** the DB delete. These two steps must stay ordered — a DB delete without the Supabase delete leaves an orphaned auth identity.
- `AdminService.List` uses page+offset pagination — admin-only, small result set, needs page-jump UX; the keyset exception in `service/CLAUDE.md` applies.
- `SetRoles` manages Clerk `publicMetadata.role` via the Clerk API — roles are **not** stored in a DB roles table. The `grantedBy` parameter is written to `admin_audit_log` by the caller's middleware, not by `SetRoles` itself.
- `GetConfig` / `PatchConfig` read/write per-user config stored as JSONB in `users.config` — never add new config keys without a matching frontend consumer.

## Non-canonical files

| File                                                                                                                                             | Purpose                                                                  |
| ------------------------------------------------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------ |
| `UserAdminService.go`                                                                                                                            | Second interface (`AdminService`) + base delegator + shared result types |
| `UserCacheService.go`                                                                                                                            | Cache decorator (only slice besides `ats/` and `company/` that has one)  |
| `UserAdminImplService.go`, `UserAdminRepositoryService.go`, `UserAdminValidationService.go`, `UserAdminRestService.go`, `UserAdminRestRouter.go` | Full admin chain                                                         |
