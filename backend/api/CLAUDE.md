# api/ — REST API

## Structure

```
router.go           — mounts all routes, applies global middleware
handler/
  respond.go        — shared respond() and respondError() helpers
  jobs.go           — JobHandler
  applications.go   — ApplicationHandler
  companies.go      — CompanyHandler
  dashboard.go      — DashboardHandler
  insights.go       — InsightsHandler
middleware/
  logger.go         — logs method, path, status, duration per request
```

## respond helpers (always use these)

```go
respond(w, http.StatusOK, payload)           // any status + any body
respondError(w, http.StatusBadRequest, "msg") // writes {"error": "msg"}
```

Never write `json.NewEncoder` or `w.WriteHeader` directly in handlers.

## Validation pattern

Validate at the handler level before touching the store.
Use a `map[string]bool` for enum checks:

```go
valid := map[string]bool{
    model.JobStatusNew:     true,
    model.JobStatusSaved:   true,
    model.JobStatusApplied: true,
}
if !valid[body.Status] {
    respondError(w, http.StatusBadRequest, "invalid status value")
    return
}
```

## Routes

```
GET    /jobs
GET    /jobs/:id
PATCH  /jobs/:id/save
PATCH  /jobs/:id/status
PATCH  /jobs/:id/notes
DELETE /jobs/:id                          — sets status=dismissed, backfills snapshot
GET    /jobs/:id/applications
POST   /jobs/:id/applications
PATCH  /jobs/:id/applications/:app_id/stage
PATCH  /jobs/:id/applications/:app_id/note
GET    /applications

GET    /companies
GET    /companies/queue                 — active scan queue entries (queued + processing)
POST   /companies
PATCH  /companies/:id
DELETE /companies/:id
GET    /companies/:id/scan-stats
POST   /companies/:id/scan              — always enqueues; manual triggers bypass the cooldown

GET    /dashboard/stats
GET    /dashboard/activity?days=N

GET    /insights/tech-stack
GET    /insights/company-activity
GET    /insights/funnel
GET    /insights/location
GET    /insights/sponsorship?days=N
GET    /insights/velocity
GET    /insights/role-categories
```

## Manual scan rule

`CompanyHandler` holds `store.ScanQueueStore` (not `kafka.Publisher`).
`POST /companies/:id/scan` enforces the same `SCAN_INTERVAL_MINUTES` cooldown as the scheduler — returns 429 if `company.LastScanAt` is within the interval. If the cooldown passes, it calls `queue.Enqueue`.

`queue.Enqueue` uses `ON CONFLICT DO NOTHING` — calling it for a company already in the queue is safe and silently no-ops.

## Snapshot backfill rule

`JobHandler` holds a `store.SnapshotWriter`. After any user action that changes job state, backfill the snapshot — but always non-fatally (`_ =`):

```go
_ = h.snapshot.UpdateSnapshotAction(id, "saved")    // PATCH /save
_ = h.snapshot.UpdateSnapshotAction(id, "applied")  // PATCH /status → applied
_ = h.snapshot.UpdateSnapshotAction(id, "dismissed") // DELETE
```

Snapshot rows may not exist for jobs enriched before migration 011 — that's expected.
