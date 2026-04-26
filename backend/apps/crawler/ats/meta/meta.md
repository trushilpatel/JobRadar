# Meta Careers Adapter

Hybrid adapter: rod for the **listing** page (Meta's `/jobsearch` is fully React-rendered with rotating GraphQL CSRF tokens, so a browser is unavoidable), HTTP + JSON-LD for **detail** pages (every job page embeds a complete schema.org `JobPosting` payload, so detail enrichment runs without a browser).

## Architecture

```
Adapter.FetchJobs()                  ← single rod tab, listing only
  → Opens Chromium via go-rod (shared browser or own instance)
  → Navigates to metacareers.com/jobsearch
  → Scrolls infinite scroll to collect all thin job cards
  → Returns FetchJobsResponse with FullContent == nil (thin items)

ContentFetcher.FetchJobContent(job)  ← analyzer calls this per thin item
  → HTTP GET https://www.metacareers.com/profile/job_details/{id}
  → Regex-finds <script type="application/ld+json"> block
  → json.Unmarshal into JobPosting struct
  → Returns FetchJobContentResponse with composed multi-section description
```

The analyzer is responsible for calling `FetchJobContent` for any item whose `FullContent` is nil — same pattern as Greenhouse, Lever, and Ashby.

## Key files

| File | Role |
|------|------|
| `rod.go` | `RodMeta` — listing-side browser automation; one tab, scroll-until-idle; thin-item publishing |
| `http_detail.go` | `JobPosting` struct + `extractJobPosting` + `Compose` + `FetchJobContent` (HTTP + JSON-LD parser) |
| `crawler.go` | `CrawlerService` — wraps `BaseWorker`, runs the poll loop |
| `http_detail_test.go` | Unit tests for JSON-LD parsing edge cases (`flexString`, `jobLocations`, `extractJobPosting`, `Compose`, `FetchJobContent` against `httptest.Server`) |
| `rod_test.go` | Tests for `RodMeta` constructor, interface compliance, listing-side helpers |

## Interfaces implemented

| Interface | Purpose |
|-----------|---------|
| `types.Adapter` | Required — `FetchJobs` (listing only) |
| `types.URLProvider` | Returns `metacareers.com/jobsearch` |
| `types.StreamPublisher` | Publishes thin items to Kafka after the listing scroll completes |
| `types.ContentFetcher` | `FetchJobContent` — pulls JSON-LD detail pages over HTTP |

## JSON-LD schema reference

Every Meta detail page embeds a single `<script type="application/ld+json">` block with a `JobPosting` (schema.org). Verified across an 11-job sample (engineering / PM / design / production / energy infra). Coverage:

| Field | Coverage in sample | Where it goes |
|-------|--------------------|---------------|
| `title` | 11/11 | `FetchJobContentResponse.Title` |
| `description` | 11/11 | composed into `Description` (intro paragraph) |
| `responsibilities` | 11/11 | composed into `Description` (Responsibilities section) |
| `qualifications` | 11/11 | composed into `Description` (Qualifications section) |
| `employmentType` | 11/11 | always `"Full-time"` → mapped to `jobcommon.EmployFullTime` |
| `datePosted` | 11/11 | `FetchJobContentResponse.PublishedDate` |
| `validThrough` | 11/11 | included in composed metadata block |
| `directApply` | 11/11 | currently composed-only (could feed future apply-method routing) |
| `hiringOrganization.name` | 11/11 | `FetchJobContentResponse.Department` ("Meta") |
| `jobLocation` (array) | 11/11 | flattened + deduped → `FetchJobContentResponse.Location` (first entry) |
| `industry` / `occupationalCategory` / `baseSalary` / `skills` / `jobBenefits` | 0/11 | optional — struct fields exist but rarely populated |

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
| `MinDelayMs` | 3000 | (legacy — was inter-detail pacing in the old per-job rod loop; unused now that detail is HTTP) |
| `MaxDelayMs` | 8000 | (legacy — same as above) |
| `TimeoutMs` | 3600000 (1 h) | Overall scan timeout for the listing tab |
| `MaxJobs` | 0 (all) | Cap on jobs collected from the listing scroll; 0 = no limit |

## Browser sharing

`WithBrowser(b *rod.Browser)` lets RodMeta share a Chromium process if other rod-based adapters exist. Currently Meta is the only rod consumer (Google was migrated to pure HTTP — see `ats/google/http.go`).

## Listing-page constraint

Meta's `/jobsearch` is a React SPA that fetches data via `POST /graphql` with persisted-query `doc_id`s and rotating CSRF tokens (`lsd`, `__hsi`, `__rev`). Reverse-engineering that endpoint would require:

1. Reading bundle JS dynamically to extract the current `doc_id` per query name (fragile — breaks on every Meta deploy).
2. Or maintaining a hand-curated `doc_id` allowlist (operational toil).

We chose to keep rod for listing instead. If a stable HTTP listing path emerges later, swap it in — the rest of this adapter (detail HTTP + JSON-LD) is unaffected.

## Header set required by Meta

Plain `urllib`-style requests (just `User-Agent` + `Accept` + `Accept-Language`) get a **400** from Meta's edge. The full Chrome header set in `fetchAndParseDetail` includes the `sec-ch-ua-*` and `Sec-Fetch-*` headers Chrome sends; with those, requests succeed. If Meta tightens further we may need to mimic TLS fingerprint (e.g. `curl-cffi` / `utls`).

## Migration history

- v1: Node.js + Playwright subprocess (`scrape.js`).
- v2: Go + go-rod for both listing and detail (single binary, removed Node from the crawler image).
- v3 (current): Hybrid — rod for listing only, plain HTTP + JSON-LD for detail. Removes the second rod tab and ~all per-job browser navigations; ~10× faster detail enrichment.
