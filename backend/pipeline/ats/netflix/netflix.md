# Netflix — Eightfold AI Adapter

Netflix uses [Eightfold AI](https://eightfold.ai) as their ATS platform, hosted at `explore.jobs.netflix.net`.

## API endpoints

### List positions

```
GET https://explore.jobs.netflix.net/api/apply/v2/jobs?domain=netflix.com&hl=en&start={start}&num={num}
```

**Parameters:**

| Param    | Type   | Description                          |
|----------|--------|--------------------------------------|
| `domain` | string | Company domain — `netflix.com`       |
| `hl`     | string | Language code — `en`                 |
| `start`  | int    | Pagination offset (0-indexed)        |
| `num`    | int    | Page size (default 10)               |

**Response:**

```json
{
  "positions": [
    {
      "id": 790315382430,
      "name": "Senior Software Engineer",
      "posting_name": "Senior Software Engineer",
      "location": "Los Angeles,California,United States of America",
      "locations": ["Los Angeles,California,United States of America"],
      "hot": 0,
      "department": "Engineering",
      "business_unit": "Streaming",
      "t_update": 1775779200,
      "t_create": 1775779200,
      "ats_job_id": "JR12345",
      "display_job_id": "JR12345",
      "type": "ATS",
      "id_locale": "JR12345-en-US",
      "job_description": "",
      "locale": "en-US",
      "stars": 0,
      "medallionProgram": null,
      "location_flexibility": null,
      "work_location_option": "onsite",
      "canonicalPositionUrl": "https://explore.jobs.netflix.net/careers/job/790315382430",
      "isPrivate": false
    }
  ],
  "count": 623
}
```

**Notes:**
- `id` is an **integer** (`int64`), not a string.
- `t_create` and `t_update` are **Unix timestamps** (seconds), not ISO date strings.
- `job_description` is empty in the list response — only populated in the detail response.
- `medallionProgram` and `location_flexibility` can be `null`.
- Total count of all positions is in `count` at the top level.

### Get position detail

```
GET https://explore.jobs.netflix.net/api/apply/v2/jobs/{id}?domain=netflix.com
```

**Response:**

The detail response is **flat** — the position fields are returned directly at the top level (no `{"position": {...}}` wrapper). Includes all list fields plus detail-only fields:

```json
{
  "id": 790315382430,
  "name": "Senior Software Engineer",
  "posting_name": "Senior Software Engineer",
  "location": "Los Angeles,California,United States of America",
  "locations": ["Los Angeles,California,United States of America"],
  "hot": 0,
  "department": "Engineering",
  "business_unit": "Streaming",
  "t_update": 1775779200,
  "t_create": 1775779200,
  "ats_job_id": "JR12345",
  "display_job_id": "JR12345",
  "type": "ATS",
  "id_locale": "JR12345-en-US",
  "locale": "en-US",
  "job_description": "<p>HTML job description...</p>",
  "stars": 0,
  "medallionProgram": null,
  "location_flexibility": null,
  "work_location_option": "remote",
  "canonicalPositionUrl": "https://explore.jobs.netflix.net/careers/job/790315382430?microsite=netflix.com",
  "isPrivate": false,
  "preApplyInfoBanner": null,
  "custom_JD": null,
  "hideUploadResumeOption": false,
  "urls": {
    "links": {
      "blogs": [],
      "videos": []
    }
  }
}
```

**Notes:**
- `job_description` contains the full HTML job description — stripped to plain text by the adapter.
- `canonicalPositionUrl` in the detail response may include `?microsite=netflix.com` query param.
- The `work_location_option` in the detail can be `null` even when the list response has a value.

## Field capture

### position (list response)

| Eightfold field        | Go field            | Type            | Captured |
|------------------------|---------------------|-----------------|----------|
| `id`                   | `ID`                | `int64`         | ✅       |
| `name`                 | `Name`              | `string`        | ✅       |
| `posting_name`         | `PostingName`       | `string`        | ✅       |
| `location`             | `Location`          | `string`        | ✅       |
| `locations`            | `Locations`         | `[]string`      | ✅       |
| `hot`                  | `Hot`               | `int`           | ✅       |
| `department`           | `Department`        | `string`        | ✅       |
| `business_unit`        | `BusinessUnit`      | `string`        | ✅       |
| `t_update`             | `TUpdate`           | `int64`         | ✅       |
| `t_create`             | `TCreate`           | `int64`         | ✅       |
| `ats_job_id`           | `AtsJobID`          | `string`        | ✅       |
| `display_job_id`       | `DisplayJobID`      | `string`        | ✅       |
| `type`                 | `Type`              | `string`        | ✅       |
| `id_locale`            | `IDLocale`          | `string`        | ✅       |
| `locale`               | `Locale`            | `string`        | ✅       |
| `job_description`      | `JobDescription`    | `string`        | ✅       |
| `stars`                | `Stars`             | `float64`       | ✅       |
| `location_flexibility` | `LocationFlexibility` | `string`      | ✅       |
| `work_location_option` | `WorkLocationOption` | `string`       | ✅       |
| `canonicalPositionUrl` | `CanonicalURL`      | `string`        | ✅       |
| `isPrivate`            | `IsPrivate`         | `bool`          | ✅       |
| `medallionProgram`     | `MedallionProgram`  | `json.RawMessage` | ✅     |

### positionDetail (detail response — additional fields)

All `position` fields, plus:

| Eightfold field           | Go field                 | Type              | Captured |
|---------------------------|--------------------------|-------------------|----------|
| `preApplyInfoBanner`      | `PreApplyInfoBanner`     | `json.RawMessage` | ✅       |
| `custom_JD`               | `CustomJD`               | `json.RawMessage` | ✅       |
| `hideUploadResumeOption`  | `HideUploadResumeOption` | `bool`            | ✅       |
| `urls`                    | `URLs`                   | `json.RawMessage` | ✅       |

## Field mapping

### FetchJobs (listing → FetchJobsItem)

| Eightfold field        | FetchJobsItem field | Notes                                       |
|------------------------|---------------------|---------------------------------------------|
| `id`                   | `ID`                | Prefixed: `netflix-{id}` (int64 → string)   |
| `name`                 | `Title`             |                                             |
| `location`             | `Location`          | Falls back to `locations[]` joined with `;` |
| `canonicalPositionUrl` | `URL`               | Falls back to constructed URL               |
| `work_location_option` | `EmploymentType`    | Mapped to `full_time` (default)             |

### FetchJobContent (detail → FetchJobContentResponse)

| Eightfold field        | Response field    | Notes                                          |
|------------------------|-------------------|------------------------------------------------|
| `id`                   | `ID`              | Prefixed: `netflix-{id}` (int64 → string)      |
| `name`                 | `Title`           |                                                |
| `location`             | `Location`        | Falls back to `locations[]`                    |
| `canonicalPositionUrl` | `URL`             |                                                |
| `job_description`      | `Description`     | HTML stripped to plain text                    |
| `department`           | `Department`      |                                                |
| `work_location_option` | `WorkplaceType`   | Raw value: remote/onsite/hybrid (can be null)  |
| `t_create`             | `PublishedDate`   | Unix timestamp → RFC 3339; empty if zero       |

## ID format

```
netflix-790315382430
```

The prefix matches `model.AdapterNetflix` in `model/constants.go`. The numeric portion is the Eightfold `id` field formatted as a decimal string.

## URL format

| Type         | Pattern                                                              |
|--------------|----------------------------------------------------------------------|
| Careers page | `https://explore.jobs.netflix.net/careers`                           |
| Job page     | `https://explore.jobs.netflix.net/careers/job/{id}`                  |
| List API     | `https://explore.jobs.netflix.net/api/apply/v2/jobs?domain=...`      |
| Detail API   | `https://explore.jobs.netflix.net/api/apply/v2/jobs/{id}?domain=...` |

## Notes

- Netflix is a **thin adapter** — `FullContent` is always nil at listing time.
  The `job_post_analyzer` calls `FetchJobContent` for jobs that pass the relevance filter.
- The Eightfold API does not expose employment type (full-time vs contract) in listings.
  All Netflix roles default to `full_time`.
- Compensation data is not available through the Eightfold public API.
- Pagination uses `start` + `num` parameters. The adapter fetches pages until fewer
  results than `pageSize` are returned.
- The detail endpoint response is **flat** (no `{"position": {...}}` wrapper) — unlike
  some Eightfold integrations that wrap the response.
- Timestamps (`t_create`, `t_update`) are Unix seconds integers, converted to RFC 3339
  strings by `unixToRFC3339()`. A zero value produces an empty string.
