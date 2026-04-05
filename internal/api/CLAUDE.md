# internal/api/ — REST API

## Structure

```
router.go           — mounts all routes, applies global middleware
handler/
  respond.go        — shared respond() and respondError() helpers
  jobs.go           — JobHandler
  applications.go   — ApplicationHandler
  companies.go      — CompanyHandler
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
DELETE /jobs/:id
GET    /jobs/:id/applications
POST   /jobs/:id/applications
GET    /companies
```
