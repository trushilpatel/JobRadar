# Apple Adapter

Fat HTTP adapter for [jobs.apple.com](https://jobs.apple.com). Scrapes listing pages where job descriptions are embedded directly in the HTML as React Router hydration data.

## Architecture

**Pattern**: Fat adapter (FullContent always populated on FetchJobs)

**No ContentFetcher**: The listing page contains the full job description in `jobSummary`, so no separate detail fetch is needed.

## API Details

### Listing Endpoint

```
GET https://jobs.apple.com/en-us/search?location=united-states-USA&page={n}
```

- **Filter**: US jobs only (applied via `location=united-states-USA` URL param)
- **Pagination**: Increment page starting at `page=1`; stop when `searchResults` is empty
- **Page size**: ~20 jobs per page (varies, don't hardcode)

### Hydration Data Extraction

Each listing page embeds React Router hydration data in an HTML script tag:

```html
<script>
  window.__staticRouterHydrationData = {"loaderData": {"search": {...}}, ...};
</script>
```

Extraction strategy:
1. Find the string `window.__staticRouterHydrationData = ` in the page HTML
2. Extract JSON from that position until the closing `;</script>`
3. Parse as JSON to get the `loaderData.search.searchResults` array

### Response Structure

```json
{
  "loaderData": {
    "search": {
      "searchResults": [
        {
          "id": "200647207-3543",
          "positionId": "200647207",
          "postingTitle": "Software Engineer",
          "jobSummary": "<p>About the position...</p><p>Key Qualifications...</p>",
          "locations": [
            {
              "city": "Cupertino",
              "stateCode": "CA",
              "countryCode": "USA"
            }
          ],
          "postDateInGMT": "2026-03-15 00:00:00",
          "team": {
            "teamName": "Software and Services",
            "teamCode": "SWDVR"
          },
          "homeOffice": false,
          "standardWeeklyHours": 40,
          "transformedPostingTitle": "software-engineer",
          "type": "Regular",
          "reqId": "200647207",
          "isMultiLocation": false
        }
      ],
      "totalRecords": 4744
    }
  }
}
```

## Mapping

### ID Format

`apple-{id}` where `id` is the job's `id` field (e.g., `apple-200647207-3543`)

### Job URL

Template: `https://jobs.apple.com/en-us/details/{id}/{transformedPostingTitle}`

Example: `https://jobs.apple.com/en-us/details/200647207-3543/software-engineer`

### Location

- **Single location**: `"City, State"` (e.g., `"Cupertino, CA"`)
- **Multiple locations**: Joined with `"; "` (e.g., `"Cupertino, CA; Austin, TX; New York, NY"`)
- **Empty city/state**: Skipped in formatting
- **All locations empty**: Return empty string

### FullContent Fields

All fields populated from the listing page (fat adapter):

| Field | Source | Notes |
|-------|--------|-------|
| `Description` | `jobSummary` (HTML stripped) | Always present; HTML tags removed via `types.StripHTML()` |
| `Department` | `team.teamName` | Always present |
| `PublishedDate` | `postDateInGMT` | As-is from API (e.g., `"2026-03-15 00:00:00"`) |
| `WorkplaceType` | — | Not available; set to empty string |
| `Compensation` | — | Not available; set to empty string |
| `Requirements` | — | Not available; included in Description via jobSummary |

### Employment Type Mapping

`type` field → `model.EmployType`:

| Input | Output | Notes |
|-------|--------|-------|
| `"Regular"` | `"full_time"` | Default |
| `""` (empty) | `"full_time"` | Treats as full-time |
| `"Part-Time"` | `"part_time"` | |
| `"Contract"` | `"contract"` | |
| `"Intern"` | `"internship"` | |
| Other | `"full_time"` | Unknown types default to full-time |

## Implementation

### Class: `Apple`

```go
type Apple struct {
    baseURL string  // "https://jobs.apple.com" (prod), overridable for tests
}

// Constructor
func New() *Apple

// Interfaces
func (a *Apple) FetchJobs(ctx context.Context, companyID string) (types.FetchJobsResponse, error)
func (a *Apple) CareersURL(id string) string
```

### Response Structs

- **`hydrationData`**: Top-level JSON from `window.__staticRouterHydrationData`
- **`loaderData`**: Container for `search` field
- **`searchData`**: Holds `searchResults` array and `totalRecords`
- **`appleJob`**: Single job in the results (captures all API fields)
- **`appleLocation`**: City, state, country of a job location
- **`appleTeam`**: Team/department info

### Key Functions

- **`FetchJobs(ctx, companyID)`**: Main entry point; paginates and aggregates all jobs
- **`fetchJobsPage(ctx, page)`**: Fetches single page of listings
- **`extractHydrationData(body string)`**: Parses JSON from HTML script tag
- **`jobToItem(job appleJob)`**: Converts API job to pipeline `FetchJobsItem`
- **`formatLocation(locations []appleLocation)`**: Formats location list
- **`mapJobType(jobType string)`**: Maps `type` to `model.EmployType`

## Headers

All requests include browser-like headers to avoid blocking:

```
User-Agent: Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36
Accept: text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8
Accept-Language: en-US,en;q=0.5
```

## Testing

### Unit Tests

- **`TestNew_DefaultSettings`**: Constructor initializes with correct base URL
- **`TestCareersURL`**: Public careers page URL is correct
- **`TestFetchJobs_SinglePage`**: Single page of jobs stops on empty page 2
- **`TestFetchJobs_MultiPage_Pagination`**: Increments page correctly across 3+ pages
- **`TestFetchJobs_EmptyFirstPage`**: Returns 0 jobs when first page is empty
- **`TestFetchJobs_HTTPError`**: Propagates HTTP errors (e.g., 500)
- **`TestFetchJobs_MalformedHTML`**: Returns error when hydration data is missing
- **`TestFetchJobs_IDFormat`**: Verifies `apple-{id}` ID format
- **`TestFetchJobs_JobURL`**: Constructs correct job URLs from id and title
- **`TestFetchJobs_FullContentAlwaysSet`**: FullContent is never nil
- **`TestFetchJobs_DescriptionStripsHTML`**: HTML tags removed from jobSummary
- **`TestFetchJobs_LocationFormat`**: Formats single and multiple locations correctly
- **`TestFetchJobs_MultiLocation`**: Multiple locations joined with `"; "`
- **`TestFetchJobs_ContextCancelled`**: Respects context cancellation
- **`TestMapJobType`**: Employment type mapping table test
- **`TestFormatLocation`**: Location formatting edge cases
- **`TestFormatSingleLocation`**: Single location formatting
- **`TestFetchJobs_Real`** (gated `TEST_REAL=1`): Live API test

### Test Server

`newTestServer(t, pages)` creates a mock HTTP server that:
- Accepts `page` query param
- Returns pre-configured job listings for each page
- Embeds `window.__staticRouterHydrationData` in HTML response
- Returns empty results for unconfigured pages

## Error Handling

- **Context cancelled**: Propagates `context.Canceled`
- **Request failure**: Wraps with `"apple: fetch page {n}: {err}"`
- **HTTP non-200**: Returns formatted error with status code
- **Missing hydration data**: `"extract hydration data: window.__staticRouterHydrationData not found in page"`
- **Malformed JSON**: `"decode hydration JSON: {json.Unmarshal error}"`

## Pagination Logic

```
for page = 1; ; page++ {
  jobs = fetchJobsPage(page)
  if len(jobs) == 0 {
    break  // Stop on empty page
  }
  allJobs.append(jobs)
}
```

## Interface Compliance

- Implements `types.Adapter` (required: `FetchJobs()`)
- Implements `types.URLProvider` (optional: `CareersURL()`)
- Does **not** implement `types.ContentFetcher` (fat adapter)

Compile-time checks in code:
```go
var _ types.Adapter = (*Apple)(nil)
var _ types.URLProvider = (*Apple)(nil)
```

## Constants

| Name | Value |
|------|-------|
| `defaultBaseURL` | `"https://jobs.apple.com"` |
| `listingPath` | `"/en-us/search?location=united-states-USA&page=%d"` |
| `userAgent` | Chrome 124 on macOS |
| `publicJobURLTemplate` | `"%s/en-us/details/%s/%s"` |

## Dependencies

- Standard library: `context`, `encoding/json`, `fmt`, `io`, `net/http`, `regexp`, `strings`
- Project: `model`, `pipeline/ats/types`
- Module: `github.com/trushilpatel/jobradar`
