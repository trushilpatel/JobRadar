# pipeline/ats/ — Careers page fetchers

## How the adapter fits into the pipeline

```
job_listing_crawler
  └─ Registry(company.adapter_type)        → Adapter.FetchJobs()
       └─ FetchJobsItem[]                  → RawJob (published to job_listings.raw)
            └─ item.FullContent != nil     → RawJob.FullContent set (full-detail adapters)

job_post_analyzer
  └─ job.FullContent != nil?
       └─ yes → fullContentToResponse()    → JSON → LLM → jobmodel.Job (DB)  [skip fetch]
       └─ no  → Registry(rawJob.adapter)   → ContentFetcher.FetchJobContent()   [if implemented]
                    └─ FetchJobContentResponse → JSON → LLM → jobmodel.Job (DB)
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

Two wire types (Kafka) — keep separate from DB model `jobmodel.Job`:
- **`RawJob`** — published to `job_listings.raw`; `FullContent` is `omitempty` so thin-adapter consumers are unaffected
- **`FetchJobsItem` / `FetchJobsResponse`** — minimal listing data for dedup + publish
- **`FullJobContent`** — full details at scan time (full-detail adapters); when set, analyzer skips HTTP fetch
- **`FetchJobContentResponse`** — union of all fields across platforms; empty string for unsupported fields

Struct definitions: [`ats/types.go`](types.go)

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
| `netflix/netflix.go` | ✅ | ✅ | Eightfold AI JSON API → `explore.jobs.netflix.net`, paginates listings, fetches detail |
| `uber/uber.go` | ✅ (full-detail) | — (FullContent at scan) | Internal Next.js API → `uber.com`, iterates by department, populates FullContent |
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

| Field          | Greenhouse | Lever | Ashby (GraphQL) | Google (Playwright) | Netflix (Eightfold) | Uber (Internal) |
|---|---|---|---|---|---|---|
| `ID`           | ✅ | ✅ | ✅ | ✅ (`google-{id}` from URL) | ✅ (`netflix-{id}`, int64) | ✅ (`uber-{id}`, int) |
| `Title`        | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `Location`     | ✅ | ✅ | ✅ (+ workplaceType in parens) | ✅ | ✅ (falls back to `locations[]`) | ✅ (`allLocations[]` joined with `"; "`) |
| `URL`          | ✅ | ✅ | — | ✅ | ✅ | ✅ |
| `Description`  | ✅ (HTML→plain) | ✅ (`descriptionPlain` + `additionalPlain`) | ✅ (HTML→plain) | ✅ (DOM innerText) | ✅ (`job_description` HTML→plain) | ✅ (`description` full Markdown) |
| `EmploymentType` | — | ✅ (normalized from `commitment`) | ✅ (normalized from `employmentType`) | ✅ (normalized from card text) | — (defaults `full_time`) | ✅ (`timeType` → constants) |
| `Department`   | ✅ (`departments[0]`) | ✅ (`department` > `team` fallback) | — | ✅ (from card metadata) | ✅ | ✅ |
| `Compensation` | ✅ (`pay_input_ranges[0]` formatted) | ✅ (`salaryDescriptionPlain` > `salaryRange` fallback) | ✅ (`compensationTierSummary`) | ✅ (if displayed) | — (not in public API) | — (not in API) |
| `WorkplaceType` | — | ✅ (raw: `on-site`/`remote`/`hybrid`) | ✅ (raw: `Remote`/`Hybrid`/`Onsite`) | ✅ (from metadata chips) | ✅ (`work_location_option`, can be null) | — (not in API) |
| `Requirements` | — | ✅ (`lists[]` HTML→plain) | — | ✅ (from qualification sections) | — | — |
| `PublishedDate` | ✅ (`updated_at` ISO 8601) | ✅ (`createdAt` Unix ms as string) | ✅ (`publishedDate` date string) | ✅ (if available) | ✅ (`t_create` Unix seconds → RFC 3339) | ✅ (`creationDate` ISO 8601 passthrough) |

---

## URL formats per adapter

| Adapter | `companies.url` value | `RawJob.URL` (web) | Content source |
|---|---|---|---|
| Greenhouse | `https://boards-api.greenhouse.io/v1/boards/{slug}/jobs` | `https://job-boards.greenhouse.io/{slug}/jobs/{id}` | `https://boards-api.greenhouse.io/v1/boards/{slug}/jobs/{id}` |
| Lever | `{company-slug}` (e.g. `netlify`) | `https://jobs.lever.co/{company}/{uuid}` | `https://api.lever.co/v0/postings/{company}/{uuid}?mode=json` |
| Ashby | `{company-slug}` (e.g. `openai`) | `https://jobs.ashbyhq.com/{company}/{uuid}` | GraphQL POST to `https://jobs.ashbyhq.com/api/non-user-graphql` |
| Google | (unused — scraper has built-in filtered URL) | `https://www.google.com/about/careers/applications/jobs/results/{id}-{slug}` | Playwright DOM extraction (full-detail, no separate fetch) |
| Netflix | (unused — adapter targets fixed host) | `https://explore.jobs.netflix.net/careers/job/{id}` | `https://explore.jobs.netflix.net/api/apply/v2/jobs/{id}?domain=netflix.com` (flat JSON) |
| Uber | (unused — adapter targets fixed host) | `https://www.uber.com/careers/list/{id}` | FullContent populated at scan time — no detail fetch |

---

## ID format

All adapter-generated IDs follow `{adapter}-{platform-id}`:

```
greenhouse-7762286
lever-d6389903-e642-4189-804c-02d271714bdb
ashby-abc-111-def
```

The prefix matches the `companycommon.Adapter*` constants in `service/company/common/`.

---

## Adding a new adapter

1. Create `{name}.go` — implement `Adapter` (`FetchJobs`)
2. Implement `ContentFetcher` (`FetchJobContent`) if the platform has a native job detail API — OR populate `FetchJobsItem.FullContent` if the adapter returns full details at listing time (full-detail path)
3. Create `{name}.md` documenting the full API response with a `Captured` column
4. Add a `case` in `registry.go`
5. Add the adapter constant to `service/company/common/CompanyServiceConstants.go`
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
