# Meta Careers Adapter

Pure-HTTP adapter — no go-rod, no Chromium, no JavaScript execution.

- **Listing**: HTTP GET `https://www.metacareers.com/jobsearch/sitemap.xml` → ~895 numeric job IDs.
- **Detail**: HTTP GET `https://www.metacareers.com/profile/job_details/{id}` → schema.org `JobPosting` JSON-LD embedded inline.

## Architecture

```
MetaAdapter.FetchJobs()
  → HTTP GET /jobsearch/sitemap.xml
  → parseSitemap → [job_id_1, job_id_2, …]
  → streamPublish thin items (FullContent == nil) for each ID
  → Returns FetchJobsResponse

MetaAdapter.FetchJobContent(job)            ← analyzer calls this per thin item
  → HTTP GET /profile/job_details/{id}
  → Regex-finds <script type="application/ld+json"> block
  → json.Unmarshal into JobPosting struct
  → Returns FetchJobContentResponse with composed multi-section description
```

The analyzer is responsible for calling `FetchJobContent` for any item whose `FullContent` is nil — same pattern as Greenhouse, Lever, and Ashby.

## Why a sitemap

Meta's `/jobsearch` page is a React SPA that fetches data via `POST /graphql` with persisted-query `doc_id`s and rotating CSRF tokens (`lsd`, `__hsi`, `__rev`). Earlier adapter versions used a headless Chromium to wait for the SPA to render, then scraped the DOM. Then we discovered that Meta exposes a public sitemap covering every active job — advertised in `https://www.metacareers.com/robots.txt` under the `Sitemap:` directive — and dropped the browser entirely.

The sitemap is the official crawler contract: stable URL, public XML, no auth, no JavaScript. Verified ~895 unique `<loc>` entries, all of the form `/profile/job_details/{numeric_id}`.

## Key files

| File | Role |
|------|------|
| `meta.go` | `MetaAdapter` struct + `Config` + `New`/`NewDefault` + `FetchJobs` (sitemap-driven listing) + `FetchJobContent` (HTTP + JSON-LD). |
| `sitemap.go` | `fetchSitemap` (HTTP) + `parseSitemap` (regex over `<loc>` tags). |
| `http_detail.go` | `JobPosting` struct + custom unmarshallers (`flexString`, `jobLocations`) + `extractJobPosting` + `Compose` + `fetchAndParseDetail`. |
| `crawler.go` | `CrawlerService` — wraps `BaseWorker`, runs the poll loop. |
| `meta_test.go` | `MetaAdapter` constructor + interface compliance + `FetchJobs` end-to-end against a `httptest.Server`. |
| `sitemap_test.go` | `parseSitemap` unit tests (dedupe, mixed case, non-job entries filtered, empty body). |
| `http_detail_test.go` | JSON-LD parsing edge cases (`flexString`, `jobLocations`, `extractJobPosting`, `Compose`, `FetchJobContent`). |

## Interfaces implemented

| Interface | Purpose |
|-----------|---------|
| `types.Adapter` | Required — `FetchJobs` (sitemap-driven). |
| `types.URLProvider` | Returns the human-facing careers page URL. |
| `types.StreamPublisher` | Publishes thin items in one batch after the sitemap fetch completes. |
| `types.ContentFetcher` | `FetchJobContent` — pulls JSON-LD detail pages over HTTP. |

## JSON-LD schema reference

Every Meta detail page embeds a single `<script type="application/ld+json">` block with a `JobPosting` (schema.org). Verified across a 21-job sample (engineering / PM / design / production / energy infra) drawn evenly from the full sitemap. Coverage:

| Field | Coverage in sample | Where it goes |
|-------|--------------------|---------------|
| `title` | 21/21 | `FetchJobContentResponse.Title` |
| `description` | 21/21 | composed into `Description` (intro paragraph) |
| `responsibilities` | 21/21 | composed into `Description` (Responsibilities section) |
| `qualifications` | 21/21 | composed into `Description` (Qualifications section) |
| `employmentType` | 21/21 | always `"Full-time"` → mapped to `jobcommon.EmployFullTime` |
| `datePosted` | 21/21 | `FetchJobContentResponse.PublishedDate` |
| `validThrough` | 21/21 | included in composed metadata block |
| `directApply` | 21/21 | currently composed-only (could feed future apply-method routing) |
| `hiringOrganization.name` | 21/21 | `FetchJobContentResponse.Department` ("Meta") |
| `jobLocation` (array) | 21/21 | flattened + deduped → `FetchJobContentResponse.Location` (first entry) |
| `industry` / `occupationalCategory` / `baseSalary` / `skills` / `jobBenefits` | 0/21 | optional — struct fields exist but rarely populated |

### `addressCountry` shape variations

Meta emits `addressCountry` in any of four shapes — handled by the custom `flexString` unmarshaller in `http_detail.go`:

- plain string: `"USA"`
- nested object: `{"@type":"Country","name":"USA"}`
- array of strings: `["USA","Canada"]`
- nested object whose `name` is a list: `{"@type":"Country","name":["USA","USA",…]}` ← the real production case

`flexString` recurses through every container and dedupes terminal values.

### `jobLocation` shape

JSON-LD allows `jobLocation` to be either a single object or an array. The custom `jobLocations` unmarshaller normalises both into a `[]jobLocation`.

## Config (`Config` struct)

| Field | Default | Description |
|-------|---------|-------------|
| `TimeoutMs` | `600000` (10 min) | Overall scan timeout. The sitemap fetch is one HTTP call; the budget here covers the publish loop and any retries. |

(All previous rod-related knobs — `MinDelayMs`, `MaxDelayMs`, `MaxJobs` — are gone in v4.)

## Header set required by Meta

Plain `urllib`-style requests (just `User-Agent` + `Accept` + `Accept-Language`) get a **400** from Meta's edge on detail pages. The full Chrome header set in `fetchAndParseDetail` includes the `sec-ch-ua-*` and `Sec-Fetch-*` headers Chrome sends; with those, requests succeed. The sitemap endpoint is more permissive but we send the same headers for consistency.

## Test seam

`sitemap.go` declares `sitemapURL` as a package `var` (not `const`) so tests can point `fetchSitemap` at a `httptest.Server` via `overrideSitemapURL(...)`. Production code never touches that helper.

## Migration history

- v1: Node.js + Playwright subprocess (`scrape.js`).
- v2: Go + go-rod for both listing and detail (single binary, removed Node from the crawler image).
- v3: Hybrid — rod for listing only, plain HTTP + JSON-LD for detail.
- v4 (current): **Pure HTTP — sitemap.xml for listing, JSON-LD for detail.** Drops rod, drops Chromium from the Docker image entirely. Listing time goes from minutes (rod scroll) to ~200 ms (one sitemap GET).
