# Apple Adapter

HTTP adapter for [jobs.apple.com](https://jobs.apple.com). Scrapes the listing page for the index and the per-job detail page for rich content (description, qualifications, salary).

## Architecture

**Pattern**: Thin adapter — `Adapter` for listing, `ContentFetcher` for detail.

The listing endpoint exposes only stub data per job (~300-char `jobSummary` and basic metadata). Description, responsibilities, minimum/preferred qualifications, salary, and homeOffice all live on the detail page, so the analyzer triggers `FetchJobContent` for every job that survives dedup. Because the pipeline ledger drops already-seen IDs upstream of the analyzer, only the newly listed jobs (~25–100 per scan in practice) trigger a detail fetch.

## API details

### Listing endpoint

```
GET https://jobs.apple.com/en-us/search?location=united-states-USA&page={n}
```

- **Filter**: US jobs only via the `location=united-states-USA` URL param.
- **Pagination**: increment `page` starting at 1; stop when `searchResults` is empty (~20 jobs/page, varies).
- **Hydration**: each page embeds `window.__staticRouterHydrationData = JSON.parse("…");` (also tolerates direct-object and single-quote variants).
- **Job shape** (`loaderData.search.searchResults[]`): `id`, `positionId`, `postingTitle`, `transformedPostingTitle`, `jobSummary` (intro only, do not rely on for content), `locations[]`, `postDateInGMT`, `team.teamName`, `homeOffice`, `type`.

### Detail endpoint

```
GET https://jobs.apple.com/en-us/details/{id}/{transformedPostingTitle}
```

- Returns the same React Router hydration script. The job payload is at `loaderData.jobDetails.jobsData`.
- Apple 301-redirects bare `/en-us/details/{id}` to the slug form — `http.DefaultClient` follows by default.
- **Captured fields**: `jobSummary`, `description`, `responsibilities`, `minimumQualifications`, `preferredQualifications`, `teamNames[]`, `locations[]` (full city/stateProvince/countryID), `employmentType`, `jobType`, `type`, `homeOffice`, `postDateInGMT`, `postingFooters[].content`.

### Locations

Listing locations populate `name`, `countryID`, `countryName`, and `level`; `city`/`stateProvince` are often empty on listing payloads. Detail-page locations populate the full structure (`city`, `stateProvince`, `code`, `zipCode`, `countryID`, etc.). The adapter tolerates both the new (`stateProvince` + `countryID`) and older (`stateCode` + `countryCode`) shapes.

`formatSingleLocation` falls back: city → name, stateProvince → stateCode.

`isUSALocation` matches `countryID == iso-country-USA`, `countryCode in {USA,US}`, or `countryName in {United States, United States of America}`.

## Mapping

### ID format

`apple-{id}` where `id` is the job's listing `id` field (e.g. `apple-200647207-3543` or `apple-PIPE-114438158`).

### Job URL

`https://jobs.apple.com/en-us/details/{id}/{transformedPostingTitle}`.

The URL is built from the adapter's `resolveBaseURL()` so tests can redirect both listing and detail fetches to one mock server.

### FullContent

Nil at listing time (thin adapter). All rich fields are returned by `FetchJobContent`.

### `FetchJobContentResponse` fields

| Field            | Source                                                                    | Notes                                                                                                                           |
| ---------------- | ------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------- |
| `Description`    | `jobSummary` + `description` + Responsibilities + Min/Pref Qualifications | Labeled sections; empty sections skipped; HTML stripped via `types.StripHTML()`                                                 |
| `Compensation`   | `postingFooters[].content`                                                | First `$X,XXX and $Y,YYY` / `$X to $Y` range; empty if absent                                                                   |
| `Department`     | `teamNames[0]`                                                            |                                                                                                                                 |
| `WorkplaceType`  | `homeOffice`                                                              | `"remote"` when true, `""` otherwise                                                                                            |
| `Location`       | `locations[]`                                                             | `"City, State"` joined with `"; "`; falls back to listing-supplied value when detail has no locations                           |
| `CountryCode`    | `locations[0].countryID`                                                  | `"US"` for USA marker; non-empty other code (3-letter ISO from `iso-country-XXX`) otherwise — feeds the analyzer's non-USA gate |
| `EmploymentType` | `type` / `jobType` / `employmentType`                                     | See mapping below; defaults to full-time for unknown values                                                                     |
| `PublishedDate`  | `postDateInGMT`                                                           | As-is, e.g. `"2026-05-22T00:00:00Z"`                                                                                            |
| `Title`          | `postingTitle`                                                            | Falls back to listing title when detail title is empty                                                                          |
| `Requirements`   | —                                                                         | Included inline in `Description`                                                                                                |

### Employment type mapping

| Input (any of `type`/`jobType`/`employmentType`, case-insensitive)                | Output       |
| --------------------------------------------------------------------------------- | ------------ |
| `Part-Time` / `part_time` / `parttime`                                            | `part_time`  |
| `Contract` / `contractor`                                                         | `contract`   |
| `Intern` / `internship`                                                           | `internship` |
| Anything else (`Regular`, `PIPE`, `REQ`, `CORPORATE`, `Standard`, empty, unknown) | `full_time`  |

### Salary regex

```
\$\d{1,3}(?:,\d{3})*(?:\.\d+)?\s*(?:and|to|–|—|-)\s*\$\d{1,3}(?:,\d{3})*(?:\.\d+)?
```

Anchored on grouped US numbers so a trailing list comma in the footer (`"$X and $Y, and your base pay…"`) cannot leak into the capture.

## Implementation

### Type: `Apple`

```go
type Apple struct {
    baseURL string // overridable for tests
}

func New() *Apple
func (a *Apple) FetchJobs(ctx context.Context, companyID string) (types.FetchJobsResponse, error)
func (a *Apple) FetchJobContent(ctx context.Context, job types.RawJob) (types.FetchJobContentResponse, error)
func (a *Apple) CareersURL(id string) string
```

### Internal structs

- `hydrationData` / `loaderData` / `searchData` — listing-page hydration shape.
- `appleJob` — one listing entry.
- `appleLocation` — tolerant of both listing (`stateProvince`/`countryID`) and detail (`stateCode`/`countryCode` may also appear) shapes.
- `appleTeam` — `teamName` / `teamCode` / `teamID`.
- `detailHydrationData` / `detailLoaderData` / `detailJobDetails` — detail-page hydration shape.
- `appleJobDetail` — `loaderData.jobDetails.jobsData`.
- `appleFooter` — one entry in `postingFooters` (carries the salary text on `POSTING_FOOTER_RULES*`).

### Key functions

- `FetchJobs(ctx, _)` — paginates listings.
- `fetchJobsPage(ctx, page)` — single page of listings.
- `FetchJobContent(ctx, job)` — GETs `job.URL`, parses detail hydration.
- `jobToItem(job)` — converts listing `appleJob` → thin `FetchJobsItem` (no `FullContent`).
- `jobsDataToResponse(job, jd)` — converts `appleJobDetail` → `FetchJobContentResponse`.
- `extractHydrationData(body)` — shared between listing and detail; handles direct-object, double-quote `JSON.parse`, and single-quote `JSON.parse` variants.
- `buildDescription(jd)` — joins labeled sections, skipping empties.
- `extractCompensation(footers)` — regex over each footer's stripped content.
- `formatLocation` / `formatSingleLocation` — city ⇢ name and stateProvince ⇢ stateCode fallbacks.
- `isUSALocation` / `primaryCountryCode` / `hasUSACountry` — country detection across listing + detail shapes.
- `mapJobType` — employment type table; full-time default.

## Headers

```
User-Agent: Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36
Accept: text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8
Accept-Language: en-US,en;q=0.5
```

## Error handling

- **Context cancelled**: propagates `context.Canceled`.
- **HTTP non-200**: `unexpected status {code}`.
- **Missing hydration data**: `window.__staticRouterHydrationData not found in page`.
- **Malformed JSON**: `decode hydration JSON: …` (listing) or `apple: decode detail JSON: …` (detail).
- **Empty `RawJob.URL` on `FetchJobContent`**: `apple: empty job URL`.

## Interface compliance

- `types.Adapter` — `FetchJobs`
- `types.URLProvider` — `CareersURL`
- `types.ContentFetcher` — `FetchJobContent`

Compile-time checks:

```go
var (
    _ types.Adapter        = (*Apple)(nil)
    _ types.URLProvider    = (*Apple)(nil)
    _ types.ContentFetcher = (*Apple)(nil)
)
```

## Constants

| Name                   | Value                                                 |
| ---------------------- | ----------------------------------------------------- |
| `defaultBaseURL`       | `https://jobs.apple.com`                              |
| `listingPath`          | `/en-us/search?location=united-states-USA&page=%d`    |
| `userAgent`            | Chrome 124 on macOS                                   |
| `publicJobURLTemplate` | `%s/en-us/details/%s/%s` (host resolved at scan time) |
| `usaCountryID`         | `iso-country-USA`                                     |

## Dependencies

- Standard library: `context`, `encoding/json`, `fmt`, `io`, `net/http`, `regexp`, `slices`, `strings`
- Project: `apps/crawler/ats/types`, `service/company/common`, `service/job/common`
