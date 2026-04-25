# Meta Careers Adapter

Fetches job listings from `metacareers.com/jobsearch` via go-rod (headless Chromium). Full-detail adapter — populates `FullContent` at scan time so the analyzer skips HTTP fetches.

## Architecture

```
Adapter.FetchJobs()
  → Opens Chromium via go-rod (shared browser or own instance)
  → Navigates to metacareers.com/jobsearch
  → Scrolls infinite scroll to collect all thin job cards
  → HasSeenBatch check for all collected IDs (one DB call)
  → Visits detail page (separate tab) for unseen jobs only
  → Publishes each enriched job immediately via streamPublish
  → Returns FetchJobsResponse with FullContent populated
```

## Key files

| File | Role |
|------|------|
| `rod.go` | `RodMeta` — browser automation, scroll, detail fetch, publish |
| `crawler.go` | `CrawlerService` — wraps `BaseWorker`, runs the poll loop |

## Interfaces implemented

| Interface | Purpose |
|-----------|---------|
| `types.Adapter` | Required — `FetchJobs` |
| `types.StreamPublisher` | Publishes each job to Kafka + DB immediately after enrichment |
| `types.URLProvider` | Returns `metacareers.com/jobsearch` |
| `types.SeenChecker` | Receives `HasSeenBatch` callback from engine — one batch DB call after scroll completes |

## Config (`Config` struct)

| Field | Default | Description |
|-------|---------|-------------|
| `MinDelayMs` | 3000 | Min delay between detail page visits |
| `MaxDelayMs` | 8000 | Max delay between detail page visits |
| `TimeoutMs` | 3600000 (1h) | Overall scan timeout |
| `MaxJobs` | 0 (all) | Cap on jobs to fetch; 0 = no limit |

## Browser sharing

`WithBrowser(b *rod.Browser)` lets RodMeta share a Chromium process with RodGoogle. Two tabs open: one stays on the listing (scroll state preserved), one navigates to detail pages.

## Note

This adapter was migrated from a Node.js/Playwright subprocess (`scrape.js`) to pure Go/rod. There is no longer any Node.js, npm, or external scraper process. The crawler Docker image does not include nodejs.
