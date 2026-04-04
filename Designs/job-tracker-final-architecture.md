# JobRadar — Final Architecture & Schema

---

## 1. System Overview

JobRadar is a single-user automated job tracking system that monitors up to 100+ company careers pages, filters jobs through an LLM pipeline, and surfaces only relevant enriched results to the user.

**Core design principles:**
- Kafka owns data in transit between workers
- DB owns only what must be queryable or permanent
- Every constrained column has a fixed set of valid values enforced at both DB and application layer
- No table exists without a clear, single reason

---

## 2. Pipeline Architecture

```
┌─────────────────────────────────────────────────────────┐
│                     SCAN ENGINE                         │
│  - Reads company list from DB                           │
│  - Fetches all careers pages concurrently               │
│  - Checks each job ID against seen_ledger               │
│  - Skips known IDs, pushes new ones to Kafka            │
│  - Writes scan summary to scan_log                      │
└────────────────────────┬────────────────────────────────┘
                         │
                         ▼
               ┌──────────────────┐
               │   jobs.raw       │  Kafka topic
               │  id · title      │  full job payload
               │  location · url  │
               └────────┬─────────┘
                        │
┌───────────────────────▼─────────────────────────────────┐
│                  LLM BATCH SCORER                        │
│  - Consumes jobs.raw in batches of 50–100               │
│  - Sends title + id only to Claude                      │
│  - Relevant   → publishes to jobs.raw_detail            │
│  - Irrelevant → writes directly to seen_ledger          │
│                 reason = 'llm_irrelevant'                │
└───────────────┬─────────────────────────────────────────┘
                │ relevant only
                ▼
      ┌──────────────────────┐
      │   jobs.raw_detail    │  Kafka topic
      │   relevant job ids   │
      └──────────┬───────────┘
                 │
┌────────────────▼────────────────────────────────────────┐
│               DETAIL FETCHER + LLM ENRICHER             │
│  - Fetches full job page per job (network I/O)          │
│  - Sends full text to Claude                            │
│  - Claude extracts: years exp, tech stack,              │
│    salary, team info, summary, other criteria           │
│  - Writes final row to job_result (DB)                  │
│  - Updates seen_ledger reason = 'fetch'                 │
│  - On failure (404/timeout): writes to scan_log error   │
│    and seen_ledger reason = 'error'                     │
└─────────────────────────────────────────────────────────┘
```

---

## 3. Kafka Topics

| Topic | Producer | Consumer | Carries |
|---|---|---|---|
| `jobs.raw` | Scan engine | LLM batch scorer | Full job payload (id, title, location, url, department) |
| `jobs.raw_detail` | LLM batch scorer | Detail fetcher + LLM enricher | Relevant job IDs only |

**No dead letter topic.** Failures are written directly to `scan_log.error` and `seen_ledger` with `reason = 'error'`. Simple, queryable, no extra infrastructure.

---

## 4. Database Tables

### 4.1 `companies`
Configuration table. Read by scan engine to know what to fetch.

```sql
CREATE TABLE companies (
    id                TEXT PRIMARY KEY,
    name              TEXT NOT NULL,
    url               TEXT NOT NULL,
    adapter_type      TEXT NOT NULL
                          CHECK(adapter_type IN (
                              'greenhouse', 'lever', 'ashby',
                              'workday', 'workable', 'chromedp'
                          )),
    active            BOOLEAN NOT NULL DEFAULT TRUE,
    created_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_scan_at      DATETIME,
    last_scan_status  TEXT
                          CHECK(last_scan_status IN ('ok', 'error', 'pending'))
);
```

**Why this and nothing more:**
- `adapter_type` tells the registry which adapter to use — must be stored, can't be derived
- `last_scan_at` + `last_scan_status` are for the dashboard status indicators
- No scan metrics here — those live in `scan_log`

---

### 4.2 `seen_ledger`
The deduplication gate. Every job ever touched by the pipeline gets a row here. Checked before any job enters Kafka.

```sql
CREATE TABLE seen_ledger (
    job_id        TEXT NOT NULL PRIMARY KEY,  -- e.g. "greenhouse-12345"
    company_id    TEXT NOT NULL REFERENCES companies(id),
    first_seen_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    reason        TEXT NOT NULL
                      CHECK(reason IN (
                          'fetch',            -- successfully processed
                          'llm_irrelevant',   -- LLM batch scorer rejected
                          'user_dismissed',   -- user dismissed from UI
                          'error'             -- fetch or enrichment failed
                      ))
);

CREATE INDEX idx_seen_company ON seen_ledger(company_id);
```

**Why this and nothing more:**
- Only needs the ID for the lookup — O(1) check per job
- `reason` tells you why it was seen — useful for debugging and future re-processing
- No job content here — content travels through Kafka, not stored twice

---

### 4.3 `scan_log`
Append-only record of every scan run per company. Powers dashboard activity feed and weekly chart.

```sql
CREATE TABLE scan_log (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    company_id    TEXT NOT NULL REFERENCES companies(id),
    scanned_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    jobs_fetched  INTEGER NOT NULL DEFAULT 0,
    jobs_new      INTEGER NOT NULL DEFAULT 0,
    error         TEXT,
    duration_ms   INTEGER
);

CREATE INDEX idx_scan_company ON scan_log(company_id);
CREATE INDEX idx_scan_time    ON scan_log(scanned_at DESC);
```

**Why this and nothing more:**
- `jobs_fetched` vs `jobs_new` tells you how many were deduplicated
- `error` replaces dead letter topic — failure reason is queryable here
- `duration_ms` shows which companies are slow to scan
- Never updated, only inserted

---

### 4.4 `job_result`
The only table the UI reads for the main jobs feed. Written once by the enrichment worker. After that, only `status`, `saved`, and `notes` change.

```sql
CREATE TABLE job_result (
    id              TEXT PRIMARY KEY,
    job_id          TEXT NOT NULL,           -- matches seen_ledger.job_id
    company_id      TEXT NOT NULL REFERENCES companies(id),

    -- job identity (from Kafka payload)
    title           TEXT NOT NULL,
    location        TEXT,
    remote          BOOLEAN NOT NULL DEFAULT FALSE,
    url             TEXT NOT NULL,
    department      TEXT,
    employ_type     TEXT
                        CHECK(employ_type IN (
                            'full_time', 'part_time', 'contract', 'internship'
                        )),

    -- enriched details (written by LLM enricher, never updated)
    years_exp_min   INTEGER,
    years_exp_max   INTEGER,
    tech_stack      TEXT,        -- JSON array: ["Go", "Kafka", "PostgreSQL"]
    salary_range    TEXT,        -- JSON: {"min": 120000, "max": 160000, "currency": "USD"}
    llm_summary     TEXT,        -- 2–3 sentence digest shown in UI card
    other_criteria  TEXT,        -- JSON: any other structured fields LLM found

    -- user state (only these three change after insert)
    status          TEXT NOT NULL DEFAULT 'new'
                        CHECK(status IN ('new', 'saved', 'applied')),
    saved           BOOLEAN NOT NULL DEFAULT FALSE,
    notes           TEXT,

    added_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_result_company ON job_result(company_id);
CREATE INDEX idx_result_status  ON job_result(status);
CREATE INDEX idx_result_saved   ON job_result(saved);
CREATE INDEX idx_result_added   ON job_result(added_at DESC);
```

**Why this and nothing more:**
- No `match_score` or `match_breakdown` — LLM already determined relevance, user decides priority
- No `enriched_at` — `added_at` is sufficient, enrichment and insert happen atomically
- `status` only covers pre-application states — once `applied`, `job_application` takes over
- `saved` is a simple boolean toggle separate from status — user can save without applying
- `tech_stack`, `salary_range`, `other_criteria` are JSON strings — flexible for varied job formats

---

### 4.5 `job_application`
Only created when the user applies. Tracks the full application lifecycle as an ordered series of stage transitions with timestamps.

```sql
CREATE TABLE job_application (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    job_result_id TEXT NOT NULL REFERENCES job_result(id),
    stage         TEXT NOT NULL
                      CHECK(stage IN (
                          'applied',
                          'phone_screen',
                          'technical',
                          'final_round',
                          'offer',
                          'rejected'
                      )),
    stage_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    note          TEXT
);

CREATE INDEX idx_app_job     ON job_application(job_result_id);
CREATE INDEX idx_app_stage   ON job_application(stage);
CREATE INDEX idx_app_time    ON job_application(stage_at DESC);
```

**Why this and nothing more:**
- One row per stage transition — gives full history with timestamps
- `note` per stage — user can record what happened at each step
- Drives the dashboard activity feed: "Phone screen at Stripe · 2 days ago"
- `job_result.status` stays `applied` once this table has any row for that job
- Never updated, only inserted — each new stage is a new row

---

### 4.6 `discarded`
Holds jobs the user explicitly dismissed from the UI. Separate from `seen_ledger` because the user may want to review what they dismissed and why.

```sql
CREATE TABLE discarded (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    job_result_id TEXT NOT NULL REFERENCES job_result(id),
    reason        TEXT NOT NULL
                      CHECK(reason IN (
                          'user_dismissed',    -- user hit dismiss in UI
                          'user_not_relevant'  -- user marked as not relevant
                      )),
    discarded_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    note          TEXT
);

CREATE INDEX idx_discarded_job ON discarded(job_result_id);
```

**Why separate from `seen_ledger`:**
- `seen_ledger` is a pipeline gate — it only cares about the job ID
- `discarded` is a user-facing record — it holds the reason and optional note
- When a job is dismissed, both tables get written:
  - `seen_ledger` → `reason = 'user_dismissed'` (blocks re-entry to pipeline)
  - `discarded` → full record (user can review later)

---

## 5. Constrained Values Summary

All valid values enforced via SQL CHECK constraints and mirrored as Go typed constants in `internal/model/constants.go`.

| Table | Column | Valid Values |
|---|---|---|
| `companies` | `adapter_type` | `greenhouse` · `lever` · `ashby` · `workday` · `workable` · `chromedp` |
| `companies` | `last_scan_status` | `ok` · `error` · `pending` |
| `seen_ledger` | `reason` | `fetch` · `llm_irrelevant` · `user_dismissed` · `error` |
| `job_result` | `employ_type` | `full_time` · `part_time` · `contract` · `internship` |
| `job_result` | `status` | `new` · `saved` · `applied` |
| `job_application` | `stage` | `applied` · `phone_screen` · `technical` · `final_round` · `offer` · `rejected` |
| `discarded` | `reason` | `user_dismissed` · `user_not_relevant` |

---

## 6. Data Flow Per Job (End to End)

```
1. Scan engine fetches job from careers page
   → CHECK seen_ledger: known? skip. new? continue.
   → INSERT seen_ledger (reason = 'fetch') ← tentative, updated later
   → PUBLISH to jobs.raw
   → INSERT scan_log row

2. LLM batch scorer consumes jobs.raw (batch of 50–100)
   → sends title + id to Claude
   → irrelevant: UPDATE seen_ledger reason = 'llm_irrelevant'
   → relevant:   PUBLISH to jobs.raw_detail

3. Detail fetcher + LLM enricher consumes jobs.raw_detail
   → fetches full job page
   → on error: UPDATE seen_ledger reason = 'error'
               UPDATE scan_log error column
   → on success: sends full text to Claude
               → Claude returns structured extraction
               → INSERT job_result
               → UPDATE seen_ledger reason = 'fetch' (confirmed)

4. User opens UI, sees job_result feed
   → saves a job:    UPDATE job_result SET saved = TRUE
   → dismisses:      INSERT discarded
                     UPDATE seen_ledger reason = 'user_dismissed'
   → applies:        UPDATE job_result SET status = 'applied'
                     INSERT job_application (stage = 'applied')
   → stage update:   INSERT job_application (stage = 'phone_screen' etc.)
   → adds note:      UPDATE job_result SET notes = '...'
                     or INSERT job_application with note on stage row
```

---

## 7. Final Count

| Type | Count | Names |
|---|---|---|
| Kafka topics | 2 | `jobs.raw` · `jobs.raw_detail` |
| Workers | 3 | scan engine · LLM batch scorer · detail fetcher + LLM enricher |
| DB tables | 6 | `companies` · `seen_ledger` · `scan_log` · `job_result` · `job_application` · `discarded` |

---

## 8. What Lives Where

| Concern | Lives In |
|---|---|
| What to scan | `companies` |
| Have I seen this job? | `seen_ledger` |
| Why was it rejected by LLM? | `seen_ledger.reason = 'llm_irrelevant'` |
| Scan metrics and errors | `scan_log` |
| Job content + enrichment | `job_result` |
| User's pre-apply actions | `job_result.status` + `job_result.saved` |
| Full application history | `job_application` |
| User-dismissed jobs | `discarded` + `seen_ledger` |
| Data in transit between workers | Kafka topics |
