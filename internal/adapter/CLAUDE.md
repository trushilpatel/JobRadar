# internal/adapter/ — Careers page fetchers

## RawJob

`RawJob` is the struct published to `jobs.raw`. It is defined here, not in `model/`.
The `model.Job` struct is the enriched DB row — don't mix them up.

```go
type RawJob struct {
    ID         string // e.g. "greenhouse-12345" — adapter prefix + external ID
    Title      string
    Location   string
    URL        string // the individual job posting URL (used by enricher)
    Department string
    EmployType string
}
```

## Adding a new adapter

1. Create `{name}.go` implementing the `Adapter` interface
2. Add a `case` in `registry.go`
3. Add the constant to `model/constants.go`
4. Add the CHECK constraint value to the `companies` migration

## Implementation status

| Adapter | Status |
|---|---|
| `greenhouse.go` | Implemented |
| `lever.go` | Stub — returns nil, nil |
| `ashby.go` | Stub — returns nil, nil |
| `workday.go` | Stub — returns nil, nil |
| `workable.go` | Stub — returns nil, nil |
| `chromedp.go` | Stub — returns nil, nil |

## Greenhouse URL format

The `companies.url` column must store the **API endpoint**, not the web UI URL.

```
correct:  https://boards-api.greenhouse.io/v1/boards/{slug}/jobs
wrong:    https://job-boards.greenhouse.io/{slug}
```
