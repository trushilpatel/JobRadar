# Lever Postings API

Public, unauthenticated API for fetching job postings from a Lever-hosted careers board.

Base URL:

```
https://api.lever.co/v0/postings/{company}
```

All job detail requests require `?mode=json` — without it Lever returns HTML.

---

## 1. List Jobs

**Endpoint:** `GET /v0/postings/{company}?mode=json`

Returns all public job postings for a company as a JSON array.

### Request

```bash
curl "https://api.lever.co/v0/postings/netlify?mode=json"
```

### Response

```json
[
  {
    "id": "abc-111-def",
    "text": "Backend Engineer",
    "hostedUrl": "https://jobs.lever.co/netlify/abc-111-def",
    "applyUrl": "https://jobs.lever.co/netlify/abc-111-def/apply",
    "country": "US",
    "categories": {
      "location": "Remote",
      "commitment": "Full-time",
      "team": "Engineering",
      "department": "Product & Engineering",
      "allLocations": ["Remote"]
    },
    "workplaceType": "remote"
  }
]
```

### Response Fields

| Field                      | Type     | Description                                 | Captured |
| -------------------------- | -------- | ------------------------------------------- | -------- |
| `id`                       | string   | Unique posting UUID                         | ✅       |
| `text`                     | string   | Job title                                   | ✅       |
| `hostedUrl`                | string   | Hosted job posting page URL                 | ✅       |
| `applyUrl`                 | string   | Direct application form URL                 | ✅ (struct) |
| `country`                  | string / null | ISO 3166-1 alpha-2 country code        | ✅ (struct) |
| `categories.location`      | string   | Primary posting location                    | ✅       |
| `categories.commitment`    | string   | Employment type (e.g. `"Full-time"`)        | ✅ → normalized to `EmploymentType` |
| `categories.team`          | string   | Team name                                   | —        |
| `categories.department`    | string   | Broader department grouping                 | —        |
| `categories.allLocations`  | string[] | All locations including primary             | ✅ (struct) |
| `workplaceType`            | enum     | `unspecified` / `on-site` / `remote` / `hybrid` | ✅ (struct) |

---

## 2. Get Single Job

**Endpoint:** `GET /v0/postings/{company}/{id}?mode=json`

Returns full details for one job posting as a JSON object (not an array).

### Request

```bash
curl "https://api.lever.co/v0/postings/netlify/abc-111-def?mode=json"
```

### Response

```json
{
  "id": "abc-111-def",
  "text": "Backend Engineer",
  "country": "US",
  "hostedUrl": "https://jobs.lever.co/netlify/abc-111-def",
  "applyUrl": "https://jobs.lever.co/netlify/abc-111-def/apply",
  "opening": "<p>We are building...</p>",
  "openingPlain": "We are building...",
  "description": "<p>We are building...<br>Join our team.</p>",
  "descriptionPlain": "We are building...\nJoin our team.",
  "descriptionBody": "<p>Join our team.</p>",
  "descriptionBodyPlain": "Join our team.",
  "lists": [
    {
      "text": "Requirements",
      "content": "<li>Go experience</li><li>5+ years backend</li>"
    },
    {
      "text": "Benefits",
      "content": "<li>Health insurance</li>"
    }
  ],
  "additional": "<p>About Netlify...</p>",
  "additionalPlain": "About Netlify...",
  "salaryRange": {
    "currency": "USD",
    "interval": "per-year-salary",
    "min": 120000,
    "max": 160000
  },
  "salaryDescription": "<p>$120K – $160K depending on experience</p>",
  "salaryDescriptionPlain": "$120K – $160K depending on experience",
  "workplaceType": "remote",
  "categories": {
    "location": "Remote",
    "commitment": "Full-time",
    "team": "Engineering",
    "department": "Product & Engineering",
    "allLocations": ["Remote", "New York"]
  },
  "createdAt": 1685000000000
}
```

### Response Fields

#### Core Fields

| Field                    | Type     | Description                                     | Captured |
| ------------------------ | -------- | ----------------------------------------------- | -------- |
| `id`                     | string   | Unique posting UUID                             | ✅       |
| `text`                   | string   | Job title                                       | ✅       |
| `country`                | string / null | ISO 3166-1 alpha-2 country code            | ✅ (struct) |
| `hostedUrl`              | string   | Hosted job posting page URL                     | ✅       |
| `applyUrl`               | string   | Direct application form URL                     | ✅ (struct) |

#### Description Fields

Lever provides multiple variants. We use `descriptionPlain` as the primary source.

| Field                    | Type   | Description                                        | Captured |
| ------------------------ | ------ | -------------------------------------------------- | -------- |
| `opening`                | string (HTML) | Opening section only                        | ✅ (struct) |
| `openingPlain`           | string | Opening section (plain text)                       | ✅ (struct) |
| `description`            | string (HTML) | Opening + body combined                     | ✅ (struct) |
| `descriptionPlain`       | string | Opening + body combined (plain text) ← **primary** | ✅ → `Description` |
| `descriptionBody`        | string (HTML) | Body only, no opening                       | ✅ (struct) |
| `descriptionBodyPlain`   | string | Body only (plain text)                             | ✅ (struct) |
| `additional`             | string (HTML) | Closing content (e.g. About the company)    | ✅ (struct) |
| `additionalPlain`        | string | Closing content (plain text)               | ✅ → appended to `Description` |

#### `lists[]`

Structured requirement/benefit sections.

| Field     | Type          | Description                               | Captured |
| --------- | ------------- | ----------------------------------------- | -------- |
| `text`    | string        | Section heading (e.g. `"Requirements"`)   | ✅       |
| `content` | string (HTML) | Unstyled HTML `<li>` items                | ✅ → HTML stripped, joined into `Requirements` |

#### Compensation Fields

| Field                    | Type    | Description                              | Captured |
| ------------------------ | ------- | ---------------------------------------- | -------- |
| `salaryRange.currency`   | string  | Currency code (e.g. `"USD"`)             | ✅ (struct) |
| `salaryRange.interval`   | string  | Pay period (e.g. `"per-year-salary"`)    | ✅ (struct) |
| `salaryRange.min`        | number  | Minimum salary                           | ✅ (struct) |
| `salaryRange.max`        | number  | Maximum salary                           | ✅ (struct) |
| `salaryDescription`      | string (HTML) | Salary narrative                   | ✅ (struct) |
| `salaryDescriptionPlain` | string  | Salary narrative (plain text) ← **primary** | ✅ → `Compensation` |

> Compensation priority: `salaryDescriptionPlain` first; falls back to formatted `salaryRange` if plain text is absent.

#### Employment & Location Fields

| Field                      | Type     | Description                                          | Captured |
| -------------------------- | -------- | ---------------------------------------------------- | -------- |
| `workplaceType`            | enum     | `unspecified` / `on-site` / `remote` / `hybrid`      | ✅ → `WorkplaceType` |
| `categories.location`      | string   | Primary posting location                             | ✅ → `Location` |
| `categories.commitment`    | string   | Free-text employment type (e.g. `"Full-time"`)       | ✅ → normalized to `EmploymentType` |
| `categories.team`          | string   | Team name                                            | ✅ → `Department` fallback |
| `categories.department`    | string   | Broader department grouping                          | ✅ → `Department` (preferred) |
| `categories.allLocations`  | string[] | All locations including primary                      | ✅ (struct) |

#### Timestamps

| Field       | Type              | Description                | Captured |
| ----------- | ----------------- | -------------------------- | -------- |
| `createdAt` | integer (Unix ms) | Posting creation timestamp | ✅ → `PublishedDate` (stored as string) |

---

## 3. Commitment → EmploymentType Normalization

| Lever value                              | Normalized constant |
| ---------------------------------------- | ------------------- |
| `Full-time`, `Fulltime`, `Full time`     | `full_time`         |
| `Part-time`, `Parttime`, `Part time`     | `part_time`         |
| `Contract`, `Contractor`                 | `contract`          |
| `Internship`, `Intern`                   | `internship`        |
| anything else                            | `""` (empty)        |

---

## 4. URL Formats

| Usage              | Format                                                          |
| ------------------ | --------------------------------------------------------------- |
| `companies.url`    | `{company-slug}` (e.g. `netlify`)                               |
| `RawJob.URL` (web) | `https://jobs.lever.co/{company}/{uuid}`                        |
| List API           | `https://api.lever.co/v0/postings/{company}?mode=json`          |
| Content API        | `https://api.lever.co/v0/postings/{company}/{uuid}?mode=json`   |

The adapter parses `RawJob.URL` to extract `{company}` and `{uuid}`, then constructs the Content API URL.
