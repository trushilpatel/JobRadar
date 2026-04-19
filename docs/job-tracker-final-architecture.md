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
│                      SCHEDULER                          │
│  - Queries DB every 1 min for companies due for scan    │
│  - Due = last_scan_at is NULL or older than             │
│    SCAN_INTERVAL_MINUTES config value                   │
│  - Publishes company IDs to company.scan_job            │
└────────────────────────┬────────────────────────────────┘
                         │
                         ▼
            ┌────────────────────────┐
            │    company.scan_job    │  Kafka topic
            │    { company_id }      │
            └────────────┬───────────┘
                         │
┌────────────────────────▼────────────────────────────────┐
│                     SCAN ENGINE                         │
│  - Consumes company IDs from company.scan_job           │
│  - Fetches careers page for that company                │
│  - Checks each job ID against job_seen_ledger           │
│  - Skips known IDs, pushes new ones to Kafka            │
│  - Writes scan summary to job_scan_log                  │
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
│  - Sends title + id only to LLM                         │
│  - Relevant   → publishes to jobs.raw_detail            │
│  - Irrelevant → updates job_seen_ledger                 │
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
│  - Sends full text to LLM                               │
│  - LLM extracts: experience years, tech stack,          │
│    salary, role brief, responsibilities,                │
│    sponsorship/citizen/clearance flags, summary         │
│  - Writes final row to job (DB)                         │
│  - Updates job_seen_ledger reason = 'fetch'             │
│  - On failure: job_seen_ledger reason = 'error'         │
└─────────────────────────────────────────────────────────┘
```

---

## 3. Kafka Topics

| Topic | Producer | Consumer | Carries |
|---|---|---|---|
| `company.scan_job` | Scheduler | Scan engine | `{ company_id }` — triggers a scan for one company |
| `jobs.raw` | Scan engine | LLM batch scorer | Full raw job payload (id, title, location, url, department, employ_type) |
| `jobs.raw_detail` | LLM batch scorer | Detail fetcher + LLM enricher | Relevant raw job payload (same struct, scorer-filtered) |

**No dead letter topic.** Failures are written directly to `job_scan_log.error` and `job_seen_ledger` with `reason = 'error'`. Simple, queryable, no extra infrastructure.

---

## 4. Database Tables

### 4.1 `companies`
Configuration table. Read by the scheduler and scan engine to know what to fetch.

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
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_scan_at      TIMESTAMPTZ,
    last_scan_status  TEXT
                          CHECK(last_scan_status IN ('ok', 'error', 'pending'))
);
```

**Why this and nothing more:**
- `adapter_type` tells the registry which adapter to use — must be stored, can't be derived
- `last_scan_at` + `last_scan_status` are for the dashboard status indicators and scheduler due-check
- No scan metrics here — those live in `job_scan_log`

---

### 4.2 `job_seen_ledger`
The deduplication gate. Every job ever touched by the pipeline gets a row here. Checked before any job enters Kafka.

```sql
CREATE TABLE job_seen_ledger (
    job_id        TEXT NOT NULL PRIMARY KEY,  -- e.g. "greenhouse-12345"
    company_id    TEXT NOT NULL REFERENCES companies(id),
    first_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    reason        TEXT NOT NULL
                      CHECK(reason IN (
                          'fetch',            -- successfully processed
                          'llm_irrelevant',   -- LLM batch scorer rejected
                          'user_dismissed',   -- user dismissed from UI
                          'error'             -- fetch or enrichment failed
                      ))
);

CREATE INDEX idx_seen_company ON job_seen_ledger(company_id);
```

**Why this and nothing more:**
- Only needs the ID for the lookup — O(1) check per job
- `reason` tells you why it was seen — useful for debugging and future re-processing
- No job content here — content travels through Kafka, not stored twice

---

### 4.3 `job_scan_log`
Append-only record of every scan run per company. Powers dashboard activity feed and weekly chart.

```sql
CREATE TABLE job_scan_log (
    id            BIGSERIAL PRIMARY KEY,
    company_id    TEXT NOT NULL REFERENCES companies(id),
    scanned_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    jobs_fetched  INTEGER NOT NULL DEFAULT 0,
    jobs_new      INTEGER NOT NULL DEFAULT 0,
    error         TEXT,
    duration_ms   INTEGER
);

CREATE INDEX idx_scan_company ON job_scan_log(company_id);
CREATE INDEX idx_scan_time    ON job_scan_log(scanned_at DESC);
```

**Why this and nothing more:**
- `jobs_fetched` vs `jobs_new` tells you how many were deduplicated
- `error` replaces dead letter topic — failure reason is queryable here
- `duration_ms` shows which companies are slow to scan
- Never updated, only inserted

---

### 4.4 `job`
The only table the UI reads for the main jobs feed. Written once by the enrichment worker. After that, only `status`, `saved`, and `notes` change.

```sql
CREATE TABLE job (
    id              TEXT PRIMARY KEY,
    job_post_id     TEXT NOT NULL,           -- matches job_seen_ledger.job_id
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

    -- LLM-enriched details (written once by enricher, never updated)
    experience_years    INTEGER,
    tech_stack          JSONB,               -- ["Go", "Kafka", "PostgreSQL"]
    salary_range        JSONB,               -- {"min": 120000, "max": 160000, "currency": "USD"}
    llm_summary         TEXT,               -- 2–3 sentence digest shown in UI card
    sponsorship         BOOLEAN NOT NULL DEFAULT FALSE,  -- mentions H1B/visa support
    requires_citizen    BOOLEAN NOT NULL DEFAULT FALSE,  -- US Citizen/Green Card required
    security_clearance  BOOLEAN NOT NULL DEFAULT FALSE,  -- Secret, TS/SCI, etc.
    role_brief          TEXT,               -- one-sentence technical mission
    responsibilities    TEXT,               -- newline-separated technical bullet points

    -- user state (only these change after insert)
    status          TEXT NOT NULL DEFAULT 'new'
                        CHECK(status IN ('new', 'saved', 'applied')),
    saved           BOOLEAN NOT NULL DEFAULT FALSE,
    notes           TEXT,

    added_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_job_company ON job(company_id);
CREATE INDEX idx_job_status  ON job(status);
CREATE INDEX idx_job_saved   ON job(saved);
CREATE INDEX idx_job_added   ON job(added_at DESC);
```

**Why this and nothing more:**
- `experience_years` replaces the previous `years_exp_min/max` split — enricher extracts a single minimum requirement
- `sponsorship`, `requires_citizen`, `security_clearance` are hard filter flags — LLM extracts these to let the UI/API filter ineligible jobs instantly
- `role_brief` + `responsibilities` give the user a fast technical read without opening the full posting
- No `match_score` — LLM already determined relevance, user decides priority
- `status` only covers pre-application states — once `applied`, `job_application` takes over

---

### 4.5 `job_application`
Only created when the user applies. Tracks the full application lifecycle as an ordered series of stage transitions with timestamps.

```sql
CREATE TABLE job_application (
    id            BIGSERIAL PRIMARY KEY,
    job_id        TEXT NOT NULL REFERENCES job(id),
    stage         TEXT NOT NULL
                      CHECK(stage IN (
                          'applied',
                          'phone_screen',
                          'technical',
                          'final_round',
                          'offer',
                          'rejected'
                      )),
    stage_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    note          TEXT
);

CREATE INDEX idx_app_job     ON job_application(job_id);
CREATE INDEX idx_app_stage   ON job_application(stage);
CREATE INDEX idx_app_time    ON job_application(stage_at DESC);
```

**Why this and nothing more:**
- One row per stage transition — gives full history with timestamps
- `note` per stage — user can record what happened at each step
- Drives the dashboard activity feed: "Phone screen at Stripe · 2 days ago"
- `job.status` stays `applied` once this table has any row for that job
- Never updated, only inserted — each new stage is a new row

---

### 4.6 `job_discarded`
Holds jobs the user explicitly dismissed from the UI. Separate from `job_seen_ledger` because the user may want to review what they dismissed and why.

```sql
CREATE TABLE job_discarded (
    id            BIGSERIAL PRIMARY KEY,
    job_id        TEXT NOT NULL REFERENCES job(id),
    reason        TEXT NOT NULL
                      CHECK(reason IN (
                          'user_dismissed',    -- user hit dismiss in UI
                          'user_not_relevant'  -- user marked as not relevant
                      )),
    discarded_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    note          TEXT
);

CREATE INDEX idx_discarded_job ON job_discarded(job_id);
```

**Why separate from `job_seen_ledger`:**
- `job_seen_ledger` is a pipeline gate — it only cares about the job ID
- `job_discarded` is a user-facing record — it holds the reason and optional note
- When a job is dismissed, both tables get written:
  - `job_seen_ledger` → `reason = 'user_dismissed'` (blocks re-entry to pipeline)
  - `job_discarded` → full record (user can review later)

---

## 5. Constrained Values Summary

All valid values enforced via SQL CHECK constraints and mirrored as Go typed constants in `internal/model/constants.go`.

| Table | Column | Valid Values |
|---|---|---|
| `companies` | `adapter_type` | `greenhouse` · `lever` · `ashby` · `workday` · `workable` · `chromedp` |
| `companies` | `last_scan_status` | `ok` · `error` · `pending` |
| `job_seen_ledger` | `reason` | `fetch` · `llm_irrelevant` · `user_dismissed` · `error` |
| `job` | `employ_type` | `full_time` · `part_time` · `contract` · `internship` |
| `job` | `status` | `new` · `saved` · `applied` |
| `job_application` | `stage` | `applied` · `phone_screen` · `technical` · `final_round` · `offer` · `rejected` |
| `job_discarded` | `reason` | `user_dismissed` · `user_not_relevant` |

---

## 6. Data Flow Per Job (End to End)

```
1. Scheduler (every 1 min) queries companies table
   → finds companies where last_scan_at IS NULL
     or last_scan_at < NOW() - SCAN_INTERVAL_MINUTES
   → PUBLISH to company.scan_job for each due company

2. Scan engine consumes company.scan_job
   → fetches careers page for that company
   → for each job on the page:
       CHECK job_seen_ledger: known? skip. new? continue.
       INSERT job_seen_ledger (reason = 'fetch')
       PUBLISH to jobs.raw
   → INSERT job_scan_log row (fetched, new, duration)
   → UPDATE companies SET last_scan_at, last_scan_status

3. LLM batch scorer consumes jobs.raw (batch of 50–100)
   → sends title + id to LLM
   → irrelevant: UPDATE job_seen_ledger reason = 'llm_irrelevant'
   → relevant:   PUBLISH to jobs.raw_detail

4. Detail fetcher + LLM enricher consumes jobs.raw_detail
   → fetches full job page
   → on fetch error: UPDATE job_seen_ledger reason = 'error'
   → on success: sends full text to LLM
               → LLM returns structured extraction
               → INSERT job row
               → UPDATE job_seen_ledger reason = 'fetch' (confirmed)

5. User opens UI, sees job feed
   → saves a job:    UPDATE job SET saved = TRUE
   → dismisses:      INSERT job_discarded
                     UPDATE job_seen_ledger reason = 'user_dismissed'
   → applies:        UPDATE job SET status = 'applied'
                     INSERT job_application (stage = 'applied')
   → stage update:   INSERT job_application (stage = 'phone_screen' etc.)
   → adds note:      UPDATE job SET notes = '...'
```

---

## 7. Final Count

| Type | Count | Names |
|---|---|---|
| Kafka topics | 3 | `company.scan_job` · `jobs.raw` · `jobs.raw_detail` |
| Binaries | 5 | scheduler · scan engine · LLM batch scorer · detail fetcher + LLM enricher · api |
| DB tables | 6 | `companies` · `job_seen_ledger` · `job_scan_log` · `job` · `job_application` · `job_discarded` |

---

## 8. What Lives Where

| Concern | Lives In |
|---|---|
| What to scan and when | `companies` + scheduler |
| Have I seen this job? | `job_seen_ledger` |
| Why was it rejected by LLM? | `job_seen_ledger.reason = 'llm_irrelevant'` |
| Scan metrics and errors | `job_scan_log` |
| Job content + enrichment | `job` |
| Visa / clearance eligibility | `job.sponsorship`, `job.requires_citizen`, `job.security_clearance` |
| User's pre-apply actions | `job.status` + `job.saved` |
| Full application history | `job_application` |
| User-dismissed jobs | `job_discarded` + `job_seen_ledger` |
| Data in transit between workers | Kafka topics |
