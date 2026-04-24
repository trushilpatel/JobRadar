# Crawler Adapter Lanes Design

**Date:** 2026-04-24  
**Status:** Approved — implementing

## Overview

Replace the global single-loop crawler with a multi-lane worker pool where each adapter type owns its own crawler service. Per-adapter scan intervals and inter-company delays are stored in `adapter_configs` (DB), replacing the global `SCAN_COMPANY_DELAY_SECONDS` and `SCAN_INTERVAL_MINUTES` env vars (kept as fallback defaults only).

## Problem

- Single global `SCAN_COMPANY_DELAY_SECONDS` applies equally to Greenhouse JSON API (fast, safe) and Google browser scraper (slow, anti-bot) — wrong
- No per-adapter scan interval — all companies scan every 24h regardless of adapter characteristics
- Single-threaded poll loop: one company at a time across all adapters, no fairness across adapter types
- Browser adapters (Google, Meta) not serialised — multiple pods could run concurrent browser scans

## Solution: Multi-Lane Worker Pool

Each adapter type gets its own crawler service (one goroutine, one worker) that manages its lane independently. All state coordination happens through PostgreSQL — no external queue, no Redis, no advisory locks held during scans.

---

## Schema

### New table: `adapter_configs`

```sql
CREATE TABLE adapter_configs (
    adapter_type            TEXT PRIMARY KEY,
    scan_interval_minutes   INT  NOT NULL DEFAULT 1440 CHECK(scan_interval_minutes > 0),
    inter_company_delay_s   INT  NOT NULL DEFAULT 5    CHECK(inter_company_delay_s >= 0),
    last_scheduled_at       TIMESTAMPTZ,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

Seed data (one row per adapter type, conservative defaults):

| adapter_type | scan_interval_minutes | inter_company_delay_s |
|---|---|---|
| greenhouse | 1440 | 3 |
| lever | 1440 | 3 |
| ashby | 1440 | 3 |
| netflix | 1440 | 5 |
| uber | 1440 | 5 |
| microsoft | 1440 | 3 |
| rippling | 1440 | 3 |
| apple | 1440 | 3 |
| google | 1440 | 30 |
| meta | 1440 | 30 |

### Modified: `company_scan_queue`

Add `adapter_type` column (denormalised from `companies` at enqueue time — avoids JOIN on every claim):

```sql
ALTER TABLE company_scan_queue ADD COLUMN adapter_type TEXT NOT NULL DEFAULT '';
UPDATE company_scan_queue q SET adapter_type = c.adapter_type FROM companies c WHERE q.company_id = c.id;
```

### Modified: `companies`

Add nullable per-company scan interval override:

```sql
ALTER TABLE companies
  ADD COLUMN scan_interval_minutes_override INT NULL
  CHECK(scan_interval_minutes_override IS NULL OR scan_interval_minutes_override > 0);
```

NULL = use adapter default. When set, takes precedence over `adapter_configs.scan_interval_minutes`.

---

## Architecture

```
Crawler Pod (N replicas, all identical)
├── GreenhouseCrawlerService  (1 goroutine)
├── LeverCrawlerService       (1 goroutine)
├── AshbyCrawlerService       (1 goroutine)
├── NetflixCrawlerService     (1 goroutine)
├── UberCrawlerService        (1 goroutine)
├── MicrosoftCrawlerService   (1 goroutine)
├── RipplingCrawlerService    (1 goroutine)
├── AppleCrawlerService       (1 goroutine)
├── GoogleCrawlerService      (1 goroutine — browser, keeps Chromium warm)
└── MetaCrawlerService        (1 goroutine — browser, keeps Node/Playwright warm)

Total goroutines per pod: 10 workers + 1 heartbeat per active scan. Fixed. Never grows.
```

Browser serialisation is guaranteed by `FOR UPDATE` on the `adapter_configs` row during `ClaimNext` — only one pod in the cluster can claim a Google or Meta item at a time.

---

## ClaimNext SQL (atomic, distributed-safe)

Each crawler service calls `ClaimNext(adapterType string)`. One transaction:

```sql
BEGIN;

-- 1. Lock this adapter's config row. Serialises concurrent pod claims for this adapter.
--    If delay has not passed: no row returned → return nil (worker sleeps, retries).
SELECT inter_company_delay_s
FROM adapter_configs
WHERE adapter_type = $1
  AND (
    last_scheduled_at IS NULL
    OR NOW() - last_scheduled_at >= inter_company_delay_s * INTERVAL '1 second'
  )
FOR UPDATE;

-- 2. Claim oldest queued item for this adapter type.
UPDATE company_scan_queue
SET status = 'processing', started_at = NOW(), heartbeat_at = NOW()
WHERE id = (
    SELECT id FROM company_scan_queue
    WHERE status = 'queued' AND adapter_type = $1
    ORDER BY priority DESC, queued_at ASC
    LIMIT 1
    FOR UPDATE SKIP LOCKED
)
RETURNING id, company_id, adapter_type, source;

-- 3. Record when this adapter was last claimed (enforces delay for next claim).
UPDATE adapter_configs SET last_scheduled_at = NOW() WHERE adapter_type = $1;

COMMIT;
```

If step 1 returns no row (delay not passed), transaction returns nil — worker sleeps for `pollInterval` and retries. No sleep inside the worker loop; delay is enforced at claim time.

---

## Folder Structure

```
apps/crawler/
├── main.go              ← wiring: build deps, register all crawler services, start registry
├── engine.go            ← unchanged
├── interfaces.go        ← add CrawlerQueueService interface
│
├── workers/             ← package workers — interface + base + registry only
│   ├── service.go       ← AdapterCrawlerService interface, Scanner interface, AdapterConfig
│   ├── base.go          ← BaseWorker: claim → heartbeat → scan → complete loop
│   └── registry.go      ← Registry: Register, StartAll(ctx, wg)
│
└── ats/
    ├── greenhouse/
    │   ├── greenhouse.go    ← unchanged
    │   └── crawler.go       ← GreenhouseCrawlerService (embeds BaseWorker)
    ├── lever/
    │   ├── lever.go
    │   └── crawler.go
    ├── ashby/
    │   └── crawler.go
    ├── netflix/
    │   └── crawler.go
    ├── uber/
    │   └── crawler.go
    ├── microsoft/
    │   └── crawler.go
    ├── rippling/
    │   └── crawler.go
    ├── apple/
    │   └── crawler.go
    ├── google/
    │   ├── google.go        ← unchanged
    │   ├── rod.go           ← unchanged
    │   └── crawler.go       ← GoogleCrawlerService (custom: keeps browser warm between scans)
    └── meta/
        ├── meta.go          ← unchanged
        └── crawler.go       ← MetaCrawlerService (custom: keeps Node process warm)
```

---

## workers/ Package

### `workers/service.go`

```go
package workers

type AdapterCrawlerService interface {
    AdapterType() string
    Run(ctx context.Context)
}

// Scanner is the narrow interface workers use to trigger scans.
// Engine in package main satisfies this.
type Scanner interface {
    RunForCompany(ctx context.Context, companyID string)
}

// AdapterConfig holds per-adapter runtime config loaded from adapter_configs table.
type AdapterConfig struct {
    AdapterType          string
    ScanIntervalMinutes  int
    InterCompanyDelayS   int
}
```

### `workers/base.go`

```go
type BaseWorker struct {
    adapterType string
    queue       CrawlerQueueService  // from interfaces.go in package main, re-declared here
    scanner     Scanner
    log         logger.Logger
    pollInterval time.Duration       // how long to sleep when queue is empty
}

func (w *BaseWorker) RunLoop(ctx context.Context) {
    for {
        item, err := w.queue.ClaimNext(ctx, w.adapterType)
        if err != nil {
            w.log.Error("claim failed", err)
            w.sleep(ctx, w.pollInterval)
            continue
        }
        if item == nil {
            w.sleep(ctx, w.pollInterval)  // queue empty or delay not passed
            continue
        }

        stop := w.startHeartbeat(ctx, item.ID)
        w.scanner.RunForCompany(ctx, item.CompanyID)
        w.queue.Complete(ctx, item.ID)
        stop()
        // no sleep here — delay enforced at ClaimNext level
    }
}
```

### `workers/registry.go`

```go
type Registry struct {
    services []AdapterCrawlerService
}

func (r *Registry) Register(svc AdapterCrawlerService) {
    r.services = append(r.services, svc)
}

func (r *Registry) StartAll(ctx context.Context, wg *sync.WaitGroup) {
    for _, svc := range r.services {
        wg.Add(1)
        go func(s AdapterCrawlerService) {
            defer wg.Done()
            s.Run(ctx)
        }(svc)
    }
}
```

---

## Per-Adapter Crawler Services

### API adapters (greenhouse, lever, ashby, netflix, uber, microsoft, rippling, apple)

All identical — embed BaseWorker, nothing else:

```go
// ats/greenhouse/crawler.go
package greenhouse

type CrawlerService struct{ workers.BaseWorker }

func NewCrawlerService(q workers.CrawlerQueueService, s workers.Scanner, log logger.Logger) *CrawlerService {
    return &CrawlerService{BaseWorker: workers.NewBaseWorker("greenhouse", q, s, log)}
}

func (s *CrawlerService) AdapterType() string { return "greenhouse" }
func (s *CrawlerService) Run(ctx context.Context) { s.RunLoop(ctx) }
```

### Browser adapters (google, meta)

Override `Run` to keep the browser/process warm between scans:

```go
// ats/google/crawler.go
package google

type CrawlerService struct {
    workers.BaseWorker
    cfg Config
}

func (s *CrawlerService) AdapterType() string { return "google" }

func (s *CrawlerService) Run(ctx context.Context) {
    // Launch browser once; reuse across scans in this lane.
    // If browser crashes, relaunch and continue.
    for {
        browser, close, err := s.launchBrowser()
        if err != nil {
            s.log.Error("browser launch failed", err)
            s.sleep(ctx, 30*time.Second)
            continue
        }
        s.runWithBrowser(ctx, browser)
        close()
        // runWithBrowser returned → ctx cancelled or fatal browser error
        if ctx.Err() != nil { return }
    }
}
```

---

## Scheduler Changes

`EnqueueDue` SQL joins `adapter_configs` to get per-adapter scan intervals. Company override takes precedence. Global `SCAN_INTERVAL_MINUTES` env var is the final fallback:

```sql
INSERT INTO company_scan_queue (company_id, source, adapter_type)
SELECT c.id, 'scheduler', c.adapter_type
FROM companies c
LEFT JOIN adapter_configs ac ON ac.adapter_type = c.adapter_type
WHERE c.active = true
  AND NOT EXISTS (
    SELECT 1 FROM company_scan_queue q
    WHERE q.company_id = c.id AND q.status IN ('queued','processing')
  )
  AND (
    c.last_scan_at IS NULL
    OR c.last_scan_at + COALESCE(
        c.scan_interval_minutes_override,
        ac.scan_interval_minutes,
        $1  -- global fallback from SCAN_INTERVAL_MINUTES env var
    ) * INTERVAL '1 minute' <= NOW()
  )
ORDER BY c.last_scan_at ASC NULLS FIRST
ON CONFLICT DO NOTHING;
```

---

## Service Layer (adapter_configs CRUD)

Lives in `service/scan/` following the existing decorator chain pattern.

### New files
- `service/scan/models/AdapterConfig.go` — GORM model
- `service/scan/dto/AdapterConfig.go` — DTO + input types
- `service/scan/services/AdapterConfigRepositoryService.go` — store (GORM)
- `service/scan/services/AdapterConfigImplService.go` — business logic
- `service/scan/services/AdapterConfigValidationService.go` — validation decorator
- `service/scan/services/AdapterConfigRestService.go` — HTTP handler
- `service/scan/services/AdapterConfigRouter.go` — route registration

### API Endpoints

```
GET    /adapter-configs                         → list all (for UI settings table)
PATCH  /adapter-configs/:adapter_type           → update interval + delay
PATCH  /companies/:id/scan-interval-override    → set/clear company override (null to clear)
GET    /companies/:id/scan-config               → effective config (merged adapter default + override)
```

Validation:
- `scan_interval_minutes`: 1–10080 (1 min to 1 week)
- `inter_company_delay_s`: 0–3600
- `scan_interval_minutes_override`: null or 1–10080

---

## UI: Settings Page

**Route:** `/settings`  
**Nav:** Settings icon in Sidebar below Insights

### Adapter Configs Table (`/src/features/settings/`)

~10-row table with inline editing:

| Column | Editable | Validation |
|---|---|---|
| Adapter Type | No | — |
| Scan Interval (min) | Yes | 1–10080 |
| Inter-Company Delay (s) | Yes | 0–3600 |
| Last Scheduled | No | — |
| Companies | No | count from API |

Inline edit: click cell → input → Enter/blur → optimistic update → save.  
On error: revert cell, show inline message.  
Confirmation modal when changing an adapter that affects N companies.

### Company Override

In `CompanyDetailDrawer` — new section after scan stats:

```
Scan Interval
├─ Adapter default: 30 min  (from adapter_configs)
└─ Custom override: — (not set)     [Set override]
```

When override is set:
```
Scan Interval
├─ Adapter default: 30 min
└─ Custom: 15 min  ✕ (click to clear)
```

### New Components

```
src/features/settings/
├── components/
│   ├── AdapterConfigsTable.tsx
│   ├── AdapterConfigCell.tsx       ← inline edit cell
│   └── ScanIntervalOverrideModal.tsx
├── hooks/
│   ├── useAdapterConfigs.ts
│   └── useAdapterConfigActions.ts
└── index.ts

src/pages/SettingsPage.tsx
src/api/adapterConfigs.ts
```

State: TanStack Query for server state, local `useState` for edit mode per cell. No new Zustand store needed.

---

## Crash Recovery

Unchanged from current design. Heartbeat ticks every `HEARTBEAT_INTERVAL_SECONDS`. `ResetStuck` (in scheduler) resets `processing` rows whose `heartbeat_at` is older than `STUCK_TIMEOUT_SECONDS`. The `adapter_configs.last_scheduled_at` is left stale after a crash — harmless, next successful claim overwrites it.

---

## Main.go After Change

```go
registry := workers.NewRegistry()

// API adapters
registry.Register(greenhouse.NewCrawlerService(queue, engine, log))
registry.Register(lever.NewCrawlerService(queue, engine, log))
registry.Register(ashby.NewCrawlerService(queue, engine, log))
registry.Register(netflix.NewCrawlerService(queue, engine, log))
registry.Register(uber.NewCrawlerService(queue, engine, log))
registry.Register(microsoft.NewCrawlerService(queue, engine, log))
registry.Register(rippling.NewCrawlerService(queue, engine, log))
registry.Register(apple.NewCrawlerService(queue, engine, log))

// Browser adapters (custom Run — keep browser warm)
registry.Register(google.NewCrawlerService(queue, engine, googleCfg, log))
registry.Register(meta.NewCrawlerService(queue, engine, log))

registry.StartAll(ctx, &wg)
wg.Wait()
```

Adding a new adapter: implement `crawler.go` in its ATS package + one `registry.Register` line. Nothing else changes.

---

## Testing Requirements

Every new file must have a corresponding test:
- DB migrations: verified by migrate runner
- `ClaimNext`: unit test with mock DB — verify delay enforcement, SKIP LOCKED behaviour
- `BaseWorker.RunLoop`: unit test with mock queue + scanner — verify claim/scan/complete cycle, sleep on nil
- Each `CrawlerService.AdapterType()`: trivial unit test
- `GoogleCrawlerService.Run`: test browser crash recovery (relaunch loop)
- Service layer: unit tests per decorator layer (validation, impl, store mock)
- API endpoints: handler tests with mock service
- UI components: vitest unit tests for AdapterConfigCell (save, error, revert)
