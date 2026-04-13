# ADR: JobRadar Scalability & Manageability Review

**Status:** Proposed
**Date:** 2026-04-12
**Reviewer:** Architecture Review

---

## Context

JobRadar is a single-user automated job tracker with a Kafka-based pipeline (crawl → filter → enrich) backed by PostgreSQL, Gemini LLM, and a React frontend. The system currently runs all workers as single-instance containers orchestrated by Docker Compose.

This document reviews the architecture for scalability bottlenecks and manageability gaps, and proposes improvements — **no code changes, suggestions only**.

---

## Current Architecture Summary

```
Scheduler (1 min tick)
    → company_scan_queue (DB, FOR UPDATE SKIP LOCKED)
        → Crawler (10s poll, 1 company at a time)
            → job_listings.raw (Kafka)
                → Filter (batch 50, flush 10s)
                    → job_listings.filtered (Kafka)
                        → Enricher (5 concurrent goroutines)
                            → job_feed + job_analytics (PostgreSQL)
```

**Strengths already in place:**
- Clean interface-based dependency injection across all workers
- Kafka decouples pipeline stages for independent scaling
- `FOR UPDATE SKIP LOCKED` on scan queue enables safe multi-instance crawlers
- Per-job error isolation — one failure never stops a worker
- Prometheus metrics + Loki/Grafana observability stack
- Health and readiness probes on the metrics server

---

## 1. LLM Rate Limit — The Primary Bottleneck

**Current state:** The enricher makes 1 LLM call per job at 15 RPM (Gemini free tier). This caps throughput at **15 jobs/minute** regardless of how many enricher instances you run, because the rate limiter is per-process.

### Suggestions

**A. Centralized rate-limit coordination.** If you scale to N enricher instances, each will enforce its own 15 RPM independently, potentially exceeding the actual API quota. Consider a shared rate limiter — either a Redis-based token bucket or a lightweight sidecar that hands out tokens. Alternatively, divide the quota across instances statically (e.g., 3 instances × 5 RPM each).

**B. LLM provider abstraction is already there — use it.** The `Scorer` interface + factory pattern means you can add a second provider (e.g., Claude, local model) and route traffic across multiple providers. The enricher could round-robin or failover between providers, effectively doubling throughput without exceeding any single provider's limits.

**C. Batch enrichment.** The filter already batches 50 jobs per LLM call. The enricher processes one job per call because each needs full page content. Consider grouping 3–5 shorter job descriptions into a single Enrich call with a multi-job prompt. This trades some output quality risk for significant RPM savings.

---

## 2. Enricher Concurrency — Hardcoded Constant

**Current state:** `enrichConcurrency = 5` is a compile-time constant. Page fetches dominate latency (up to 15s each), so this is the main throughput lever, but changing it requires a rebuild.

### Suggestions

**A. Make it a config variable** (e.g., `ENRICHER_CONCURRENCY`). This lets you tune it at deploy time without rebuilding the image.

**B. Adaptive concurrency.** Track p99 page-fetch latency via the existing Prometheus histogram. If latency drops (fast APIs like Greenhouse JSON), increase concurrency automatically. If it spikes (rate-limited sites), back off. A simple AIMD (additive increase, multiplicative decrease) approach works well for HTTP clients.

---

## 3. Crawler Throughput — Sequential Company Processing

**Current state:** The crawler claims one company at a time from the scan queue, processes it fully, then claims the next. The 60-second inter-company delay (`SCAN_COMPANY_DELAY_SECONDS`) means a crawl of 100 companies takes ~100 minutes minimum.

### Suggestions

**A. Run multiple crawler instances.** The `FOR UPDATE SKIP LOCKED` pattern already supports this safely. Running 3 crawlers triples throughput with zero code changes — just add more containers in docker-compose.

**B. Per-adapter concurrency limits.** Different ATS platforms have different rate tolerances. Greenhouse's public JSON API is generous; a headless-browser scraper is not. Consider tagging companies by adapter type and giving each adapter its own concurrency pool, so one slow adapter doesn't block fast ones.

**C. Priority-aware scheduling.** The scan queue already has a `priority` column and ordering. Consider auto-boosting priority for companies the user has saved/applied jobs from (higher signal = check more often). This makes the fixed scan budget more valuable.

---

## 4. Error Recovery — Manual-Only Reprocessing

**Current state:** Failed jobs get `reason='error'` in `job_pipeline_ledger`. Recovery requires manually deleting that row. There's no dead-letter topic, no retry count, no automatic backoff retry.

### Suggestions

**A. Add a `retry_count` and `last_error` column to `job_pipeline_ledger`.** This lets you distinguish "tried once, transient network blip" from "tried 5 times, permanently broken." A simple cron or scheduler tick can re-queue jobs with retry_count < 3 by resetting their reason to `fetch`.

**B. Separate transient vs. permanent errors.** LLM rate-limit errors and HTTP 429s are transient — they should retry with exponential backoff. JSON parse errors or 404s are permanent — mark them as `error_permanent` and stop retrying. The current design treats all errors identically.

**C. Error dashboard.** The API already exposes `pipeline_errors` in dashboard stats. Consider adding a `GET /jobs/errors` endpoint that lists failed jobs with their error reason and retry count, so the user can see what's stuck and manually trigger retries from the UI.

---

## 5. Database — Scan Queue as a Work Queue

**Current state:** `company_scan_queue` is a PostgreSQL table used as a durable work queue with `FOR UPDATE SKIP LOCKED`. This works well at small scale but has known scaling ceilings.

### Suggestions

**A. For current scale, this is fine.** PostgreSQL advisory locks + skip-locked handles hundreds of companies without issue. Don't over-engineer this unless you're tracking 1,000+ companies.

**B. If you do outgrow it,** the natural evolution is to move the scan queue to Kafka (a `company_scans.pending` topic). This eliminates polling overhead, gives you natural backpressure, and aligns with the existing Kafka infrastructure. The DB table becomes a status tracker rather than a queue.

**C. Stuck-job detection is good but passive.** `ResetStuck(30min)` runs at crawler startup. Consider running it periodically (e.g., every 5 minutes in the scheduler) so stuck items recover without requiring a restart.

---

## 6. Kafka — Single Partition, No DLQ

**Current state:** Topics are auto-created with default settings (likely 1 partition). No dead-letter topic exists — errors go to the DB.

### Suggestions

**A. Partition count = desired consumer parallelism.** If you want 3 enricher instances consuming `job_listings.filtered`, you need at least 3 partitions. With 1 partition, only one consumer in the group is active. This is likely the first thing to change if you scale horizontally.

**B. Key by company_id.** Currently messages have no key (round-robin partitioning). Keying by `company_id` ensures all jobs from one company land in the same partition, which simplifies deduplication and makes per-company ordering guarantees possible without extra coordination.

**C. Retention policy.** No explicit retention is configured. For debugging and replay, set retention to 7 days. This lets you re-consume from an earlier offset if you deploy a bug fix without needing to re-crawl.

**D. The no-DLQ decision is sound for single-user scale.** DB-based error tracking is simpler and more queryable than a DLQ topic. Keep it unless you move to multi-tenant.

---

## 7. Configuration & Operational Manageability

### Suggestions

**A. Hot-reloadable config.** Several values (scan interval, batch size, flush timeout, LLM rate limits) are read once at startup. Supporting SIGHUP-based config reload — or watching a config file — would let you tune the pipeline without restarting workers and losing in-flight work.

**B. Graceful drain on shutdown.** The enricher's `wg.Wait()` drains in-flight jobs, which is good. Verify the filter also flushes its partial batch on shutdown (the code suggests it does via context cancellation → flush). Document the expected shutdown behavior for each worker in a runbook.

**C. Structured health checks.** The metrics server exposes `/health` and `/ready`, but the docker-compose healthchecks only verify the container process is running. Wire the readiness probe to verify the Kafka consumer group is joined and the DB connection is alive. This prevents the orchestrator from routing traffic to a worker that's technically up but not actually processing.

**D. Environment variable documentation.** The config struct loads ~20 env vars. Consider generating a `.env.example` with descriptions and defaults, so new deployments don't require reading the Go source to understand what's configurable.

---

## 8. Observability Gaps

**Current state:** Prometheus metrics cover HTTP requests, crawler scans, filter outcomes, enricher outcomes, and scheduler enqueues. Loki aggregates container logs. Grafana dashboards are provisioned.

### Suggestions

**A. Pipeline lag metric.** Track the time between when a job enters `job_listings.raw` and when it lands in `job_feed`. This end-to-end latency metric is the single most useful indicator of pipeline health. Publish a `pipeline_e2e_duration_seconds` histogram keyed by adapter type.

**B. Consumer lag monitoring.** Kafka consumer group lag (how far behind the consumer is from the latest offset) is the earliest warning sign of a bottleneck. Export it via Prometheus using a lightweight exporter like `kafka-lag-exporter` or instrument the franz-go consumer directly.

**C. LLM token usage tracking.** The rate limiter estimates tokens pessimistically. Track actual token usage from API responses to calibrate the estimator and surface cost data. A `llm_tokens_total{direction="input|output", call="score|enrich"}` counter would enable cost dashboards.

**D. Alerting rules.** The Grafana stack is there, but without alert rules, you're relying on someone watching dashboards. Add alerts for: pipeline_errors > 10 in 1 hour, consumer lag > 100, enricher success rate < 90%, scan queue depth > 50.

---

## 9. ATS Adapter Layer — Extensibility

**Current state:** 4 fully implemented adapters (Greenhouse, Lever, Ashby, Google) and 3 stubs (Workday, Workable, Chromedp). The adapter registry is a simple function-based lookup.

### Suggestions

**A. Adapter health tracking.** When an adapter starts failing consistently (e.g., Greenhouse changes their API), surface that at the company level. A `consecutive_errors` counter on the companies table would let the UI show "Greenhouse adapter failing for 3 scans" rather than just generic errors.

**B. Adapter-specific rate limiting.** Each ATS has its own rate limits (Greenhouse is generous, Ashby is moderate, Google/Playwright is very slow). Currently all adapters share the same `SCAN_COMPANY_DELAY_SECONDS`. Per-adapter delays would let fast APIs run faster and slow scrapers run slower.

**C. Content caching for the enricher.** When a job's content hasn't changed between scans (same title, same URL), the enricher could skip the HTTP fetch and LLM call entirely. A content hash in `job_pipeline_ledger` would enable this optimization. This is especially valuable for the Google adapter, where full content is already fetched at crawl time.

---

## 10. Multi-Instance & Horizontal Scaling Readiness

**Current state:** Single-instance workers, Docker Compose orchestration, single Kafka broker (KRaft mode), single PostgreSQL instance.

### Suggestions for "next level" scaling (not needed now, but good to know the path):

**A. Worker scaling is almost free.** The crawler (skip-locked), filter (consumer group), and enricher (consumer group) can all run multiple instances today with minimal changes. The prerequisites are: Kafka partitions ≥ instance count, and shared rate-limit coordination for LLM calls.

**B. PostgreSQL read replicas for the API.** The API is read-heavy (feed listing, insights, dashboard stats). If query load grows, add a read replica and route read-only store interfaces to it. The store layer's interface design makes this a clean change.

**C. Kafka scaling.** The single-broker KRaft setup is appropriate for single-user. If you move to multi-user, a 3-broker cluster with replication factor 2 gives you fault tolerance. The franz-go client already supports multi-broker configs via `KAFKA_BROKERS`.

**D. Container orchestration.** Docker Compose is fine for single-host. If you move to multi-host, the containerized architecture maps cleanly to Kubernetes (each worker = a Deployment, scan queue = no change, Kafka/Postgres = StatefulSets or managed services).

---

## 11. API Layer Improvements

### Suggestions

**A. Pagination.** `GET /jobs` and `GET /companies` appear to return all results. Add cursor-based pagination (keyed on `added_at` or `id`) to keep response sizes bounded as the job feed grows.

**B. Request validation middleware.** Status and stage transitions are validated inline in handlers via enum maps. A lightweight validation middleware (or shared validator) would centralize this and make it harder to miss validation in new endpoints.

**C. API versioning.** If the frontend and API evolve independently, a `/v1/` prefix now prevents breaking changes later. This is cheap to add early and expensive to retrofit.

**D. Rate limiting on the API itself.** The manual scan trigger returns 429 if scanned too recently, which is good. Consider a global rate limiter on the API to prevent accidental DoS from a misbehaving frontend or script.

---

## Summary — Priority Ranking

| Priority | Area | Effort | Impact |
|----------|------|--------|--------|
| **P0** | Make `enrichConcurrency` configurable | Trivial | Unblocks tuning |
| **P0** | Add Kafka partitions (≥3 per topic) | Config change | Enables horizontal scaling |
| **P1** | Centralized LLM rate limiting | Medium | Enables multi-instance enricher |
| **P1** | Retry logic with retry_count | Medium | Reduces manual intervention |
| **P1** | Pipeline lag + consumer lag metrics | Small | Critical observability gap |
| **P2** | Per-adapter rate limits & health tracking | Medium | Better ATS resilience |
| **P2** | Hot-reloadable config | Medium | Operational convenience |
| **P2** | API pagination | Small | Prevents feed query blowup |
| **P3** | Content caching / hash-based skip | Medium | LLM cost savings |
| **P3** | Periodic stuck-job reset | Trivial | Resilience without restarts |
| **P3** | Alerting rules in Grafana | Small | Proactive issue detection |

---

## What's Already Well-Done

The architecture has strong foundations that shouldn't be changed:

- **Interface-based dependency injection** across all workers — makes testing and swapping implementations trivial
- **Kafka for inter-stage communication** — the right choice for decoupled scaling
- **`job_pipeline_ledger` as a single source of truth** for deduplication and pipeline state
- **Append-only `job_analytics`** — clean separation of operational data from analytical data
- **Non-fatal snapshot writes** — the enricher doesn't fail a job because analytics recording broke
- **Eligibility rules in Go, not LLM** — deterministic, testable, no prompt drift
- **Structured logging with field constants** — makes log aggregation and querying reliable
- **Store pattern with reader/writer splits** — workers only get the DB access they need
