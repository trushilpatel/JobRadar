# Google Careers Adapter

## Overview

Google Careers (`careers.google.com`) is a JS-rendered SPA with no public API. The adapter uses **go-rod** (headless Chromium, in-process) to scrape it.

## Architecture

```
Adapter.FetchJobs()
  → Launches Chromium via go-rod (or reuses shared browser)
  → Navigates to pre-filtered careers URL
  → Per-page loop:
       1. Extract thin job cards from listing (DOM eval)
       2. HasSeenBatch check for all IDs on page (one DB call)
       3. Click card for each unseen job → detail panel opens inline (SPA)
       4. Extract description from panel
       5. Publish immediately via streamPublish → Kafka + DB
       6. Wait inter-page delay → click next page
  → Returns FetchJobsResponse (all published jobs)
```

## Key files

| File | Role |
|------|------|
| `rod.go` | `RodGoogle` — browser automation, per-page loop, card click, panel extraction |
| `crawler.go` | `CrawlerService` — wraps `BaseWorker`, runs the poll loop |
| `google.go` | `Config` struct + `defaultConfig()` |

## Interfaces implemented

| Interface | Purpose |
|-----------|---------|
| `types.Adapter` | Required — `FetchJobs` |
| `types.StreamPublisher` | Publishes each job immediately after panel extraction |
| `types.URLProvider` | Returns the pre-filtered careers URL |
| `types.SeenChecker` | Receives `HasSeenBatch` callback — one batch DB call per listing page |

## Pre-set filters

Starting URL targets relevant positions only:

```
https://www.google.com/about/careers/applications/jobs/results
  ?location=United%20States
  &target_level=EARLY&target_level=MID
  &employment_type=FULL_TIME
  &q=Software%20Engineering
```

## Pagination

- 20 jobs per page, page-based (not infinite scroll)
- Next page: `a[aria-label="Go to next page"]`
- Listing tab stays open throughout — clicking "next page" is a SPA transition, no full reload

## DOM selectors — resilient strategy

All selectors avoid obfuscated CSS class names. Anchored on stable attributes:

| Element | Selector |
|---------|---------|
| Job card link | `a[aria-label^="Learn more about"]` |
| Job card | `linkEl.closest("li")` |
| Title | `h3` inside card |
| Location | Material icon text `"place"` → parent text |
| Organisation | Material icon text `"corporate_fare"` or `"business"` → parent text |
| Next page | `a[aria-label="Go to next page"]` |
| Card click | `a[href*="jobs/results/{id}"]` — click opens detail panel inline |
| Detail panel | `extractJobDetail` — finds h3 headings by text, walks siblings for bullets |

## Output fields

| Field | Captured | Notes |
|-------|---------|-------|
| `id` | Yes | `google-{numeric_id}` from URL |
| `title` | Yes | From listing card |
| `url` | Yes | Full detail URL |
| `location` | Yes | From listing card |
| `employ_type` | Yes | Always `full_time` |
| `full_content.description` | Yes | Title + location + min quals + pref quals + responsibilities |
| `full_content.department` | Yes | Organisation from listing card |
| All other full_content fields | No | LLM parses from description |

## Timing config

| Field | Default | Description |
|-------|---------|-------------|
| `MinDelayMs` | 55000 (55s) | Min delay between listing pages |
| `MaxDelayMs` | 75000 (75s) | Max delay between listing pages |
| `EnrichMinMs` | 5000 (5s) | Min delay between card clicks on same page |
| `EnrichMaxMs` | 10000 (10s) | Max delay between card clicks |
| `TimeoutMs` | 18000000 (5h) | Overall scan timeout |
| `MaxPages` | 0 (all) | Cap on pages; 0 = paginate to end |
| `MaxDetailJobs` | 0 (all) | Cap on card clicks per page; 0 = all new |

## Anti-bot measures

| Technique | Implementation |
|-----------|---------------|
| go-rod/stealth | Patches `navigator.webdriver`, canvas, plugins JS signals |
| User agent | Chrome 121 on macOS (set at CDP level) |
| Timezone/locale | `America/New_York`, `en-US` |
| Viewport | 1366×768 |
| Random delays | 55-75s between pages, 5-10s between card clicks |
| Card click (not navigate) | Stays on listing page — less suspicious than full navigations |

## Removed job detection

The engine diffs each scan against `GetActiveJobIDsByCompany`. Jobs absent from the current scan get `removed_at` set in `job_pipeline_ledger`.

## Note

This adapter was migrated from a Node.js/Playwright subprocess to pure Go/rod. There is no `scrape.js`, no npm, no subprocess. The crawler Docker image contains only Go binary + Chromium.
