# internal/ats_fetcher/ — Careers page fetchers

## RawJob

`RawJob` is the struct published to `job_listings.raw`. It is defined here, not in `model/`.
The `model.Job` struct is the enriched DB row — don't mix them up.

```go
type RawJob struct {
    ID         string // e.g. "greenhouse-12345" — adapter prefix + external ID
    CompanyID  string // UUID from companies table — set by scan engine before publishing
    Adapter    string // e.g. "greenhouse", "lever", "ashby"
    Title      string
    Location   string
    URL        string // hosted web URL — used by job_post_analyzer as fallback fetch target
    Department string
    EmployType string
}
```

## Two interfaces

```go
// Adapter — required. Fetches the job listing (scan phase).
type Adapter interface {
    FetchJobs(ctx context.Context, companyURL string) ([]RawJob, error)
}

// ContentFetcher — optional. Fetches full content for a single job (enrich phase).
// Implement this when the adapter has a native API cleaner than scraping job.URL.
// Adapters that don't implement it fall back to a plain HTTP GET on job.URL.
type ContentFetcher interface {
    FetchJobContent(ctx context.Context, job RawJob) (string, error)
}
```

The job_post_analyzer checks for `ContentFetcher` via type assertion. No job_post_analyzer changes needed when adding a new fetcher — just implement the interface if applicable.

## Implementation status

| Adapter | FetchJobs | FetchJobContent | Source |
|---|---|---|---|
| `greenhouse.go` | Implemented | Implemented | Parses slug+ID from `job.URL`, calls `boards-api.greenhouse.io` |
| `lever.go` | Implemented | Implemented | Parses company+ID from `job.URL`, calls `api.lever.co` |
| `ashby.go` | Implemented | Implemented | Parses company+ID from `job.URL`, GraphQL POST to `jobs.ashbyhq.com` |
| `workday.go` | Stub | — | Falls back to HTML scrape |
| `workable.go` | Stub | — | Falls back to HTML scrape |
| `chromedp.go` | Stub | — | Falls back to HTML scrape |

## Adding a new adapter

1. Create `{name}.go` implementing `Adapter`
2. Optionally implement `ContentFetcher` if the platform has a native job detail API
3. Add a `case` in `registry.go`
4. Add the constant to `model/constants.go`
5. Add the CHECK constraint value to the `companies` migration

## URL formats per adapter

| Adapter | `companies.url` value | `RawJob.URL` (web) | Content API |
|---|---|---|---|
| Greenhouse | `https://boards-api.greenhouse.io/v1/boards/{slug}/jobs` | `https://job-boards.greenhouse.io/{slug}/jobs/{id}` | `https://boards-api.greenhouse.io/v1/boards/{slug}/jobs/{id}` |
| Lever | `{company-slug}` | `https://jobs.lever.co/{company}/{uuid}` | `https://api.lever.co/v0/postings/{company}/{uuid}?mode=json` |
| Ashby | `{company-slug}` | `https://jobs.ashbyhq.com/{company}/{uuid}` | GraphQL POST to `https://jobs.ashbyhq.com/api/non-user-graphql` |
