# Distributed Crawler & Infra Migration Plan

**Status:** Phase 1+2 complete · Phase 3 next
**Branch:** `claude/design-distributed-crawler-srGHO`
**Last updated:** 2026-05-21

---

## Goals

1. Crawler horizontally scalable with no per-node config — adding a node = it joins, removing = its work is reclaimed.
2. Multiple crawler nodes may crawl the same ATS in parallel because each node sits behind a distinct egress IP (own rate-limit budget).
3. Replace in-cluster infrastructure with free hosted services where it does not compromise reliability.
4. Adopt gRPC for the crawler ↔ orchestrator boundary — clean versioned contract + streaming + mTLS.

## Non-goals (for now)

- Crawler node hosting decisions — owner deploys nodes themselves.
- Multi-region active-active orchestrator.
- Replacing Supabase Auth — kept as JWT issuer only; only the app data tables move.
- Frontend changes.

---

## Target architecture

Two binaries replace today's `apps/crawler`:

| Binary | Role | External deps |
|---|---|---|
| `apps/orchestrator` | Owns DB, streaming bus, Redis, scheduling, claim/heartbeat/dedup, scan_log. Merges today's `apps/scheduler`. | Postgres + NATS + Redis |
| `apps/crawler-service` | Stateless gRPC server. Wraps adapters. Holds no infra credentials. Can run anywhere — different cloud, different region, even a laptop. | None (outbound HTTP to ATSes only) |

```
                        Neon Postgres
                       (queue, ledger,
                        scan_log, …)
                              ▲
                              │
                  ┌───────────┴────────────┐
                  │      Orchestrator      │
                  │   (2 replicas, HA)     │──── Redis (in-cluster, for now)
                  │   - claim queue         │──── NATS JetStream (Synadia)
                  │   - dedup + publish     │──── Honeycomb traces
                  └───────────┬────────────┘──── Grafana Cloud logs+metrics
                              │
                  gRPC + mTLS │ (server-streaming Crawl, bidi Watch)
                              │
        ┌─────────────────────┼─────────────────────┐
        ▼                     ▼                     ▼
   Crawler N1            Crawler N2            Crawler N3       ← owner-deployed,
   IP: A                 IP: B                 IP: C              any provider
   stateless             stateless             stateless
        │                     │                     │
        └─────── HTTP(s) ─────┴───────────── to ATSes ─────────┘
                       (Greenhouse, Lever, Ashby, Meta, …)
```

### gRPC contract (sketch — final proto pending)

```
service Crawler {
  rpc CrawlListing(CrawlListingRequest) returns (stream CrawlListingChunk);
  rpc FetchDetail(FetchDetailRequest)   returns (FetchDetailResponse);
  rpc Capabilities(CapabilitiesRequest) returns (CapabilitiesResponse);
  rpc Watch(stream WatchClientMsg)      returns (stream WatchServerMsg);
}
```

- Server-streaming on `CrawlListing` so the orchestrator dedup-and-publishes incrementally; no proxy idle timeouts.
- Bidi `Watch` carries capacity updates and drain signals — replaces polling.
- Auth: mTLS via small internal CA. No shared bearer tokens.
- Tracing: W3C `traceparent` in metadata; OTel gRPC interceptors on both sides.

---

## Migration phases

Ordered by cutover risk. Each phase ships independently, each rollback is a config flip + redeploy. Old infra stays running for one week post-cutover before decommission.

### Phase 1+2 — Observability: self-hosted stack → Grafana Cloud (all three pillars) ✅ DONE

**Cutover style:** Agent swap · **Downtime:** none · **Rollback:** keep self-hosted stack idle for 1 week

**Decision:** Single vendor for all three pillars — metrics (Mimir), logs (Loki), traces (Tempo). Honeycomb skipped; Tempo's trace UI is sufficient for our scale. Re-evaluate if p99 trace debugging becomes painful.

- Create Grafana Cloud stack; generate Alloy access tokens (Prom, Loki, OTLP).
- Deploy Grafana Alloy DaemonSet (scrapes `/metrics`, tails container logs, receives OTLP traces, forwards all three).
- Point app OTLP exporter at in-cluster Alloy (`alloy.observability.svc:4317`) instead of Jaeger.
- Migrate 9 custom dashboards from `k8s/helm/observability/dashboards/` (JSON export → import).
- Migrate Prometheus alert rules from `templates/prometheus-rules.yaml` to Grafana Cloud Alerting.
- Update `supabaseAuditSync.lokiUrl` to Grafana Cloud Loki endpoint + add basic_auth header.
- After 1 week of clean operation: `helm uninstall observability` (removes Prometheus, Grafana, Alertmanager, Loki, Jaeger).
- **Resources freed:** 15 Gi PVC storage, ~1.3 GB RAM.
- Quotas comfortable: <2 GB/mo logs vs 50 GB free; ~500–800 metric series vs 10K free; <1 GB/mo traces vs 50 GB free.
- Retention improves: 24h → 13d metrics, 24h → 14d logs, in-memory → 14d traces.

**New env:** `GRAFANA_CLOUD_PROM_URL`, `GRAFANA_CLOUD_PROM_TOKEN`, `GRAFANA_CLOUD_LOKI_URL`, `GRAFANA_CLOUD_LOKI_TOKEN`, `OTEL_EXPORTER_OTLP_ENDPOINT` (→ Alloy), `OTEL_EXPORTER_OTLP_AUTH`.

### Phase 3 — Redis: in-cluster → Upstash (Pay-as-you-go)

**Cutover style:** Connection-string swap · **Downtime:** none (cache rewarms) · **Rollback:** keep in-cluster Redis idle for 1 week

**Plan locked: Upstash Pay-as-you-go from day one.** Free-tier cap (500 K commands/mo) is at ~150% of projected use at 10 K API requests/day; PAYG costs <$1/mo at our scale and removes the quota cliff.

- Create Upstash database (protocol mode, TLS).
- Swap connection string to `rediss://default:<pwd>@<host>:<port>` — go-redis v9 handles `rediss://` natively.
- Tune `PoolSize` down to ~20 (Upstash recommends low concurrent connection counts).
- Verify token-bucket Lua script (`backend/platform/ratelimit/redis.go`) runs against Upstash protocol mode — it should; no code change expected.
- Cache rewarms on first traffic; rate-limit buckets refill within seconds.
- **Add Grafana Cloud alert at 80% of monthly command budget** (rolling 30 d) — early warning, not a hard cap.
- Decommission in-cluster Redis from Helm after 1 week of clean operation.

**Sizing (10 K API requests/day):**
- Commands: ~25 K/day → ~750 K/month → PAYG ~$0.50/mo
- Storage: <2 MB steady-state vs 256 MB cap (100× headroom)

**Reconsider when traffic reaches ~50 K req/day** — that's when cache TTLs may need tightening and storage starts mattering.

**New env:** `UPSTASH_REDIS_URL` (single `rediss://` URL containing host, port, password).

### Phase 4 — Streaming: Kafka → NATS JetStream (Synadia Cloud)

**Cutover style:** Dual-bus over 4 days · **Downtime:** none · **Rollback:** consumers flip back to Kafka

| Kafka today | NATS JetStream target |
|---|---|
| Topic `job_listings.raw` | Stream `JOBS_RAW`, subject `jobs.raw.{adapter}` |
| Topic `job_listings.filtered` | Stream `JOBS_FILTERED`, subject `jobs.filtered.{adapter}` |
| DLQ topic | Stream `JOBS_DLQ` |
| Consumer group `filter` | Durable pull consumer `FILTER` |
| Consumer group `analyzer` | Durable pull consumer `ANALYZER` |
| Offset commits | `msg.Ack()` per message |
| Retry ladder (1m → 10m → 1h → DLQ) | Consumer `max_deliver=4` + `backoff=[1m, 10m, 1h]` |
| Header-based dedup via ledger | `Nats-Msg-Id: <job_id>` for stream-level dedup + ledger as defense-in-depth |

**Cutover sequence:**

```
Day 0   Create Synadia account → streams JOBS_RAW / JOBS_FILTERED / JOBS_DLQ.
        Store .creds file as Secret.
Day 1   Producers (crawler, filter) dual-publish to both Kafka and NATS.
        Consumers still read from Kafka.
Day 3   Switch filter consumer to NATS. Crawler stops dual-publishing.
Day 4   Switch analyzer consumer to NATS. Filter stops dual-publishing.
Day 11  Decommission Kafka from Helm. Delete backend/common/kafka/.
```

**Code:**
- New: `backend/common/jetstream/` — thin wrapper exposing the same `Publisher` / `PullConsumer` interfaces today's Kafka wrapper exposes, so callers DI-swap cleanly.
- Edits: `apps/crawler/engine.go` publish path, `apps/filter/` consumer, `apps/analyzer/` consumer — interface-level swap.
- Retry/DLQ logic moves from Kafka topics into consumer config (simpler).
- Env: `NATS_URL`, `NATS_CREDS_FILE`.

### Phase 5 — Postgres: Supabase → Neon

**Cutover style:** Pause writes · dump + restore · swap URL · **Downtime:** 15–30 min · **Rollback:** flip URL back; Supabase kept RW for 1 week

**Split kept simple — Supabase Auth stays; only data tables move:**

| Stays at Supabase | Moves to Neon |
|---|---|
| `auth.users` (JWT identity) | All migrations in `backend/migrations/`: `users`, `roles`, `user_roles`, `companies`, `company_scan_settings`, `company_scan_queue`, `company_scan_log`, `jobs`, `user_job_state`, `job_pipeline_ledger`, `job_analytics`, `job_application_tracker`, `admin_audit_log` |
| JWKS endpoint (JWT signing) | |
| `supabaseadmin.DeleteUser` REST calls (unchanged) | |

`users.id` == `auth.users.id` by UUID convention (already the case) — no cross-DB FK needed.

**Cutover sequence:**

```
T-7d   Provision Neon project. Run goose migrations against direct endpoint.
T-3d   Dry-run pg_dump from Supabase → pg_restore to Neon; time it; validate counts.
T-0    Maintenance window starts. Set API read-only flag.
T+1m   Scale crawler / orchestrator / scheduler to 0.
T+2m   pg_dump (data-only) from Supabase → pg_restore to Neon (incremental delta).
T+5m   Validate: row counts, sample queries (latest scan_log, top jobs, user count).
T+10m  Swap DATABASE_URL secret → Neon. Restart all services.
T+15m  Smoke: sign-in, fetch jobs, save, dismiss, admin pipeline.
T+20m  Lift read-only flag.
T+7d   Decommission Supabase data tables (auth schema stays).
```

**Code touches:**
- `backend/common/config/` — add `DATABASE_URL_DIRECT` for migrations + advisory locks (none today, but room kept).
- `backend/apps/migrate/` — point at `DATABASE_URL_DIRECT` (Neon PgBouncer in txn mode doesn't suit goose).
- `backend/common/db/` — pgx config audit for prepared-statement caching under txn-mode pooling.
- Env: `DATABASE_URL` (pooled), `DATABASE_URL_DIRECT` (migrations).

**Capacity:** DB <100 MB vs Neon free 3 GB — 30× headroom.

---

## Cross-cutting

### Order rationale
1. Grafana Cloud (all three pillars) — destination change for agents; observability data is non-critical and rolls back via env flip. Establishes alerting before later, riskier phases.
2. Upstash Redis — connection-string swap; cache rewarms naturally; Grafana Cloud alert on command budget already in place.
3. NATS — dual-bus = zero-downtime; one new abstraction at a time.
4. Neon — only phase requiring a maintenance window; do last when everything else is settled.

### Secrets
Six new secrets accumulated. One per provider, rotated independently:
- `GRAFANA_CLOUD_PROM_TOKEN`, `GRAFANA_CLOUD_LOKI_TOKEN`, `OTEL_EXPORTER_OTLP_AUTH`
- `UPSTASH_REDIS_URL`
- `NATS_CREDS_FILE`
- `NEON_DATABASE_URL_POOLED`, `NEON_DATABASE_URL_DIRECT`

### Universal rollback recipe
Every phase ships with the old infra still running for 7 days post-cutover. To roll back any phase: flip env/Secret to the old endpoint, redeploy. No code rollback needed — config flip only. Old infra is torn down only after the new one has run clean for a full week.

---

## Open questions

| # | Question | Decision needed before |
|---|---|---|
| 1 | Synadia free-tier specific quotas (connections, GB/month) | Phase 4 Day 0 |
| 2 | gRPC transport: HTTP/JSON fallback or gRPC-only? | Architecture cut |
| 3 | mTLS CA management: internal Go-generated CA vs cert-manager vs Smallstep | Architecture cut |

---

## Out of scope (deferred / declined)

- Crawler node hosting (Fly.io, Render, Oracle, etc.) — owner-controlled.
- Replacing Supabase Auth.
- Multi-region orchestrator.
- Browser-based crawling (Playwright/Chromium) — pure HTTP only.
- Frontend changes.
