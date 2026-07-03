# Grafana Observability Dashboards Design

**Date:** 2026-07-02
**Scope:** Grafana Cloud dashboards + alert rules for the full pipeline (scheduler → orchestrator → crawler-node → filter → analyzer → api)

---

## Problem statement

JobRadar already ships rich Prometheus metrics (~40 counters/histograms across every service), OTel tracing with W3C propagation over NATS, and structured JSON logging — all forwarded to Grafana Cloud (Mimir/Tempo/Loki) via the Alloy DaemonSet. No dashboards exist. This is a dashboard-and-alerting design, not an instrumentation design — with one exception: the orchestrator's `/metrics` endpoint is declared in Helm and scraped by Alloy on `:9104`, but `apps/orchestrator/main.go` never calls `metrics.Serve()`, so nothing listens there today.

---

## Preliminary fix 1: orchestrator metrics endpoint

**File:** `backend/apps/orchestrator/main.go`

Wire `metrics.Serve(addr, readyFn)` following the same pattern already used in `apps/scheduler/main.go` and `apps/crawler-node/main.go`. Address comes from `METRICS_ADDR`, default `:9104` (already the value in `k8s/helm/jobradar/values.yaml` and the Alloy scrape config — no Helm changes needed). This must land before the Overview dashboard's orchestrator row is built, since that row has no data source otherwise. Also wire the existing `apimiddleware.Metrics()` chi middleware (already used by `api`, never wired into orchestrator) so `/node/*` and `/admin/crawler-nodes/*` get RED-method HTTP metrics for free — the orchestrator has no business-logic metrics of its own today, so without this the wired `/metrics` endpoint would only expose generic Go runtime metrics.

## Preliminary fix 2: current-state gauge metrics

**Files:** `backend/common/metrics/metrics.go` + new store methods across `scan`, `crawlernode`, `company`, `job` domains + `schedulerService.RefreshGaugeMetrics`

Every metric in the codebase today is a `Counter` or `Histogram` — there are zero `Gauge`s. The stat-strip panels below need current-state snapshots ("how many right now"), which counters cannot express. Rather than add a Postgres datasource or a NATS exporter to Grafana Cloud, add 5 new `prometheus.Gauge` vars, refreshed once per scheduler tick (default 60s) by a new `RefreshGaugeMetrics` method that reads 5 cheap `COUNT`/`MIN` queries through existing store layers (no new tables, no raw SQL outside the store layer):

- `jobradar_scheduler_queue_pending` — `company_scan_queue` rows in `status='queued'`
- `jobradar_scheduler_queue_oldest_pending_age_seconds` — age of the oldest queued row
- `jobradar_crawler_nodes_online` — `crawler_nodes` with `status='active'` and a heartbeat within `OnlineThreshold` (90s)
- `jobradar_scheduler_companies_auto_paused_current` — `company_scan_settings` rows with `auto_paused=true` (a point-in-time snapshot, distinct from the existing `jobradar_scheduler_companies_auto_paused_total` pause-_event_ counter)
- `jobradar_job_pipeline_ledger_retry_count` — `job_pipeline_ledger` rows at `reason='retry'`, `removed_at IS NULL` — the sweeper's backlog size

A true DLQ current-backlog count is out of scope for this pass — DLQ messages live in a NATS JetStream stream with no Prometheus exporter today, and wiring one is materially more work than the Postgres-gauge pattern above. The Overview's DLQ panel instead shows a rate (`rate(filter_dlq_publishes_total[5m]) + rate(enricher_dlq_publishes_total[5m])`) — a reasonable proxy, though a sustained non-zero rate doesn't distinguish a growing backlog from a draining one. A real DLQ gauge via JetStream stream introspection is a good follow-up.

Each of the 5 gauge reads is independent — a failure on one is joined into `RefreshGaugeMetrics`'s returned error (via `errors.Join`) but never blocks the others from refreshing. `NodeReader` and `SettingsReader` are optional dependencies on `schedulerService` (nil-safe), mirroring the existing `Ledger`/`Publisher`/`Auditor` optional-deps pattern.

---

## Delivery approach: dashboards-as-code, manual push during iteration

- Dashboard JSON lives under `infra/observability/dashboards/` (one file per dashboard: `overview.json`, and later `scheduler.json`, `orchestrator.json`, `crawler-node.json`, `filter.json`, `analyzer.json`, `api.json`, `business.json`).
- Alert rule definitions live under `infra/observability/alerts/` in Grafana Alerting provisioning YAML format, grouped by dashboard.
- Git is the source of truth. While the dashboard set is being iterated on, JSON is pushed to Grafana Cloud manually via the Grafana MCP tools (once the MCP server restart lands). No sync script or CI wiring yet.
- Once the dashboard set is finalized, a follow-up adds `scripts/grafana-sync.sh` (pushes via Grafana Cloud's REST API using a service-account token sourced from Doppler) plus a CI step that runs it on merge, so Grafana Cloud can never silently drift from git. This follow-up is explicitly out of scope for this pass.

---

## Phase 1 — Overview dashboard

**File:** `infra/observability/dashboards/overview.json`

Single dashboard, built first, optimized for "is the pipeline healthy right now and where's it broken" during an incident.

**Stat strip** (top row, 4 panels, backed by the 5 new gauges from Preliminary fix 2):

1. Queue depth (`jobradar_scheduler_queue_pending`) + oldest-pending-scan age (`jobradar_scheduler_queue_oldest_pending_age_seconds`)
2. DLQ publish rate (`filter_dlq_publishes_total` + `enricher_dlq_publishes_total`, rate proxy — see Preliminary fix 2) + retry-ladder current count (`jobradar_job_pipeline_ledger_retry_count`)
3. Online crawler-node count (`jobradar_crawler_nodes_online`) + auto-paused company current count (`jobradar_scheduler_companies_auto_paused_current`)
4. DB pooler transient-retry rate (`jobradar_db_transient_retries_total`)

**Stage rows** (one per service, each with 3 panels using the RED method — Rate, Errors, Duration p50/p95):

- Scheduler — `SchedulerRunDuration`, `SchedulerErrorsTotal{reason}` (aggregated), `SchedulerCompaniesEnqueuedTotal`
- Orchestrator — depends on the preliminary fix; rate/error/duration from whatever `metrics.Serve` exposes once wired
- Crawler-node — `CrawlerScansTotal{status}`, `CrawlerScanDuration`, `CrawlerJobsFetchedTotal`
- Filter — `FilterJobsTotal{outcome}`, `FilterLLMDuration`, `FilterChunkDuration{outcome}` (aggregated)
- Analyzer — `EnricherJobsTotal{outcome}`, `EnricherLLMDuration`, `EnricherGroupDuration{outcome}` (aggregated)
- API — `HTTPRequestsTotal{method,route,status}` (aggregated), `HTTPRequestDuration{method,route}`

Error panels show an aggregate rate only — label breakdowns (which adapter, which reason, which route) are deferred to each service's Phase 2 dashboard, reached via a panel link. This keeps the Overview scannable during an incident.

**Alert rules** (rules defined now, notification routing deferred — no contact point configured yet):

| Alert                   | Signal                                                     | Condition                                                                 |
| ----------------------- | ---------------------------------------------------------- | ------------------------------------------------------------------------- |
| `QueueBacklog`          | oldest-pending-scan age                                    | > 2× `SCAN_INTERVAL_MINUTES` default (172800s), for 10m                   |
| `DLQPublishRateNonZero` | DLQ publish rate                                           | > 0, for 5m                                                               |
| `RetryLadderBacklog`    | retry-ladder current count                                 | > 50 (initial conservative guess), for 30m                                |
| `DBTransientRetrySpike` | `rate(jobradar_db_transient_retries_total[5m])`            | > 0 sustained, for 10m                                                    |
| Per-stage error rate    | `rate(<stage>_errors_total[5m]) / rate(<stage>_total[5m])` | deferred — not part of this pass, added once baseline traffic is observed |

Per-stage error-rate alerts are explicitly out of scope for this implementation pass (not merely an unset threshold) — they are added in a follow-up once the dashboard has run against real traffic for a few days. Setting a number before there's a baseline risks either alert fatigue (too low) or missed incidents (too high). The 4 alerts shipped in this pass use structural or conservative-initial thresholds that don't require a baseline to be directionally useful.

---

## Phase 2 — Per-service deep-dive dashboards

**Files:** `infra/observability/dashboards/{scheduler,orchestrator,crawler-node,filter,analyzer,api}.json`

Built after Phase 1 ships. One dashboard per binary, showing the full label-broken-down metric surface for that service (e.g. filter's board breaks down `FilterJobsTotal` by `outcome`, `FilterLedgerMissing` by `reason`, adds `FilterLLMExhaustionTotal` and `LLMCooldownMissTotal` panels not shown on the Overview at all). Each links back to the Overview and out to Loki (log correlation via the `trace_id` field already present in structured logs) for investigating a specific spike. No alert rules on these boards — alerting lives on the Overview aggregate; these are diagnosis tools reached after the Overview has already flagged a problem.

---

## Phase 3 — Business/product dashboard

**File:** `infra/observability/dashboards/business.json`

Built last. Jobs found per day, companies scanned per day, and the raw → filtered → analyzed → saved funnel with drop-off percentage at each stage, all sourced from the same Prometheus counters used elsewhere (not a separate DB query layer, so this stays consistent with the infra dashboards rather than drifting from a second source of truth). No alert rules — this dashboard is for visibility, not paging.

---

## Implementation sequence

1. Orchestrator metrics fix (`main.go`) — must land first, Phase 1's orchestrator row has no data otherwise
2. Phase 1 Overview dashboard JSON + alert rule YAML, pushed manually via Grafana MCP tools
3. Observe real traffic for a few days, fill in the per-stage error-rate alert thresholds
4. Phase 2 per-service dashboards (can be built in parallel with each other once Phase 1 is stable)
5. Phase 3 business dashboard
6. Follow-up (separate pass): `scripts/grafana-sync.sh` + CI wiring to replace manual MCP pushes

---

## Testing

- Orchestrator `main.go` changes are excluded from the unit coverage gate (project convention) — verified by build + lint only, not a new test
- Each new store method (`CountQueued`, `OldestQueuedAgeSeconds`, `CountOnline`, `CountAutoPaused`, `CountByReason`) gets unit tests matching its domain's existing test conventions (DryRun SQL-shape assertions for Postgres-only syntax, real in-memory SQLite execution for portable SQL)
- `schedulerService.RefreshGaugeMetrics` gets unit tests covering: all 5 gauges set correctly, nil optional readers skipped without panic/error, partial failure joins errors but still sets the succeeding gauges
- Dashboard JSON is validated by successfully pushing to Grafana Cloud and confirming panels render with real data (no unit-testable logic in JSON dashboard definitions themselves)
- Alert rule YAML validated the same way — confirm each rule evaluates without a query error in Grafana Cloud before considering Phase 1 done
