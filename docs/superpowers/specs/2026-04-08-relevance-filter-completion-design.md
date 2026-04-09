# Design: Complete job_listing_relevance_filter

**Date:** 2026-04-08  
**Scope:** Fix two LLM scorer bugs and add unit tests for the relevance filter worker.

---

## Context

The `job_listing_relevance_filter` worker is structurally complete. It consumes
`job_listings.raw`, scores jobs in batches via LLM, publishes relevant jobs to
`job_listings.filtered`, and marks irrelevant/errored jobs in `job_seen_ledger`.

Three things remain to make it production-ready.

---

## Fix 1 — Gemini ScoreBatch prompt format bug

**File:** `backend/internal/llm/gemini.go`

**Problem:** `fmt.Sprintf` has 2 `%s` placeholders but 3 arguments: `roles`,
`s.yearsExp` (int), `sb.String()`. The int consumes the second `%s`, so the job
list never lands in the prompt body; it spills as `%!(EXTRA string=...)` at the
end. The LLM sees no job titles.

Additionally, years-of-experience is hardcoded as `"~4.5 years (mid-level)"`
instead of using the `yearsExp` field from config.

**Fix:** Add a `%d` verb for `yearsExp`, replacing the hardcoded text. The three
format verbs (`%s`, `%d`, `%s`) now match the three arguments (`roles`,
`s.yearsExp`, `sb.String()`).

```go
// Before
`- Target roles: %s
- Years of experience: ~4.5 years (mid-level)
...
Job list:
%s`, roles, s.yearsExp, sb.String()

// After
`- Target roles: %s
- Years of experience: %d+ years
...
Job list:
%s`, roles, s.yearsExp, sb.String()
```

---

## Fix 2 — Anthropic ScoreBatch MaxTokens

**File:** `backend/internal/llm/anthropic.go`

**Problem:** `MaxTokens: 1024` is too low for a batch of up to 50 job IDs. A
full batch response can exceed 1024 tokens, causing truncated JSON that fails to
parse.

**Fix:** Set `MaxTokens: 5000`.

---

## Fix B — Worker unit tests

**File:** `backend/internal/worker/job_listing_relevance_filter/worker_test.go`

Three lightweight fakes defined in the test file (no external mocking library):

| Fake | Interface | Behaviour |
|---|---|---|
| `fakeScorer` | `llm.Scorer` | Returns configurable `relevantIDs`; can be set to return an error |
| `fakeLedger` | `store.SeenLedgerWriter` | Records all `UpdateSeenReasonBatch` calls |
| `fakePublisher` | `kafka.Publisher` | Records all published payloads |

Four test cases covering `flushBatch` directly:

| Test | Scorer result | Expected outcome |
|---|---|---|
| `TestFlushBatch_AllRelevant` | Returns all IDs | All jobs published; ledger not called |
| `TestFlushBatch_PartialRelevant` | Returns subset of IDs | Relevant published; irrelevant marked `llm_irrelevant` |
| `TestFlushBatch_ScorerError` | Returns error | All jobs marked `error`; nothing published |
| `TestFlushBatch_EmptyBatch` | — | Scorer never called; no publishes; no ledger calls |

Tests call `flushBatch` directly (it is an unexported method on `*Worker` —
tests are in the same package `job_listing_relevance_filter`).

---

## What is NOT in scope

- No changes to `job_post_analyzer` or any downstream worker.
- No changes to Kafka wiring, config, or migrations.
- No integration test against real Kafka/LLM — the unit tests use fakes.
