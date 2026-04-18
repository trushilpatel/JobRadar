# api/ — REST API

## Structure

```
router.go           — NewRouter: mounts global middleware, operational endpoints, delegates to RouteGroups
handler/
  handler.go        — RouteGroup interface
  respond.go        — Respond[T], RespondPaged[T], RespondError, HandleError helpers
middleware/
  logger.go         — logs method, path, status, duration per request
  metrics.go        — Prometheus request counter + latency histogram
request/            — shared request parsing helpers
response/           — shared response type helpers
```

## RouteGroup pattern

Each service package implements `handler.RouteGroup` and registers its own routes:

```go
type RouteGroup interface {
    Register(r chi.Router)
}
```

`NewRouter` accepts `[]handler.RouteGroup` and calls `Register` on each. To add a new resource: create a handler in the service package, implement `RouteGroup`, and add it to the Wire provider set. `NewRouter` never needs to change.

## Respond helpers (always use these)

```go
handler.Respond(w, http.StatusOK, payload)           // typed JSON response
handler.RespondPaged(w, items, total, page, limit)   // paginated JSON
handler.HandleError(w, err)                          // maps domain errors → HTTP status automatically
```

Never write `json.NewEncoder` or `w.WriteHeader` directly in handlers.

## Error mapping

Handlers call `handler.HandleError(w, err)`. The helper inspects the error:

- `*domain.ValidationError` → 400 Bad Request
- `*domain.ServiceError` → uses `.Status` field (404, 409, 400, 500)
- anything else → 500 Internal Server Error

Service methods return typed domain errors using sentinels in `domain/errors.go`:

```go
return domain.ErrNotFound.WithMessage("job not found")   // 404
return domain.ErrInternal.WithMessage("db failed")       // 500
```

## Validation pattern

Validate at the handler level via the domain's `Validator` before calling the service:

```go
if err := h.validator.ValidateDismiss(input); err != nil {
    handler.HandleError(w, err)  // ValidationError → 400
    return
}
```

## Operational endpoints

Served by the router directly (not by any RouteGroup):

- `GET /health` — always 200
- `GET /ready` — 200 if DB ping succeeds, 503 otherwise
- `GET /metrics` — Prometheus scrape endpoint

## Routes (by service package)

Routes are defined in each service package's `handler.go`. See each package for the full route table.

- Job routes: `service/job/handler.go`
- Company routes: `service/company/handler.go`
- Application routes: `service/application/handler.go`
- Dashboard routes: `service/dashboard/handler.go`
- Insights routes: `service/insights/handler.go`
