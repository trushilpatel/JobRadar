# Distributed Crawler & NATS Migration Design

**Date:** 2026-05-21
**Status:** Approved — implementation starting

---

## Goals

1. Extract the crawler into a stateless, owner-deployable gRPC service (`crawler/`) with no infra credentials.
2. Replace Kafka with NATS JetStream as the pipeline bus across all stages.
3. Replace `company_scan_queue` (Postgres-backed DB queue) with a NATS stream — eliminating all queue coordination code.
4. Collapse `apps/scheduler` and `apps/crawler` into a single `apps/orchestrator` binary that owns all stateful pipeline coordination.
5. Filter and analyzer remain separate services — transport layer only changes (Kafka → NATS pull consumer).

---

## Repository Structure

```
JobRadar/
├── crawler/                    ← new top-level (own go.mod)
│   ├── ats/                    ← moved from backend/apps/crawler/ats/
│   ├── proto/                  ← gRPC contract (crawler.proto)
│   ├── server/                 ← gRPC server implementation
│   ├── main.go
│   └── go.mod
│
├── backend/
│   └── apps/
│       ├── orchestrator/       ← new (absorbs scheduler + crawler engine loop)
│       ├── filter/             ← transport swap only (Kafka → NATS)
│       ├── analyzer/           ← transport swap only (Kafka → NATS)
│       └── api/                ← minimal change (scan trigger → NATS publish)
│
└── backend/common/
    └── jetstream/              ← new NATS wrapper (replaces common/kafka)
```

**Deleted:** `backend/apps/crawler/`, `backend/apps/scheduler/`, `backend/common/kafka/`, `company_scan_queue` table.

---

## Component Responsibilities

### `crawler/` — Dumb gRPC Service

- Pure HTTP to ATSes. No DB, no NATS, no Redis, no infra credentials.
- gRPC server exposing three RPCs: `CrawlListing` (server-streaming), `FetchDetail` (unary), `Capabilities` (unary).
- Can be deployed anywhere — different cloud, different IP = different rate-limit budget per node.
- Auth: API key validated in a gRPC server interceptor (`Authorization: Bearer <token>` in metadata).
- Multiple nodes supported — orchestrator routes by adapter capability and current node capacity.

### `apps/orchestrator/` — Stateful Coordinator

Owns all coordination: scheduling, queue consumption, dedup, publishing, scan recording.

Two goroutine pools running concurrently:

**Scheduler goroutine** (every 1 min):

- Reads `company_scan_settings` for companies due for scan.
- Publishes `ScanRequest` to NATS `SCAN_REQUESTS` stream.
- `Nats-Msg-Id: {company_id}:{YYYY-MM-DD}` with 24h dedup window — multiple replicas publish safely with no lock.

**Crawl worker pool** (pull consumer on `SCAN_REQUESTS`):

- Competing consumer across replicas — each message claimed by exactly one orchestrator.
- `msg.InProgress()` every 2 min (replaces `UpdateHeartbeat`).
- Lookup: which crawler-service node supports this `adapter_type`? (from in-memory capabilities table).
- Call `CrawlListing` via gRPC (server-streaming) — dedup and publish incrementally per chunk.
- Detect removed jobs: diff current scan vs `GetActiveJobIDsByCompany`.
- Record scan result to `company_scan_log`, update `last_scan_at`.
- `msg.Ack()` on success. `msg.Nak()` on failure → NATS retry ladder. Crash → `AckWait` expiry → redelivery.

**Crawler-service registration:**

- Orchestrator config carries a list of crawler-service endpoints.
- On startup: call `Capabilities` on each node → build `adapter_type → []node` routing table.
- Route each `ScanRequest` round-robin across capable nodes. Capacity-aware routing (least-loaded) is deferred until `Watch` RPC is implemented.
- If all nodes for an adapter are unreachable: `msg.Nak()` → retry.

**Long-running scans:**

- `AckWait` = 15 min (crash-detection window, not a scan deadline).
- `msg.InProgress()` every 2 min extends the window indefinitely while the scan is active.
- Goroutine per scan, bounded by semaphore (default: 10 concurrent scans per replica).
- gRPC keepalives (HTTP/2 PING every 60s) keep streams alive through proxies/NAT.
- Server-streaming on `CrawlListing` means partial results are published before the scan finishes — crash mid-scan loses only the un-streamed tail.

### `apps/filter/` — NATS Pull Consumer

- Pull consumer on `JOBS_RAW` stream, consumer group `FILTER_WORKERS`.
- Business logic unchanged. Transport swap only: `kafka.Consumer` → `jetstream.PullConsumer`.
- On pass: publish to `JOBS_FILTERED`, `msg.Ack()`.
- On fail: `msg.Nak()` → retry ladder [1m, 10m, 1h] → DLQ after 4 attempts.

### `apps/analyzer/` — NATS Pull Consumer

- Pull consumer on `JOBS_FILTERED` stream, consumer group `ANALYZER_WORKERS`.
- Business logic unchanged. Transport swap only.
- Writes to `jobs` table, `msg.Ack()`.

### `apps/api/` — Manual Scan Trigger

- `POST /api/companies/:id/scan` publishes to `SCAN_REQUESTS` with `Nats-Msg-Id: {company_id}:manual:{uuid}`.
- The `:manual:{uuid}` suffix makes the ID unique — bypasses the 24h dedup window so manual triggers always enqueue.

---

## NATS Stream Definitions

| Stream          | Subject pattern           | Consumer               | AckWait | MaxDeliver | Backoff       |
| --------------- | ------------------------- | ---------------------- | ------- | ---------- | ------------- |
| `SCAN_REQUESTS` | `scan.requests`           | `ORCHESTRATOR_WORKERS` | 15 min  | 3          | [2m, 10m]     |
| `JOBS_RAW`      | `jobs.raw.{adapter}`      | `FILTER_WORKERS`       | 2 min   | 4          | [1m, 10m, 1h] |
| `JOBS_FILTERED` | `jobs.filtered.{adapter}` | `ANALYZER_WORKERS`     | 5 min   | 4          | [1m, 10m, 1h] |
| `JOBS_DLQ`      | `jobs.dlq`                | manual                 | —       | 1          | —             |

Dedup window on `SCAN_REQUESTS`: 24h (matches default scan interval).

---

## gRPC Contract (`crawler/proto/crawler.proto`)

```
service Crawler {
  rpc CrawlListing(CrawlListingRequest) returns (stream CrawlListingChunk);
  rpc FetchDetail(FetchDetailRequest)   returns (FetchDetailResponse);
  rpc Capabilities(CapabilitiesRequest) returns (CapabilitiesResponse);
}
```

- `CrawlListingRequest`: `company_id`, `adapter_type`, `adapter_config` (bytes/JSON), `known_job_ids` (repeated string).
- `CrawlListingChunk`: `repeated JobListing items`. Each item: `id`, `title`, `location`, `url`, `employment_type`, `full_content` (bytes, optional).
- `FetchDetailRequest`: `job_id`, `job_url`, `adapter_type`.
- `FetchDetailResponse`: `title`, `location`, `description`, `employment_type`, `salary_min`, `salary_max`, `salary_currency`.
- `CapabilitiesRequest`: empty.
- `CapabilitiesResponse`: `repeated string adapter_types`.

Auth: `Authorization: Bearer <token>` in gRPC metadata. Interceptor on crawler-service validates against env var `CRAWLER_API_KEY`.

---

## Data Flow Summary

```
Orchestrator scheduler ──Nats-Msg-Id dedup──► SCAN_REQUESTS stream
                                                      │
                                        competing pull consumers
                                        (one msg → one orchestrator)
                                                      │
                                          gRPC CrawlListing (streaming)
                                                      │
                                              crawler-service
                                                      │
                                          HTTP to ATS platforms
                                                      │
                              ◄── stream CrawlListingChunk per page ────
                                                      │
                              dedup (HasSeenBatch) + publish per chunk
                                                      │
                                               JOBS_RAW stream
                                                      │
                                        filter (NATS pull consumer)
                                                      │
                                             JOBS_FILTERED stream
                                                      │
                                        analyzer (NATS pull consumer)
                                                      │
                                           Postgres jobs table
```

---

## What Disappears

| Today                                                       | Replaced by                                                 |
| ----------------------------------------------------------- | ----------------------------------------------------------- |
| `company_scan_queue` table                                  | `SCAN_REQUESTS` NATS stream                                 |
| `ClaimNext` / `Complete` / `UpdateHeartbeat` / `ResetStuck` | NATS pull consumer + `msg.InProgress()` + `AckWait`         |
| `apps/scheduler/` binary                                    | Scheduler goroutine inside orchestrator                     |
| `apps/crawler/` binary                                      | `crawler/` gRPC service + orchestrator crawl worker         |
| `backend/common/kafka/`                                     | `backend/common/jetstream/`                                 |
| Kafka StatefulSet in Helm                                   | NATS via Synadia Cloud (env: `NATS_URL`, `NATS_CREDS_FILE`) |
| Kafka retry topics                                          | Consumer `backoff` + `max_deliver` config                   |
| DB advisory lock for HA scheduler                           | `Nats-Msg-Id` 24h dedup window                              |

---

## Implementation Order

1. **`backend/common/jetstream/`** — NATS wrapper (Publisher + PullConsumer interfaces matching today's kafka wrapper).
2. **`backend/apps/filter/`** — swap transport (Kafka → NATS). Tests stay green.
3. **`backend/apps/analyzer/`** — swap transport (Kafka → NATS). Tests stay green.
4. **`crawler/`** — new top-level module: move ATS adapters, write gRPC server.
5. **`backend/apps/orchestrator/`** — new binary: scheduler goroutine + crawl worker pool + gRPC client.
6. **`backend/apps/api/`** — swap scan trigger to publish NATS instead of DB queue insert.
7. **Helm / infra** — add NATS, remove Kafka, remove `company_scan_queue` migration.

Steps 2 and 3 are independent and can be parallelised.
Steps 4 and 5 are sequential (orchestrator imports proto types from crawler/). During local development, `backend/go.mod` uses a `replace` directive pointing to the local `../crawler` path. Remove the replace and publish the crawler module before any external deployment.

---

## New Environment Variables

| Var                 | Used by                                                        |
| ------------------- | -------------------------------------------------------------- |
| `NATS_URL`          | orchestrator, filter, analyzer                                 |
| `NATS_CREDS_FILE`   | orchestrator, filter, analyzer                                 |
| `CRAWLER_ENDPOINTS` | orchestrator (comma-separated `host:port` list)                |
| `CRAWLER_API_KEY`   | orchestrator (outbound) + crawler-service (inbound validation) |

---

## Open Questions (deferred, not blocking)

| #   | Question                                                                                    |
| --- | ------------------------------------------------------------------------------------------- |
| 1   | `Watch` bidi RPC for capacity signalling — defer until multiple crawler nodes are running   |
| 2   | Upgrade auth from API key to mTLS once gRPC contract is stable                              |
| 3   | Semaphore size (concurrent scans per orchestrator replica) — tune after observing real load |
