# Meta Careers Adapter

Fetches job listings from Meta Careers at `metacareers.com/jobs/` via Playwright scraping.

## Why Playwright

Meta's careers site uses Facebook's Comet/Relay framework. Job listing data is loaded via POST requests with session-specific tokens (`lsd`, `jazoest`, `__rev`) that change per session. These tokens can only be obtained by first loading the page in a real browser. A Playwright scraper is the only viable approach.

## Architecture

The adapter is a **full-detail adapter** — it populates `FullContent` at scan time via the scraper, so the `job_post_analyzer` skips HTTP fetches and goes straight to LLM enrichment for jobs that passed relevance filtering.

```
Adapter.FetchJobs()
  → Resolves scraper directory
  → Runs scrape.js via `node` or `xvfb-run node`
  → Passes config as env vars (MIN_DELAY, MAX_DELAY, TIMEOUT_MS, MAX_JOBS)
  → Parses JSON output from scrape.js
  → Returns FetchJobsResponse with FullContent populated
```

## Scraper phases

### Phase 1: Load the jobs page and scroll to load all jobs

1. Navigate to `https://www.metacareers.com/jobs/`
2. Wait for initial DOM to load
3. Scroll repeatedly to trigger lazy loading until no new jobs appear
4. Extract job IDs, URLs, titles, and locations from the DOM

**Resilient selectors:**
- Job cards: `<a>` elements linking to `/jobs/\d+/`
- Title: First line of card text content
- Location: Text containing location pattern (comma, "Remote", "Hybrid")
- Team/department: Secondary card text lines

### Phase 2: Visit each job detail page

1. For each job URL, navigate to the detail page
2. Wait for JS to render full content
3. Extract full description from the DOM
4. Random delay between pages (configurable min/max, default 3-8s)
5. Stop early if timeout is reached or max jobs limit hit

### Phase 3: Output JSON

Return a JSON array of jobs with full_content populated:

```json
[
  {
    "id": "meta-12345678",
    "title": "Software Engineer",
    "url": "https://www.metacareers.com/jobs/12345678/",
    "location": "Menlo Park, CA",
    "employ_type": "full_time",
    "full_content": {
      "description": "plain text description...",
      "requirements": "",
      "compensation": "",
      "workplace_type": "",
      "department": "Engineering",
      "published_date": ""
    }
  }
]
```

## Configuration

All config is passed from Go to Node.js via environment variables:

| Env Var | Default | Description |
|---------|---------|-------------|
| `META_SCRAPER_TIMEOUT_MS` | 3600000 (1h) | Overall scraper timeout in milliseconds |
| `META_SCRAPER_MIN_DELAY` | 3000 (3s) | Minimum delay between job detail page loads |
| `META_SCRAPER_MAX_DELAY` | 8000 (8s) | Maximum delay between job detail page loads |
| `META_SCRAPER_MAX_JOBS` | 0 (all) | Maximum jobs to fetch; 0 = no limit |

Delays are chosen randomly in the range `[MIN_DELAY, MAX_DELAY]` to mimic human browsing.

## Go adapter usage

```go
import "github.com/trushilpatel/jobradar/pipeline/ats/meta"

// Create with custom config
adapter := meta.New(meta.Config{
  MinDelayMs: 3000,
  MaxDelayMs: 8000,
  TimeoutMs: 3600000,
  MaxJobs: 0,
})

// Or use defaults (recommended for production)
adapter := meta.NewDefault()

// Fetch jobs
response, err := adapter.FetchJobs(ctx, "") // companyID unused
if err != nil {
  log.Fatal(err)
}

for _, job := range response.Jobs {
  fmt.Printf("%s: %s (%s)\n", job.ID, job.Title, job.Location)
  if job.FullContent != nil {
    fmt.Printf("Description: %s\n", job.FullContent.Description[:100])
  }
}
```

## Scraper directory resolution

The Go adapter resolves the scraper directory in this order:

1. `scraperDir` field (for tests)
2. `META_SCRAPER_DIR` environment variable
3. `scraper/` relative to the executable
4. `scraper` relative to current working directory

For production, no configuration is needed — the scraper is expected in `scraper/` next to the binary.

## Installation

One-time setup before first use:

```bash
cd pipeline/ats/meta/scraper
bash install.sh
```

This installs Node.js dependencies and Playwright's Chromium browser.

## Testing

```bash
# Run all tests (uses mock scraper scripts)
go test -v ./pipeline/ats/meta

# Run integration test against real metacareers.com
TEST_REAL=1 go test -v ./pipeline/ats/meta -run TestFetchJobs_Real
```

Mock tests create temporary scripts that output fixed JSON, so they run instantly without network or Chromium overhead.

## Limitations and notes

- **No filtering by location yet**: Currently loads all jobs and does not filter by "United States". The scraper should ideally apply the US filter at load time if Meta's API supports it. For now, each job's location is extracted from the DOM as displayed.

- **No compensation/requirements extraction**: These fields are left empty. Meta doesn't prominently display this info on listing pages. The relevance filter and analyzer can still use the job description.

- **Employment type**: Always normalized to "full_time" since Meta primarily posts full-time roles. This may need adjustment if part-time or contract roles are posted.

- **Timeout on detail pages**: If enrichment times out mid-run, jobs already enriched are returned. Jobs not yet visited will have nil `FullContent` — the analyzer falls back to HTTP fetch for those.

- **DOM obfuscation**: Meta's DOM uses obfuscated class names that change frequently. Selectors are anchor on semantic tags (`<a>`, `<h1>`) and text content patterns, which are more stable.

## Troubleshooting

**Scraper exits with "page blocked"**
- Meta may be detecting automation. Try increasing `MIN_DELAY` and `MAX_DELAY` in config.
- Ensure `xvfb-run` and Chromium are installed.

**Scraper finds 0 jobs**
- The page structure may have changed. Check `scrape.js` selector logic.
- Verify network connectivity to `metacareers.com`.

**Node.js dependencies error**
- Run `bash scraper/install.sh` again to reinstall dependencies.
- Ensure Node.js >= 18 is installed.

**Timeout during scraping**
- Increase `TimeoutMs` in config (e.g., from 1h to 2h).
- Or reduce `MAX_JOBS` to process fewer jobs per scan.
