# ATS Adapter `CareersURL` Refactor

**Date:** 2026-04-09  
**Status:** Approved

## Problem

`company.url` currently serves two conflicting purposes:

1. **API endpoint** — for Greenhouse, the stored URL is the exact endpoint the crawler calls (`https://boards-api.greenhouse.io/v1/boards/{slug}/jobs`)
2. **Display/link** — for Lever and Ashby, the frontend auto-fills a fake API-style URL that the backend can't use as-is

The frontend `AddCompanyModal` generates wrong URLs for Lever and Ashby:
```
lever: https://api.lever.co/v0/postings/netlify     ← backend can't use this directly
ashby: https://api.ashbyhq.com/posting-api/job-board/openai  ← SlugFromURL saves it today
```

The crawler passes `company.url` to every adapter's `FetchJobs`. Lever embeds it directly into the API path, producing a broken URL. Ashby survives only due to `SlugFromURL` stripping it back down to a slug.

## Design

### 1. Add `CareersURL` to the `Adapter` interface

```go
// ats_fetcher/types/types.go
type Adapter interface {
    FetchJobs(ctx context.Context, companyID string) (FetchJobsResponse, error)
    CareersURL(id string) string
}
```

Each adapter owns both pieces of knowledge about itself: how to fetch jobs and what URL to show users. Adding a new adapter requires implementing both.

### 2. Implement `CareersURL` per adapter

| Adapter | `CareersURL(id)` | Notes |
|---|---|---|
| Greenhouse | `https://boards-api.greenhouse.io/v1/boards/{id}/jobs` | Same as what was stored before — also the API endpoint |
| Lever | `https://jobs.lever.co/{id}` | Public page. `FetchJobs` constructs `api.lever.co` URL internally |
| Ashby | `https://jobs.ashbyhq.com/{id}` | Public page. `FetchJobs` constructs GraphQL URL internally |

### 3. Simplify `FetchJobs` implementations

**Greenhouse** — add `listURL(slug string) string` method (mirrors existing `contentURL`). `FetchJobs` receives the slug and calls `listURL` directly. No more full URL passed in.

**Lever** — no change. Already uses `companyID` as slug to construct its own URL.

**Ashby** — remove `SlugFromURL` call. Use `companyID` directly as the slug. Delete `SlugFromURL` function.

### 4. Crawler change

```go
// engine.go — line 78, before:
result, err := adp.FetchJobs(ctx, company.URL)

// after:
result, err := adp.FetchJobs(ctx, company.ID)
```

`company.url` is no longer read by the crawler for these three adapters.

### 5. Frontend `AddCompanyModal` — update URL templates

```ts
const URL_TEMPLATES = {
  greenhouse: id => `https://boards-api.greenhouse.io/v1/boards/${id}/jobs`,
  lever:      id => `https://jobs.lever.co/${id}`,
  ashby:      id => `https://jobs.ashbyhq.com/${id}`,
  workable:   id => `https://${id}.workable.com/spi/v3/jobs`,
}
```

Lever and Ashby now show the public page URL instead of the internal API URL. `company.url` becomes a pure display/link-out field.

### 6. Migration `013_normalize_company_urls.sql`

One-time update to fix existing Lever and Ashby rows stored with API-style URLs:

```sql
-- +goose Up
UPDATE companies
SET url = 'https://jobs.lever.co/' || id
WHERE adapter_type = 'lever';

UPDATE companies
SET url = 'https://jobs.ashbyhq.com/' || id
WHERE adapter_type = 'ashby';

-- +goose Down
UPDATE companies
SET url = 'https://api.lever.co/v0/postings/' || id
WHERE adapter_type = 'lever';

UPDATE companies
SET url = 'https://api.ashbyhq.com/posting-api/job-board/' || id
WHERE adapter_type = 'ashby';
```

Greenhouse rows are already correct — their stored URL is the API endpoint, which matches `CareersURL`.

## Scope

- Workday, Workable, Chromedp are **not touched** for fetch logic — they scrape arbitrary URLs with no standard slug pattern. They must still implement `CareersURL` to satisfy the interface; they return `""` since a public URL cannot be derived from just the company ID.
- `FetchJobContent` for all three adapters is **not touched** — it already parses `job.URL` (the hosted web URL from the listing API) directly.

## File changes

| File | Change |
|---|---|
| `internal/ats_fetcher/types/types.go` | Add `CareersURL(id string) string` to `Adapter` interface |
| `internal/ats_fetcher/greenhouse/greenhouse.go` | Add `listURL(slug)`, update `FetchJobs` to use slug, implement `CareersURL` |
| `internal/ats_fetcher/lever/lever.go` | Implement `CareersURL` |
| `internal/ats_fetcher/ashby/ashby.go` | Remove `SlugFromURL`, update `FetchJobs` to use `companyID` directly, implement `CareersURL` |
| `internal/ats_fetcher/workday/workday.go` | Implement `CareersURL` stub |
| `internal/ats_fetcher/workable/workable.go` | Implement `CareersURL` stub |
| `internal/ats_fetcher/chromedp/chromedp.go` | Implement `CareersURL` stub |
| `internal/ats_fetcher/registry.go` | No change (interface is satisfied by each adapter) |
| `internal/worker/job_listing_crawler/engine.go` | Pass `company.ID` instead of `company.URL` |
| `frontend/src/components/companies/AddCompanyModal.tsx` | Update Lever and Ashby URL templates |
| `backend/migrations/013_normalize_company_urls.sql` | Backfill Lever and Ashby URLs |
