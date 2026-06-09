# Eightfold AI adapter

Configurable ATS adapter for Eightfold AI's SmartApply product. Powers careers boards for Netflix, Bloomberg, HSBC, Activision, Capital One, Vodafone, and many other large enterprises.

## Key design decisions

- **ConfigurableListingFetcher** — each customer runs on its own vanity host with its own `?domain=` parameter; neither can be inferred from a company slug, so `FetchJobListing` always errors and directs callers to `FetchJobListingWithConfig`.
- **Two-phase** — the listing phase emits thin `JobSummary` rows (no description); the enricher calls `FetchJobPost` for full content.
- **`scheme` field** — set to `"http"` in tests so `httptest.Server` (plain HTTP) is usable without TLS. Production always uses `"https"`.
- **Domain inference** — when `summary.URL` carries no `?domain=` query param, `apexDomain()` extracts the last two labels of the host (e.g. `explore.jobs.netflix.net` → `netflix.net`).

## Per-company config

Stored in `adapter_configs.config` / `company_scan_settings.adapter_config`:

```json
{
  "host": "explore.jobs.netflix.net",
  "domain": "netflix.com",
  "locale": "en",
  "max_jobs_per_scan": 0
}
```

| Field               | Required | Default |
| ------------------- | -------- | ------- |
| `host`              | yes      | —       |
| `domain`            | yes      | —       |
| `locale`            | no       | `"en"`  |
| `max_jobs_per_scan` | no       | 5000    |

## Files

| File                            | Purpose                                                       |
| ------------------------------- | ------------------------------------------------------------- |
| `eightfold.go`                  | Adapter struct, config validation, HTTP pagination, URL parse |
| `mapper.go`                     | `Position` → `core.JobSummary` / `core.JobPost` conversion    |
| `api/types.go`                  | Raw JSON response types (`ListResponse`, `Position`)          |
| `eightfold_test.go`             | Unit tests (httptest servers, no real network)                |
| `mapper_test.go`                | Pure mapping + config validation tests                        |
| `eightfold_integration_test.go` | Real-API tests (skipped under `-short`)                       |
| `testdata/listing.json`         | Fixture: two-position listing response                        |
| `testdata/job_post.json`        | Fixture: single position detail response with HTML desc       |
