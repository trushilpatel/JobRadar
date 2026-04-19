# Uber — Internal Next.js API Adapter

Uber's careers site at `uber.com/us/en/careers/list/` is powered by an internal
Next.js API. The adapter calls two endpoints to collect all current job listings.
No public ATS platform (Greenhouse, Lever, etc.) is involved.

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
      "Engineering": ["Backend", "Machine Learning", "Frontend", ...],
      "Design": ["Product Design", "UX Research"],
      "Product": ["Product Management"],
      ...
    },
    "location": [ ... ]
  }
}
```

The `department` map keys are the department names used to paginate the job search.
Sub-team arrays (values) are informational only — not used by this adapter.

**Departments (as of April 2026):**

| Department                   | Job Count |
| ---------------------------- | --------- |
| Engineering                  | 388       |
| Sales & Account Management   | 338       |
| Operations                   | 163       |
| Community Operations         | 84        |
| Data Science                 | 68        |
| Product                      | 56        |
| Marketing                    | 39        |
| Finance & Accounting         | 27        |
| People & Places              | 27        |
| University                   | 24        |
| Business Development         | 23        |
| Design                       | 13        |
| Safety, Security & Insurance | 11        |
| Legal                        | 9         |
| Communications               | 5         |
| Public Policy                | 3         |
| **Total**                    | **1,278** |

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
        "type": "",
        "programAndPlatform": "",
        "location": {
          "country": "CAN",
          "region": "Ontario",
          "city": "Toronto",
          "countryName": "Canada"
        },
        "featured": false,
        "level": "5",
        "creationDate": "2026-03-10T00:00:00.000Z",
        "otherLevels": null,
        "team": "Backend",
        "portalID": "EXTERNAL",
        "isPipeline": false,
        "statusID": "D31001",
        "statusName": "Approved",
        "updatedDate": "2026-04-09T11:00:00.000Z",
        "uniqueSkills": "",
        "timeType": "Full-Time",
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

**Key notes:**

- `description` is **full Markdown** — no separate detail fetch needed.
- `totalResults` is a protobuf Int64 value: `{ low: int32, high: int32, unsigned: bool }`.
  For realistic job counts, `high` is always 0 — use `low` directly.
- The API hard caps at 1000 results per call regardless of `limit`. Each department
  has fewer than 1000 jobs, so one call per department retrieves all jobs.
- `page` parameter has no effect — pagination only works via department filtering.
- The API returns 403 if `x-csrf-token` header is absent. The value is not validated.

## CSRF protection

Uber uses a **custom request header CSRF strategy**: the server checks that
`x-csrf-token` is present. The value is not validated against a cookie, so
the static value `"x"` is sufficient for server-to-server requests.

Browsers can't set arbitrary headers cross-origin (CORS blocks it), so header
presence proves same-origin intent.

## Pagination strategy

```
loadFilterOptions → [dept1, dept2, ..., deptN]
    ↓ for each dept
loadSearchJobsResults(department=[deptN]) → all jobs for that dept
    ↓ dedup by id
complete job set
```

Each job belongs to exactly one department (confirmed: per-dept totals sum to
the API's global total of 1,278). Dedup by numeric `id` is a safety net only.

## Field capture

| API field            | Go field             | Type              | Notes                                  |
| -------------------- | -------------------- | ----------------- | -------------------------------------- |
| `id`                 | `ID`                 | `int`             | Numeric job ID                         |
| `title`              | `Title`              | `string`          |                                        |
| `description`        | `Description`        | `string`          | Full Markdown — not HTML               |
| `department`         | `Department`         | `string`          |                                        |
| `type`               | `Type`               | `string`          | Usually empty                          |
| `programAndPlatform` | `ProgramAndPlatform` | `string`          | Usually empty                          |
| `location`           | `Location`           | `uberLocation`    | Primary location object                |
| `featured`           | `Featured`           | `bool`            |                                        |
| `level`              | `Level`              | `string`          | Uber level (e.g. "5", "6")             |
| `creationDate`       | `CreationDate`       | `string`          | ISO 8601                               |
| `otherLevels`        | `OtherLevels`        | `json.RawMessage` | Nullable                               |
| `team`               | `Team`               | `string`          | Sub-team within department             |
| `portalID`           | `PortalID`           | `string`          | Always "EXTERNAL" for public listings  |
| `isPipeline`         | `IsPipeline`         | `bool`            |                                        |
| `statusID`           | `StatusID`           | `string`          |                                        |
| `statusName`         | `StatusName`         | `string`          |                                        |
| `updatedDate`        | `UpdatedDate`        | `string`          | ISO 8601                               |
| `uniqueSkills`       | `UniqueSkills`       | `string`          | Usually empty                          |
| `timeType`           | `TimeType`           | `string`          | "Full-Time", "Part-Time", etc.         |
| `allLocations`       | `AllLocations`       | `[]uberLocation`  | All locations for multi-location roles |

## Field mapping

### FetchJobs (job → FetchJobsItem)

| API field      | FetchJobsItem field         | Notes                                                                                 |
| -------------- | --------------------------- | ------------------------------------------------------------------------------------- |
| `id`           | `ID`                        | Prefixed: `uber-{id}` (int → string)                                                  |
| `title`        | `Title`                     |                                                                                       |
| `allLocations` | `Location`                  | Multi: joined with `"; "`. Single: `"City, Region, Country"`                          |
| `timeType`     | `EmploymentType`            | Mapped: Full-Time→`full_time`, Part-Time→`part_time`, etc.                            |
| `description`  | `FullContent.Description`   | Markdown passthrough + `"\n\nUber Level: <level>"` appended when `level` is non-empty |
| `level`        | `FullContent.Description`   | Appended as `"Uber Level: <value>"` to expose seniority to LLM enricher               |
| `department`   | `FullContent.Department`    |                                                                                       |
| `creationDate` | `FullContent.PublishedDate` | ISO 8601 string                                                                       |

**Note:** `FullContent` is always populated (never nil). This is a **fat adapter**
— no `ContentFetcher` interface is implemented.

## ID format

```
uber-157927
```

Prefix is `model.AdapterUber`. The numeric portion is the Uber `id` field formatted as decimal.

## URL format

| Type         | Pattern                                                             |
| ------------ | ------------------------------------------------------------------- |
| Careers page | `https://www.uber.com/us/en/careers/list/`                          |
| Job page     | `https://www.uber.com/careers/list/{id}`                            |
| Filter API   | `POST https://www.uber.com/api/loadFilterOptions?localeCode=en`     |
| Search API   | `POST https://www.uber.com/api/loadSearchJobsResults?localeCode=en` |

**Note:** Job page URLs like `/careers/list/{id}` redirect to
`/global/en/careers/list/{id}/`. Both resolve to the same page.

## Notes

- **Fat adapter** — `FullContent` is always populated in `FetchJobs`. The description
  is complete Markdown, so `job_post_analyzer` does not need a separate HTTP fetch.
- **No ContentFetcher** — description is included in the list response.
- **Employment type** defaults to `full_time` when `timeType` is empty or unrecognized.
  Uber does not expose compensation ranges in this API.
- **Workplace type** is not available in this API (no remote/onsite/hybrid field).
- `level` contains Uber's internal level system (e.g., "5" = L5 = Software Engineer II, "5B" = between L5/L6).
  When non-empty, appended to `FullContent.Description` as `"Uber Level: <value>"` so the LLM
  enricher can infer seniority and experience requirements.
- The adapter fetches departments fresh on each run — new departments added by Uber
  are automatically covered without adapter changes.
