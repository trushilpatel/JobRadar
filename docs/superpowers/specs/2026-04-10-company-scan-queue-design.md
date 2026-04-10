# Company Scan Queue Design

**Date:** 2026-04-10  
**Status:** Implemented

## Problem

The scheduler→crawler handoff previously used Kafka (`company.scan_request`). This had two gaps:

1. **No visibility** — no way to query which companies were waiting to be scanned.
2. **No dedup** — a company published to Kafka but not yet processed could be re-published by the scheduler or a manual trigger.

## Solution

Replaced the Kafka hop with a persistent `company_scan_queue` DB table. The table acts as a durable, queryable work queue. The scheduler and manual trigger insert into it; the crawler polls it. `TopicCompanyScanRequest` removed entirely.

The other Kafka topics (`job_listings.raw`, `job_listings.filtered`) are unchanged.

## Database

**Migration:** `backend/migrations/014_company_scan_queue.sql`

```sql
CREATE TABLE company_scan_queue (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id  TEXT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    status      TEXT NOT NULL DEFAULT 'queued',    -- 'queued' | 'processing'
    source      TEXT NOT NULL DEFAULT 'scheduler', -- 'scheduler' | 'manual'
    queued_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at  TIMESTAMPTZ
);

CREATE UNIQUE INDEX uq_company_scan_queue_active
    ON company_scan_queue(company_id)
    WHERE status IN ('queued', 'processing');
```

The unique partial index enforces at most one active entry per company at the DB level — concurrent inserts are safe without any application-level locking.

## Store

**File:** `backend/internal/store/scan_queue.go`

```go
type ScanQueueItem struct {
    ID        string     `json:"id"`
    CompanyID string     `json:"company_id"`
    Status    string     `json:"status"`
    Source    string     `json:"source"`
    QueuedAt  time.Time  `json:"queued_at"`
    StartedAt *time.Time `json:"started_at"`
}

type ScanQueueStore interface {
    Enqueue(companyID, source string) error          // INSERT ON CONFLICT DO NOTHING
    ClaimNext(ctx context.Context) (*ScanQueueItem, error) // UPDATE ... FOR UPDATE SKIP LOCKED
    Complete(id string) error                        // DELETE
    ResetStuck(staleDur time.Duration) error         // reset processing rows older than staleDur
    ListQueue() ([]ScanQueueItem, error)             // SELECT all active rows
}
```

`ClaimNext` uses `UPDATE ... WHERE id = (SELECT ... FOR UPDATE SKIP LOCKED LIMIT 1) RETURNING ...` — safe for concurrent crawler instances.

## Architecture

### Scheduler (`cmd/company_scan_scheduler`)

`enqueuedue` calls `queue.Enqueue(c.ID, model.QueueSourceScheduler)`. Kafka producer removed. Dedup is enforced by the unique index — re-enqueuing a company already in the queue silently no-ops.

### Crawler (`cmd/job_listing_crawler`)

Kafka consumer replaced with a DB poll loop (10s interval):

```
on startup  → queue.ResetStuck(30 minutes)
on each tick:
    item ← queue.ClaimNext()
    if nil → continue (queue empty)
    engine.RunForCompany(ctx, item.CompanyID)
    queue.Complete(item.ID)
```

`engine.RunForCompany` is unchanged. `queue.Complete` is called whether the scan succeeds or fails — the scheduler re-enqueues on the next cycle if needed.

### API

`CompanyHandler` now holds `store.ScanQueueStore` instead of `kafka.Publisher`.

- `POST /companies/:id/scan` — calls `queue.Enqueue(id, model.QueueSourceManual)`. Scan-interval cooldown (429) unchanged.
- `GET /companies/queue` — returns all active queue entries ordered by `queued_at`.

## Lifecycle

```
scheduler / POST /companies/:id/scan
    → queue.Enqueue(companyID, source)   -- ON CONFLICT DO NOTHING

crawler tick (every 10s)
    → item = queue.ClaimNext()           -- FOR UPDATE SKIP LOCKED
    → if nil: skip
    → engine.RunForCompany(companyID)    -- unchanged
    → queue.Complete(item.ID)            -- DELETE row
    → company_scan_log written           -- unchanged
    → companies.last_scan_at updated     -- unchanged
```

## Crash Recovery

If the crawler crashes mid-scan, the queue row stays in `processing`. On next startup, `ResetStuck(30min)` resets stale rows back to `queued` so they are retried on the next poll.

## What Did Not Change

- `job_listings.raw` and `job_listings.filtered` Kafka topics
- `engine.RunForCompany` internals
- `company_scan_log` writes
- `companies.last_scan_at` update
- Scan interval cooldown on `POST /companies/:id/scan`
- `job_seen_ledger` dedup for individual job listings

## Future Queue Management

The table is designed for extension without touching other tables:

- **Priority** — add `priority INT`, `ClaimNext` orders by `priority DESC, queued_at ASC`
- **Cancellation** — `DELETE /companies/queue/:id` removes a queued (not processing) entry
- **Pause/resume** — add a `paused` status; scheduler skips paused companies
- **Retry count** — add `attempt INT`; cap retries before deactivating a company
