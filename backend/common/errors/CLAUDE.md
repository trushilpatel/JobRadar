# domain/ — Shared error types

## Types

- **`ServiceError`** — typed error returned by all service methods. Carries an HTTP status code and a short machine-readable code. Handlers call `handler.HandleError(w, err)` which inspects this type to pick the HTTP status.
- **`ValidationError`** — a single field-level validation failure (field, code, message). Kept for cases where only one check is possible; callers should prefer `ValidationErrors`.
- **`ValidationErrors`** — a slice of `ValidationError` returned by validators that collect all failures instead of stopping at the first. All five validator services return this type. `HandleError` detects it via `errors.As` (checked before `*ValidationError`) and writes `{ "errors": [...] }` with HTTP 400. A single `*ValidationError` is normalised into a one-element `ValidationErrors` slice by `HandleError` so the response shape is always consistent.

## Sentinels

```go
domain.ErrNotFound   // Code: NOT_FOUND,   Status: 404
domain.ErrConflict   // Code: CONFLICT,    Status: 409
domain.ErrBadRequest // Code: BAD_REQUEST, Status: 400
domain.ErrInternal   // Code: INTERNAL,    Status: 500
```

### Pipeline sentinels (internal — not user-facing)

Returned by store/worker layers so callers can branch on a specific failure instead of a swallowed nil. Compared via `errors.Is` (a Code-based `ServiceError.Is` makes this work even after `.WithMessage()`):

```go
ErrLedgerRowMissing  // Code: LEDGER_ROW_MISSING, Status: 409 — JobSeenLedger UPDATE matched 0/partial rows
ErrQueueRowMissing   // Code: QUEUE_ROW_MISSING,  Status: 404 — ScanQueue Complete/Remove/UpdateHeartbeat matched 0 rows
ErrJobAlreadyExists  // Code: JOB_DUPLICATE,      Status: 200 — InsertJob hit ON CONFLICT (job_post_id); not a client error
```

`ErrLedgerRowMissing` is the contract that closes the crawler/filter phantom-row race: the filter Naks on it (NATS redelivers), the analyzer logs it non-fatally. `ErrQueueRowMissing` from `UpdateHeartbeat` signals the crawler its queue row was reassigned (abort the scan). `ErrJobAlreadyExists` is Status 200 because a duplicate insert is the expected idempotent outcome, not a failure.

## Pattern

Services return typed errors by calling `.WithMessage()` on a sentinel:

```go
if domain.IsNotFound(err) {
    return nil, domain.ErrNotFound.WithMessage("job not found")
}
return nil, domain.ErrInternal.WithMessage("failed to fetch job")
```

Handlers pass the error directly to `handler.HandleError` — no type switching:

```go
result, err := h.svc.Get(ctx, id)
if err != nil {
    handler.HandleError(w, err)
    return
}
```

## Rules

- All service methods return `error` in their signature — callers use `handler.HandleError` which does the type assertion internally
- Never use `fmt.Errorf` for sentinel errors — always use the domain sentinels so HTTP mapping works
- `ValidationError` is returned by validators, not services — validators run before service calls
