# Uber — Internal Next.js API Adapter

Uber's careers site at `uber.com/us/en/careers/list/` is powered by an internal
Next.js API. The adapter calls two endpoints to collect all current job listings.
No public ATS platform (Greenhouse, Lever, etc.) is involved.

## Adapter type

**Fat adapter** — `JobContent` is always set in `FetchJobListing`. The description
is full Markdown, so no separate post-fetch is needed. No `PostFetcher` is
implemented.

## API endpoints

### 1. Filter options (department list)

```
POST https://www.uber.com/api/loadFilterOptions?localeCode=en
Content-Type: application/json
x-csrf-token: x

{}
```

**Response:**

```json
{
  "data": {
    "department": {
      "Engineering": ["Backend", "Machine Learning", "Frontend"],
      "Design": ["Product Design", "UX Research"],
      "Product": ["Product Management"]
    }
  }
}
```

The `department` map keys are the department names used to paginate the job
search. Sub-team arrays (values) are informational only — not used by this adapter.

### 2. Job search (per department)

```
POST https://www.uber.com/api/loadSearchJobsResults?localeCode=en
Content-Type: application/json
x-csrf-token: x

{
  "params": {
    "query": "",
    "limit": 1000,
    "page": 0,
    "department": ["Engineering"]
  }
}
```

**Response:**

```json
{
  "status": "success",
  "data": {
    "results": [
      {
        "id": 157927,
        "title": "Software Engineer II - Discovery & Monetization",
        "description": "**About the Role**\n\nBuild...",
        "department": "Engineering",
        "timeType": "Full-Time",
        "location": {
          "country": "CAN",
          "region": "Ontario",
          "city": "Toronto",
          "countryName": "Canada"
        },
        "creationDate": "2026-03-10T00:00:00.000Z",
        "allLocations": [
          {
            "country": "CAN",
            "region": "Ontario",
            "city": "Toronto",
            "countryName": "Canada"
          }
        ]
      }
    ],
    "totalResults": { "low": 388, "high": 0, "unsigned": false }
  }
}
```

## CSRF protection

Uber uses a **custom request header CSRF strategy**: the server checks that
`x-csrf-token` is present. The value is not validated against a cookie, so
the static value `"x"` is sufficient for server-to-server requests.

## Pagination strategy

```
loadFilterOptions → [dept1, dept2, ..., deptN]
    ↓ for each dept
loadSearchJobsResults(department=[deptN]) → all jobs for that dept
    ↓ dedup by numeric id
complete job set
```

Each job belongs to exactly one department. Dedup by numeric `id` is a safety net.

## Field mapping

| API field      | JobSummary field           | Notes                                                          |
| -------------- | -------------------------- | -------------------------------------------------------------- |
| `id`           | `ID`                       | Prefixed: `uber-{id}` (int → string)                           |
| `title`        | `Title`                    |                                                                |
| `allLocations` | `Location`                 | Single: `City, Region, CountryName`. Multi: joined with `"; "` |
| `allLocations` | `AllLocations`             | One string per entry                                           |
| `timeType`     | `EmployType`               | Full-Time→`full_time`, Part-Time→`part_time`, etc.             |
| (baseURL)      | `URL`                      | `{baseURL}/careers/list/{id}`                                  |
| (always true)  | `LocationKnown`            | Always true for Uber jobs                                      |
| `description`  | `JobContent.Description`   | Full Markdown                                                  |
| `department`   | `JobContent.Department`    |                                                                |
| `creationDate` | `JobContent.PublishedDate` | ISO 8601 string                                                |

## ID format

```
uber-157927
```

## URL format

| Type       | Pattern                                                             |
| ---------- | ------------------------------------------------------------------- |
| Job page   | `https://www.uber.com/careers/list/{id}`                            |
| Filter API | `POST https://www.uber.com/api/loadFilterOptions?localeCode=en`     |
| Search API | `POST https://www.uber.com/api/loadSearchJobsResults?localeCode=en` |

## Notes

- `companyID` is ignored — the adapter targets a fixed Uber host.
- Employment type defaults to `full_time` when `timeType` is empty or unrecognised.
- Workplace type (remote/onsite/hybrid) is not available in the Uber API.
- `totalResults` is a protobuf Int64Value `{ low, high, unsigned }`. For realistic
  job counts `high` is always 0; only `low` is relevant.
- The API hard-caps at 1000 results per call regardless of `limit`. Each department
  has fewer than 1000 jobs, so one call per department retrieves all jobs.
