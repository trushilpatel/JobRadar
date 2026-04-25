# Google Careers Adapter

## Overview

Google Careers (`www.google.com/about/careers/applications/jobs`) is server-side rendered. A plain HTTP GET returns the same DOM the browser sees, **including** the qualifications sections on the detail page. The adapter uses Go's `net/http` + RE2 regex extraction. **No headless browser, no Chromium, no JavaScript execution.**

The adapter was previously rod/Chromium-based; that path was retired once we verified the page is fully SSR.

## Architecture

```
GoogleAdapter.FetchJobs()
  → For pageNum = 1, 2, …:
       1. HTTP GET careers URL with &page=N
       2. parseListing(html) → 20 cards + total job count from "Showing X to Y of N"
       3. SeenChecker batch call (one DB round-trip per listing page)
       4. For each new card (capped by MaxDetailJobs):
            HTTP GET card.URL → parseDetail(html) → description with 3 sections
            On error: still publish a thin item (analyzer's HTTP fallback enriches later)
            Sleep EnrichMinMs..EnrichMaxMs between detail fetches
       5. streamPublish each item to Kafka + DB
       6. Stop when pageNum*20 >= total OR MaxPages reached
       7. Sleep MinDelayMs..MaxDelayMs between listing pages
  → Returns FetchJobsResponse (all published items)
```

## Key files

| File | Role |
|------|------|
| `http.go` | `GoogleAdapter` — HTTP client, regex extraction, scan loop, never-drop publishing |
| `crawler.go` | `CrawlerService` — wraps `BaseWorker`, runs the poll loop |
| `google.go` | `Config` struct + `defaultConfig()` |
| `cmd/main.go` | Standalone smoke-test binary (`google-http-test`) |
| `google_test.go` | Unit tests with synthetic listing/detail fixtures |

## Interfaces implemented

| Interface | Purpose |
|-----------|---------|
| `types.Adapter` | Required — `FetchJobs` |
| `types.URLProvider` | Returns the pre-filtered careers URL |
| `types.StreamPublisher` | Publishes each item immediately after the detail fetch |
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

URL-based — append `&page=N` to the careers URL. Verified against live HTML: page 2 returns "Showing 21 to 40 of 764", page 3 returns "Showing 41 to 60 of 764", etc. The total count is read once from the indicator and used to terminate cleanly when `pageNum * 20 >= total`.

## Regex patterns — extraction strategy

All patterns are RE2-compatible (Go's `regexp` engine). They anchor on stable HTML markers (icon names, aria attributes, h3 headings) — never on Google's obfuscated CSS class names.

### Listing page

| What | Pattern |
|------|---------|
| Total job count | `Showing\s+\d+\s+to\s+\d+\s+of\s+(\d+)` |
| Per-card title (anchor) | `aria-label="Learn more about ([^"]+)"` |
| Job ID + slug (anchor href) | `href="jobs/results/(\d+)-([^"?]+)` |
| Location text block | `>place</[^>]+>(.{0,800}?)<button` |
| Org text block | `>corporate_fare</[^>]+>(.{0,400}?)<i\b` |

### Per-card slicing

Each "Learn more about" anchor sits at the **end** of its card's markup. The metadata for card `i` lives in `html[aria[i-1].end : aria[i].start]` (or `[0 : aria[0].start]` for the first card). Within that slice:
- The **last** href match is the card's anchor URL.
- The **first** `>place<` match is its location.
- The **first** `>corporate_fare<` match is its org.

The location/org regexes capture a window up to a stable terminator (`<button>` for location, next `<i…>` for org), then post-process strips tags via `<[^>]+>`, removes `+N more` truncation indicators, and collapses whitespace.

### Detail page

| What | Pattern |
|------|---------|
| Title (from `<title>`) | `<title>([^<]+?)\s*(?:—|-)\s*Google Careers</title>` |
| Section block (×3) | `<h3[^>]*>[^<]*{NAME}[^<]*</h3>\s*<ul[^>]*>(.*?)</ul>` |
| Bullet | `<li[^>]*>(.*?)</li>` |

Section names: `Minimum qualifications`, `Preferred qualifications`, `Responsibilities`. The `[^<]*` flanks tolerate the trailing colon Google emits (`"Minimum qualifications:"`).

## Output fields

| Field | Captured | Notes |
|-------|---------|-------|
| `id` | Yes | `google-{numeric_id}` from anchor href |
| `title` | Yes | From `aria-label` (listing) or `<title>` (detail) |
| `url` | Yes | Canonical detail URL: `{detailBase}{id}-{slug}` (no query string) |
| `location` | Yes | Listing card; multi-locations preserved, `+N more` stripped |
| `employ_type` | Yes | Always `full_time` (matches search filter) |
| `full_content.description` | Yes | Title + location + min quals + pref quals + responsibilities |
| `full_content.department` | Yes | Org from listing card (always `Google`) |
| All other `full_content.*` | No | Google does not expose them; analyzer's LLM derives from description |

## Pacing and timing config

| Field | Default | Description |
|-------|---------|-------------|
| `MinDelayMs` | `100000` (100 s) | Min delay between listing-page fetches |
| `MaxDelayMs` | `120000` (120 s) | Max delay between listing-page fetches |
| `EnrichMinMs` | `5000` (5 s) | Min delay between detail-page fetches |
| `EnrichMaxMs` | `10000` (10 s) | Max delay between detail-page fetches (`0` = skip enrichment) |
| `TimeoutMs` | `18000000` (5 h) | Overall scan timeout |
| `MaxPages` | `0` (all) | Cap on pages; `0` = paginate to `total` |
| `MaxDetailJobs` | `0` (all) | Cap on detail fetches per listing page |

Configured intervals are honoured strictly — `MinDelayMs/MaxDelayMs` between listing pages and `EnrichMinMs/EnrichMaxMs` between detail fetches. There are no hidden waits inside the adapter.

## Anti-bot measures

| Technique | Implementation |
|-----------|---------------|
| User-Agent | Pinned Chrome 121 on macOS |
| Accept / Accept-Language | `text/html,…` and `en-US,en;q=0.9` |
| Pacing | Random 100-120 s between listing pages, 5-10 s between detail fetches |
| Sequential, single-flight | One outstanding HTTP request at a time per scan |

The previous rod-based adapter relied on `go-rod/stealth`, viewport patches, and CDP timezone overrides; none of that is needed for HTTP-only.

## Reliability — never-drop publishing

If a detail-page fetch fails (HTTP error, timeout, parse error), the adapter publishes the **thin** item (no `FullContent`) instead of dropping it. The downstream analyzer has its own HTTP-fetch fallback that picks up thin items and enriches them later. This guarantees a successful listing parse always reaches Kafka, even if the detail step is fragile.

## Removed-job detection

The engine diffs each scan against `GetActiveJobIDsByCompany`. Jobs absent from the current scan get `removed_at` set in `job_pipeline_ledger`. Unchanged from the previous adapter.

## Smoke-test command

```bash
cd backend
go run ./apps/crawler/ats/google/cmd \
  -max-pages 1 -max-detail 1 \
  -delay-min 5000 -delay-max 8000
```

Flags: `-max-pages`, `-max-detail`, `-delay-min`, `-delay-max`, `-enrich-min`, `-enrich-max`, `-timeout`. Use small delays in dev; production reads from env (`GOOGLE_SCRAPER_*`).

## Migration history

- v1: Node.js + Playwright subprocess.
- v2: Go + go-rod (in-process Chromium), removed npm/Node from the crawler image.
- v3 (current): Pure Go `net/http` + RE2 regex. Chromium is no longer required for Google. The Meta adapter still uses rod, so Chromium remains in the crawler image for now.
