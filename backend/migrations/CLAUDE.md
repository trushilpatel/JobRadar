# migrations/ — Database migrations

Uses goose. Every file needs `-- +goose Up` and `-- +goose Down` sections.
Run: `go run ./apps/migrate` or `make migrate`. Currently at migration **075** (`075_user_preferences.sql`). Note: `069` exists in the repo and is applied to the DB but is absent from older snapshots — the sequence has no other gaps.

## Deploy-ordering-sensitive migrations

Some migrations must run **before** or **after** a code rollout — the file header states which. Get it wrong and you either break the running binary or let the old binary re-dirty freshly-cleaned data (see the `cleanup-migration-deploy-gap` history):

- `071_jobs_dedupe_and_unique_job_post_id` — run **BEFORE** deploy. Dedupes `jobs` by `job_post_id` (keep oldest) then adds the unique index `InsertJob`'s `ON CONFLICT` depends on.
- `073_cleanup_post_deploy_phantom_fetch_rows_v2` — run **AFTER** deploy. Sweeps residual phantom `reason='fetch'` rows; if it runs before the new crawler/filter binaries are live, the old ones keep leaking fresh phantoms.

## Rules

- Never edit an already-applied migration — add a new one instead
- Numbering is sequential: `NNN_description.sql`
- A migration whose Up runs an irreversible data change (e.g. `071`'s dedupe DELETE) must say so in its header and make its Down revert only what it safely can (index shape, not deleted rows)
- Constrained value columns use SQL `CHECK` constraints mirrored as Go constants in `common/{Domain}ServiceConstants.go`; FK to a canonical table (e.g. `adapter_configs`) is preferred over CHECK when a registry table already exists
- Use `TIMESTAMPTZ` not `TIMESTAMP` for all time columns
- Use `BIGSERIAL` for auto-increment integer PKs, `TEXT` for string PKs
