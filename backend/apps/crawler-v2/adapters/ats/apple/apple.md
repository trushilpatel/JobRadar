# Apple Adapter

HTTP adapter for [jobs.apple.com](https://jobs.apple.com). Scrapes listing pages for the index and per-job detail pages for rich content (description, qualifications, compensation).

## Architecture

**Pattern**: Thin adapter — `ListingFetcher` for the listing, `PostFetcher` for the detail.

The listing endpoint exposes only stub data per job (basic metadata). Description, responsibilities, minimum/preferred qualifications, and compensation live on the detail page, so callers invoke `FetchJobPost` for each job that requires enrichment.

## API Details

### Listing endpoint

```
GET https://jobs.apple.com/en-us/search?location=united-states-USA&page={n}
```

- **Filter**: US jobs via the `location=united-states-USA` URL param.
- **Pagination**: increment `page` starting at 1; stop when `searchResults` is empty.
- **Hydration**: each page embeds `window.__staticRouterHydrationData` (direct object, double-quote `JSON.parse`, or single-quote `JSON.parse` variants).
- **Job shape** (`loaderData.search.searchResults[]`): `id`, `positionId`, `postingTitle`, `transformedPostingTitle`, `locations[]`, `workHours`, `type`.

### Detail endpoint

```
GET https://jobs.apple.com/en-us/details/{id}/{transformedPostingTitle}
```

- Returns the same React Router hydration script. The job payload is at `loaderData.jobDetails.jobsData`.
- **Captured fields**: `jobSummary`, `boardDescription`, `responsibilities`, `minimumQualifications`, `preferredQualifications`, `locations[]`, `postingFooters[].content`.

### Locations

Listing locations populate `name`, `countryID`, `countryName`; `city`/`stateProvince` may be empty. Detail locations populate the full structure. `formatSingleLocation` falls back: city → name, stateProvince → stateCode.

`isUSALocation` matches `countryID == iso-country-USA`, `countryCode in {USA, US}`, or `countryName in {United States, United States of America}`.

## Mapping

### ID format

`apple-{id}` where `id` is the job's listing `id` field (e.g. `apple-200647207`).

### Job URL

`https://jobs.apple.com/en-us/details/{id}/{transformedPostingTitle}`. The URL is built from `resolveBaseURL()` so tests can redirect both listing and detail fetches to one mock server.

### `FetchJobPost` fields

| Field          | Source                                                                                                                              |
| -------------- | ----------------------------------------------------------------------------------------------------------------------------------- |
| `ID`           | `"apple-" + jd.ID`                                                                                                                  |
| `Title`        | `jd.PostingTitle`                                                                                                                   |
| `Location`     | First US location as `"City, State"`; falls back to first location when no US location                                              |
| `Description`  | `Summary`, `Description`, `Responsibilities`, `Requirements`, `Preferred` — labeled sections; HTML stripped; empty sections skipped |
| `Compensation` | First `$X,XXX and $Y,YYY` / `$X to $Y` range from `postingFooters[].content`                                                        |
| `CountryCode`  | `"US"` for USA marker; ISO code from `countryCode` or `countryID` prefix otherwise                                                  |

### Employment type mapping (workHours)

| Input (case-insensitive)                 | Output       |
| ---------------------------------------- | ------------ |
| `PART_TIME` / `part-time` / `parttime`   | `part_time`  |
| `INTERNSHIP` / `intern`                  | `internship` |
| Anything else (incl. `FULL_TIME`, empty) | `full_time`  |

### Salary regex

```
\$\d{1,3}(?:,\d{3})*(?:\.\d+)?\s*(?:and|to|–|—|-)\s*\$\d{1,3}(?:,\d{3})*(?:\.\d+)?
```

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

## Interface compliance

- `core.ListingFetcher` — `FetchJobListing`
- `core.PostFetcher` — `FetchJobPost`

Compile-time checks at the bottom of `apple.go`.

## Files

| File                        | Role                                                                     |
| --------------------------- | ------------------------------------------------------------------------ |
| `apple.go`                  | Adapter struct, HTTP fetch, pagination, hydration extraction             |
| `mapper.go`                 | `mapJobSummary`, `mapJobPost`, location helpers, compensation extraction |
| `api/types.go`              | Listing + detail response struct definitions                             |
| `apple_test.go`             | Unit tests for HTTP layer and hydration extraction                       |
| `mapper_test.go`            | Unit tests for all mapping functions                                     |
| `apple_integration_test.go` | Integration tests (skipped under `-short`)                               |
| `testdata/listing.html`     | HTML fixture with 3 jobs (2 US + 1 UK)                                   |
| `testdata/job_post.html`    | HTML fixture with all description sections + pay range                   |
