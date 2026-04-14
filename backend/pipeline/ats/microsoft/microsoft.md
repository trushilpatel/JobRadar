# Microsoft — Careers API Adapter

Microsoft's careers site at `careers.microsoft.com` is powered by a structured JSON API.
The adapter calls two endpoints to collect all current US job listings.

## API endpoints

### 1. Job search (paginated)

```
GET https://apply.careers.microsoft.com/api/pcsx/search?domain=microsoft.com&query=&location=United+States%2C+Multiple+Locations%2C+Multiple+Locations&start={n}&num=20
```

**Response:**

```json
{
  "status": "success",
  "error": "",
  "data": {
    "count": 1211,
    "positions": [
      {
        "id": 1812345678,
        "displayJobId": "1812345678",
        "name": "Senior Software Engineer",
        "locations": ["Redmond, Washington, United States"],
        "standardizedLocations": {"US": ["Redmond, WA"]},
        "postedTs": 1712678400,
        "department": "Engineering",
        "workLocationOption": "Up to 50% work from home",
        "atsJobId": "REQ-12345",
        "positionUrl": "https://careers.microsoft.com/v2/global/en/job/1812345678/senior-software-engineer"
      }
    ],
    "filterDef": {},
    "sortBy": {},
    "appliedFilters": {}
  }
}
```

**Key notes:**
- Paginate using the `start` parameter, incrementing by `num` (20) per page
- Stop when `len(positions) < 20`
- The `location` query parameter is hard-coded to filter US jobs only: `"United States, Multiple Locations, Multiple Locations"`
- The `status` field indicates success/error at the top level

### 2. Job detail (single position)

```
GET https://apply.careers.microsoft.com/api/pcsx/position_details?position_id={int64_id}&domain=microsoft.com&hl=en
```

**Response:**

```json
{
  "id": 1812345678,
  "displayJobId": "1812345678",
  "name": "Senior Software Engineer",
  "locations": ["Redmond, Washington, United States"],
  "postedTs": 1712678400,
  "department": "Engineering",
  "workLocationOption": "Up to 50% work from home",
  "jobDescription": "<p>HTML job description...</p>",
  "location": "Redmond, Washington, United States",
  "efcustomTextWorkSite": ["Hybrid"],
  "efcustomTextEmploymentType": ["Full-Time"],
  "efcustomTextRoletype": ["Individual Contributor"],
  "efcustomTextRequiredTravel": ["0-25%"],
  "publicUrl": "https://careers.microsoft.com/v2/global/en/job/1812345678/...",
  "atsJobId": "REQ-12345",
  "positionUrl": "https://careers.microsoft.com/v2/global/en/job/1812345678/..."
}
```

**Key notes:**
- The `position_id` parameter requires the numeric `id` field (not `displayJobId`)
- `jobDescription` contains HTML — must be stripped to plain text
- `efcustomTextEmploymentType[0]` contains the employment type ("Full-Time", "Part-Time", etc.)

## Field capture

| API field                     | Go field                      | Type             | Notes                              |
|-------------------------------|-------------------------------|------------------|------------------------------------|
| `id`                          | `ID`                          | `int64`          | Numeric position ID                |
| `displayJobId`                | `DisplayJobID`                | `string`         | Human-readable ID                  |
| `name`                        | `Name`                        | `string`         |                                    |
| `locations`                   | `Locations`                   | `[]string`       | All job locations                  |
| `standardizedLocations`       | `StandardizedLocations`       | `map[string][]string` | Normalized location codes    |
| `postedTs`                    | `PostedTs`                    | `int64`          | Unix timestamp (seconds)           |
| `department`                  | `Department`                  | `string`         |                                    |
| `workLocationOption`          | `WorkLocationOption`          | `string`         | e.g. "Up to 50% work from home"   |
| `atsJobId`                    | `AtsJobID`                    | `string`         |                                    |
| `positionUrl`                 | `PositionURL`                 | `string`         |                                    |
| `jobDescription`              | `JobDescription`              | `string`         | HTML — strip before use            |
| `location`                    | `Location`                    | `string`         | Primary location (detail only)     |
| `efcustomTextWorkSite`        | `EfcustomTextWorkSite`        | `[]string`       | e.g. ["Hybrid"]                    |
| `efcustomTextEmploymentType`  | `EfcustomTextEmploymentType`  | `[]string`       | e.g. ["Full-Time"]                 |
| `efcustomTextRoletype`        | `EfcustomTextRoletype`        | `[]string`       | Role classification               |
| `efcustomTextRequiredTravel`  | `EfcustomTextRequiredTravel`  | `[]string`       | Travel percentage                 |
| `publicUrl`                   | `PublicURL`                   | `string`         | Detail page public URL             |

## Field mapping

### FetchJobs (position → FetchJobsItem)

| API field       | FetchJobsItem field | Notes                                               |
|-----------------|---------------------|----------------------------------------------------|
| `id`            | (internal URL)      | Stored in URL for later ID extraction             |
| `displayJobId`  | `ID`                | Prefixed: `microsoft-{displayJobId}` (string)     |
| `name`          | `Title`             |                                                     |
| `locations`     | `Location`          | Joined with `"; "` if multiple                    |
| (default)       | `EmploymentType`    | Defaults to `full_time` (details fetched later)   |
| (none)          | `FullContent`       | Always nil — thin adapter                         |

**Note:** The internal URL stores the int64 `id` field for extraction by `FetchJobContent`.

### FetchJobContent (positionDetail → FetchJobContentResponse)

| API field                    | FetchJobContentResponse field | Notes                                          |
|------------------------------|-------------------------------|------------------------------------------------|
| `displayJobId`               | `ID`                          | Prefixed: `microsoft-{displayJobId}`          |
| `name`                        | `Title`                       |                                                |
| `location` or `locations`    | `Location`                    | Prefer `location` (detail), fall back to `locations` |
| `positionUrl`                | `URL`                         | Public career page URL                        |
| `jobDescription`             | `Description`                 | HTML stripped to plain text                   |
| `efcustomTextEmploymentType` | `EmploymentType`              | Mapped to model constants                     |
| `department`                 | `Department`                  |                                                |
| `workLocationOption`         | `WorkplaceType`               | e.g. "Up to 50% work from home"               |
| `postedTs`                   | `PublishedDate`               | Converted to RFC 3339 string                  |

## ID format

```
microsoft-1812345678
```

Prefix is `model.AdapterMicrosoft`. The numeric portion is the `displayJobId` string field.

**Important:** The internal storage URL includes the int64 `id` field for position detail API lookups:
```
https://apply.careers.microsoft.com/careers/job/{int64_id}
```

The `FetchJobContent` method extracts this int64 ID from the URL's last path segment to call the position_details API.

## URL format

| Type             | Pattern                                                                                  |
|------------------|------------------------------------------------------------------------------------------|
| Careers page     | `https://careers.microsoft.com/us/en/careers`                                            |
| Job page         | `https://careers.microsoft.com/v2/global/en/job/{displayJobId}/job-title-slug`          |
| Search API       | `GET https://apply.careers.microsoft.com/api/pcsx/search?domain=microsoft.com&...`       |
| Detail API       | `GET https://apply.careers.microsoft.com/api/pcsx/position_details?position_id={id}&...` |

## Pagination strategy

```
search(start=0)    → [positions 0-19]
    ↓
search(start=20)   → [positions 20-39]
    ↓
search(start=40)   → [positions 40-49] (< 20, stop)
    ↓
complete job set (50 jobs)
```

The adapter increments `start` by `num` (20) until fewer than 20 positions are returned.

## Employment type mapping

The `efcustomTextEmploymentType[0]` field is mapped to model constants:

| API value           | Model constant      |
|---------------------|---------------------|
| `"Full-Time"`       | `EmployFullTime`    |
| `"Part-Time"`       | `EmployPartTime`    |
| `"Contract"`        | `EmployContract`    |
| `"Intern"` or `"Internship"` | `EmployInternship` |
| (empty or unknown)  | `EmployFullTime`    |

## US filtering

The adapter filters to US jobs only by setting the `location` query parameter in the search API:
```
location=United+States%2C+Multiple+Locations%2C+Multiple+Locations
```

This URL-encoded string decodes to: `"United States, Multiple Locations, Multiple Locations"`

## Notes

- **Thin adapter** — `FullContent` is always nil in `FetchJobs`. Details are fetched via
  `FetchJobContent` by the job_post_analyzer.
- **ID storage** — The int64 position ID is stored in the internal URL for later extraction
  by the detail fetcher. This allows the position_details API call to use the correct ID.
- **Workplace type** — `workLocationOption` is stored as `WorkplaceType` in the response
  (e.g., "Up to 50% work from home", "Hybrid").
- **Published date** — Unix timestamps (`postedTs`) are converted to RFC 3339 format.
- **HTML stripping** — The `jobDescription` field contains HTML tags which are stripped
  to plain text by the `types.StripHTML()` utility.
- **User-Agent** — Requests include a Chrome user-agent to avoid blocking or redirects.
