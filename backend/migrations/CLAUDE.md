# migrations/ — Database migrations

Uses goose. Every file needs `-- +goose Up` and `-- +goose Down` sections.
Run: `go run ./apps/migrate` or `make migrate`. Currently at migration **043** (`043_phase1_users_roles_audit.sql` — adds `users`, `roles`, `user_roles`, `admin_audit_log`).

## Rules

- Never edit an already-applied migration — add a new one instead
- Numbering is sequential: `NNN_description.sql`
- All constrained columns use SQL `CHECK` constraints — mirror them as Go constants in the owning service's `common/{Domain}ServiceConstants.go`
- Use `TIMESTAMPTZ` not `TIMESTAMP` for all time columns
- Use `BIGSERIAL` for auto-increment integer PKs, `TEXT` for string PKs
