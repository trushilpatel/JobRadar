# pipeline/ats/ — Careers page fetchers

## Pipeline role

```
job_listing_crawler
  └─ Registry(company.adapter_type) → Adapter.FetchJobs()
       └─ FetchJobsItem[] → RawJob (published to job_listings.raw)
            └─ item.FullContent != nil → RawJob.FullContent set  (full-detail adapters)

job_post_analyzer
  └─ job.FullContent != nil?
       └─ yes → fullContentToResponse() → JSON → LLM → jobmodel.Job (DB)   [skip fetch]
       └─ no  → Registry(rawJob.adapter) → ContentFetcher.FetchJobContent()   [if implemented]
                    └─ FetchJobContentResponse → JSON → LLM → jobmodel.Job (DB)
             → fallback: HTTP GET rawJob.URL → StripHTML → FetchJobContentResponse
```

Two phases with distinct types:

| Phase  | Interface        | Input         | Output                                 |
| ------ | ---------------- | ------------- | -------------------------------------- |
| Scan   | `Adapter`        | `company.url` | `FetchJobsResponse` → `RawJob` (Kafka) |
| Enrich | `ContentFetcher` | `RawJob.URL`  | `FetchJobContentResponse` (→ LLM)      |

Full-detail adapters (e.g. custom company scrapers) collapse both phases — they populate `FetchJobsItem.FullContent` at scan time and the analyzer skips the fetch step entirely.

---

## Design rule: capture everything, transform cleanly

Each adapter's raw response struct mirrors the **complete** API response — every documented field is decoded, even ones not yet used downstream. A `toFetch*Response()` converter picks only what `FetchJobsResponse` / `FetchJobContentResponse` need.

```
API JSON → raw struct (captures everything) → toFetch*Response() → pipeline types
```

Adding a new downstream field never requires touching HTTP/decode logic — only the converter and destination struct.

---

## Wire types

Kept separate from the DB model `jobmodel.Job`:

- `RawJob` — published to `job_listings.raw`; `FullContent` is `omitempty` so thin-adapter consumers are unaffected
- `FetchJobsItem` / `FetchJobsResponse` — minimal listing data for dedup + publish
- `FullJobContent` — full details at scan time (full-detail adapters); when set, analyzer skips HTTP fetch
- `FetchJobContentResponse` — union of all fields across platforms; empty string for unsupported fields

Struct definitions: [`types.go`](types.go).

---

## Interfaces

```go
// Required for all fetchers. Called by job_listing_crawler.
type Adapter interface {
    FetchJobs(ctx context.Context, companyURL string) (FetchJobsResponse, error)
}

// Optional. Called by job_post_analyzer via type assertion.
// Implement when the platform has a native structured JSON API for job detail.
// Adapters that don't implement it fall back to plain HTTP GET on RawJob.URL.
type ContentFetcher interface {
    FetchJobContent(ctx context.Context, job RawJob) (FetchJobContentResponse, error)
}
```

The analyzer asserts `ContentFetcher` at runtime — no changes there when adding a new fetcher.

---

## Implementation status

| Adapter                           | FetchJobs        | FetchJobContent | Transport                                                        |
| --------------------------------- | ---------------- | --------------- | ---------------------------------------------------------------- |
| `greenhouse`                      | ✅               | ✅              | REST (`boards-api.greenhouse.io`)                                |
| `lever`                           | ✅               | ✅              | REST (`api.lever.co` with `?mode=json`)                          |
| `ashby`                           | ✅               | ✅              | GraphQL POST to `jobs.ashbyhq.com/api/non-user-graphql`          |
| `google`                          | ✅ (full-detail) | —               | HTTP + RE2 regex over server-rendered HTML (FullContent at scan) |
| `meta`                            | ✅               | ✅              | Sitemap listing + JSON-LD detail                                 |
| `netflix`                         | ✅               | ✅              | Eightfold JSON API (`explore.jobs.netflix.net`)                  |
| `uber`                            | ✅ (full-detail) | —               | Internal Next.js API (FullContent at scan)                       |
| `workday`, `workable`, `chromedp` | Stub             | —               | HTML scrape fallback                                             |

Per-adapter field coverage + URL formats: see the `.md` files linked at the bottom.

### Ashby gotcha

Two separate code paths exist:

| Method                                      | API                                    | Auth    | Used by             |
| ------------------------------------------- | -------------------------------------- | ------- | ------------------- |
| `FetchJobs` / `FetchJobContent`             | Public GraphQL (`jobs.ashbyhq.com`)    | None    | Pipeline (default)  |
| `FetchJobsViaAPI` / `FetchJobContentViaAPI` | Authenticated REST (`api.ashbyhq.com`) | API key | Manual / future use |

The REST API does **not** return `descriptionHtml` — use GraphQL when you need job content.

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

1. Create `{name}.go` implementing `Adapter` (`FetchJobs`)
2. Implement `ContentFetcher` (`FetchJobContent`) **OR** populate `FetchJobsItem.FullContent` at scan time (full-detail path)
3. Create `{name}.md` documenting the full API response with a `Captured` column
4. Add a `case` in `registry.go`
5. Add the adapter constant to `service/company/common/CompanyServiceConstants.go`
6. Add the CHECK constraint value to the `companies` migration
7. Add unit tests in `{name}_test.go` using `httptest.NewServer` — no real HTTP in unit tests

The raw response struct must capture **all** documented platform fields. The converter (`toFetch*Response()`) maps to the pipeline types and stays separate from HTTP logic.

**Full-detail adapters** (custom scrapers returning everything at scan):

- Populate `FetchJobsItem.FullContent` instead of implementing `ContentFetcher`
- Do NOT implement `ContentFetcher` — the analyzer checks `RawJob.FullContent` first and skips the fetch
- Set every field you have; leave unset fields as empty strings

---

## API documentation

Per-adapter API reference with full response schemas and `Captured` columns:

| File                                   | Covers                                                                              |
| -------------------------------------- | ----------------------------------------------------------------------------------- |
| [`greenhouse.md`](greenhouse.md)       | Greenhouse Boards API — list jobs, get single job, pay ranges                       |
| [`lever.md`](lever.md)                 | Lever Postings API — list jobs, get single job, description variants, salary fields |
| [`ashby.md`](ashby.md)                 | Ashby public GraphQL + authenticated REST API                                       |
| [`google/google.md`](google/google.md) | Google Careers — HTTP + RE2 regex over server-rendered HTML                         |
