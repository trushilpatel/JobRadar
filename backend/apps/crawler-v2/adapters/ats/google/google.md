# Google Careers Adapter

## Overview

Google Careers (`www.google.com/about/careers/applications/jobs`) is server-side rendered. A plain HTTP GET returns the same DOM the browser sees, including job descriptions. The adapter uses Go's `net/http` + RE2 regex extraction — no headless browser, no Chromium, no JavaScript execution.

## Architecture

```
Google.FetchJobListing()
  → For pageNum = 1, 2, …:
       1. HTTP GET listing URL with &page=N
       2. parseListing(html) → up to 20 cards + total count from "Showing X to Y of N"
       3. Stop when pageNum*20 >= total OR page returns 0 cards
       4. Sleep minDelayMs..maxDelayMs between listing pages (0 in tests)
  → Returns core.JobListing (all parsed job summaries)

Google.FetchJobPost()
  → HTTP GET summary.URL (detail page)
  → extractDescription(html) — AF_initDataCallback JSON state machine
  → Returns core.JobPost with stripped plain-text description
```

## Key files

| File                         | Role                                                        |
| ---------------------------- | ----------------------------------------------------------- |
| `google.go`                  | Adapter struct, FetchJobListing, FetchJobPost, HTTP helpers |
| `mapper.go`                  | mapParsedJobToSummary, mapDetailToJobPost                   |
| `api/types.go`               | ParsedJob, JobDetail — intermediate structs from HTML parse |
| `google_test.go`             | Unit tests with httptest servers and file fixtures          |
| `mapper_test.go`             | Unit tests for mapping functions                            |
| `google_integration_test.go` | Live Google Careers tests (skipped under -short)            |
| `testdata/listing.html`      | Minimal listing page with 2 job cards                       |
| `testdata/job_post.html`     | Detail page with AF_initDataCallback block                  |

## Interfaces implemented

| Interface             | Purpose                                     |
| --------------------- | ------------------------------------------- |
| `core.ListingFetcher` | `FetchJobListing` — walks paginated listing |
| `core.PostFetcher`    | `FetchJobPost` — extracts job detail page   |

## Pre-set listing URL filters

The listing URL is fixed and not runtime-configurable:

```
https://www.google.com/about/careers/applications/jobs/results
  ?location=United%20States
  &target_level=EARLY&target_level=MID
  &employment_type=FULL_TIME
  &q=Software%20Engineering
```

The `companyID` parameter to `FetchJobListing` is intentionally ignored — Google Careers is a single global search result set.

## Pagination

URL-based: append `&page=N` to the listing URL. Terminates when `pageNum * 20 >= total` (from the "Showing X to Y of N" counter) or when a page returns no cards.

## Regex patterns

All patterns are RE2-compatible. They anchor on stable HTML markers (icon names, aria attributes) — never on Google's obfuscated CSS class names.

| What                | Pattern                                  |
| ------------------- | ---------------------------------------- |
| Total job count     | `Showing\s+\d+\s+to\s+\d+\s+of\s+(\d+)`  |
| Per-card title      | `aria-label="Learn more about ([^"]+)"`  |
| Job ID + slug       | `href="jobs/results/(\d+)-([^"?]+)`      |
| Location text block | `>place</[^>]+>(.{0,800}?)<button`       |
| Org text block      | `>corporate_fare</[^>]+>(.{0,400}?)<i\b` |

### Per-card slicing

Each "Learn more about" anchor sits at the **end** of its card. Metadata for card `i` lives in `html[aria[i-1].end : aria[i].start]`. The last `href` match in each slice is the card's URL.

## Detail page extraction: AF_initDataCallback state machine

Google detail pages embed job data in `AF_initDataCallback({key: 'ds:0'…})` script blocks. The focal job is in `ds:0`; sibling jobs from the same search are in `ds:1` and are deliberately excluded.

The extractor:

1. Finds the `AF_initDataCallback({key: 'ds:0'` marker
2. Truncates at the next `AF_initDataCallback` marker (start of `ds:1`)
3. Walks the block byte-by-byte with a JSON-string state machine (tracks `inString`, `escaped` state)
4. Collects every JSON-quoted string containing the `<` six-byte escape sequence (Go's `json.Marshal` encoding of `<`)
5. JSON-decodes each qualifying string, strips HTML tags, deduplicates, and joins with blank lines

## Output fields

| Field         | Source                               | Notes                              |
| ------------- | ------------------------------------ | ---------------------------------- |
| `ID`          | Listing page anchor href             | `google-{numeric_id}`              |
| `Title`       | `aria-label="Learn more about …"`    |                                    |
| `Location`    | `>place<` icon block                 | Multi-location; `+N more` stripped |
| `URL`         | Constructed from ID + slug           | No query parameters                |
| `EmployType`  | Constant                             | Always `full_time` (search filter) |
| `Description` | `ds:0` AF_initDataCallback block     | HTML stripped; plain text          |
| `Department`  | Not available from listing or detail | Always empty string                |

## Pacing

| Field        | Default      | Notes                                  |
| ------------ | ------------ | -------------------------------------- |
| `minDelayMs` | 20000 (20 s) | Min delay between listing-page fetches |
| `maxDelayMs` | 40000 (40 s) | Max delay between listing-page fetches |

Set both to `0` in tests — the adapter returns immediately when both are 0.

## Anti-bot measures

| Technique      | Implementation                       |
| -------------- | ------------------------------------ |
| User-Agent     | Pinned Chrome 121 on macOS           |
| Accept headers | `text/html,…` and `en-US,en;q=0.9`   |
| Pacing         | Random 20–40 s between listing pages |
