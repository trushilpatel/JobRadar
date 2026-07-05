# pipeline/ — Live pipeline health snapshot for the admin Pipeline page

## Decorator chain

```
RepositoryService → ImplService → ValidationService → RestService → RestRouter
```

No `CacheService` — this data is polled every 30s from the admin UI and
changes constantly; caching it would just serve stale operational state.

## Interface

`PipelineService` — `GetStats`, `TriggerRetry`. All layers implement this interface.

## Key invariants

- `GetStats` runs a single live aggregate query (`pipelineStatsQuery` in
  `PipelineRepositoryService.go`) across `companies`, `company_scan_settings`,
  `company_scan_queue`, `jobs`, and `job_pipeline_ledger` — no materialized
  view. The query is written to be dialect-portable (no Postgres-only
  `FILTER (WHERE …)`, no DB-side `NOW()`) so the same SQL runs against
  Postgres in prod and SQLite in unit tests; the 24h cutoff is computed in Go
  and passed as a bound parameter.
- `awaiting_filter` / `awaiting_enrichment` are derived the same way the
  retired `mv_pipeline_db_stats` view derived them: a `job_pipeline_ledger`
  row for a stage (`reason='fetch'` / `reason='filtered'`) with no matching
  `jobs.job_post_id` yet means the job is still stuck at that stage.
- `TriggerRetry`'s (worker, tier) pair is validated against a fixed enum in
  `PipelineValidationService`; the repository's `TriggerRetry` is a
  documented no-op — NATS JetStream's BackOff/MaxDeliver already redelivers
  automatically, so there is nothing to manually drain. This was already the
  behavior before this slice existed; not a regression.
- The admin REST surface (`GET /admin/pipeline/stats`, `POST
/admin/pipeline/retry`) is mounted under the same `/admin` subrouter as
  `ats`/company-admin/user-admin in `apps/api/multiuser_setup.go`.

## History

This slice replaces `backend/service/dashboard`'s `PipelineImplService` /
`PipelineRepositoryService`, deleted wholesale when the dashboard/insights
frontend features were removed. Pipeline health monitoring is a distinct,
still-used admin feature that happened to be homed inside the `dashboard`
package — it was collateral damage from that removal, not the target of it.
The route moved from `/admin/dashboard/pipeline` to `/admin/pipeline/stats`
to drop the stale "dashboard" naming; `frontend/src/lib/api/pipeline.ts` was
updated to match. The old implementation read from a materialized view
(`mv_pipeline_db_stats`, refreshed by a scheduled `dashboard_refresher.go`
worker); both were dropped by migration `085_drop_dashboard_artifacts.sql`.
This slice computes the same stats live instead of resurrecting the
materialized view + refresher — admin-only, polled every 30s, not a hot path.
