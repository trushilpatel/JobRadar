# TODO

## Migrate API handlers to huma

**What:** Replace Chi's raw `http.ResponseWriter` handler pattern with [huma](https://huma.rocks) — a type-safe API framework that wraps Chi.

**Why it's worth doing:**

| Problem today | How huma solves it |
|---|---|
| Query params parsed manually in every handler | Defined once as struct fields with `query:` tags — parsed and validated automatically |
| Request body decoded manually (`json.Decode`) | Declared as a struct field — decoded and validated automatically |
| Custom `Respond`, `RespondError`, `HandleError` helpers needed everywhere | Return a value or an error — huma handles serialization |
| No API docs without extra tooling | OpenAPI 3.1 spec generated from Go types at startup — always in sync |
| Validation scattered across handlers and validators | Struct tags on input types — one place |

**Before (current Chi handler):**
```go
func (h *RestService) List(w http.ResponseWriter, r *http.Request) {
    q := r.URL.Query()
    active := q.Get("active")           // manual param parsing
    page, _ := strconv.Atoi(q.Get("page")) // manual, no validation
    result, err := h.next.List(...)
    if err != nil {
        handler.HandleError(w, err)     // custom error routing
        return
    }
    handler.Respond(w, http.StatusOK, result) // custom helper
}
```

**After (huma handler — same business logic, less plumbing):**
```go
type ListInput struct {
    Active *bool `query:"active"`
    Page   int   `query:"page" minimum:"1" default:"1"`
    Limit  int   `query:"limit" minimum:"1" maximum:"100" default:"20"`
}

huma.Register(api, huma.Operation{
    Method:  http.MethodGet,
    Path:    "/companies",
    Summary: "List companies",
}, func(ctx context.Context, input *ListInput) (*ListOutput, error) {
    return h.next.List(ctx, input.Active, input.Page, input.Limit)
})
```

**What stays the same:** Chi router, all middleware (auth, OTEL, metrics, body limit, security headers), Supabase, everything else.

**Migration plan:**
1. Add huma + Chi adapter, wire up Scalar UI at `/docs`
2. Migrate one domain at a time — run existing tests after each domain
3. Update frontend error handling alongside (huma's error envelope differs from current `ErrorResponse`)

**Estimated effort:** ~1 day
**Suggested order:** companies → jobs → scan → applications → dashboard → insights → adapter-configs
