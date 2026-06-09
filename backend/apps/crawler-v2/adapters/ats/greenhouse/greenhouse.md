# Greenhouse Job Board API

Public, unauthenticated API for fetching job postings from a Greenhouse-hosted careers board.

Base URL:

```
https://boards-api.greenhouse.io/v1/boards/{board_token}
```

---

## 1. List Jobs

**Endpoint:** `GET /v1/boards/{board_token}/jobs`

Returns all public job postings for a board. Optional `?content=true` query param includes `content`, `departments`, and `offices` inline (we do not use this — we fetch content individually via endpoint 2).

### Request

```bash
curl "https://boards-api.greenhouse.io/v1/boards/stripe/jobs"
```

### Response

```json
{
  "jobs": [
    {
      "id": 127817,
      "internal_job_id": 144381,
      "title": "Vault Designer",
      "updated_at": "2016-01-14T10:55:28-05:00",
      "requisition_id": "50",
      "location": {
        "name": "NYC"
      },
      "absolute_url": "https://boards.greenhouse.io/vaulttec/jobs/127817",
      "language": "en",
      "metadata": null
    }
  ],
  "meta": {
    "total": 1
  }
}
```

### Response Fields

#### `jobs[]`

| Field             | Type              | Description                | Captured |
| ----------------- | ----------------- | -------------------------- | -------- |
| `id`              | integer           | Public Greenhouse job ID   | ✅       |
| `internal_job_id` | integer           | Internal Greenhouse job ID | —        |
| `title`           | string            | Job title                  | ✅       |
| `updated_at`      | string (ISO 8601) | Last updated timestamp     | —        |
| `requisition_id`  | string            | Requisition identifier     | —        |
| `location.name`   | string            | Candidate-facing location  | ✅       |
| `absolute_url`    | string            | Public hosted job URL      | ✅       |
| `language`        | string            | Language code              | —        |
| `metadata`        | array or null     | Custom metadata fields     | —        |

#### `meta`

| Field   | Type    | Description             |
| ------- | ------- | ----------------------- |
| `total` | integer | Total jobs in the board |

---

## 2. Get Single Job

**Endpoint:** `GET /v1/boards/{board_token}/jobs/{id}`

Returns full details for one job posting including description HTML, application questions, metadata, data compliance info, and pay ranges.

### Request

```bash
curl "https://boards-api.greenhouse.io/v1/boards/stripe/jobs/44444"
```

### Response

```json
{
  "id": 44444,
  "title": "Product Engineer",
  "updated_at": "2013-07-02T19:39:23Z",
  "requisition_id": "50",
  "location": {
    "name": "San Francisco, CA"
  },
  "content": "<p>This is the job description.</p>",
  "absolute_url": "https://boards.greenhouse.io/stripe/jobs/44444",
  "language": "en",
  "internal_job_id": 55555,
  "departments": [
    {
      "id": 1,
      "name": "Engineering",
      "parent_id": null,
      "child_ids": []
    }
  ],
  "offices": [
    {
      "id": 1,
      "name": "San Francisco",
      "location": "San Francisco, CA",
      "parent_id": null,
      "child_ids": []
    }
  ],
  "location_questions": [...],
  "questions": [...],
  "metadata": [
    {
      "id": 12345,
      "name": "Field Name",
      "value_type": "text",
      "value": "Some value"
    }
  ],
  "data_compliance": [
    {
      "type": "gdpr",
      "requires_consent": true,
      "requires_processing_consent": true,
      "requires_retention_consent": true,
      "retention_period": 12345
    }
  ],
  "pay_input_ranges": [
    {
      "min_cents": 5000000,
      "max_cents": 7500000,
      "currency_type": "USD",
      "title": "NYC Salary Range",
      "blurb": "In order to provide transparency..."
    }
  ]
}
```

### Response Fields

#### Core Fields

| Field             | Type              | Description                     | Captured                    |
| ----------------- | ----------------- | ------------------------------- | --------------------------- |
| `id`              | integer           | Public job post ID              | ✅                          |
| `title`           | string            | Job title                       | ✅                          |
| `updated_at`      | string (ISO 8601) | Last updated timestamp          | ✅ → `PublishedDate`        |
| `requisition_id`  | string            | Requisition ID                  | —                           |
| `location.name`   | string            | Job location                    | ✅                          |
| `content`         | string (HTML)     | Full job description            | ✅ → stripped to plain text |
| `absolute_url`    | string            | Public application/job page URL | ✅                          |
| `language`        | string            | Language code                   | —                           |
| `internal_job_id` | integer           | Internal Greenhouse job ID      | —                           |

#### `departments[]`

| Field       | Type           | Description          | Captured                       |
| ----------- | -------------- | -------------------- | ------------------------------ |
| `id`        | integer        | Department ID        | —                              |
| `name`      | string         | Department name      | ✅ → `Department` (first item) |
| `parent_id` | integer / null | Parent department ID | —                              |
| `child_ids` | integer[]      | Child department IDs | —                              |

#### `offices[]`

| Field       | Type           | Description      | Captured |
| ----------- | -------------- | ---------------- | -------- |
| `id`        | integer        | Office ID        | —        |
| `name`      | string         | Office name      | —        |
| `location`  | string         | Office location  | —        |
| `parent_id` | integer / null | Parent office ID | —        |
| `child_ids` | integer[]      | Child office IDs | —        |

#### `pay_input_ranges[]`

| Field           | Type    | Description                   | Captured                           |
| --------------- | ------- | ----------------------------- | ---------------------------------- |
| `min_cents`     | integer | Minimum compensation in cents | ✅ → formatted into `Compensation` |
| `max_cents`     | integer | Maximum compensation in cents | ✅ → formatted into `Compensation` |
| `currency_type` | string  | Currency code (e.g. `"USD"`)  | ✅ → appended to `Compensation`    |
| `title`         | string  | Range label                   | —                                  |
| `blurb`         | string  | Explanatory text              | —                                  |

> Compensation format: `"$50000 – $75000 USD"` (cents ÷ 100)

#### `questions[]` / `location_questions[]`

Application form fields — not captured. These are candidate-facing input definitions, not job content.

#### `metadata[]`

| Field        | Type    | Description         | Captured |
| ------------ | ------- | ------------------- | -------- |
| `id`         | integer | Metadata field ID   | —        |
| `name`       | string  | Metadata field name | —        |
| `value_type` | string  | Value type          | —        |
| `value`      | string  | Value               | —        |

#### `data_compliance[]`

GDPR/compliance fields — not captured.

---

## 3. URL Formats

| Usage              | Format                                                        |
| ------------------ | ------------------------------------------------------------- |
| `companies.url`    | `https://boards-api.greenhouse.io/v1/boards/{slug}/jobs`      |
| `RawJob.URL` (web) | `https://job-boards.greenhouse.io/{slug}/jobs/{id}`           |
| Content API        | `https://boards-api.greenhouse.io/v1/boards/{slug}/jobs/{id}` |

The adapter parses `RawJob.URL` to extract `{slug}` and `{id}`, then constructs the Content API URL.
