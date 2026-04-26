# Reliability & Infrastructure Hardening

**Date:** 2026-04-26  
**Scope:** CI/CD test gate, worker panic recovery + malformed message DLQ, alerting (5 critical rules), Helm hardening

---

## #1 — CI/CD Test Gate

**Files:** `.github/workflows/lint.yml`, `.github/workflows/build.yml`

Add `go test ./...` as a step in `lint.yml` immediately after the existing `golangci-lint` step. All existing tests use in-memory stubs — no real infra required.

Add `needs: lint` to the deploy job in `build.yml` so a failing test or lint check blocks deployment.

**Constraints:**
- Integration tests (`make test-adapters`) are excluded — they hit real ATS APIs and must stay manual
- Frontend lint/tsc already run in CI; no change there

---

## #2 — Worker Panic Recovery + Malformed Message DLQ

**Files:** `apps/crawler/workers/base.go`, `apps/filter/worker.go`, `apps/analyzer/worker.go`

### Panic recovery

Wrap the per-iteration body in each worker loop with `defer/recover`. On panic: log the stack trace at ERROR level, mark the job `reason='error'` in the ledger where a `job_id` is available, then continue the loop. Recovery must check `ctx.Err()` — do not restart if the context is cancelled (shutdown in progress).

- Crawler: wrap `scanner.RunForCompany()` call — no ledger write (job_id not yet known)
- Filter: wrap `processChunk()` — write `reason='error'` for all jobs in the chunk
- Analyzer: wrap `processGroup()` — write `reason='error'` for all jobs in the group

### Malformed message DLQ

Both filter and analyzer currently warn-log and skip unparseable Kafka records. Change to: publish the raw record bytes to the existing DLQ topic with a `reason=malformed` header, then commit the offset. No new topic needed. No ledger write (no `job_id` available from a malformed record).

---

## #3 — Alerting (5 Critical Rules)

**Files:** `k8s/helm/observability/values.yaml`, `k8s/helm/observability/templates/prometheus-rules.yaml` (new)

Enable alertmanager in `values.yaml` with a webhook receiver (URL injected via `values.local.yaml` — not committed). Create a `PrometheusRule` CRD with these 5 rules:

| Rule | Metric | Condition | Severity |
|------|--------|-----------|----------|
| DLQ spike | `filter_dlq_publishes_total` | rate > 1/min for 5min | critical |
| Consumer lag | `kafka_consumer_group_lag` | > 10k for 5min | warning |
| LLM exhaustion | `filter_llm_duration_seconds` p99 | > 60s | warning |
| Crawler scan timeout | `crawler_scan_duration_seconds` p95 | > 300s | warning |
| API 5xx rate | `http_requests_total{status=~"5.."}` / total | > 1% for 2min | critical |

**Constraints:**
- Consumer lag rule requires `kafka_consumer_group_lag` to be exposed — confirm metric exists before writing the rule; if absent, stub the rule as a comment with a TODO
- Alert thresholds are initial values; expect tuning after first production observation

---

## #4 — Helm Hardening

**Files:** `k8s/helm/jobradar/values.yaml`, `k8s/helm/jobradar/templates/crawler/deployment.yaml`, `k8s/helm/jobradar/templates/migrate/job.yaml`, `.gitignore`

### Parameterize secrets
- Remove `supabaseProjectUrl` and `allowedEmail` hardcoded values from `values.yaml`
- Add placeholder comments pointing to `values.local.yaml`
- Add `values.local.yaml` to `.gitignore` if not already present
- Document the pattern in `k8s/helm/jobradar/README.md` or a comment block in `values.yaml`

### CPU limit on crawler
- Add `resources.limits.cpu: "1000m"` to the crawler deployment
- Memory limit stays at 3Gi (Chromium requires it; fits within 8GB M1 budget)

### Migrate job resilience
- Increase `backoffLimit` from 3 → 10
- Add `activeDeadlineSeconds: 300` (5 min ceiling — prevents hanging indefinitely)

---

## Out of scope

- Memory limit increases (constrained by 8GB M1 running full k3d stack)
- Integration test automation in CI
- Huma migration, LLM cost metrics, service layer tracing (deferred to future iterations)
