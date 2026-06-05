# Meta Careers Adapter

Pure-HTTP adapter — no go-rod, no Chromium, no JavaScript execution.

- **Listing**: GET `https://www.metacareers.com/jobsearch/sitemap.xml` → numeric job IDs.
- **Detail**: GET `https://www.metacareers.com/profile/job_details/{id}` → schema.org `JobPosting` JSON-LD embedded inline.

## Architecture

```
Meta.FetchJobListing()
  → GET /jobsearch/sitemap.xml
  → parseSitemap → [id_1, id_2, …]
  → mapJobSummary for each ID
  → Returns JobListing (thin — Title and Location empty)

Meta.FetchJobPost(summary)
  → GET summary.URL (or construct from summary.ID)
  → extractJobPosting → api.JobPosting
  → mapJobPost → core.JobPost (full fields populated)
```

## Key files

| File                       | Role                                                                                                  |
| -------------------------- | ----------------------------------------------------------------------------------------------------- |
| `meta.go`                  | `Meta` struct + `New` + `FetchJobListing` + `FetchJobPost` + HTTP helpers                             |
| `sitemap.go`               | `parseSitemap` — regex over `<loc>` tags, deduplication, order preservation                           |
| `detail.go`                | `extractJobPosting` (3 JSON-LD shapes) + location/country helpers + `metaCountryToCode`               |
| `mapper.go`                | `mapJobSummary`, `mapJobPost`, `buildDescription`, `buildCompensation`                                |
| `api/types.go`             | Raw JSON-LD types: `JobPosting`, `JobLocations`, `FlexString`, `Address`                              |
| `meta_test.go`             | Adapter constructor, interface compliance, `FetchJobListing`/`FetchJobPost` against `httptest.Server` |
| `sitemap_test.go`          | `parseSitemap` unit tests — dedup, order, non-job URL filtering, empty input                          |
| `detail_test.go`           | JSON-LD extraction (3 shapes), `FlexString`/`JobLocations` unmarshal, country/location helpers        |
| `mapper_test.go`           | All mapping, compensation formatting, description assembly, HTML stripping                            |
| `meta_integration_test.go` | Real network tests — skipped under `-short`                                                           |

## Interfaces implemented

| Interface             | Method                                                |
| --------------------- | ----------------------------------------------------- |
| `core.ListingFetcher` | `FetchJobListing` — sitemap-driven, companyID ignored |
| `core.PostFetcher`    | `FetchJobPost` — JSON-LD detail pages                 |

## JSON-LD schema reference

Every Meta detail page embeds a `<script type="application/ld+json">` block with schema.org `JobPosting`. Three shapes are handled:

- Plain object: `{ "@type": "JobPosting", … }`
- Array: `[ { "@type": "JobPosting", … }, … ]`
- `@graph` wrapper: `{ "@graph": [ { "@type": "JobPosting", … } ] }`

| Field                                                 | Where it goes                                                                        |
| ----------------------------------------------------- | ------------------------------------------------------------------------------------ |
| `title`                                               | `core.JobPost.Title`                                                                 |
| `description` + `responsibilities` + `qualifications` | Composed into `core.JobPost.Description` with section headers                        |
| `datePosted`                                          | `core.JobPost.PublishedDate`                                                         |
| `hiringOrganization.name`                             | `core.JobPost.Department`                                                            |
| `jobLocation[].address`                               | First entry → `core.JobPost.Location`; `addressCountry` → `core.JobPost.CountryCode` |
| `baseSalary`                                          | `core.JobPost.Compensation` (optional)                                               |
| `employmentType`                                      | Always mapped to `core.EmployFullTime`                                               |

## `addressCountry` shape variations

Handled by the `FlexString` custom unmarshaller in `api/types.go`:

- Plain string: `"USA"`
- Nested object: `{"@type":"Country","name":"USA"}`
- Array: `["USA","Canada"]`
- Object whose `name` is a list: `{"@type":"Country","name":["USA","USA",…]}`

## `jobLocation` shape

May be a single object or an array. `JobLocations` custom unmarshaller normalises both into `[]JobLocation`.

## Listing-time thin items

`FetchJobListing` returns summaries with `Title = ""` and `Location = ""` — these fields are not available in the sitemap. Callers must use `FetchJobPost` to populate them.

## Header set required by Meta

Plain requests (just `User-Agent` + `Accept` + `Accept-Language`) may get a `400` from Meta's CDN. The full Chrome 121 header set including `sec-ch-ua-*` and `Sec-Fetch-*` headers is required. Both `FetchJobListing` and `FetchJobPost` use the same headers.

## Test seam

`meta.baseURL` is unexported. Tests use `newTestAdapter(srv.URL)` which sets `baseURL` to the httptest server base; the adapter's `resolveSitemapURL` prefixes `/jobsearch/sitemap.xml` to it. `FetchJobPost` uses `summary.URL` directly, so tests simply set a test-server URL there.
