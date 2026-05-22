---
name: jobradar-data-quality
description: Use when checking JobRadar data quality, diagnosing pipeline anomalies, auditing job enrichment completeness, verifying company scan health, inspecting filter/enricher correctness, or producing a health report on jobs and companies data
---

# JobRadar Data Quality Checker

## Overview

Structured audit of jobs, companies, pipeline ledger, enrichment fields, and user state against known invariants. Produces a prioritized anomaly report.

## Connect to DB

```bash
psql "$(grep '^SUPABASE_DB_URL=' /Users/trushil/Projects/JobRadar/.env | cut -d= -f2-)"
```

Run each section below. Collect results. Report at the end using the **Anomaly Report Format**.

---

## 1. Pipeline Ledger Integrity

**1a. Orphaned jobs (no ledger entry)**

```sql
SELECT COUNT(*) AS orphaned_jobs
FROM jobs j
LEFT JOIN job_pipeline_ledger l ON l.job_id = j.id
WHERE l.job_id IS NULL;
-- Expected: 0. Any value = job inserted bypassing the ledger.
```

**1b. Ledger reason distribution**

```sql
SELECT reason, COUNT(*) AS cnt,
  ROUND(100.0 * COUNT(*) / SUM(COUNT(*)) OVER (), 1) AS pct
FROM job_pipeline_ledger
GROUP BY reason ORDER BY cnt DESC;
-- Flag: llm_irrelevant > 80% = filter too aggressive
-- Flag: llm_irrelevant < 10% = filter not working (almost everything passing)
-- Flag: error > 5% = systematic processing failures
-- Flag: non_usa_location > 20% = location parsing too aggressive
```

**1c. Stuck fetch (filter backlog > 30 min)**

```sql
SELECT COUNT(*) AS stuck_awaiting_filter
FROM job_pipeline_ledger
WHERE reason = 'fetch'
  AND first_seen_at < NOW() - INTERVAL '30 minutes';
-- Expected: 0. Any value = filter worker not consuming NATS.
```

**1d. Stuck filtered (enricher backlog > 30 min)**

```sql
SELECT COUNT(*) AS stuck_awaiting_enrichment
FROM job_pipeline_ledger
WHERE reason = 'filtered'
  AND first_seen_at < NOW() - INTERVAL '30 minutes';
-- Expected: 0. Any value = enricher worker not consuming NATS.
```

**1e. Overdue retries**

```sql
SELECT COUNT(*) AS overdue_retries
FROM job_pipeline_ledger
WHERE reason = 'retry' AND next_attempt_at < NOW();
-- Expected: 0. Any value = retry worker stalled.
```

**1f. Error rate last 24h**

```sql
SELECT
  COUNT(*) FILTER (WHERE reason = 'error') AS errors_24h,
  COUNT(*) AS total_24h,
  ROUND(100.0 * COUNT(*) FILTER (WHERE reason = 'error') / NULLIF(COUNT(*), 0), 1) AS error_pct
FROM job_pipeline_ledger
WHERE first_seen_at > NOW() - INTERVAL '24 hours';
-- Flag: error_pct > 5% = systematic issue
```

---

## 2. Job Enrichment Quality

**2a. Null enrichment fields**

```sql
SELECT
  COUNT(*) AS total_jobs,
  COUNT(*) FILTER (WHERE experience_years IS NULL)            AS missing_exp_years,
  COUNT(*) FILTER (WHERE tech_stack IS NULL OR tech_stack = '[]'::jsonb) AS missing_tech_stack,
  COUNT(*) FILTER (WHERE country IS NULL)                     AS missing_country,
  COUNT(*) FILTER (WHERE llm_summary IS NULL OR llm_summary = '') AS missing_summary,
  COUNT(*) FILTER (WHERE role_brief IS NULL OR role_brief = '') AS missing_role_brief,
  ROUND(100.0 * COUNT(*) FILTER (WHERE experience_years IS NULL) / NULLIF(COUNT(*),0), 1) AS exp_null_pct,
  ROUND(100.0 * COUNT(*) FILTER (WHERE country IS NULL) / NULLIF(COUNT(*),0), 1) AS country_null_pct
FROM jobs;
-- Flag: exp_null_pct > 30% = enricher not extracting experience
-- Flag: country_null_pct > 15% = location extraction failing
-- Flag: missing_summary > 10% = LLM enricher not producing summaries
```

**2b. Contradictory enrichment flags**

```sql
SELECT COUNT(*) AS contradictory_flags
FROM jobs
WHERE requires_citizen = TRUE AND sponsorship = TRUE;
-- Expected: 0. These are mutually exclusive — any value = LLM inconsistency.
```

**2c. Jobs without analytics snapshot**

```sql
SELECT COUNT(*) AS jobs_without_analytics
FROM jobs j
LEFT JOIN job_analytics a ON a.id = j.id
WHERE a.id IS NULL;
-- Expected: 0. Every job written by the enricher should produce a job_analytics row.
```

**2d. Analytics vs jobs field drift**

```sql
SELECT
  COUNT(*) FILTER (WHERE j.requires_citizen != a.requires_citizen) AS citizen_drift,
  COUNT(*) FILTER (WHERE j.sponsorship != a.sponsorship)           AS sponsorship_drift,
  COUNT(*) FILTER (WHERE j.remote != a.remote)                     AS remote_drift,
  COUNT(*) FILTER (WHERE j.experience_years IS DISTINCT FROM a.experience_years) AS exp_drift
FROM jobs j
JOIN job_analytics a ON a.id = j.id;
-- Expected: 0 each. Any drift = analytics snapshot not updated on re-enrichment.
```

**2e. Filter pass rate sanity**

```sql
WITH c AS (
  SELECT
    COUNT(*) FILTER (WHERE reason = 'llm_irrelevant')               AS irrelevant,
    COUNT(*) FILTER (WHERE reason = 'non_usa_location')             AS non_usa,
    COUNT(*) FILTER (WHERE reason NOT IN ('llm_irrelevant','non_usa_location','user_dismissed')) AS other,
    (SELECT COUNT(*) FROM jobs)                                     AS in_jobs
  FROM job_pipeline_ledger
)
SELECT
  irrelevant, non_usa, other, in_jobs,
  irrelevant + non_usa + other + in_jobs AS total_seen,
  ROUND(100.0 * irrelevant / NULLIF(irrelevant + non_usa + other + in_jobs, 0), 1) AS irrelevant_pct,
  ROUND(100.0 * non_usa    / NULLIF(irrelevant + non_usa + other + in_jobs, 0), 1) AS non_usa_pct
FROM c;
-- Flag: irrelevant_pct > 80% = over-filtering
-- Flag: irrelevant_pct < 10% = under-filtering
-- Flag: non_usa_pct > 25% = location gate too aggressive
```

**2f. Employ type distribution (spot check)**

```sql
SELECT employ_type, COUNT(*) AS cnt
FROM jobs
GROUP BY employ_type ORDER BY cnt DESC;
-- Flag: NULL employ_type > 20% = enricher not classifying job type
-- Flag: unexpected values = constant drift between enricher output and DB enum
```

**2g. Experience years distribution**

```sql
SELECT
  experience_years,
  COUNT(*) AS cnt,
  ROUND(100.0 * COUNT(*) / SUM(COUNT(*)) OVER (), 1) AS pct
FROM jobs
GROUP BY experience_years
ORDER BY experience_years NULLS LAST;
-- Spot check: 0-2 (junior), 2-5 (mid), 5+ (senior) should be roughly balanced
-- Flag: all NULL = enricher not running or field not persisted
-- Flag: all 0 = LLM defaulting to 0 instead of NULL when unknown
```

---

## 3. Company Scan Health

**3a. Companies missing scan settings**

```sql
SELECT c.id, c.name
FROM companies c
LEFT JOIN company_scan_settings s ON s.company_id = c.id
WHERE s.company_id IS NULL;
-- Expected: 0. Missing settings = company will never be scanned.
```

**3b. High consecutive error companies**

```sql
SELECT c.name, s.adapter_type, s.consecutive_errors,
  s.last_error_message, s.last_error_at
FROM company_scan_settings s
JOIN companies c ON c.id = s.company_id
WHERE s.consecutive_errors >= 3
ORDER BY s.consecutive_errors DESC;
-- Flag: consecutive_errors >= 5 = adapter likely broken
-- Flag: same error_message across multiple companies = shared infra issue
```

**3c. Active companies never scanned (added > 24h ago)**

```sql
SELECT c.name, s.adapter_type, c.created_at
FROM company_scan_settings s
JOIN companies c ON c.id = s.company_id
WHERE s.active = TRUE
  AND s.last_scan_at IS NULL
  AND c.created_at < NOW() - INTERVAL '24 hours';
-- Expected: 0. Scheduler should have picked these up within 24h.
```

**3d. Companies overdue for scan (2× interval elapsed)**

```sql
SELECT c.name, s.adapter_type,
  s.last_scan_at,
  COALESCE(s.scan_interval_override, 1440) AS interval_min,
  ROUND(EXTRACT(EPOCH FROM (NOW() - s.last_scan_at))/60) AS minutes_since_scan
FROM company_scan_settings s
JOIN companies c ON c.id = s.company_id
WHERE s.active = TRUE
  AND s.last_scan_at IS NOT NULL
  AND s.last_scan_at < NOW() - (COALESCE(s.scan_interval_override, 1440) * 2 || ' minutes')::INTERVAL
ORDER BY minutes_since_scan DESC;
-- Any row here = scheduler or worker not processing on schedule.
```

**3e. Stuck crawler (running with stale heartbeat > 5 min)**

```sql
SELECT q.id, c.name, q.started_at, q.heartbeat_at,
  ROUND(EXTRACT(EPOCH FROM (NOW() - q.heartbeat_at))/60) AS minutes_stale
FROM company_scan_queue q
JOIN companies c ON c.id = q.company_id
WHERE q.status = 'running'
  AND q.heartbeat_at < NOW() - INTERVAL '5 minutes';
-- Expected: 0. Any row = crashed crawler; ResetStuck should have cleared it.
```

**3f. Queue depth**

```sql
SELECT status, COUNT(*) AS cnt
FROM company_scan_queue
GROUP BY status;
-- Flag: queued > 50 = workers falling behind
-- Flag: running > 5 = possible zombie entries (check heartbeat above)
```

**3g. Active companies successfully scanned but with zero jobs**

```sql
SELECT c.name, s.adapter_type, s.last_success_at,
  COUNT(j.id) AS job_count
FROM company_scan_settings s
JOIN companies c ON c.id = s.company_id
LEFT JOIN jobs j ON j.company_id = c.id
WHERE s.active = TRUE AND s.last_success_at IS NOT NULL
GROUP BY c.name, s.adapter_type, s.last_success_at
HAVING COUNT(j.id) = 0;
-- Flag: adapter crawls succeed but returns 0 jobs consistently = adapter parsing broken
```

**3h. Scan success rate by adapter (last 7 days)**

```sql
SELECT
  s.adapter_type,
  COUNT(*) AS total_scans,
  COUNT(*) FILTER (WHERE l.error IS NULL) AS successful,
  ROUND(100.0 * COUNT(*) FILTER (WHERE l.error IS NULL) / NULLIF(COUNT(*), 0), 1) AS success_pct,
  ROUND(AVG(l.duration_ms)) AS avg_ms
FROM company_scan_log l
JOIN company_scan_settings s ON s.company_id = l.company_id
WHERE l.scanned_at > NOW() - INTERVAL '7 days'
GROUP BY s.adapter_type
ORDER BY success_pct ASC;
-- Flag: success_pct < 90% for any adapter = rate-limited or broken
```

**3i. Recent scan error messages**

```sql
SELECT l.error, s.adapter_type, COUNT(*) AS occurrences
FROM company_scan_log l
JOIN company_scan_settings s ON s.company_id = l.company_id
WHERE l.error IS NOT NULL
  AND l.scanned_at > NOW() - INTERVAL '24 hours'
GROUP BY l.error, s.adapter_type
ORDER BY occurrences DESC
LIMIT 20;
```

---

## 4. User State Integrity

**4a. Status/timestamp mismatches**

```sql
SELECT
  COUNT(*) FILTER (WHERE status = 'saved'     AND saved_at IS NULL)     AS saved_no_ts,
  COUNT(*) FILTER (WHERE status = 'applied'   AND applied_at IS NULL)   AS applied_no_ts,
  COUNT(*) FILTER (WHERE status = 'dismissed' AND dismissed_at IS NULL) AS dismissed_no_ts
FROM user_job_state;
-- Expected: 0 each. Timestamp must be set when status transitions.
```

**4b. Orphaned application tracker rows**

```sql
SELECT COUNT(*) AS tracker_without_state
FROM job_application_tracker t
LEFT JOIN user_job_state ujs
  ON ujs.job_id = t.job_result_id AND ujs.user_id = t.user_id
WHERE ujs.user_id IS NULL;
-- Expected: 0. Tracker stages imply user_job_state row exists.
```

**4c. User job status distribution**

```sql
SELECT status, COUNT(*) AS cnt
FROM user_job_state
GROUP BY status ORDER BY cnt DESC;
-- Spot check: dismissed should not dominate >80% — would indicate jobs irrelevant to users
-- Flag: large 'dismissed' rate = filter/enrichment may be letting through poor-quality jobs
```

---

## 5. Data Freshness

**5a. New jobs per day — last 7 days**

```sql
SELECT DATE(added_at) AS day, COUNT(*) AS jobs_added
FROM jobs
WHERE added_at > NOW() - INTERVAL '7 days'
GROUP BY 1 ORDER BY 1 DESC;
-- Flag: any day with 0 = pipeline likely had an outage that day
-- Flag: sudden drop vs previous days = crawler or filter issue
```

**5b. Companies with last_error_at > last_success_at**

```sql
SELECT c.name, s.adapter_type, s.last_error_at, s.last_success_at, s.last_error_message
FROM company_scan_settings s
JOIN companies c ON c.id = s.company_id
WHERE s.last_error_at > s.last_success_at;
-- Any row = most recent scan for this company failed.
```

---

## 6. Anomaly Report Format

After running all sections, produce this report:

```
## JobRadar Data Quality Report — <YYYY-MM-DD HH:MM>

### CRITICAL  (immediate action required)
- <description> | affected: <N> | hypothesis: <cause>

### WARNING  (investigate within 24h)
- <description> | affected: <N>

### INFO  (no action needed)
- Filter irrelevant rate: X% (normal: 20–60%)
- New jobs last 24h: N
- Total jobs: N | Total companies: N

### PASSED
- ✓ No orphaned jobs
- ✓ No stuck crawler entries
- ✓ No contradictory enrichment flags
- ...
```

---

## Thresholds

| Check                                 | Normal | Warning | Critical       |
| ------------------------------------- | ------ | ------- | -------------- |
| Orphaned jobs (no ledger)             | 0      | 1–5     | > 5            |
| Stuck fetch > 30min                   | 0      | 1–10    | > 10           |
| Stuck filtered > 30min                | 0      | 1–10    | > 10           |
| Overdue retries                       | 0      | 1–5     | > 5            |
| Error rate 24h                        | < 2%   | 2–5%    | > 5%           |
| Filter irrelevant rate                | 20–60% | 60–80%  | > 80% or < 10% |
| Non-USA filtered rate                 | < 15%  | 15–25%  | > 25%          |
| Experience years null                 | < 20%  | 20–40%  | > 40%          |
| Country null                          | < 15%  | 15–30%  | > 30%          |
| Contradictory flags (citizen+sponsor) | 0      | —       | any            |
| Jobs without analytics                | 0      | 1–5     | > 5            |
| Companies missing settings            | 0      | —       | any            |
| Consecutive scan errors               | 0–2    | 3–4     | ≥ 5            |
| Stuck crawler (stale heartbeat)       | 0      | —       | any            |
| Active companies w/ 0 jobs            | 0      | 1–2     | > 2            |
| Adapter scan success rate             | > 95%  | 90–95%  | < 90%          |
| Queue depth (queued)                  | < 20   | 20–50   | > 50           |
| User dismiss rate                     | < 50%  | 50–80%  | > 80%          |

---

## Self-Improvement

After this skill completes, invoke `skill-self-improvement` to check whether any finding warrants updating this file or `CLAUDE.md`. Focus especially on: threshold calibration from real observed data, new anomaly patterns discovered, schema changes (new tables/fields), or queries that produced false positives.
