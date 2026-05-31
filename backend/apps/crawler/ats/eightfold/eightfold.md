# Eightfold AI API

Eightfold AI powers careers boards for many large enterprises (Netflix, Bloomberg, HSBC, Activision, Capital One, Vodafone, and others). Each customer is served from their own vanity host with their own domain query parameter — neither can be derived from a company slug alone.

This is a **ConfigurableAdapter**: the engine calls `FetchJobsWithConfig` instead of `FetchJobs`, passing the per-company `adapter_config` JSON from `company_scan_settings`.

Base URL pattern:

```
https://{host}/api/apply/v2
```

Where `{host}` is the customer-specific host (e.g. `explore.jobs.netflix.net`, `careers.activision.com`).

---

## Per-company `adapter_config` schema

Stored in `company_scan_settings.adapter_config` as a JSON blob:

```json
{
  "host": "explore.jobs.netflix.net",
  "domain": "netflix.com",
  "max_jobs_per_scan": 5000,
  "locale": "en"
}
```

| Field               | Type    | Required | Description                                                           |
| ------------------- | ------- | -------- | --------------------------------------------------------------------- |
| `host`              | string  | ✅       | FQDN serving the Eightfold API for this customer (no scheme or slash) |
| `domain`            | string  | ✅       | Value for the `?domain=` query param (e.g. `"netflix.com"`)           |
| `max_jobs_per_scan` | integer | —        | Cap on total listings; defaults to 5000 when 0 or absent              |
| `locale`            | string  | —        | `?hl=` locale pin; defaults to `"en"`                                 |

---

## 1. List Jobs

**Endpoint:** `GET /api/apply/v2/jobs?domain={domain}&start={start}&num={page_size}&hl={locale}`

Paginated listing endpoint. Page size is fixed at 10 (Eightfold's documented max; larger values are silently capped server-side). Pagination uses `start` offset. The adapter stops when fewer than 10 results are returned or when `max_jobs_per_scan` is reached.

### Request

```bash
curl -H "Accept: application/json" \
  "https://explore.jobs.netflix.net/api/apply/v2/jobs?domain=netflix.com&start=0&num=10&hl=en"
```

### Response

```json
{
  "positions": [
    {
      "id": 921234567,
      "name": "Software Engineer (L5)",
      "posting_name": "Software Engineer L5 - Backend",
      "location": "Los Angeles, CA",
      "locations": ["Los Angeles, CA", "Remote, USA"],
      "department": "Engineering",
      "business_unit": "Product",
      "t_create": 1700000000,
      "t_update": 1710000000,
      "display_job_id": "JR123",
      "work_location_option": "hybrid",
      "location_flexibility": "hybrid",
      "canonicalPositionUrl": "https://explore.jobs.netflix.net/jobs/921234567?domain=netflix.com",
      "job_description": "<p>Full HTML description...</p>",
      "type": "full_time"
    }
  ],
  "count": 342
}
```

### Response Fields

#### `positions[]`

| Field                  | Type     | Description                                               | Captured                                           |
| ---------------------- | -------- | --------------------------------------------------------- | -------------------------------------------------- |
| `id`                   | int64    | Eightfold position ID                                     | ✅ → `eightfold-{id}`                              |
| `name`                 | string   | Internal job name                                         | ✅ → `Title` (preferred)                           |
| `posting_name`         | string   | Public posting name (fallback for `name`)                 | ✅ → `Title` (fallback when `name` empty)          |
| `location`             | string   | Primary location string                                   | ✅ → `Location`                                    |
| `locations`            | string[] | All location strings for the posting                      | ✅ → `AllLocations`                                |
| `department`           | string   | Department name                                           | ✅ → used in `FetchJobContent` response            |
| `business_unit`        | string   | Business unit                                             | —                                                  |
| `t_create`             | int64    | Creation Unix timestamp (ms)                              | —                                                  |
| `t_update`             | int64    | Last-updated Unix timestamp (ms)                          | ✅ → `PublishedDate` (in `FetchJobContent`)        |
| `display_job_id`       | string   | Human-readable requisition ID                             | —                                                  |
| `work_location_option` | string   | `remote` / `hybrid` / `onsite` / etc.                     | ✅ → `WorkplaceType`                               |
| `location_flexibility` | string   | Alternate workplace type field (same values)              | ✅ → `WorkplaceType` (fallback)                    |
| `canonicalPositionUrl` | string   | Canonical public URL for the job                          | ✅ → `RawJob.URL`                                  |
| `job_description`      | string   | Full HTML description (list API only; also in detail)     | ✅ → `Description` in `FetchJobContent` (stripped) |
| `type`                 | string   | Employment type (`full_time`, `part_time`, `contract`, …) | ✅ → `EmploymentType`                              |

#### Top-level

| Field   | Type    | Description              | Captured                                 |
| ------- | ------- | ------------------------ | ---------------------------------------- |
| `count` | integer | Total positions matching | — (used only for pagination termination) |

---

## 2. Get Single Job (FetchJobContent)

**Endpoint:** `GET /api/apply/v2/jobs/{id}?domain={domain}`

Returns the same fields as the listing `position` object. The adapter reconstructs the URL from `RawJob.URL`, extracting `host`, `id`, and `domain` (derived from the URL's query string or from the host's apex domain).

### Request

```bash
curl -H "Accept: application/json" \
  "https://explore.jobs.netflix.net/api/apply/v2/jobs/921234567?domain=netflix.com"
```

### Response

Flat object matching the `position` schema above. The response may include additional detail-only keys (`custom_JD`, `preApplyInfoBanner`, etc.) that are ignored.

### Fields captured by `FetchJobContent`

| `FetchJobContentResponse` field | Source field            | Notes                                        |
| ------------------------------- | ----------------------- | -------------------------------------------- |
| `ID`                            | `id`                    | `eightfold-{id}`                             |
| `Title`                         | `name` / `posting_name` | `name` preferred; `posting_name` as fallback |
| `Location`                      | `location`              |                                              |
| `URL`                           | `canonicalPositionUrl`  |                                              |
| `Description`                   | `job_description`       | HTML stripped to plain text                  |
| `WorkplaceType`                 | `work_location_option`  | Falls back to `location_flexibility`         |
| `Department`                    | `department`            |                                              |
| `EmploymentType`                | `type`                  | Normalized to internal constants             |
| `PublishedDate`                 | `t_update`              | Unix timestamp → RFC 3339 string             |
| `Compensation`                  | —                       | Not available from the Eightfold public API  |
| `Requirements`                  | —                       | Not available as a separate field            |
| `CountryCode`                   | —                       | Not populated by this adapter                |

---

## 3. URL Formats

| Usage           | Format                                                                 |
| --------------- | ---------------------------------------------------------------------- |
| `companies.url` | Typically the host slug or a fallback — prefer `companies.careers_url` |
| `RawJob.URL`    | `https://{host}/jobs/{id}?domain={domain}` (canonical position URL)    |
| Detail API      | `https://{host}/api/apply/v2/jobs/{id}?domain={domain}`                |

---

## 4. Adapter notes

- **ConfigurableAdapter**: `FetchJobs` always errors. The engine calls `FetchJobsWithConfig` when `adapter_config` is present.
- **Two-phase**: `FetchJobsWithConfig` emits thin `FetchJobsItem` rows (no `FullContent`); the analyzer calls `FetchJobContent` for each posting to retrieve the description.
- The listing endpoint returns an empty result (HTTP 404) for unknown or retired boards — the adapter treats this as a zero-job scan rather than an error.
- Inter-page delay: 150 ms between paginated listing requests (configurable in tests).
- CloudFront blocks the default Go user agent — a real browser UA is pinned in every request.
