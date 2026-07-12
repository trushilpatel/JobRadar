# ATS company discovery POC

Throwaway exploration script for GitHub issue #108 (Automated ATS Company
Discovery). Not part of the production Go pipeline, not wired into CI/lint.

Simple CLI, three deterministic stages, no LLM anywhere:

1. **Extract** — pull candidate company slugs from a free CDX index.
   - Greenhouse and Ashby allow Common Crawl's bot per their `robots.txt`,
     so they're queried against the last N Common Crawl indexes.
   - **Lever explicitly blocks Common Crawl's bot** (`User-agent: CCBot /
Disallow: /` in `jobs.lever.co/robots.txt`) — Common Crawl has almost
     no coverage of Lever boards as a result. Lever instead uses the
     Internet Archive's Wayback Machine CDX API (`web.archive.org/cdx`),
     which is not blocked, is equally free, and has fresh 2026 captures.
   - Tolerates the source's occasional 5xx/timeout/malformed-line hiccups
     with retry + backoff instead of aborting the whole run.
2. **Validate** — call the ATS's own free, unauthenticated public job board
   API for each candidate slug. `200` + non-empty postings = valid.
3. **Classify location** — `location_classifier.py` is a line-for-line
   Python port of the production USA/non-USA classifier
   (`backend/service/job/location/classifier.go`): "any USA wins" across a
   multi-part location, non-USA tokens win within one segment, whole-word
   matching. Applied per-job using whatever location field each ATS already
   returns in the validation response — no extra API calls. Verified
   against the Go file's own docstring examples.

Adding a new ATS is meant to be one new entry in `ats_specs.py`
(`ATSSpec` + a `parse_validation` function) — `discover.py` itself has no
per-ATS branching.

## Run it

```bash
python3 -m venv .venv
.venv/bin/pip install -r requirements.txt

.venv/bin/python discover.py --ats greenhouse
.venv/bin/python discover.py --ats ashby
.venv/bin/python discover.py --ats lever
.venv/bin/python discover.py --ats all      # default
```

Output lands in `output/` (gitignored), three files per ATS:

- `{ats}_candidates_<timestamp>.csv` — every checked slug with its status
- `{ats}_valid_companies_<timestamp>.csv` — valid ones only (slug, company
  name, total open job count, USA job count, API URL)
- `{ats}_usa_jobs_<timestamp>.csv` — every individual job classified as USA
  (company name, slug, title, location, apply URL)

## Flags

| Flag            | Default | Meaning                                                                                                                                   |
| --------------- | ------- | ----------------------------------------------------------------------------------------------------------------------------------------- |
| `--ats`         | `all`   | `greenhouse`, `ashby`, `lever`, or `all`                                                                                                  |
| `--max-slugs`   | 300     | cap on distinct candidate slugs to validate, per ATS                                                                                      |
| `--num-indexes` | 3       | how many recent Common Crawl indexes to query (ignored for Lever)                                                                         |
| `--page-limit`  | 2000    | max CDX records fetched per query                                                                                                         |
| `--delay`       | 0.4     | seconds between validation API calls (kept conservative — Greenhouse publishes no rate limit but warns aggressive hammering gets blocked) |

Lever extraction queries Wayback per starting character (`WAYBACK_SLUG_PREFIXES`
in `discover.py`) rather than one flat query, since a single query returns
results in alphabetical-urlkey order and would otherwise over-sample slugs
starting with digits/`a`.

To merge the three per-ATS `*_valid_companies_<timestamp>.csv` /
`*_usa_jobs_<timestamp>.csv` files into one combined file across all ATS
(each row already carries an `ats` column):

```bash
TS=<timestamp>
head -1 greenhouse_valid_companies_$TS.csv > all_valid_companies_$TS.csv
for f in greenhouse ashby lever; do tail -n +2 ${f}_valid_companies_$TS.csv >> all_valid_companies_$TS.csv; done
```

## Known POC limitations

- No dedup/persistence across runs — every run re-validates from scratch.
  The production design (issue #108) adds a `company_discovery_candidates`
  table with a unique index for that.
- Lever's total discoverable universe via Wayback is smaller than
  Greenhouse/Ashby's via Common Crawl regardless of pagination — Wayback's
  crawl frequency and depth for `jobs.lever.co` is simply lower.
