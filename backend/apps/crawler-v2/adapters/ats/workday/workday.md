# Workday CXS Adapter

Adapter for Workday-hosted careers boards at `<tenant>.wd<N>.myworkdayjobs.com/<site>`.

Implements `core.ConfigurableListingFetcher` (per-company JSON config required) and
`core.PostFetcher` (two-phase: listing then detail).

## Config (per-company JSON)

```json
{
  "host": "nvidia.wd5.myworkdayjobs.com",
  "site": "NVIDIAExternalCareerSite",
  "locale": "en-US",
  "max_jobs_per_scan": 2000
}
```

`host` must match `^[a-z0-9-]+\.wd[0-9]+\.myworkdayjobs\.com$`. Tenant is the
leftmost subdomain. `max_jobs_per_scan` defaults to 2000 when 0 or omitted.

## Endpoints

| Phase   | Method | URL pattern                                             |
| ------- | ------ | ------------------------------------------------------- |
| Listing | POST   | `https://<host>/wday/cxs/<tenant>/<site>/jobs`          |
| Detail  | GET    | `https://<host>/wday/cxs/<tenant>/<site><externalPath>` |

`externalPath` from the listing already starts with `/job/…`; concatenate directly.
Adding an extra `/job` prefix produces `/job/job/…` which CXS rejects with HTTP 406.

## Pagination

- Page size: 20 (server-side cap)
- Increment `offset += len(jobPostings)` per page
- Stop when: empty page, `offset >= total`, `len(result) >= max_jobs_per_scan`,
  or 200 pages (hard defensive cap)
- 404 on listing → treated as retired board, returns empty result (not an error)
- 250ms inter-page delay in production; set to 0 in tests via `interPageDelay` field

## ID format

Listing: prefers `bulletFields[0]` (requisition ID), falls back to trailing
`_<id>` in `externalPath`, then full path, then `"unknown"`. All prefixed `workday-`.

Detail: prefers `jobPostingInfo.jobReqId`, same fallback chain.

## Country code

`jobPostingInfo.country` is a JSON object `{"descriptor":"…","id":"US"}`. We
extract the `id` field. Null or missing returns `""`.

## Employment type

`timeType` is normalised: `Full time` → `full_time`, `Part time` → `part_time`,
`Contract/Contractor/Consultant/Fixed term` → `contract`, `Intern/Internship` →
`internship`. Unknown values default to `full_time`.

## Anti-bot

A Chrome desktop `User-Agent` header is required — Workday's edge CDN 403s the
default Go agent. No auth, no cookies, no CSRF token needed.

## See also

`/Users/trushil/Projects/JobRadar/backend/apps/crawler/ats/workday/workday.md`
for the original full API reference and field-level documentation.
