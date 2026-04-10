# migrations/ — Database migrations

## Tool: goose

Files use goose format. Every file needs both Up and Down sections.

```sql
-- +goose Up
CREATE TABLE ...;

-- +goose Down
DROP TABLE ...;
```

Run migrations: `go run cmd/migrate/main.go`

## Naming convention

Files are numbered sequentially: `001_`, `002_`, etc.
| Table | Migration file |
|---|---|
| `companies` | `001_companies.sql` |
| `job_pipeline_ledger` (was `job_seen_ledger`) | `002_job_seen_ledger.sql` |
| `company_scan_log` | `003_company_scan_log.sql` |
| `job_feed` (was `job`) | `004_job.sql` |
| `job_application_tracker` (was `job_application`) | `005_job_application.sql` |
| `job_dismissal_log` (was `job_discarded`) | `006_discarded.sql` |
| cascade deletes | `007_cascade_delete_company.sql` |
| cascade seen_ledger | `008_cascade_delete_seen_ledger.sql` |
| `job_feed` enrich fields | `009_job_enrich_fields.sql` |
| gaps + `country`/`state`/`removed_at` | `010_fix_gaps.sql` |
| `job_analytics` (was `job_snapshot`) | `011_job_snapshot.sql` |
| table renames | `012_rename_tables.sql` |
| normalize company URLs | `013_normalize_company_urls.sql` |
| `company_scan_queue` | `014_company_scan_queue.sql` |
| scan queue priority | `015_scan_queue_priority.sql` |
| `job_feed.responsibilities` → JSONB | `016_responsibilities_jsonb.sql` |

## Rules

- Never edit an already-applied migration — add a new one instead
- All constrained columns use SQL `CHECK` constraints — mirror them as Go constants in `model/constants.go`
- Use `TIMESTAMPTZ` not `TIMESTAMP` for all time columns (timezone-aware)
- Use `BIGSERIAL` for auto-increment integer PKs, `TEXT` for string PKs
