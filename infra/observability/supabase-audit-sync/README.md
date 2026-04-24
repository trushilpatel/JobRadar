# supabase-audit-sync

Small Go daemon. Every `AUDIT_SYNC_INTERVAL_SECONDS`, it queries
`auth.audit_log_entries` from Supabase (direct Postgres connection) for rows
newer than the persisted watermark, then pushes them to Loki as JSON log lines
tagged `{service="supabase_auth"}`. Supabase's managed audit retention is
short; this closes the gap.

## Env

| Var                           | Default                        | Required | Purpose                                    |
| ----------------------------- | ------------------------------ | -------- | ------------------------------------------ |
| `SUPABASE_DB_URL`             | -                              | yes      | Postgres URL with creds (read-only role).  |
| `LOKI_URL`                    | -                              | yes      | Full push URL, e.g. `http://loki:3100/loki/api/v1/push`. |
| `AUDIT_SYNC_INTERVAL_SECONDS` | `900`                          | no       | Poll interval.                             |
| `STATE_FILE`                  | `/var/lib/audit-sync/state.json` | no     | Watermark location. Mount a volume here.   |

## Run

```
docker build -t jobradar/supabase-audit-sync infra/observability/supabase-audit-sync
docker run --rm \
  -e SUPABASE_DB_URL="postgres://readonly:pw@db.example.supabase.co:5432/postgres?sslmode=require" \
  -e LOKI_URL="http://loki:3100/loki/api/v1/push" \
  -v audit_sync_state:/var/lib/audit-sync \
  jobradar/supabase-audit-sync
```

On cold start the watermark is set to `now - 24h` so the first push backfills
a day of events without dumping the entire history.

## Operational notes

- Uses a read-only Supabase role. Never ship the service-role key here.
- If Loki is down the watermark is **not** advanced for that batch, so entries
  are retried on the next tick.
- State file is written atomically via `tmp` + rename.
