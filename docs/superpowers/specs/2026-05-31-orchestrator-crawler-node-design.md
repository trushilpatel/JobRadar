# Orchestrator + Distributed Crawler Node — Design

**Date:** 2026-05-31
**Status:** Approved — ready for implementation planning
**Supersedes:** `2026-05-21-distributed-crawler-nats-migration-design.md` (gRPC approach replaced by HTTP pull; `company_scan_queue` retained; existing NATS pipeline unchanged)

---

## Problem Statement

The current crawler runs exclusively inside the k8s cluster. Scaling it requires adding more k8s resources, and all crawler instances share the same egress IP — making IP-based rate limiting from ATS platforms more likely as volume grows.

The goal is to allow crawler workers to run on **any server** (different cloud, VPS, residential) while keeping the orchestration brain inside the cluster where it has DB and NATS access.

---

## Design Principles

- **Crawler is dumb.** It knows only: poll for work, run the adapter, return results. No DB, no NATS, no business logic.
- **Orchestrator is the only source of truth.** All dedup, ledger writes, NATS publishing, and scan recording happen in the orchestrator — the same invariants as the existing engine.
- **Nothing breaks.** Existing per-adapter crawler workers (`backend/apps/crawler/`) continue running unmodified in parallel during transition.
- **Pull-based, outbound-only.** Crawler nodes initiate all connections. No inbound ports required. Works behind NAT, on spot instances, on residential connections.
- **SOLID throughout.** The existing ATS adapter interface hierarchy (`Adapter`, `ConfigurableAdapter`, `SeenChecker`, `StreamPublisher`, `KnownJobsProvider`) is unchanged — the crawler node wires HTTP callbacks into the injection points instead of DB callbacks. Zero adapter code changes.

---

## Repository Structure

```
backend/
├── apps/
│   ├── crawler/                                     ← UNTOUCHED (existing per-adapter workers)
│   ├── crawler-node/                                ← NEW binary (separate Docker image)
│   │   ├── main.go                                  — signal handling, config, wire, start PollLoop
│   │   ├── config.go                                — Config struct, Load(), Validate()
│   │   ├── interfaces.go                            — OrchestratorClient, AdapterExecutor
│   │   ├── client.go                                — httpOrchestratorClient (Bearer auth, retry/backoff)
│   │   ├── executor.go                              — wires adapter interfaces → HTTP callbacks
│   │   ├── pollloop.go                              — PollLoop: semaphore, holding list, goroutines
│   │   └── types.go                                 — WorkUnit, PageBatch, WorkError wire types
│   └── orchestrator/                                ← NEW binary (runs in k8s cluster)
│       ├── main.go                                  — infra init (DB + NATS), HTTP server, ResetStuck ticker
│       ├── wire.go                                  — Wire injector (build tag: wireinject)
│       └── wire_gen.go                              — Generated wire
│
└── service/
    └── crawlernode/                                 ← NEW domain slice
        ├── wire.go
        ├── CLAUDE.md
        ├── common/
        │   └── CrawlerNodeConstants.go              — status consts, token prefix length, heartbeat expiry
        ├── dto/
        │   ├── WorkUnit.go
        │   ├── ResultsRequest.go
        │   ├── ErrorRequest.go
        │   ├── NodeView.go
        │   └── CreateNodeResponse.go
        ├── models/
        │   └── CrawlerNode.go                       — GORM model for crawler_nodes table
        └── services/
            ├── CrawlerNodeService.go                — CONTRACT: all interfaces + BaseNodeService
            ├── CrawlerNodeRepositoryService.go      — DB terminal: crawler_nodes CRUD
            ├── CrawlerNodeAssignmentRepositoryService.go  — DB terminal: ClaimBatch, Complete, Fail, ResetStuck
            ├── CrawlerNodeLedgerRepositoryService.go      — DB terminal: narrow ledger facade
            ├── CrawlerNodeScanLogRepositoryService.go     — DB terminal: delegates to scan.ScanLogWriter
            ├── CrawlerNodeImplService.go            — business logic (implements NodeService)
            ├── CrawlerNodeValidationService.go      — input validation (implements NodeService)
            ├── CrawlerNodeRestService.go            — HTTP handlers: /node/* endpoints
            ├── CrawlerNodeAdminRestService.go       — HTTP handlers: /admin/crawler-nodes
            └── CrawlerNodeRestRouter.go             — route registration only

infra/
    └── Dockerfile.crawler-node                      ← NEW (build context: ./backend)

backend/migrations/
    ├── 078_crawler_nodes.sql
    └── 079_scan_queue_locked_by.sql
```

---

## Component Architecture

```
┌──────────────────────────── k8s cluster ─────────────────────────────────┐
│                                                                           │
│  scheduler ──► company_scan_queue ◄──── ORCHESTRATOR                     │
│                      │                  ├─ GET  /node/work   (node auth)  │
│                      │ (existing path)  ├─ POST /node/results (node auth) │
│                      ▼                  ├─ POST /node/error  (node auth)  │
│               legacy crawler            └─ /admin/crawler-nodes           │
│              (untouched, parallel)               │                        │
│                      │                           │ NATS publish           │
│                      └──────────────────────────►│                        │
│                                                  ▼                        │
│                               jobs.raw.<ats> → filter → analyzer → jobs  │
└───────────────────────────────────────────────────────────────────────────┘
                ▲ outbound HTTP (Bearer token, TLS)
                │
┌───────────────┴──────────────────────────────────────────────────────────┐
│  any server (VPS / cloud / on-prem / spot instance)                      │
│                                                                           │
│  CRAWLER NODE                                                             │
│  ├─ poll loop (single goroutine)                                          │
│  ├─ worker pool (MAX_CONCURRENT goroutines, semaphore-bounded)            │
│  ├─ ATS adapter registry (14+ adapters — existing, zero changes)          │
│  └─ Docker + Watchtower (auto-update, auto-restart on crash/reboot)       │
└──────────────────────────────────────────────────────────────────────────┘
```

---

## Service Interfaces (`CrawlerNodeService.go`)

All interfaces live in `CrawlerNodeService.go`. Terminal implementations live in their respective Repository files.

### Store Interfaces (terminal — no next, no BaseService)

```go
type NodeStore interface {
    Create(ctx context.Context, node crawlernodemodel.CrawlerNode) (*crawlernodemodel.CrawlerNode, error)
    GetByTokenPrefix(ctx context.Context, prefix string) (*crawlernodemodel.CrawlerNode, error)
    UpdateLastSeen(ctx context.Context, nodeID string) error
    SetStatus(ctx context.Context, nodeID, status string) error
    List(ctx context.Context, status string) ([]crawlernodemodel.CrawlerNode, error)
    Retire(ctx context.Context, nodeID string) error  // sets status=retired, clears token_hash
}

type AssignmentStore interface {
    ClaimNextForNode(ctx context.Context, nodeID, adapterType string) (*dto.ScanQueueItem, error)
    ExtendHeartbeats(ctx context.Context, nodeID string, queueIDs []string) (acked, missed []string, err error)
    CompleteForNode(ctx context.Context, queueID, nodeID string) error  // ErrQueueRowMissing on mismatch
    FailForNode(ctx context.Context, queueID, nodeID string) error      // resets to 'queued'
    ResetStuckForNodes(ctx context.Context, staleDur time.Duration) (int, error)
}

type LedgerStore interface {
    HasSeenBatch(ctx context.Context, jobIDs []string) (map[string]bool, error)
    MarkSeenBatch(ctx context.Context, jobIDs []string, companyID, reason string) error
    MarkRemovedBatch(ctx context.Context, jobIDs []string) error
}

type ScanLogStore interface {
    RecordScanResult(ctx context.Context, companyID string, r dto.ScanResult) error
}

type StreamPublisher interface {
    PublishWithMsgID(ctx context.Context, subject string, payload []byte, msgID string) error
}
```

### Service Interface (decorator chain)

```go
type NodeService interface {
    // node-facing
    GetWork(ctx context.Context, nodeID, adapterType string, holdingIDs []string) (*dto.WorkResponse, error)
    SubmitResults(ctx context.Context, nodeID string, req dto.ResultsRequest) (*dto.ResultsResponse, error)
    ReportError(ctx context.Context, nodeID string, req dto.ErrorRequest) error

    // admin-facing
    ProvisionNode(ctx context.Context, name string) (*dto.CreateNodeResponse, error) // plaintext token returned once
    ListNodes(ctx context.Context, status string) ([]dto.NodeView, error)
    SetNodeStatus(ctx context.Context, nodeID, status string) error
    RetireNode(ctx context.Context, nodeID string) error
}

// BaseNodeService delegates all NodeService methods to next.
type BaseNodeService struct{ next NodeService }
```

---

## Decorator Chain

### `/node/*` endpoints (crawler node auth)

```
RequireNodeToken middleware
    ↓
CrawlerNodeRestService       — parse request, call next.*, respond via handler.Respond[T]
    ↓
CrawlerNodeValidationService — embeds BaseNodeService; overrides node-facing methods
    ↓
CrawlerNodeImplService       — embeds BaseNodeService; orchestrates multi-store calls
    ↓
{ NodeStore, AssignmentStore, LedgerStore, ScanLogStore, StreamPublisher }
```

### `/admin/crawler-nodes/*` endpoints

```
RequireIdentity + RequireRole("admin") + AuditAdminMutation
    ↓
CrawlerNodeAdminRestService  — separate handler struct
    ↓
CrawlerNodeValidationService — overrides admin methods
    ↓
CrawlerNodeImplService
    ↓
{ NodeStore, AssignmentStore }
```

No CacheService — token lookups are security-sensitive and the table is small.

---

## DB Schema

### Migration 078 — `crawler_nodes`

```sql
-- +goose Up
CREATE TABLE crawler_nodes (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name          TEXT        NOT NULL UNIQUE,
    token_prefix  CHAR(8)     NOT NULL,           -- first 8 chars of plaintext token; fast index lookup
    token_hash    TEXT,                           -- HMAC-SHA256(token, CRAWLER_TOKEN_SECRET); NULL after retirement
    status        TEXT        NOT NULL DEFAULT 'active'
                              CHECK (status IN ('active', 'disabled', 'retired')),
    last_seen_at  TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_crawler_nodes_token_prefix ON crawler_nodes (token_prefix);
CREATE INDEX idx_crawler_nodes_status       ON crawler_nodes (status);

-- +goose Down
DROP TABLE crawler_nodes;
```

### Migration 079 — `locked_by` on `company_scan_queue`

```sql
-- +goose Up
-- NULL = claimed by legacy direct-DB crawler (unchanged behavior).
-- Non-null = claimed by orchestrator on behalf of a crawler node.
-- Both paths use FOR UPDATE SKIP LOCKED — no double-claiming possible.
ALTER TABLE company_scan_queue
    ADD COLUMN locked_by UUID REFERENCES crawler_nodes(id) ON DELETE SET NULL;

-- Supports ResetStuckForNodes: WHERE locked_by IS NOT NULL AND status = 'running'
CREATE INDEX idx_csq_locked_by ON company_scan_queue (locked_by)
    WHERE locked_by IS NOT NULL;

-- +goose Down
DROP INDEX idx_csq_locked_by;
ALTER TABLE company_scan_queue DROP COLUMN locked_by;
```

---

## Authentication Design

### Token Format

UUID v4 plaintext. Unguessable (128-bit entropy), widely supported, human-copyable.

### Token Storage

`HMAC-SHA256(token, CRAWLER_TOKEN_SECRET)` stored in `token_hash`. The first 8 characters of the plaintext token stored in `token_prefix` for fast index lookup. The plaintext token is never stored — returned once at provisioning, never again.

**Why HMAC-SHA256 over bcrypt:** Crawler nodes poll `GET /node/work` every few seconds. bcrypt at cost 12 takes ~250ms per check — an unbearable tax on the hot path. HMAC-SHA256 is microseconds. The 128-bit UUID v4 token provides sufficient brute-force resistance without password-stretching.

### `RequireNodeToken` Middleware

```
1. Extract Bearer token from Authorization header
2. token_prefix = token[:8]
3. SELECT * FROM crawler_nodes WHERE token_prefix = ?   (index scan)
4. expected = HMAC-SHA256(token, CRAWLER_TOKEN_SECRET)
5. !hmac.Equal(expected, node.token_hash) → 401 NODE_UNAUTHORIZED  (constant-time)
6. node.status != 'active' → 401/403 per status
7. Fire-and-forget: UPDATE crawler_nodes SET last_seen_at = NOW() WHERE id = ?
8. Inject NodeIdentity{ID, Name} via nodeidentity.NewContext()
```

The `last_seen_at` write is async (goroutine) — not on the request critical path. This write on every `/node/work` poll is the de-facto node heartbeat; no separate heartbeat endpoint is needed.

### `NodeIdentity` Context Pattern

Lives in `backend/apps/orchestrator/nodeidentity/` — mirrors `common/identity` exactly.

```go
type NodeIdentity struct { ID, Name string }
```

### New Error Sentinels (add to `common/errors/`)

```go
ErrNodeUnauthorized   // 401
ErrNodeDisabled       // 403
ErrAssignmentNotFound // 404
ErrAssignmentMismatch // 403
ErrInvalidAdapter     // 400
```

---

## HTTP API Contract

### `GET /node/work`

```
Query params:
  adapter   string   required — ATS adapter type
  holding   string   optional — comma-separated in-flight queue UUIDs (heartbeat piggyback)

Response 200:
{
  "data": {
    "assignment": {
      "id":             "uuid",          // company_scan_queue.id (lease handle)
      "company_id":     "uuid",
      "adapter_type":   "greenhouse",
      "adapter_config": null,            // JSON blob for ConfigurableAdapter; null for simple adapters
      "known_ids":      ["gh-123"],      // ledger snapshot — inject into KnownJobsProvider + echo back in results
      "source":         "scheduler"
    },
    "heartbeat_acked":  ["uuid1"],       // leases successfully extended
    "heartbeat_missed": ["uuid2"]        // leases gone — node must abort these assignments immediately
  }
}

No work: 204 (assignment: null shortcut)
```

**Orchestrator behavior:**

1. `NodeStore.UpdateLastSeen` (async)
2. `AssignmentStore.ExtendHeartbeats(nodeID, holdingIDs)` → populate `heartbeat_acked` / `heartbeat_missed`
3. `AssignmentStore.ClaimNextForNode(nodeID, adapterType)` → nil = respond 204
4. `ScanSettingsReader.Get(companyID)` for `adapter_config`
5. `LedgerStore.GetActiveJobIDsByCompany(companyID)` for `known_ids`
6. Assemble and return `WorkUnit`

### `POST /node/results`

```go
type ResultsRequest struct {
    AssignmentID     string       `json:"assignment_id"`      // required; UUID
    CompanyID        string       `json:"company_id"`         // required; UUID
    AdapterType      string       `json:"adapter_type"`       // required
    Jobs             []CrawledJob `json:"jobs"`               // all found jobs; empty array = no jobs
    PreviousKnownIDs []string     `json:"previous_known_ids"` // echo of WorkUnit.KnownIDs (removed-job detection)
    FetchedCount     int          `json:"fetched_count"`
    DurationMs       int64        `json:"duration_ms"`
    Final            bool         `json:"final"` // false = streaming partial; true = scan complete
}

type CrawledJob struct {
    ID             string             `json:"id"`
    Title          string             `json:"title"`
    URL            string             `json:"url"`
    Location       string             `json:"location"`
    EmploymentType string             `json:"employment_type"`
    LocationKnown  bool               `json:"location_known"`
    AllLocations   []string           `json:"all_locations"`
    FullContent    *CrawledJobContent `json:"full_content,omitempty"`
}
```

**Orchestrator behavior (mirrors `engine.publishNewJobs` exactly):**

1. Verify assignment ownership (`locked_by = nodeID`)
2. Non-USA gate: drop `LocationKnown=true` jobs where all locations are non-USA → `MarkSeenBatch(reason=non_usa)`
3. `LedgerStore.MarkSeenBatch` **before** NATS publish (existing ledger-before-publish invariant)
4. Publish new jobs to `jobs.raw.<adapter_type>` via NATS
5. Removed-job detection: diff `PreviousKnownIDs` vs `jobs[].ID` in-memory → `MarkRemovedBatch`
6. On `final: false` — do NOT complete queue row (streaming: more batches coming)
7. On `final: true` — `AssignmentStore.CompleteForNode`, `ScanLogStore.RecordScanResult`

**Idempotent:** `MarkSeenBatch` uses `ON CONFLICT DO NOTHING`; `CompleteForNode` swallows `ErrQueueRowMissing`.

**Streaming support:** Adapters using `StreamPublisher` (Google, Meta) call `POST /node/results` with `final: false` per page and `final: true` at end. Non-streaming adapters: single call with `final: true`.

### `POST /node/error`

```go
type ErrorRequest struct {
    AssignmentID string `json:"assignment_id"` // required
    CompanyID    string `json:"company_id"`    // required
    AdapterType  string `json:"adapter_type"`  // required
    ErrorMessage string `json:"error_message"` // required; max 2000 chars
    DurationMs   int64  `json:"duration_ms"`
}
```

**Orchestrator behavior:** `AssignmentStore.FailForNode` (resets to 'queued') + `ScanLogStore.RecordScanResult` with error. Idempotent: `ErrQueueRowMissing` silently ignored.

### Admin Endpoints (`/admin/crawler-nodes`)

| Method   | Path                       | Auth        | Purpose                                                |
| -------- | -------------------------- | ----------- | ------------------------------------------------------ |
| `GET`    | `/admin/crawler-nodes`     | JWT + admin | List nodes; online/offline derived from `last_seen_at` |
| `POST`   | `/admin/crawler-nodes`     | JWT + admin | Provision node; returns plaintext token once           |
| `PATCH`  | `/admin/crawler-nodes/:id` | JWT + admin | Enable / disable                                       |
| `DELETE` | `/admin/crawler-nodes/:id` | JWT + admin | Retire; clears `token_hash` immediately                |

---

## Crawler Node Internal Design

### Interfaces (`backend/apps/crawler-node/interfaces.go`)

```go
// OrchestratorClient is the node's only external dependency.
type OrchestratorClient interface {
    GetWork(ctx context.Context, holding []string) (*WorkUnit, error)
    SubmitBatch(ctx context.Context, batch PageBatch) error
    ReportError(ctx context.Context, e WorkError) error
}

// AdapterExecutor runs one WorkUnit; injectable for tests.
type AdapterExecutor interface {
    Execute(ctx context.Context, unit WorkUnit) error
}
```

### Adapter Wiring (`executor.go`)

```
Execute(ctx, unit):
  adp = ats.RegistryWithGoogleConfig(unit.AdapterType, googleCfg, log)

  KnownJobsProvider → inject unit.KnownIDs directly (no HTTP call needed)
  SeenChecker       → inject closure over knownIDsSet (O(1) map lookup — no /node/seen endpoint)
  StreamPublisher   → inject fn calling client.SubmitBatch(..., Final: false) per page

  result = adp.FetchJobs / FetchJobsWithConfig

  if !streamed:
    client.SubmitBatch(PageBatch{Items: result.Jobs, Final: true})
  else:
    client.SubmitBatch(PageBatch{Items: nil, Final: true})  // terminal signal
```

**Why no `/node/seen` endpoint:** The `known_ids` snapshot in `WorkUnit` is sufficient for all current adapters. The orchestrator prevents duplicate dispatch via `FOR UPDATE SKIP LOCKED`, so the "stale snapshot" risk (another node inserting a job during this scan) cannot occur for the same company. The orchestrator's `MarkSeenBatch` (`ON CONFLICT DO NOTHING`) is the authoritative dedup gate regardless.

### Concurrency Model

```
main goroutine
└── PollLoop.Run(ctx)                       ← single poll goroutine
     │
     │  sem = make(chan struct{}, MAX_CONCURRENT)
     │  holding []string  (protected by sync.Mutex)
     │
     │  loop:
     │    GetWork(ctx, holding)             ← heartbeat piggyback via ?holding=
     │    if heartbeat_missed: remove from holding
     │    if unit != nil:
     │      sem <- struct{}{}              ← blocks if at MAX_CONCURRENT
     │      wg.Add(1)
     │      go workerGoroutine(WithoutCancel(ctx), unit)
     │    else: sleep(POLL_INTERVAL)
     │
     │  shutdown: wg.Wait() then return
     │
     ├─ [worker G]  addHolding → Execute → removeHolding → <-sem → wg.Done
     └─             defer recover() → ReportError on panic
```

`context.WithoutCancel` (Go 1.21+) on worker goroutines: adapter scans complete naturally on graceful shutdown rather than being aborted mid-page.

### Configuration

| Env Var                   | Default | Validation |
| ------------------------- | ------- | ---------- |
| `ORCHESTRATOR_URL`        | —       | required   |
| `CRAWLER_TOKEN`           | —       | required   |
| `POLL_INTERVAL_SECONDS`   | `10`    | ≥ 1        |
| `MAX_CONCURRENT`          | `3`     | [1, 20]    |
| `ADAPTER_TIMEOUT_SECONDS` | `600`   | ≥ 30       |
| `HTTP_TIMEOUT_SECONDS`    | `15`    | ≥ 5        |
| `RETRY_MAX_ATTEMPTS`      | `3`     | [1, 10]    |
| `LOG_LEVEL`               | `info`  |            |

---

## Deployment

### `infra/Dockerfile.crawler-node`

Multi-stage Go build. Build context: `./backend` (same as existing `Dockerfile.crawler` — no change to CI build context).

- CGO_ENABLED=0, pure Go, alpine final image (~8 MB)
- Non-root UID 65532 — matches existing security posture
- Only `ORCHESTRATOR_URL` and `CRAWLER_TOKEN` required at runtime

### Provisioning a Node (two steps)

```bash
# Step 1: Admin provisions node record (UI later, or API now)
# POST /admin/crawler-nodes {"name": "prod-us-east-1"}
# → {"token": "xxxxxxxx-xxxx-4xxx-..."} — save this, never shown again

# Step 2: On target server
docker run -d \
  --name crawler-node \
  --restart=always \
  -e ORCHESTRATOR_URL=https://orchestrator.internal \
  -e CRAWLER_TOKEN=<token-from-step-1> \
  jobradar/crawler-node:latest
```

Watchtower on the same host handles auto-updates when a new image is pushed to the registry. `--restart=always` handles process crashes and host reboots automatically.

### Orchestrator (k8s)

Deployed as a standard k8s Deployment alongside the existing API. Wires into the same Postgres and NATS JetStream connections. Runs a background goroutine that calls `ResetStuckForNodes` every 5 minutes to reclaim assignments from silent nodes.

---

## Coexistence Strategy

The existing per-adapter crawler workers and the new orchestrator both claim from `company_scan_queue` using `FOR UPDATE SKIP LOCKED`. The existing unique partial index (`uq_company_scan_queue_active`) prevents any company from having more than one active row — so two claimants can never get the same row. No code changes to the existing crawler.

The `locked_by` column (migration 079) is additive metadata: `NULL` for rows claimed by the old crawler, non-null for rows claimed by the orchestrator. This enables node-targeted `ResetStuck` and per-node audit in the admin UI without affecting existing behavior.

**Transition:** Start with a small set of companies routed to the new system. Validate correctness. Expand. Retire old workers when confidence is high.

---

## Wire File — Orchestrator

The orchestrator wires only two service domains:

```
OrchestratorServer
├── crawlernode.NodeService      ← NEW service slice
│   ├── CrawlerNodeRepositoryService      → crawler_nodes table
│   ├── CrawlerNodeAssignmentRepositoryService → company_scan_queue (node-mode claiming)
│   ├── CrawlerNodeLedgerRepositoryService     → job_pipeline_ledger (narrow facade)
│   ├── CrawlerNodeScanLogRepositoryService    → company_scan_log (delegates to scan.ScanLogWriter)
│   └── common/jetstream.Publisher             → NATS publishing
│
└── scan.ScanSettingsReader      ← REUSED (scan settings for adapter_config + careers_url)
```

The orchestrator does **not** wire the API's user-facing REST chains, company CRUD, or dashboard services.

---

## CI Additions

New job in `.github/workflows/backend.yml` path filter:

```yaml
orchestrator:
  - 'backend/apps/orchestrator/**'
  - 'backend/apps/crawler-node/**'
  - 'backend/service/crawlernode/**'
```

New `build-image` step for `Dockerfile.crawler-node` on push to `main`, pushing to registry as `jobradar/crawler-node:<sha>` and `:latest`.

---

## What Does Not Change

| Component                                                            | Status                                                        |
| -------------------------------------------------------------------- | ------------------------------------------------------------- |
| `backend/apps/crawler/` (per-adapter workers)                        | Untouched                                                     |
| `backend/apps/scheduler/`                                            | Untouched                                                     |
| `backend/apps/filter/`                                               | Untouched                                                     |
| `backend/apps/analyzer/`                                             | Untouched                                                     |
| `backend/apps/api/`                                                  | Untouched (scan trigger still writes to `company_scan_queue`) |
| All 14+ ATS adapter implementations                                  | Untouched                                                     |
| `ats.Adapter`, `ConfigurableAdapter`, `SeenChecker`, etc. interfaces | Untouched                                                     |
| NATS stream subjects (`jobs.raw.*`, `jobs.filtered.*`)               | Untouched                                                     |
| `job_pipeline_ledger` semantics                                      | Untouched                                                     |

---

## Open Questions (deferred, not blocking implementation)

| #   | Question                                                                                                                                                 |
| --- | -------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1   | Per-node adapter type restrictions (allow only certain adapters per node) — `adapter_types[]` column deferred; all nodes accept all adapters initially   |
| 2   | Node capacity signalling (load-aware dispatch) — orchestrator assigns round-robin initially; weighted dispatch deferred until multiple nodes are running |
| 3   | mTLS upgrade from Bearer token — defer until fleet reaches >10 nodes                                                                                     |
| 4   | Crawler node UI (fleet view, per-node status, manual drain) — backend endpoints are built now; frontend deferred                                         |
