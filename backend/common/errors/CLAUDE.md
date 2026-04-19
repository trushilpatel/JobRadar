# domain/ — Shared error types

## Types

**`ServiceError`** — typed error returned by all service methods. Carries an HTTP status code and a short machine-readable code. Handlers call `handler.HandleError(w, err)` which inspects this type to pick the HTTP status.

**`ValidationError`** — returned by validators for malformed or out-of-range input. Always maps to 400.

## Sentinel errors

```go
domain.ErrNotFound   // Code: NOT_FOUND,   Status: 404
domain.ErrConflict   // Code: CONFLICT,    Status: 409
domain.ErrBadRequest // Code: BAD_REQUEST, Status: 400
domain.ErrInternal   // Code: INTERNAL,    Status: 500
```

## Usage pattern

Services return typed errors by calling `.WithMessage()` on a sentinel:

```go
// service layer
if domain.IsNotFound(err) {
    return nil, domain.ErrNotFound.WithMessage("job not found")
}
return nil, domain.ErrInternal.WithMessage("failed to fetch job")
```

Handlers pass the error directly to `handler.HandleError` — no type switching needed in handlers:

```go
// handler layer
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
