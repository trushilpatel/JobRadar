# Rippling Adapter

Thin HTTP adapter for fetching job listings from Rippling's careers pages and detail pages.

## Architecture

### FetchJobs (Listing)

**URL**: `https://www.rippling.com/careers/open-roles`

This is a Next.js SSG page. All ~716 job listings are embedded in a single `<script id="__NEXT_DATA__">` tag containing the full JSON payload.

**Extraction**:
1. HTTP GET the careers page
2. Extract the JSON between `<script id="__NEXT_DATA__" type="application/json">` and `</script>`
3. Parse `props.pageProps.jobs.items[]` to get job objects
4. Filter: only return jobs with at least one location where `countryCode == "US"`
5. Return as thin items with `FullContent = nil`

**Output format**:
```
ID:       "rippling-{uuid}"
Title:    job.name
URL:      job.url
Location: Formatted "City, StateCode" from all US locations (joined with "; ")
```

### FetchJobContent (Detail)

**URL**: `https://ats.rippling.com/rippling/jobs/{uuid}`

This is a Next.js SSR page. Full job details are in `props.pageProps.apiData`.

**Extraction**:
1. HTTP GET the detail page
2. Extract `__NEXT_DATA__` JSON (same method as listing)
3. Parse `props.pageProps.apiData` for full job fields
4. Use flexible field name matching to handle API variations

**Field name mappings**:
- Description: Try `descriptionHtml` → `description` → `jobDescription` → `body`, then strip HTML
- Compensation: Try `compensation` → `compensationRange` → `salary`
- WorkplaceType: Try `workplaceType` → `workplace_type`
- Department: Try `department.name` → `departmentName`

**Fallback**: If JSON parsing fails, fall back to `StripHTML(page_body)` to extract plain text.

## Implementation Notes

### ID Format
- Listing ID: `rippling-{uuid}` where uuid is the `id` field from the API
- Detail URL: Extract uuid via `stripPrefix()` to get the ATS job ID

### Location Filtering
- Only US jobs returned (country code filter on listing)
- Location string: "City, StateCode" for each US location, joined by "; "
- Example: "San Francisco, CA; New York, NY; Austin, TX"

### Adapter Type
- **Thin adapter**: FullContent is `nil` on listing, fetched separately via ContentFetcher
- No Playwright/Chromedp needed — plain HTTP GET only
- User-Agent spoofing prevents Cloudflare/WAF blocks

### Error Handling
- HTTP errors bubble up with context (status code, endpoint)
- Missing `__NEXT_DATA__` tag → error in FetchJobs
- Missing `__NEXT_DATA__` tag → fallback to StripHTML in FetchJobContent
- JSON decode errors → error with first 500 bytes of raw response

## Testing

### Test Server Pattern
Tests use `httptest.NewServer` to serve mock HTML pages with embedded `__NEXT_DATA__` JSON.

```go
html := buildNextJSHTML(t, nextJobsJSON(jobs))
srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    w.Write([]byte(html))
}))
defer srv.Close()
```

### Test Coverage
- `TestNew_DefaultSettings`: Verify constructor defaults
- `TestCareersURL`: Verify careers URL generation
- `TestFetchJobs_SingleUSJob`: One US job in listing
- `TestFetchJobs_FiltersNonUSJobs`: Mixed US + non-US, only US returned
- `TestFetchJobs_AllNonUSJobs`: No US jobs → empty result
- `TestFetchJobs_HTTPError`: HTTP 500 response
- `TestFetchJobs_MalformedHTML`: No `__NEXT_DATA__` tag
- `TestFetchJobs_MultipleLocations`: Job with multiple US locations
- `TestFetchJobs_ContextCancelled`: Cancelled context propagates error
- `TestFetchJobs_IDFormat`: ID has correct `rippling-` prefix
- `TestFetchJobs_FullContentNil`: Thin adapter confirms nil FullContent
- `TestFetchJobContent_Success`: Full detail page parsing
- `TestFetchJobContent_HTTPError`: Detail page HTTP error
- `TestFetchJobContent_MalformedHTML`: Detail page fallback to StripHTML
- `TestFetchJobs_Real`: Real API test (gated on `TEST_REAL=1`)

## API Response Examples

### Listing page (`__NEXT_DATA__`)
```json
{
  "props": {
    "pageProps": {
      "jobs": {
        "items": [
          {
            "id": "550e8400-e29b-41d4-a716-446655440000",
            "name": "Software Engineer",
            "url": "https://ats.rippling.com/rippling/jobs/550e8400-e29b-41d4-a716-446655440000",
            "department": {
              "name": "Engineering"
            },
            "locations": [
              {
                "name": "San Francisco, CA",
                "country": "United States",
                "countryCode": "US",
                "state": "California",
                "stateCode": "CA",
                "city": "San Francisco",
                "workplaceType": "ON_SITE"
              }
            ],
            "language": "en"
          }
        ]
      }
    }
  }
}
```

### Detail page (`__NEXT_DATA__`)
```json
{
  "props": {
    "pageProps": {
      "apiData": {
        "id": "550e8400-e29b-41d4-a716-446655440000",
        "title": "Software Engineer",
        "descriptionHtml": "<h2>About the role</h2><p>...</p>",
        "compensation": "$180,000 - $245,000 USD/year",
        "workplaceType": "ON_SITE",
        "department": {
          "name": "Engineering"
        },
        "locations": [
          {
            "city": "San Francisco",
            "stateCode": "CA",
            "countryCode": "US",
            "workplaceType": "ON_SITE"
          }
        ],
        "createdAt": "2026-03-01T00:00:00.000Z"
      }
    }
  }
}
```

## Constants

- `defaultCareersBaseURL`: `https://www.rippling.com`
- `defaultATSBaseURL`: `https://ats.rippling.com`
- `userAgent`: Mozilla 5.0 Chrome/124 (standard browser UA)

## Interface Compliance

- ✅ `types.Adapter` (FetchJobs)
- ✅ `types.ContentFetcher` (FetchJobContent)
- ✅ `types.URLProvider` (CareersURL)
