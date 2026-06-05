# Rippling Adapter

Thin HTTP adapter for the `crawler-v2` module. Fetches job listings and detail pages from Rippling's public careers site by parsing embedded Next.js `__NEXT_DATA__` JSON.

## Architecture

### FetchJobListing

**URL**: `https://www.rippling.com/careers/open-roles`

Next.js SSG page — all ~716 jobs are embedded in a single `<script id="__NEXT_DATA__">` tag.

**Steps:**

1. GET the careers page with a browser User-Agent
2. Extract JSON from `<script id="__NEXT_DATA__">...</script>`
3. Decode `props.pageProps.jobs.items[]`
4. Filter: only jobs where any `location.countryCode == "US"`
5. Return as `core.JobListing` with `EmployType = full_time` (Rippling does not expose type in listing)

### FetchJobPost

**URL**: `https://ats.rippling.com/rippling/jobs/{id}`

Next.js SSR page. Job ID is extracted from the last path segment of `summary.URL`.

**Steps:**

1. Extract job ID from `summary.URL` path
2. GET `{atsBaseURL}/rippling/jobs/{id}` with a browser User-Agent
3. Extract `__NEXT_DATA__` JSON
4. Decode `props.pageProps.apiData`
5. Return as `core.JobPost` with HTML stripped from description

## ID Format

- Listing / post: `rippling-{uuid}` where `{uuid}` is the `id` field from the API

## Field Mappings

### Listing → JobSummary

| JobSummary field | Source                                            |
| ---------------- | ------------------------------------------------- |
| `ID`             | `"rippling-" + item.ID`                           |
| `Adapter`        | `"rippling"`                                      |
| `Title`          | `item.Name`                                       |
| `Location`       | First US location display name                    |
| `URL`            | `item.URL` (falls back to constructed ATS URL)    |
| `EmployType`     | `full_time` (hardcoded; not available in listing) |
| `LocationKnown`  | `true`                                            |
| `AllLocations`   | Display names of all US locations                 |

### Detail → JobPost

| JobPost field   | Source                                                                       |
| --------------- | ---------------------------------------------------------------------------- |
| `ID`            | `"rippling-" + detail.ID`                                                    |
| `Title`         | `detail.Title`                                                               |
| `Location`      | Semicolon-joined display names of all locations                              |
| `URL`           | Constructed `{atsBaseURL}/rippling/jobs/{id}`                                |
| `Description`   | `descriptionHtml` → `description` → `jobDescription` → `body`, HTML stripped |
| `Compensation`  | `compensation` → `compensationRange` → `salary`                              |
| `WorkplaceType` | `workplaceType` → `workplace_type`                                           |
| `Department`    | `department.name` → `departmentName`                                         |
| `PublishedDate` | `createdAt`                                                                  |

## HTML Extraction

`extractNextData(body []byte) ([]byte, error)` uses string search (not regex) to find:

```
<script id="__NEXT_DATA__" ...> ... </script>
```

Returns the raw JSON bytes between `>` and `</script>`.

## Testing

Unit tests use `httptest.NewServer` with HTML fixtures in `testdata/`. The test adapter exposes `careersBaseURL` and `atsBaseURL` fields for override.

- `listing.html` — 3 jobs: 2 US (SF, NY), 1 non-US (London/GB)
- `job_post.html` — detail page for `aaa-111` with full fields

Integration tests (`rippling_integration_test.go`) are gated with `testing.Short()`.

## Interface Compliance

- `core.ListingFetcher` (FetchJobListing)
- `core.PostFetcher` (FetchJobPost)
