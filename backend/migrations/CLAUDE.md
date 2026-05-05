# migrations/ — Database migrations

Uses goose. Every file needs `-- +goose Up` and `-- +goose Down` sections.
Run: `go run ./apps/migrate` or `make migrate`. Currently at migration **044** (`044_phase2_multiuser_data_model.sql` — renames `job_feed`→`jobs`, creates `user_job_state`, `company_scan_settings`, drops `job_dismissal_log`).

## Rules

- Never edit an already-applied migration — add a new one instead
- Numbering is sequential: `NNN_description.sql`
- All constrained columns use SQL `CHECK` constraints — mirror them as Go constants in the owning service's `common/{Domain}ServiceConstants.go`
- Use `TIMESTAMPTZ` not `TIMESTAMP` for all time columns
- Use `BIGSERIAL` for auto-increment integer PKs, `TEXT` for string PKs
