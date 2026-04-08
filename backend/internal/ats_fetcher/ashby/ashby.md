# Ashby API

Ashby exposes two separate APIs for job data:

| API | Auth | Used by |
| --- | ---- | ------- |
| **Public GraphQL** (`jobs.ashbyhq.com`) | None | `FetchJobs`, `FetchJobContent` |
| **Authenticated REST** (`api.ashbyhq.com`) | API key (Basic auth) | `FetchJobsViaAPI`, `FetchJobContentViaAPI` |

---

## Part 1 — Public GraphQL API

Unauthenticated GraphQL endpoint used for public careers pages. This is what the adapter uses by default.

Base URL:

```
https://jobs.ashbyhq.com/api/non-user-graphql
```

---

### 1.1 List Jobs (`ApiJobBoardWithTeams`)

**Operation:** `POST /api/non-user-graphql?op=ApiJobBoardWithTeams`

#### Request

```json
{
  "operationName": "ApiJobBoardWithTeams",
  "variables": {
    "organizationHostedJobsPageName": "openai"
  },
  "query": "query ApiJobBoardWithTeams($organizationHostedJobsPageName: String!) { jobBoardWithTeams(organizationHostedJobsPageName: $organizationHostedJobsPageName) { jobPostings { id title locationName workplaceType employmentType compensationTierSummary } } }"
}
```

#### Response

```json
{
  "data": {
    "jobBoardWithTeams": {
      "jobPostings": [
        {
          "id": "abc-111",
          "title": "Backend Engineer",
          "locationName": "San Francisco",
          "workplaceType": "Hybrid",
          "employmentType": "FullTime",
          "compensationTierSummary": "$180K – $220K"
        }
      ]
    }
  }
}
```

#### Response Fields

| Field                    | Type   | Description                                          | Captured |
| ------------------------ | ------ | ---------------------------------------------------- | -------- |
| `id`                     | string | Posting UUID                                         | ✅       |
| `title`                  | string | Job title                                            | ✅       |
| `locationName`           | string | Candidate-facing location name                       | ✅ → `Location` (with workplaceType appended) |
| `workplaceType`          | string | `Remote` / `Hybrid` / `Onsite`                       | ✅ → appended to `Location` display |
| `employmentType`         | string | `FullTime` / `PartTime` / `Contract` / `Intern`      | ✅ → normalized to `EmploymentType` |
| `compensationTierSummary`| string | Human-readable pay summary (e.g. `"$288K – $425K"`) | —        |

---

### 1.2 Get Single Job (`ApiJobPosting`)

**Operation:** `POST /api/non-user-graphql?op=ApiJobPosting`

#### Request

```json
{
  "operationName": "ApiJobPosting",
  "variables": {
    "organizationHostedJobsPageName": "openai",
    "jobPostingId": "abc-111"
  },
  "query": "query ApiJobPosting($organizationHostedJobsPageName: String!, $jobPostingId: String!) { jobPosting(organizationHostedJobsPageName: $organizationHostedJobsPageName jobPostingId: $jobPostingId) { id title locationName workplaceType employmentType descriptionHtml compensationTierSummary publishedDate } }"
}
```

#### Response

```json
{
  "data": {
    "jobPosting": {
      "id": "abc-111",
      "title": "Backend Engineer",
      "locationName": "San Francisco",
      "workplaceType": "Hybrid",
      "employmentType": "FullTime",
      "descriptionHtml": "<p>Join us.</p>",
      "compensationTierSummary": "$180K – $220K",
      "publishedDate": "2026-01-15"
    }
  }
}
```

#### Response Fields

| Field                    | Type          | Description                                            | Captured |
| ------------------------ | ------------- | ------------------------------------------------------ | -------- |
| `id`                     | string        | Posting UUID                                           | ✅       |
| `title`                  | string        | Job title                                              | ✅       |
| `locationName`           | string        | Candidate-facing location name                         | ✅ → `Location` (with workplaceType appended) |
| `workplaceType`          | string        | `Remote` / `Hybrid` / `Onsite`                         | ✅ → `WorkplaceType` |
| `employmentType`         | string        | `FullTime` / `PartTime` / `Contract` / `Intern`        | ✅ → normalized to `EmploymentType` |
| `descriptionHtml`        | string (HTML) | Full job description                                   | ✅ → HTML stripped to plain text → `Description` |
| `compensationTierSummary`| string        | Human-readable pay summary                             | ✅ → `Compensation` |
| `publishedDate`          | string (date) | Date the posting was published (e.g. `"2026-01-15"`)  | ✅ → `PublishedDate` |

---

## Part 2 — Authenticated REST API

Used when an Ashby API key is available. Provides richer internal job data but does **not** include `descriptionHtml` — use the public GraphQL API when you need job content.

Base URL:

```
https://api.ashbyhq.com
```

Authentication: HTTP Basic auth with API key as username and empty password.

```bash
-u "${API_KEY}:"
```

---

### 2.1 List Jobs (`job.list`)

**Endpoint:** `POST /job.list`

Supports full sync, incremental sync (via `syncToken`), and pagination (via `cursor`).

#### Request

```json
{
  "limit": 100,
  "status": ["Open"],
  "expand": ["location", "compensation"]
}
```

#### Response

```json
{
  "success": true,
  "moreDataAvailable": true,
  "nextCursor": "Rl",
  "results": [
    {
      "id": "123e4567-e89b-12d3-a456-426614174000",
      "title": "Software Engineer",
      "status": "Open",
      "employmentType": "FullTime",
      "locationId": "223e4567-...",
      "departmentId": "323e4567-...",
      "createdAt": "2026-04-01T10:00:00Z",
      "updatedAt": "2026-04-07T12:00:00Z",
      "openedAt": "2026-04-02T09:00:00Z",
      "closedAt": null,
      "location": {
        "id": "223e4567-...",
        "name": "San Francisco",
        "externalName": "San Francisco, CA",
        "workplaceType": "Hybrid"
      },
      "compensation": {
        "compensationTiers": [
          {
            "tierSummary": "$120,000 – $160,000 per year"
          }
        ]
      }
    }
  ]
}
```

#### Response Fields

##### Core `results[]` Fields

| Field                  | Type   | Description                                   | Captured |
| ---------------------- | ------ | --------------------------------------------- | -------- |
| `id`                   | string | Internal job UUID                             | ✅       |
| `title`                | string | Job title                                     | ✅       |
| `confidential`         | bool   | Whether job is confidential                   | ✅ (struct) |
| `status`               | string | `Open` / `Closed` / `Archived` / `Draft`      | ✅ (struct) |
| `employmentType`       | string | `FullTime` / `PartTime` / `Contract` / `Intern` | ✅ → normalized to `EmploymentType` |
| `locationId`           | string | Location UUID                                 | ✅ (struct) |
| `departmentId`         | string | Department UUID                               | ✅ (struct) |
| `jobPostingIds`        | string[] | Associated public posting UUIDs             | ✅ (struct) |
| `customRequisitionId`  | string | Custom requisition ID                         | ✅ (struct) |
| `createdAt`            | string (ISO 8601) | Job creation timestamp            | ✅ (struct) |
| `updatedAt`            | string (ISO 8601) | Last updated timestamp            | ✅ (struct) |
| `openedAt`             | string (ISO 8601) | When job was opened               | ✅ → `PublishedDate` |
| `closedAt`             | string / null | When job was closed                   | ✅ (struct) |
| `hiringTeam`           | object[] | Hiring team members                       | ✅ (struct) |
| `author`               | object | Job author details                            | ✅ (struct) |
| `customFields`         | object[] | Custom fields                             | ✅ (struct) |

##### Expanded `location` Object (requires `"expand": ["location"]`)

| Field           | Type   | Description                              | Captured |
| --------------- | ------ | ---------------------------------------- | -------- |
| `id`            | string | Location UUID                            | ✅ (struct) |
| `name`          | string | Internal location name                   | ✅ (struct) |
| `externalName`  | string | Candidate-facing location name ← **primary** | ✅ → `Location` |
| `workplaceType` | string | `Remote` / `Hybrid` / `OnSite`           | ✅ → `WorkplaceType` |

##### Expanded `compensation` Object (requires `"expand": ["compensation"]`)

| Field                              | Type   | Description                          | Captured |
| ---------------------------------- | ------ | ------------------------------------ | -------- |
| `compensationTiers[].tierSummary`  | string | Human-readable pay summary ← **primary** | ✅ → `Compensation` (first tier) |
| `compensationTiers[].title`        | string | Tier label                           | ✅ (struct) |
| `compensationTiers[].components[]` | object[] | Individual compensation components | ✅ (struct) |

##### Pagination / Sync Fields

| Field               | Type   | Description                                      |
| ------------------- | ------ | ------------------------------------------------ |
| `moreDataAvailable` | bool   | Whether more pages exist                         |
| `nextCursor`        | string | Pass as `cursor` in next request                 |
| `syncToken`         | string | Returned on final page; use for incremental sync |

---

### 2.2 Get Single Job (`job.info`)

**Endpoint:** `POST /job.info`

Returns details for one job by ID. Same shape as a single `results[]` item from `job.list`.

#### Request

```json
{
  "id": "123e4567-e89b-12d3-a456-426614174000",
  "expand": ["location", "compensation"]
}
```

#### Response

```json
{
  "success": true,
  "results": {
    "id": "123e4567-...",
    "title": "Software Engineer",
    "employmentType": "FullTime",
    "openedAt": "2026-04-02T09:00:00Z",
    "location": {
      "name": "San Francisco",
      "externalName": "San Francisco, CA",
      "workplaceType": "Hybrid"
    },
    "compensation": {
      "compensationTiers": [
        {
          "tierSummary": "$120,000 – $160,000 per year"
        }
      ]
    }
  }
}
```

Same field coverage as `job.list` results above.

---

## Part 3 — EmploymentType Normalization

| Ashby value           | Normalized constant |
| --------------------- | ------------------- |
| `FullTime`            | `full_time`         |
| `PartTime`            | `part_time`         |
| `Contract`, `Contractor` | `contract`       |
| `Intern`, `Internship` | `internship`       |
| anything else         | `""` (empty)        |

> Note: Matching is case-sensitive. `"FULLTIME"` will not match.

---

## Part 4 — URL Formats

| Usage              | Format                                                                    |
| ------------------ | ------------------------------------------------------------------------- |
| `companies.url`    | `{company-slug}` (e.g. `openai`) or full API URL                         |
| `RawJob.URL` (web) | `https://jobs.ashbyhq.com/{company}/{uuid}`                               |
| GraphQL list       | `POST https://jobs.ashbyhq.com/api/non-user-graphql?op=ApiJobBoardWithTeams` |
| GraphQL detail     | `POST https://jobs.ashbyhq.com/api/non-user-graphql?op=ApiJobPosting`    |
| REST list          | `POST https://api.ashbyhq.com/job.list`                                   |
| REST detail        | `POST https://api.ashbyhq.com/job.info`                                   |

The adapter parses `RawJob.URL` to extract `{company}` and `{uuid}` for GraphQL variable injection.
