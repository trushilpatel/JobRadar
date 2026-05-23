# migrations/ — Database migrations

Uses goose. Every file needs `-- +goose Up` and `-- +goose Down` sections.
Run: `go run ./apps/migrate` or `make migrate`. Currently at migration **065** (`065_scan_queue_source_check.sql` — CHECK constraint on `company_scan_queue.source`).

## Rules

- Never edit an already-applied migration — add a new one instead
- Numbering is sequential: `NNN_description.sql`
- Constrained value columns use SQL `CHECK` constraints mirrored as Go constants in `common/{Domain}ServiceConstants.go`; FK to a canonical table (e.g. `adapter_configs`) is preferred over CHECK when a registry table already exists
- Use `TIMESTAMPTZ` not `TIMESTAMP` for all time columns
- Use `BIGSERIAL` for auto-increment integer PKs, `TEXT` for string PKs
