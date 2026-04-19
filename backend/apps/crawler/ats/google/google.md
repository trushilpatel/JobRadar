# Google Careers Adapter

## Overview

Unlike other adapters that call structured APIs, the Google adapter uses a **Playwright
browser automation** approach because Google Careers (`careers.google.com`) is a
JavaScript-rendered SPA with no public API or RSS feed.

## Architecture

```
Go adapter (google.go)
  └─ exec.CommandContext("node", "scraper/scrape.js")
       └─ Playwright + stealth plugin
            └─ careers.google.com → paginate all pages → extract DOM → enrich each job
       └─ JSON to stdout
  └─ parse JSON → FetchJobsResponse (with FullContent)
```

## Full-detail adapter

This is a **full-detail adapter**: it populates `FetchJobsItem.FullContent` at scan
time. The `job_post_analyzer` skips the enrichment fetch and goes straight to LLM
scoring. This avoids hitting Google twice per job (once for listing, once for detail).

## Pre-set filters

The scraper starts with a pre-filtered URL to target only relevant positions:

| Filter | URL param | Value |
|---|---|---|
| Location | `location` | `United States` |
| Experience | `target_level` | `EARLY` + `MID` (repeated param) |
| Job type | `employment_type` | `FULL_TIME` |
| Search query | `q` | `Software Engineering` |

Starting URL:
```
https://www.google.com/about/careers/applications/jobs/results
  ?location=United%20States
  &target_level=EARLY
  &target_level=MID
  &employment_type=FULL_TIME
  &q=Software%20Engineering
```

This typically yields ~680 results (as of April 2026).

## Pagination

Google Careers uses **page-based pagination** (not infinite scroll).

- 20 jobs per page
- "Next page" link: `<a aria-label="Go to next page">` (class `WpHeLc VfPpkd-mRLv6`)
- URL param: `&page=2`, `&page=3`, etc.
- No "previous page" link on page 1

The scraper clicks the next-page link after a 30-45 second random delay per page.

## DOM selectors — resilient strategy

All selectors avoid obfuscated CSS class names (e.g. `lLd3Je`, `p1N2lc`) that
break on every Google redeploy. Instead we anchor on stable attributes:

- **`aria-label`** — accessibility attributes that describe purpose
- **Material icon text** — icon names like `place`, `corporate_fare`, `bar_chart`
- **Semantic HTML tags** — `h3`, `li`, `ul`/`ol` for structure
- **Text content matching** — heading text like "Minimum qualifications"

### Listing page

| Element | Strategy | How |
|---|---|---|
| Job link | `aria-label` | `a[aria-label^="Learn more about"]` |
| Job card | DOM traversal | `linkEl.closest("li")` — walk up from link |
| Title | Semantic tag | `h3` inside card |
| Location | Material icon | Find `<i>`/`<span>` with text `"place"` → parent text minus icon |
| Experience | Material icon | Find element with text `"bar_chart"` → parent text minus icon |
| Organization | Material icon | Find element with text `"corporate_fare"` or `"business"` → parent text minus icon |
| Next page | `aria-label` | `a[aria-label="Go to next page"]` |

### Job detail page

Title and location are **not re-extracted** on the detail page — they come from
the listing card data (already captured). Only the qualification/responsibility
sections are extracted:

| Section | Strategy | How |
|---|---|---|
| Minimum qualifications | h3 text match | Find `<h3>` where text matches, walk siblings for `<ul>` bullets |
| Preferred qualifications | h3 text match | Same approach |
| Responsibilities | h3 text match | Same approach |

This means the detail page extraction uses **zero obfuscated CSS classes**.

### Job URL format

```
/about/careers/applications/jobs/results/{numeric-id}-{slug}
Example: /about/careers/applications/jobs/results/100056351576597190-data-center-facilities-technician-mechanical
```

Job ID: the numeric prefix (e.g. `100056351576597190`), stored as `google-{id}`.

## Scraper output schema

The Node.js scraper outputs a JSON array to stdout. Each element:

| Field | Type | Captured | Notes |
|---|---|---|---|
| `id` | string | Yes | `"google-{numeric_id}"` extracted from URL path |
| `title` | string | Yes | Job title text from card heading |
| `url` | string | Yes | Full URL to job detail page |
| `location` | string | Yes | Location text from card |
| `employ_type` | string | Yes | Normalized: `full_time`, `part_time`, `contract`, `internship` |
| `full_content.description` | string | Yes | Title + location (from listing card) + min quals + pref quals + responsibilities (from detail page), double-newline separated |
| `full_content.requirements` | string | — | Empty — LLM parses from description |
| `full_content.compensation` | string | — | Empty — LLM parses from description if present |
| `full_content.workplace_type` | string | — | Empty — LLM parses from description if present |
| `full_content.department` | string | Yes | Organization from listing card |
| `full_content.published_date` | string | — | Empty — not reliably available on Google Careers |

## Anti-bot measures

| Technique | Implementation |
|---|---|
| Stealth plugin | `playwright-extra` + `puppeteer-extra-plugin-stealth` masks `navigator.webdriver` and other automation signals |
| Random delays | 30-45 seconds between page navigations, 15-25 seconds between detail page visits |
| Human mouse movements | Random mouse moves before each action |
| Human scrolling | Random small scrolls to simulate reading behavior |
| Realistic viewport | 1366x768 with standard Chrome user agent |
| Headless Chromium | Default, with automation flags disabled |

## Configuration

| Env var | Default | Description |
|---|---|---|
| `GOOGLE_SCRAPER_TIMEOUT_MS` | `2700000` (45 min) | Overall scraper timeout |
| `GOOGLE_SCRAPER_MIN_DELAY` | `30000` (30s) | Minimum delay between page navigations |
| `GOOGLE_SCRAPER_MAX_DELAY` | `45000` (45s) | Maximum delay between page navigations |
| `GOOGLE_SCRAPER_ENRICH_MIN` | `15000` (15s) | Minimum delay between detail page visits |
| `GOOGLE_SCRAPER_ENRICH_MAX` | `25000` (25s) | Maximum delay between detail page visits |
| `GOOGLE_SCRAPER_DIR` | (auto-detected) | Override path to scraper/ directory |

## Company record

| Field | Value |
|---|---|
| `id` | `"google"` |
| `adapter_type` | `"google"` |
| `url` | (unused — scraper has built-in URL with filters) |

The `companyID` parameter in `FetchJobs()` is unused — the scraper always targets
the same Google Careers URL with pre-set filters.

## Deployment

### Local development

```bash
cd pipeline/ats/google/scraper
bash install.sh   # installs npm deps + Chromium
node scrape.js    # test run
```

### Docker

```bash
docker compose build google-scraper
docker compose run --rm google-scraper
```

The scraper runs in a separate container (`jobradar_google_scraper`) using the
`mcr.microsoft.com/playwright` base image with Chromium pre-installed.

## Removed job detection

The crawler engine diffs each scan's results against previously known active jobs
(via `SeenLedgerStore.GetActiveJobIDsByCompany`). Jobs that disappear from the
careers page get `removed_at` set in `job_pipeline_ledger`.
