# Pipeline Ledger Reason Gap After Kafka→NATS Migration

**Date:** 2026-05-22  
**Symptom:** "Awaiting Filter" showed 1617 — far more than expected. "Awaiting Enrichment" was always 0.  
**Fixed in:** PR #62 — migration 058, `apps/filter/worker.go`, `service/dashboard/`

---

## What Was Wrong

### Bug 1 — Filter worker never marked relevant jobs in the ledger

The filter worker correctly stamped terminal reasons on jobs it rejected:

| Outcome          | Ledger reason set  |
| ---------------- | ------------------ |
| Non-USA location | `non_usa_location` |
| LLM-irrelevant   | `llm_irrelevant`   |
| Retry            | `retry`            |
| DLQ / error      | `error`            |

But for **relevant** jobs (published to `jobs.filtered.*`) it set **nothing**. They stayed as `reason='fetch'` until the enricher wrote them to the `jobs` table.

The materialized view `mv_pipeline_db_stats` counted `pipeline_pending` as:

```sql
SELECT COUNT(*) AS pipeline_pending
FROM job_pipeline_ledger lp
LEFT JOIN jobs jt ON jt.job_post_id = lp.job_id
WHERE lp.reason = 'fetch'
  AND jt.job_post_id IS NULL
```

So `pipeline_pending` = **awaiting filter** + **awaiting enrichment** combined. The UI labeled the whole number "Awaiting Filter".

### Bug 2 — `awaiting_enrichment` always 0 after NATS migration

`DashboardPipelineImplService` sourced `awaitingEnrichment` from a Kafka consumer-group lag reader:

```go
awaitingEnrichment, err := s.lag.GetLag(ctx, "job-post-analyzer", kafka.TopicJobListingsFiltered)
```

After Kafka was replaced with NATS JetStream, the wired `LagReader` became a noop:

```go
func (*noopLagReader) GetLag(_ context.Context, _, _ string) (int64, error) { return 0, nil }
```

So `awaiting_enrichment` in the API response was permanently 0 regardless of queue depth.

---

## The Fix

### 1. New ledger reason: `filtered`

Added `SeenReasonFiltered = "filtered"` to `service/job/common/JobServiceConstants.go`.

Filter worker now stamps this on **all** jobs that pass through to `jobs.filtered.*`:

```go
// relevant (scored by LLM)
if len(relevantIDs) > 0 {
    w.ledger.UpdateSeenReasonBatch(ctx, relevantIDs, jobcommon.SeenReasonFiltered)
}

// pass-through (no title — skips LLM, published directly)
if len(passThroughIDs) > 0 {
    w.ledger.UpdateSeenReasonBatch(ctx, passThroughIDs, jobcommon.SeenReasonFiltered)
}
```

### 2. Migration 058 — widen CHECK + add `pipeline_filtered` column

```sql
ALTER TABLE job_pipeline_ledger
    ADD CONSTRAINT job_pipeline_ledger_reason_check
    CHECK (reason IN ('fetch','filtered','llm_irrelevant','user_dismissed','error','retry','non_usa_location'));
```

`mv_pipeline_db_stats` gains a `pipeline_filtered` column:

```sql
(SELECT COUNT(*) AS pipeline_filtered
 FROM job_pipeline_ledger lf
 LEFT JOIN jobs jt ON jt.job_post_id = lf.job_id
 WHERE lf.reason = 'filtered'
   AND jt.job_post_id IS NULL) f
```

### 3. Dashboard service uses DB for `awaiting_enrichment`

```go
// Before (dead noop):
awaitingEnrichment, _ := s.lag.GetLag(ctx, "job-post-analyzer", kafka.TopicJobListingsFiltered)

// After (accurate):
awaitingEnrichment := dbStats.PipelineFiltered
```

---

## Resulting Pipeline State Semantics

| Ledger reason      | Meaning                                                                   |
| ------------------ | ------------------------------------------------------------------------- |
| `fetch`            | Crawled, published to `JOBS_RAW`, not yet consumed by filter              |
| `filtered`         | Passed filter, published to `JOBS_FILTERED`, not yet consumed by enricher |
| `llm_irrelevant`   | Rejected by LLM relevance scoring — terminal                              |
| `non_usa_location` | Rejected pre-LLM because location is non-USA — terminal                   |
| `retry`            | Failed enrichment, in NATS retry backoff — transient                      |
| `error`            | Exhausted all retries, routed to DLQ — terminal                           |
| `user_dismissed`   | User dismissed from the UI — terminal                                     |

---

## How to Diagnose Similar Issues in Future

If a pipeline counter looks inflated or always zero:

1. **Check what the DB query actually counts.** Look at `mv_pipeline_db_stats` and `DashboardRepositoryService.go`. The view definition is the source of truth.
2. **Check what the filter/enricher worker writes to the ledger.** Workers that publish without updating the ledger leave jobs in an ambiguous state that inflates earlier-stage counts.
3. **Check whether lag readers are noops.** After infrastructure migrations (Kafka→NATS, etc.) lag readers wired in `apps/api/wire.go` may be stubs. Grep for `noopLagReader` or `GetLag` returning constant 0.
4. **Query the ledger directly** to see the distribution of `reason` values and spot which stage is accumulating.
