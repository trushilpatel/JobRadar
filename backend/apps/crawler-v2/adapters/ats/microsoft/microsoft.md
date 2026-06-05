# Microsoft — Careers API Adapter (crawler-v2)

Microsoft's careers site at `careers.microsoft.com` is powered by a structured JSON API.
The adapter implements `core.ListingFetcher` and `core.PostFetcher`.

## Files

| File                            | Role                                                                                                                           |
| ------------------------------- | ------------------------------------------------------------------------------------------------------------------------------ |
| `microsoft.go`                  | Adapter struct, `FetchJobListing`, `FetchJobPost`, HTTP helpers, URL builders                                                  |
| `mapper.go`                     | Pure mapping functions: `mapJobSummary`, `mapJobPost`, `stripReqID`, `extractPositionID`, `mapEmploymentType`, `unixToRFC3339` |
| `api/types.go`                  | API response types: `SearchResponse`, `SearchResponseData`, `Position`, `PositionDetail`, `PositionDetailResponse`             |
| `microsoft_test.go`             | Unit tests for HTTP layer                                                                                                      |
| `mapper_test.go`                | Unit tests for mapping functions                                                                                               |
| `microsoft_integration_test.go` | Integration tests (skipped under `-short`)                                                                                     |
| `testdata/listing.json`         | Search API fixture (2 positions)                                                                                               |
| `testdata/job_post.json`        | Position detail API fixture                                                                                                    |

## API endpoints

### Search (paginated)

```
GET /api/pcsx/search?domain=microsoft.com&query=&location=United+States%2C+Multiple+Locations%2C+Multiple+Locations&start={n}&num=20
```

Paginate by incrementing `start` by the number of positions returned. Stop when
`len(positions)==0` or `len(all) >= data.count`.

### Position detail

```
GET /api/pcsx/position_details?position_id={int64}&domain=microsoft.com&hl=en
```

The `position_id` is extracted from the internal URL stored in `JobSummary.URL`
(last path segment of `https://apply.careers.microsoft.com/careers/job/{id}`).

## Adapter design decisions

- **`companyID` ignored** — adapter always targets `careers.microsoft.com`
- **Internal URL** — `JobSummary.URL` stores the internal `apply.careers.microsoft.com/careers/job/{id}` URL so `FetchJobPost` can extract the numeric position ID for the detail API call
- **User-Agent required** — requests set a Chrome-like User-Agent to avoid 403s
- **Req-ID stripping** — job titles end with `" - {REQID}"` (e.g. `"Engineer - 17248K"`); `stripReqID` removes this suffix
- **`StandardizedLocations`** is `json.RawMessage` in both `Position` and `PositionDetail` — the API returns `map[string][]string` in search and `[]string` in detail responses
