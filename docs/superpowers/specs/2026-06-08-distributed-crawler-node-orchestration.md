# Distributed Crawler-Node Orchestration

**Date:** 2026-06-08  
**Branch:** `refactor/rename-jobingest-to-crawler-v2`

## What landed

Pull-based distributed crawling: an orchestrator process exposes an HTTP API that crawler-node workers poll for work. Multiple crawler-nodes run concurrently in Kubernetes, claim `company_scan_queue` rows using `FOR UPDATE SKIP LOCKED`, crawl the assigned company, and submit results back.

## Components

- `backend/apps/orchestrator/` — HTTP server exposing `/node/*` (work assignment, result submission, heartbeat) and `/admin/crawler-nodes/*` (provisioning, status)
- `backend/service/crawlernode/` — full decorator-chain service slice: Impl → Validation → Rest layers, plus four repository services (node, assignment, ledger/scan-log, scan-settings)
- `infra/Dockerfile.orchestrator` — pure-Go orchestrator image
- `k8s/helm/jobradar/templates/orchestrator/` — Deployment + Service + ConfigMap
- `k8s/helm/jobradar/templates/crawler-node/` — Deployment (scalable replica count) + ConfigMap
- `scripts/k8s-bootstrap.sh` — provisions `CRAWLER_TOKEN_SECRET` and `CRAWLER_TOKEN` secrets

## Coordination mechanism

- `company_scan_queue.locked_by` (migration 079) — node ID written atomically with `FOR UPDATE SKIP LOCKED` claim
- `heartbeat_at` — refreshed every poll cycle via `ExtendHeartbeats`; `ResetStuckForNodes` reclaims rows whose heartbeat expired (crash recovery)
- Node authentication — HMAC-SHA256 Bearer tokens; hash stored in `crawler_nodes.token_hash`

## Key invariants

- Orchestrator is adapter-agnostic: empty `adapter` param claims any queued row
- `ScanReasonNonUSA = "non_usa_location"` — must match the DB CHECK constraint on `job_pipeline_ledger.reason`
- Node IDs are generated in application code (`crypto/rand`) — no DB-default UUID to avoid SQLite incompatibility in tests
- Dev-bypass auth (`X-Dev-Token`) bypasses DB UUID lookup in the identity middleware; synthetic identity returned directly

## Test coverage

`service/crawlernode/services` package: **96.8%** (threshold 95%).  
All tests race-clean; linter reports 0 issues.

## Verified in k3d

- 3 concurrent crawler-nodes processing simultaneously with visible log coordination
- Fault tolerance: killing one pod mid-crawl → Kubernetes replaced it within ~3 seconds; other nodes continued without interruption; `ResetStuckForNodes` reclaimed any orphaned queue rows
