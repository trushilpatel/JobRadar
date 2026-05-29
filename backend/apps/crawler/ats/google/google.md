# Google Careers Adapter

## Overview

Google Careers (`www.google.com/about/careers/applications/jobs`) is server-side rendered. A plain HTTP GET returns the same DOM the browser sees. The adapter uses Go's `net/http` + RE2 regex extraction. **No headless browser, no Chromium, no JavaScript execution.**

The adapter was previously rod/Chromium-based; that path was retired once we verified the page is fully SSR.

## Architecture

```
GoogleAdapter.FetchJobs()
  → For pageNum = 1, 2, …:
       1. HTTP GET careers URL with &page=N
       2. parseListing(html) → up to 20 cards + total count from "Showing X to Y of N"
       3. SeenChecker batch call (one DB round-trip per listing page)
       4. For each new card: streamPublish thin item to NATS JetStream + DB
       5. Stop when pageNum*20 >= total OR MaxPages reached
       6. Sleep MinDelayMs..MaxDelayMs between listing pages
  → Returns FetchJobsResponse (all published items)
```

Detail-page fetching and LLM enrichment are handled downstream by the analyzer.

## Key files

| File             | Role                                                                       |
| ---------------- | -------------------------------------------------------------------------- |
| `http.go`        | `GoogleAdapter` — HTTP client, regex extraction, scan loop, stream publish |
| `google.go`      | `Config` struct + `defaultConfig()`                                        |
| `google_test.go` | Unit tests with synthetic listing fixtures                                 |
| `cmd/main.go`    | Standalone smoke-test binary (`google-http-test`)                          |

## Interfaces implemented

| Interface               | Purpose                                                               |
| ----------------------- | --------------------------------------------------------------------- |
| `types.Adapter`         | Required — `FetchJobs`                                                |
| `types.URLProvider`     | Returns the pre-filtered careers URL                                  |
| `types.StreamPublisher` | Publishes each thin item immediately per page                         |
| `types.SeenChecker`     | Receives `HasSeenBatch` callback — one batch DB call per listing page |

`types.ContentFetcher` is **not** implemented — the analyzer's generic HTTP fallback enriches detail pages downstream.

## Pre-set filters

Starting URL targets relevant positions only:

```
https://www.google.com/about/careers/applications/jobs/results
  ?location=United%20States
  &target_level=EARLY&target_level=MID
  &employment_type=FULL_TIME
  &q=Software%20Engineering
```

These filters are intentional and not runtime-configurable.

## Pagination

URL-based — append `&page=N` to the careers URL. Verified against live HTML: page 2 returns "Showing 21 to 40 of 764", etc. Terminates when `pageNum * 20 >= total`.

## Regex patterns — extraction strategy

All patterns are RE2-compatible (Go's `regexp` engine). They anchor on stable HTML markers (icon names, aria attributes) — never on Google's obfuscated CSS class names.

### Listing page

| What                        | Pattern                                  |
| --------------------------- | ---------------------------------------- |
| Total job count             | `Showing\s+\d+\s+to\s+\d+\s+of\s+(\d+)`  |
| Per-card title (anchor)     | `aria-label="Learn more about ([^"]+)"`  |
| Job ID + slug (anchor href) | `href="jobs/results/(\d+)-([^"?]+)`      |
| Location text block         | `>place</[^>]+>(.{0,800}?)<button`       |
| Org text block              | `>corporate_fare</[^>]+>(.{0,400}?)<i\b` |

### Per-card slicing

Each "Learn more about" anchor sits at the **end** of its card's markup. The metadata for card `i` lives in `html[aria[i-1].end : aria[i].start]` (or `[0 : aria[0].start]` for the first card). Within that slice:

- The **last** href match is the card's anchor URL.
- The **first** `>place<` match is its location.
- The **first** `>corporate_fare<` match is its org (parsed but not yet published downstream — no `Department` field on `FetchJobsItem`).

## Output fields

| Field          | Captured              | Notes                                                                |
| -------------- | --------------------- | -------------------------------------------------------------------- |
| `id`           | Yes                   | `google-{numeric_id}` from anchor href                               |
| `title`        | Yes                   | From `aria-label`                                                    |
| `url`          | Yes                   | Canonical detail URL: `{detailBase}{id}-{slug}` (no query string)    |
| `location`     | Yes                   | Listing card; multi-locations preserved, `+N more` stripped          |
| `employ_type`  | Yes                   | Always `full_time` (matches search filter)                           |
| `full_content` | No                    | Always `nil` — analyzer fetches detail page downstream               |
| `org`          | Parsed, not published | `corporate_fare` block; no `Department` field on `FetchJobsItem` yet |

## Pacing and timing config

| Field        | Default         | Description                             |
| ------------ | --------------- | --------------------------------------- |
| `MinDelayMs` | `20000` (20 s)  | Min delay between listing-page fetches  |
| `MaxDelayMs` | `40000` (40 s)  | Max delay between listing-page fetches  |
| `TimeoutMs`  | `7200000` (2 h) | Overall scan timeout                    |
| `MaxPages`   | `0` (all)       | Cap on pages; `0` = paginate to `total` |

## Anti-bot measures

| Technique                 | Implementation                                  |
| ------------------------- | ----------------------------------------------- |
| User-Agent                | Pinned Chrome 121 on macOS                      |
| Accept / Accept-Language  | `text/html,…` and `en-US,en;q=0.9`              |
| Pacing                    | Random 20–40 s between listing pages            |
| Sequential, single-flight | One outstanding HTTP request at a time per scan |

## Deduplication

Two layers:

1. **In-memory** (`seenIDs map[string]struct{}`) — prevents re-publishing the same ID if Google returns duplicate cards across pages.
2. **DB ledger** (`SeenChecker.HasSeenBatch`) — one batch call per listing page against `job_pipeline_ledger`. If this call fails, all cards on that page are treated as already-seen (safe skip rather than double-publish).

## Smoke-test command

```bash
cd backend
go run ./apps/crawler/ats/google/cmd \
  -max-pages 1 \
  -delay-min 5000 -delay-max 8000
```

Flags: `-max-pages`, `-delay-min`, `-delay-max`, `-timeout`.

## Migration history

- v1: Node.js + Playwright subprocess.
- v2: Go + go-rod (in-process Chromium), removed npm/Node from the crawler image.
- v3 (current): Pure Go `net/http` + RE2 regex. Chromium is no longer required for Google. The Meta adapter still uses rod, so Chromium remains in the crawler image for now.
