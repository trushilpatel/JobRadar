# pipeline/ats/ — Careers page fetchers

## How the adapter fits into the pipeline

```
job_listing_crawler
  └─ Registry(company.adapter_type)        → Adapter.FetchJobs()
       └─ FetchJobsItem[]                  → RawJob (published to job_listings.raw)
            └─ item.FullContent != nil     → RawJob.FullContent set (full-detail adapters)

job_post_analyzer
  └─ job.FullContent != nil?
       └─ yes → fullContentToResponse()    → JSON → LLM → model.Job (DB)  [skip fetch]
       └─ no  → Registry(rawJob.adapter)   → ContentFetcher.FetchJobContent()   [if implemented]
                    └─ FetchJobContentResponse → JSON → LLM → model.Job (DB)
             → fallback: HTTP GET rawJob.URL   → StripHTML → FetchJobContentResponse
```

There are **two phases** with distinct types:

| Phase | Interface | Input | Output |
|---|---|---|---|
| Scan | `Adapter` | `company.url` | `FetchJobsResponse` → `RawJob` (Kafka) |
| Enrich | `ContentFetcher` | `RawJob.URL` | `FetchJobContentResponse` (→ LLM) |

Full-detail adapters (e.g. custom company scrapers) collapse both phases — they populate `FetchJobsItem.FullContent` at scan time, so the analyzer skips the fetch step entirely.

---

## Design rule: capture everything, transform cleanly

Each adapter's raw response struct mirrors the **complete API response** — every field from the platform's documentation is decoded, even fields not yet used downstream. The `toFetch*Response()` converter methods then pick only what `FetchJobsResponse` / `FetchJobContentResponse` need.

```
API response JSON
  → raw struct (captures everything)
  → toFetch*Response() (picks what the pipeline needs)
  → FetchJobsResponse / FetchJobContentResponse (pipeline types)
```

This means adding a new downstream field never requires touching HTTP/decode logic — only the converter and the destination struct.

---

## Types

### RawJob

Kafka message published to `job_listings.raw`. Defined here, not in `model/`.
`model.Job` is the enriched DB row — don't mix them up.

```go
type RawJob struct {
    ID          string          // e.g. "greenhouse-12345" — adapter prefix + external ID
    CompanyID   string          // UUID from companies table — set by scan engine before publishing
    Adapter     string          // "greenhouse", "lever", "ashby", etc.
    Title       string
    Location    string
    URL         string          // hosted web URL — ContentFetcher parses this; fallback scrape target
    EmployType  string          // full_time | part_time | contract | internship (model constants)
    FullContent *FullJobContent `json:"full_content,omitempty"` // nil for thin adapters
}
```

`FullContent` is `omitempty` — existing Kafka consumers that don't know about it are unaffected.

### FetchJobsItem / FetchJobsResponse

Minimal listing data — only what's needed for dedup and Kafka publish.

```go
type FetchJobsItem struct {
    ID             string         // adapter-prefixed, e.g. "lever-abc-123"
    Title          string
    URL            string         // hosted web URL → becomes RawJob.URL
    Location       string
    EmploymentType string         // normalized via model constants
    FullContent    *FullJobContent // nil for thin adapters; set by full-detail adapters
}

type FetchJobsResponse struct {
    Jobs []FetchJobsItem
}
```

### FullJobContent

Optional full job details returned by adapters that provide everything at listing time (e.g. custom company scrapers). Fields map 1:1 to `FetchJobContentResponse`. When present, `job_post_analyzer` skips the HTTP fetch and LLM structuring steps.

```go
type FullJobContent struct {
    Description   string
    Requirements  string
    Compensation  string
    WorkplaceType string
    Department    string
    PublishedDate string
}
```

### FetchJobContentResponse

Union of all fields available across all platforms. Fields not provided by a platform are left as empty strings. The `job_post_analyzer` marshals the whole struct to JSON and sends it to the LLM.

```go
type FetchJobContentResponse struct {
    ID             string // all platforms
    Title          string // all platforms
    Location       string // all platforms (Ashby appends workplaceType in parens)
    URL            string // Greenhouse, Lever
    Description    string // plain text — HTML stripped; primary content for LLM
    EmploymentType string // Lever (commitment), Ashby (employmentType)
    Department     string // Greenhouse (departments[0]), Lever (department > team fallback)
    Compensation   string // Greenhouse (pay_input_ranges formatted), Lever (salaryDescriptionPlain > salaryRange fallback), Ashby (compensationTierSummary)
    WorkplaceType  string // Lever (workplaceType), Ashby (workplaceType) — raw platform value
    Requirements   string // plain text — Lever (lists[])
    PublishedDate  string // Lever (createdAt ms as string), Ashby (publishedDate), Greenhouse (updated_at)
}
```

---

## Interfaces

```go
// Adapter — required for all fetchers. Called by job_listing_crawler.
type Adapter interface {
    FetchJobs(ctx context.Context, companyURL string) (FetchJobsResponse, error)
}

// ContentFetcher — optional. Called by job_post_analyzer.
// Implement when the platform has a native structured JSON API for job detail.
// Adapters that don't implement it fall back to plain HTTP GET on RawJob.URL.
type ContentFetcher interface {
    FetchJobContent(ctx context.Context, job RawJob) (FetchJobContentResponse, error)
}
```

The `job_post_analyzer` checks for `ContentFetcher` via type assertion — no changes needed there when adding a new fetcher.

```go
// from job_post_analyzer/worker.go — fetchContent()
if cf, ok := adp.(ats.ContentFetcher); ok {
    return cf.FetchJobContent(ctx, job)
}
// else: falls back to HTTP GET + StripHTML
```

---

## Implementation status

| Adapter | FetchJobs | FetchJobContent | Notes |
|---|---|---|---|
| `greenhouse.go` | ✅ | ✅ | Parses `{slug}/{id}` from `RawJob.URL`, calls `boards-api.greenhouse.io` REST |
| `lever.go` | ✅ | ✅ | Parses `{company}/{uuid}` from `RawJob.URL`, calls `api.lever.co` REST (`?mode=json`) |
| `ashby.go` | ✅ | ✅ | Parses `{company}/{uuid}` from `RawJob.URL`, GraphQL POST to `jobs.ashbyhq.com` |
| `google/google.go` | ✅ (full-detail) | — (FullContent at scan) | Playwright subprocess → `careers.google.com`, populates FullContent |
| `workday.go` | Stub | — | Falls back to HTML scrape |
| `workable.go` | Stub | — | Falls back to HTML scrape |
| `chromedp.go` | Stub | — | Falls back to HTML scrape (headless browser) |

### Ashby note

`ashby.go` has two separate code paths:

| Method | API | Auth | Used by |
|---|---|---|---|
| `FetchJobs` / `FetchJobContent` | Public GraphQL (`jobs.ashbyhq.com`) | None | Pipeline (default) |
| `FetchJobsViaAPI` / `FetchJobContentViaAPI` | Authenticated REST (`api.ashbyhq.com`) | API key | Manual / future use |

The REST API does **not** return `descriptionHtml` — use GraphQL when you need job content.

---

## Field coverage per adapter

### FetchJobContent fields

| Field          | Greenhouse | Lever | Ashby (GraphQL) | Google (Playwright) |
|---|---|---|---|---|
| `ID`           | ✅ | ✅ | ✅ | ✅ (`google-{id}` from URL) |
| `Title`        | ✅ | ✅ | ✅ | ✅ |
| `Location`     | ✅ | ✅ | ✅ (+ workplaceType in parens) | ✅ |
| `URL`          | ✅ | ✅ | — | ✅ |
| `Description`  | ✅ (HTML→plain) | ✅ (`descriptionPlain` + `additionalPlain`) | ✅ (HTML→plain) | ✅ (DOM innerText) |
| `EmploymentType` | — | ✅ (normalized from `commitment`) | ✅ (normalized from `employmentType`) | ✅ (normalized from card text) |
| `Department`   | ✅ (`departments[0]`) | ✅ (`department` > `team` fallback) | — | ✅ (from card metadata) |
| `Compensation` | ✅ (`pay_input_ranges[0]` formatted) | ✅ (`salaryDescriptionPlain` > `salaryRange` fallback) | ✅ (`compensationTierSummary`) | ✅ (if displayed) |
| `WorkplaceType` | — | ✅ (raw: `on-site`/`remote`/`hybrid`) | ✅ (raw: `Remote`/`Hybrid`/`Onsite`) | ✅ (from metadata chips) |
| `Requirements` | — | ✅ (`lists[]` HTML→plain) | — | ✅ (from qualification sections) |
| `PublishedDate` | ✅ (`updated_at` ISO 8601) | ✅ (`createdAt` Unix ms as string) | ✅ (`publishedDate` date string) | ✅ (if available) |

---

## URL formats per adapter

| Adapter | `companies.url` value | `RawJob.URL` (web) | Content source |
|---|---|---|---|
| Greenhouse | `https://boards-api.greenhouse.io/v1/boards/{slug}/jobs` | `https://job-boards.greenhouse.io/{slug}/jobs/{id}` | `https://boards-api.greenhouse.io/v1/boards/{slug}/jobs/{id}` |
| Lever | `{company-slug}` (e.g. `netlify`) | `https://jobs.lever.co/{company}/{uuid}` | `https://api.lever.co/v0/postings/{company}/{uuid}?mode=json` |
| Ashby | `{company-slug}` (e.g. `openai`) | `https://jobs.ashbyhq.com/{company}/{uuid}` | GraphQL POST to `https://jobs.ashbyhq.com/api/non-user-graphql` |
| Google | (unused — scraper has built-in filtered URL) | `https://www.google.com/about/careers/applications/jobs/results/{id}-{slug}` | Playwright DOM extraction (full-detail, no separate fetch) |

---

## ID format

All adapter-generated IDs follow `{adapter}-{platform-id}`:

```
greenhouse-7762286
lever-d6389903-e642-4189-804c-02d271714bdb
ashby-abc-111-def
```

The prefix matches the `model.Adapter*` constants in `model/constants.go`.

---

## Adding a new adapter

1. Create `{name}.go` — implement `Adapter` (`FetchJobs`)
2. Implement `ContentFetcher` (`FetchJobContent`) if the platform has a native job detail API — OR populate `FetchJobsItem.FullContent` if the adapter returns full details at listing time (full-detail path)
3. Create `{name}.md` documenting the full API response with a `Captured` column
4. Add a `case` in `registry.go`
5. Add the adapter constant to `model/constants.go`
6. Add the CHECK constraint value to the `companies` migration
7. Add unit tests in `{name}_test.go` using `httptest.NewServer` — no real HTTP in unit tests

The raw response struct must capture **all** documented fields from the platform API.
The converter (`toFetch*Response()`) maps to the pipeline types — keep it separate from the HTTP logic.

**Full-detail adapters** (custom company scrapers that return everything at once):
- Populate `FetchJobsItem.FullContent` instead of implementing `ContentFetcher`
- Do NOT implement `ContentFetcher` — the analyzer checks `RawJob.FullContent` first and skips the fetch
- Set all fields you have; leave unset fields as empty strings

---

## API documentation

Per-adapter API reference with full response schemas and `Captured` column:

| File | Covers |
|---|---|
| [`greenhouse.md`](greenhouse.md) | Greenhouse Boards API — list jobs, get single job, pay ranges |
| [`lever.md`](lever.md) | Lever Postings API — list jobs, get single job, all description variants, salary fields |
| [`ashby.md`](ashby.md) | Ashby public GraphQL + authenticated REST API |
| [`google/google.md`](google/google.md) | Google Careers — Playwright scraper, DOM extraction, anti-bot config |
