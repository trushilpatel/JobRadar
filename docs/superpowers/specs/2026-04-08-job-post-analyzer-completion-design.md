# Design: Complete job_post_analyzer

**Date:** 2026-04-08  
**Scope:** Fix four gaps in the enricher worker. ATS scope: Greenhouse, Lever, Ashby only.

---

## Gaps

### 1. `Remote` field never set

`EnrichResult` has no `Remote` field. `buildJob` always sets `Remote: false`.

Fix: Add `Remote bool` to `EnrichResult`. Update both LLM prompts (Anthropic + Gemini)
to extract `remote` as a boolean. `country` remains the location restriction (e.g. `"USA"`
for "Remote - US only", `""` for globally remote). Update `buildJob` and `InsertSnapshot`
to use `enriched.Remote`.

### 2. Dead ledger update

`UpdateSeenReason(job.ID, SeenReasonFetch)` at the end of a successful `enrich` is a no-op.
The crawler already set the ledger reason to `"fetch"`. The pipeline-pending stat relies on the
absence of a `job` row, not the ledger reason. Remove this call.

### 3. Shared `http.Client`

`fetchPage` creates a new `http.Client{Timeout: 15s}` on every call. Move it to a field on
`Worker`, initialized in `NewWorker`. `fetchPage` becomes a method receiving the shared client.

### 4. Unit tests

- `buildJob` — pure function, table-driven test covering field mapping and nullable strings
- `enrich` — uses fakes for `llm.Scorer`, `store.JobWriter`, `store.SeenLedgerWriter`,
  `store.SnapshotWriter`. Cases: happy path, fetch error, LLM error, insert error.

---

## Files changed

| File | Change |
|---|---|
| `internal/llm/llm.go` | Add `Remote bool` to `EnrichResult` |
| `internal/llm/anthropic.go` | Add `"remote"` field to Enrich prompt + parse |
| `internal/llm/gemini.go` | Same |
| `internal/worker/job_post_analyzer/worker.go` | Fix Remote, remove dead ledger call, shared client |
| `internal/worker/job_post_analyzer/worker_test.go` | New test file |

---

## Out of scope

- Workday, Workable, Chromedp adapters
- HTTP fallback path improvements
