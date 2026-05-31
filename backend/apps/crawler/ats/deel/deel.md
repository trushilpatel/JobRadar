# Deel Careers Adapter

Deel's hosted careers product (`jobs.deel.com`) renders every job board with Next.js Server Components. There is no public JSON REST API — job data is embedded in the React Server Components (RSC) stream.

The adapter requests the RSC stream by setting the `RSC: 1` header, then extracts job records using balanced-brace parsing of the numbered "T" (text) chunks.

Base URL:

```
https://jobs.deel.com
```

---

## 1. Fetch Job Listings

**Method:** `GET /{slug}` with `RSC: 1` and `Accept: text/x-component` headers

Returns the raw RSC stream for the company's job board page. Job records are embedded as JSON objects within numbered "T" chunks in the stream.

### Request

```bash
curl -H "RSC: 1" -H "Accept: text/x-component" "https://jobs.deel.com/klarna"
```

### RSC stream format

The response is a newline-delimited stream of chunks, each prefixed with a hex ID and length:

```
0:{"id":"...","chunks":...}
1:T<len>,<html or text payload>
2:["$","div",...]
...
```

Job posting records appear as JSON objects embedded in the T-chunks. Each job record has the format:

```json
{
  "id": "abc12345-...",
  "jobId": "def67890-...",
  "title": "Software Engineer",
  "richtextDescription": "$1f",
  "job": {
    "jobEmploymentTypes": [{ "employmentType": { "name": "Full-time" } }],
    "jobLocations": [{ "location": { "name": "Stockholm, Sweden" } }],
    "jobDepartments": [{ "department": { "name": "Engineering" } }],
    "jobTeams": [{ "team": { "name": "Payments" } }],
    "currentCompensation": {
      "currencyIsoCode": "USD",
      "minAmount": 80000,
      "maxAmount": 120000
    }
  },
  "jobPostingPublications": [{ "currentState": { "createdAt": "2025-03-01T10:00:00Z" } }]
}
```

### RSC Record Fields

#### Top-level (`rawJobPosting`)

| Field                    | Type             | Description                                             | Captured                                                                       |
| ------------------------ | ---------------- | ------------------------------------------------------- | ------------------------------------------------------------------------------ |
| `id`                     | string (UUID)    | Job posting ID (used in public URL)                     | ✅ → `deel-{id}`                                                               |
| `jobId`                  | string (UUID)    | Internal Deel job ID                                    | —                                                                              |
| `title`                  | string           | Job title                                               | ✅                                                                             |
| `richtextDescription`    | string / RSC ref | HTML description or `"$<hexId>"` reference to a T-chunk | ✅ → dereferenced from chunk map, then `StripHTML` → `FullContent.Description` |
| `job`                    | object           | Nested job data (see below)                             | ✅ (nested fields)                                                             |
| `jobPostingPublications` | array            | Publication state records                               | ✅ → first entry's `createdAt`                                                 |

#### `job` object (`rawJob`)

| Field                 | Type          | Description                  | Captured                                        |
| --------------------- | ------------- | ---------------------------- | ----------------------------------------------- |
| `jobEmploymentTypes`  | array         | Employment types (see below) | ✅ → `EmploymentType` (first entry, normalized) |
| `jobLocations`        | array         | Locations (see below)        | ✅ → `Location` (all joined with `; `)          |
| `jobDepartments`      | array         | Departments (see below)      | ✅ → `FullContent.Department` (first entry)     |
| `jobTeams`            | array         | Teams within departments     | —                                               |
| `currentCompensation` | object / null | Pay range (see below)        | ✅ → `FullContent.Compensation`                 |

#### `jobEmploymentTypes[]`

| Field                 | Type   | Description     | Captured                                           |
| --------------------- | ------ | --------------- | -------------------------------------------------- |
| `employmentType.name` | string | Employment type | ✅ (first entry, normalized to internal constants) |

Employment type normalization: `"Full-time"` → `full_time`, `"Part-time"` → `part_time`, `"Contract"` / `"Consultant"` → `contract`, `"Internship"` → `internship`.

#### `jobLocations[]`

| Field           | Type   | Description   | Captured                          |
| --------------- | ------ | ------------- | --------------------------------- |
| `location.name` | string | Location name | ✅ (all entries joined with `; `) |

#### `jobDepartments[]`

| Field             | Type   | Description     | Captured                                         |
| ----------------- | ------ | --------------- | ------------------------------------------------ |
| `department.name` | string | Department name | ✅ → `FullContent.Department` (first entry only) |

#### `currentCompensation`

| Field             | Type    | Description          | Captured                                 |
| ----------------- | ------- | -------------------- | ---------------------------------------- |
| `currencyIsoCode` | string  | ISO currency code    | ✅ → appended to compensation string     |
| `minAmount`       | float64 | Minimum compensation | ✅ → formatted as `"min – max CURRENCY"` |
| `maxAmount`       | float64 | Maximum compensation | ✅ → formatted as `"min – max CURRENCY"` |

#### `jobPostingPublications[]`

| Field                    | Type   | Description                    | Captured                                       |
| ------------------------ | ------ | ------------------------------ | ---------------------------------------------- |
| `currentState.createdAt` | string | Publication creation timestamp | ✅ → `FullContent.PublishedDate` (first entry) |

---

## 2. URL Formats

| Usage              | Format                                          |
| ------------------ | ----------------------------------------------- |
| `companies.url`    | `klarna` (board slug only — no URL prefix)      |
| `RawJob.URL` (web) | `https://jobs.deel.com/{slug}/job-details/{id}` |
| Careers page       | `https://jobs.deel.com/{slug}`                  |

---

## 3. Adapter notes

- **Full-detail adapter**: `FullContent` is always populated at scan time; the analyzer skips the per-job HTTP fetch.
- No `ContentFetcher` implemented — job detail data is fully captured from the listing RSC stream.
- `richtextDescription` is usually an RSC chunk reference (`"$<hexId>"`); the adapter dereferences it against the chunk map and strips HTML. Legacy records may carry the HTML inline.
- `WorkplaceType` is not present in the Deel RSC structure; the field is left empty.
- The `RSC: 1` request header is mandatory — without it, Next.js returns the full HTML wrapper document instead of the data stream.
