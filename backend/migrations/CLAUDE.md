# migrations/ — Database migrations

Uses goose. Every file needs `-- +goose Up` and `-- +goose Down` sections.
Run: `go run ./apps/migrate` or `make migrate`. Currently at migration **036**.

## Rules

- Never edit an already-applied migration — add a new one instead
- Numbering is sequential: `NNN_description.sql`
- All constrained columns use SQL `CHECK` constraints — mirror them as Go constants in the owning service's `common/{Domain}ServiceConstants.go`
- Use `TIMESTAMPTZ` not `TIMESTAMP` for all time columns
- Use `BIGSERIAL` for auto-increment integer PKs, `TEXT` for string PKs
