# Workday CXS API

Public, unauthenticated REST endpoints powering every Workday-hosted careers
board at `<tenant>.wd<N>.myworkdayjobs.com/<site>` (NVIDIA, Salesforce, JPMC,
Capital One, …). Two phases: a paginated listings POST and a single-job detail
GET.

This adapter requires per-company configuration — Workday tenants/sites cannot
be inferred from a slug — and is therefore wired through
`types.ConfigurableAdapter`. The engine reads
`company_scan_settings.adapter_config` and calls `FetchJobsWithConfig`.

---

## 0. Adapter Config

Stored in `company_scan_settings.adapter_config` (JSON):

```json
{
  "host": "nvidia.wd5.myworkdayjobs.com",
  "site": "NVIDIAExternalCareerSite",
  "locale": "en-US",
  "max_jobs_per_scan": 2000
}
```

| Field               | Required | Description                                             |
| ------------------- | -------- | ------------------------------------------------------- |
| `host`              | yes      | Must match `^[a-z0-9-]+\.wd[0-9]+\.myworkdayjobs\.com$` |
| `site`              | yes      | Site path segment after the host; no `/`                |
| `locale`            | no       | Reserved; CXS endpoint ignores it today                 |
| `max_jobs_per_scan` | no       | Cap on listings returned in one scan; defaults to 2000  |

The tenant is the leftmost subdomain of `host` (e.g. `nvidia.wd5.…` → `nvidia`).

---

## 1. List Jobs

**Endpoint:** `POST https://<host>/wday/cxs/<tenant>/<site>/jobs`

### Request

```http
POST /wday/cxs/nvidia/NVIDIAExternalCareerSite/jobs HTTP/1.1
Host: nvidia.wd5.myworkdayjobs.com
Content-Type: application/json
Accept: application/json
User-Agent: Mozilla/5.0 ... Chrome/124.0.0.0 Safari/537.36

{"appliedFacets":{},"limit":20,"offset":0,"searchText":""}
```

`limit` is hard-capped to 20 by Workday — larger values are silently truncated.
The adapter paginates by incrementing `offset` until either `offset >= total`,
the server returns zero postings, or the per-scan cap is hit.

### Response

```json
{
  "total": 1234,
  "jobPostings": [
    {
      "title": "Senior Software Engineer",
      "externalPath": "/job/San-Jose/Senior-SWE_R-12345",
      "locationsText": "San Jose, CA",
      "postedOn": "Posted 5 Days Ago",
      "bulletFields": ["R-12345"]
    }
  ]
}
```

### Response Fields

| Field                         | Type     | Description                                                | Captured                                    |
| ----------------------------- | -------- | ---------------------------------------------------------- | ------------------------------------------- |
| `total`                       | int      | Total open postings under the active facet                 | ✅ (drives pagination)                      |
| `jobPostings[].title`         | string   | Job title                                                  | ✅ → `Title`                                |
| `jobPostings[].externalPath`  | string   | Path segment under `<host>/<site>/job` for the detail page | ✅ → `URL` + detail-endpoint reconstruction |
| `jobPostings[].locationsText` | string   | Pre-formatted location label                               | ✅ → `Location`                             |
| `jobPostings[].postedOn`      | string   | Display string ("Posted N Days Ago"); not parseable        | ✅ (struct)                                 |
| `jobPostings[].bulletFields`  | []string | Requisition ID + ancillary chips, in tenant-specific order | ✅ → primary source for `ID`                |

The listing endpoint carries no employment-type metadata, so
`FetchJobsItem.EmploymentType` defaults to `full_time`. The analyzer overwrites
this from the detail endpoint via `FetchJobContent`.

---

## 2. Get Single Job

**Endpoint:** `GET https://<host>/wday/cxs/<tenant>/<site><externalPath>`

`externalPath` already begins with `/job/...`. Concatenate it directly — DO NOT
add an extra `/job` prefix. Live CXS returns HTTP 406 (Not Acceptable) for the
doubled `/job/job/` form, even though the host accepts the request.

### Request

```http
GET /wday/cxs/nvidia/NVIDIAExternalCareerSite/job/San-Jose/Senior-SWE_R-12345 HTTP/1.1
Host: nvidia.wd5.myworkdayjobs.com
Accept: application/json
User-Agent: Mozilla/5.0 ...
```

### Response

```json
{
  "jobPostingInfo": {
    "title": "Senior Software Engineer",
    "jobDescription": "<p>We are looking for…</p>",
    "location": "San Jose, CA",
    "jobPostingDate": "2026-05-01",
    "timeType": "Full time",
    "jobRequisitionId": "R-12345",
    "externalUrl": "https://nvidia.wd5.myworkdayjobs.com/NVIDIAExternalCareerSite/job/…",
    "country": "United States of America"
  }
}
```

### Response Fields

| Field                             | Type          | Description                                               | Captured                            |
| --------------------------------- | ------------- | --------------------------------------------------------- | ----------------------------------- |
| `jobPostingInfo.title`            | string        | Job title                                                 | ✅ → `Title`                        |
| `jobPostingInfo.jobDescription`   | string (HTML) | Long-form posting body                                    | ✅ → `Description` (HTML stripped)  |
| `jobPostingInfo.location`         | string        | Primary location string                                   | ✅ → `Location`                     |
| `jobPostingInfo.jobPostingDate`   | string        | ISO-style date (`YYYY-MM-DD`)                             | ✅ → `PublishedDate`                |
| `jobPostingInfo.timeType`         | string        | `"Full time"` / `"Part time"` / `"Contract"` / `"Intern"` | ✅ → normalised to `EmploymentType` |
| `jobPostingInfo.jobRequisitionId` | string        | Stable requisition number (e.g. `R-12345`)                | ✅ → `ID`                           |
| `jobPostingInfo.externalUrl`      | string        | Public detail-page URL                                    | ✅ (struct)                         |
| `jobPostingInfo.startDate`        | string        | Posting start date                                        | ✅ (struct)                         |
| `jobPostingInfo.endDate`          | string        | Posting end date (may be empty)                           | ✅ (struct)                         |
| `jobPostingInfo.country`          | string        | Country display name                                      | ✅ (struct)                         |
| `jobPostingInfo.remoteType`       | string        | Sometimes populated — many tenants leave it blank         | ✅ (struct)                         |

`WorkplaceType` is intentionally left empty: Workday does not expose a reliable
machine-readable workplace flag in CXS, and `remoteType` is unset for the
majority of tenants. The LLM enricher infers it from the description.

---

## 3. Pagination Protocol

```
offset := 0
loop:
  POST /wday/cxs/<tenant>/<site>/jobs with {"limit":20,"offset":offset,...}
  append all jobPostings to result
  offset += len(jobPostings)
  if len(jobPostings) == 0: break          (server has nothing more)
  if total > 0 && offset >= total: break   (declared end)
  if len(result) >= max_jobs_per_scan: break
  if page_count >= 200: break              (hard defensive cap)
  sleep 250ms                              (be polite to Workday's edge cache)
```

A 404 on the listing endpoint is treated as an empty result (board retired);
any other non-2xx is wrapped as an error so the scan is recorded as failed.

---

## 4. URL Formats

| Usage              | Format                                                                    |
| ------------------ | ------------------------------------------------------------------------- |
| `companies.url`    | Free-form display URL — adapter ignores it; config is the source of truth |
| `RawJob.URL` (web) | `https://<host>/<site>/job<externalPath>`                                 |
| List API           | `POST https://<host>/wday/cxs/<tenant>/<site>/jobs`                       |
| Content API        | `GET https://<host>/wday/cxs/<tenant>/<site>/job<externalPath>`           |

The adapter parses `RawJob.URL` in `FetchJobContent` to recover `host`,
`site`, and `externalPath`, then constructs the Content API URL.

---

## 5. EmploymentType Normalisation (timeType)

| Workday value                                        | Normalised constant |
| ---------------------------------------------------- | ------------------- |
| `Full time`, `Full-time`, `Regular`                  | `full_time`         |
| `Part time`, `Part-time`                             | `part_time`         |
| `Contract`, `Contractor`, `Consultant`, `Fixed term` | `contract`          |
| `Intern`, `Internship`                               | `internship`        |
| anything else                                        | `full_time`         |

---

## 6. ID Format

Listing-phase IDs prefer `bulletFields[0]` (requisition number in every tenant
observed so far), falling back to the trailing `_<id>` suffix of
`externalPath`, then to the path itself, then to the literal `"unknown"`.

Detail-phase IDs prefer `jobPostingInfo.jobRequisitionId`, with the same path
fallback chain.

Both phases prefix the result with `workday-`:

```
workday-R-12345
workday-12345
```

---

## 7. Anti-Bot Notes

- Workday's edge fronts (Cloudfront/Akamai variants) silently 403 the default
  Go `User-Agent`. The adapter sets a Chrome desktop UA.
- The CXS endpoints accept anonymous traffic but rate-limit aggressively when
  hammered. The 250 ms inter-page sleep is the empirically-derived floor that
  avoids 429s on the NVIDIA, Capital One, and JPMC boards.
- No cookies, no CSRF, no auth header required.
