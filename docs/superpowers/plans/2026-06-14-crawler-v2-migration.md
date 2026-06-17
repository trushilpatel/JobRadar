# Crawler v2 Migration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `apps/crawler-v2` the single ATS adapter library for the whole pipeline, rewire crawler-node + orchestrator + filter + analyzer to v2's `core` types and two-phase contract, and delete the entire v1 `apps/crawler/` tree.

**Architecture:** v2 `core` types (`JobSummary`/`JobPost`) become canonical end-to-end. `core` relocates to `backend/common/ats/core` (neutral leaf) to fix a `service/→apps/` layering inversion. The crawler is two-phase: cheap `FetchJobListing` at crawl time, expensive `FetchJobPost` on demand in the analyzer. NATS wire shape changes (`full_content`→`job_content`); streams are purged at cutover.

**Tech Stack:** Go (backend module), NATS JetStream, k3s/Helm, golangci-lint, `go test -race`, coverage gate ≥95% per package (`scripts/check-coverage.sh`).

**Spec:** `docs/superpowers/specs/2026-06-14-crawler-v2-migration-design.md`

**Branch:** create `feat/crawler-v2-migration` off `dev` (never commit to `dev`/`main`).

---

## Conventions for every task

- **TDD:** write the failing test first, run it red, implement, run it green, then lint + commit.
- **Per-task verification commands** (run from `backend/`):
  - Package test: `go test ./<pkg>/... -run <TestName> -v`
  - Race (touched pkgs): `go test -race -short ./<pkg>/...`
  - Lint: `golangci-lint run --config ../.github/.golangci.yml --timeout 5m ./<pkg>/...`
  - Coverage (touched pkg): `go test -coverprofile=/tmp/cov.out ./<pkg>/... && scripts/check-coverage.sh /tmp/cov.out 95`
- **Commits:** conventional messages, no AI attribution. Commit after each task passes.
- **Read before editing:** for rewrite tasks, the executor reads the named current file first; the plan gives exact targets + the new code shape.
- **Phase gate:** after each phase, run the full backend build `go build ./...` and `go test -short ./...` — must be green before the next phase.

---

## File structure (decomposition)

New / moved:

- `backend/common/ats/core/{interfaces,types,constants,slog}.go` — moved from `apps/crawler-v2/core/` (package `core`).
- `backend/common/logger/{logger,zerolog,noop,fields}.go` — gains `WithContext` + new field constants.
- `apps/crawler-v2/adapters/registry.go` — gains `ErrInvalidAdapter` (registry-local).

Rewritten:

- `apps/crawler-node/{executor,types,main,config}.go`
- `service/crawlernode/dto/{WorkUnit,ResultsRequest}.go`, `service/crawlernode/services/CrawlerNodeImplService.go`
- `apps/filter/{worker,helpers}.go`
- `apps/analyzer/{fetcher,enricher,worker}.go`
- `service/job/dto/EnrichedJobInput.go`, `service/job/services/JobPipelineImplService.go`

Deleted (Phase D): entire `apps/crawler/`, `apps/crawler-v2/common/`, `infra/Dockerfile.crawler*`, Helm `templates/crawler/`, CI crawler jobs, Makefile crawler targets.

---

# Phase A — Foundation: relocate `core`, unify `common/`, fix mappers

This phase has zero behavior change to the running pipeline (v1 still live) and must compile before any rewire.

### Task A1: Move `core` to `backend/common/ats/core`

**Files:**

- Create: `backend/common/ats/core/interfaces.go`, `types.go`, `constants.go`, `slog.go` (moved content; package stays `core`)
- Delete: `backend/apps/crawler-v2/core/` (after move)
- Modify: every file importing `apps/crawler-v2/core` (all v2 adapters + registry)

- [ ] **Step 1: Move the directory and repoint imports**

```bash
cd backend
git mv apps/crawler-v2/core common/ats/core
# repoint every importer
grep -rl '"github.com/trushilpatel/jobradar/apps/crawler-v2/core"' --include=*.go . \
  | xargs sed -i '' 's#apps/crawler-v2/core#common/ats/core#g'
```

- [ ] **Step 2: Verify build + existing v2 tests pass**

Run: `go build ./... && go test -short ./common/ats/core/... ./apps/crawler-v2/...`
Expected: PASS (pure move, no logic change).

- [ ] **Step 3: Commit**

```bash
git add -A && git commit -m "refactor(crawler-v2): relocate core to common/ats/core (neutral leaf)"
```

### Task A2: Add `WithContext` + field constants to shared logger (atomic)

**Files:**

- Modify: `common/logger/logger.go` (interface), `common/logger/zerolog.go`, `common/logger/noop.go`, `common/logger/fields.go`
- Modify (test doubles): `common/tracing/tracing_test.go`, `apps/api/middleware/logger_test.go`, `apps/analyzer/extra_test.go`
- Test: `common/logger/logger_test.go`

- [ ] **Step 1: Write failing tests for `WithContext`**

In `common/logger/logger_test.go` add:

```go
func TestWithContext_NoSpan_ReturnsUsableLogger(t *testing.T) {
	l := NewNoop().WithContext(context.Background())
	l.Info("ok") // must not panic
}

func TestZerolog_WithContext_NoSpan(t *testing.T) {
	var buf bytes.Buffer
	l := newZerologTo(&buf, "test") // existing test ctor; adapt to current helper
	l.WithContext(context.Background()).Info("hello")
	if !strings.Contains(buf.String(), "hello") {
		t.Fatalf("missing log line: %s", buf.String())
	}
}
```

- [ ] **Step 2: Run, verify fail (method undefined)**

Run: `go test ./common/logger/... -run WithContext -v`
Expected: FAIL — `WithContext` not in interface.

- [ ] **Step 3: Add `WithContext` to the interface**

In `common/logger/logger.go`, add to `Logger`:

```go
	// WithContext returns a child logger with trace_id + span_id baked in from the
	// active OTel span in ctx. No-ops (returns an equivalent logger) when no span.
	WithContext(ctx context.Context) Logger
```

- [ ] **Step 4: Implement in zerolog + noop**

In `common/logger/zerolog.go` (mirror v2's deleted impl — use `trace.SpanFromContext`; if `span.SpanContext().IsValid()` bake `FieldTraceID`/`FieldSpanID` via `With`, else return self). In `common/logger/noop.go`:

```go
func (n noopLogger) WithContext(_ context.Context) Logger { return n }
```

- [ ] **Step 5: Add the 9 field constants**

In `common/logger/fields.go` add (do NOT re-add existing `FieldDuration`/`FieldOffset`/etc.):

```go
	FieldDepartment = "department"
	FieldJobCount   = "job_count"
	FieldPage       = "page"
	FieldSpanID     = "span_id"
	FieldStatusCode = "status_code"
	FieldTotal      = "total"
	FieldTotalPages = "total_pages"
	FieldTraceID    = "trace_id"
	FieldDurationMs = "duration_ms" // distinct from FieldDuration="duration"
```

- [ ] **Step 6: Add `WithContext` to the 3 test doubles**

In each of `common/tracing/tracing_test.go` (`captureLogger`), `apps/api/middleware/logger_test.go` (`spyLogger`), `apps/analyzer/extra_test.go` (`capturingLogger`):

```go
func (l *<doubleType>) WithContext(_ context.Context) logger.Logger { return l }
```

- [ ] **Step 7: Run all affected packages green**

Run: `go build ./... && go test -short ./common/logger/... ./common/tracing/... ./apps/api/middleware/... ./apps/analyzer/...`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add -A && git commit -m "feat(logger): add WithContext + crawler field constants to shared logger"
```

### Task A3: Repoint v2 adapters to shared logger; relocate `ErrInvalidAdapter`; delete v2 `common/`

**Files:**

- Modify: all `apps/crawler-v2/adapters/**` importing `apps/crawler-v2/common/logger`
- Modify: `apps/crawler-v2/adapters/registry.go`, `registry_test.go` (errors import)
- Delete: `apps/crawler-v2/common/` (logger, errors, telemetry)

- [ ] **Step 1: Move `ErrInvalidAdapter` registry-local**

In `apps/crawler-v2/adapters/registry.go` add `var ErrInvalidAdapter = errors.New("invalid adapter type")` (stdlib `errors`), replace the `crawlerrors.ErrInvalidAdapter` usage, drop the `common/errors` import. Update `registry_test.go` references.

- [ ] **Step 2: Repoint logger imports + fix `FieldDuration` call sites**

```bash
cd backend
grep -rl 'apps/crawler-v2/common/logger' --include=*.go apps/crawler-v2 \
  | xargs sed -i '' 's#apps/crawler-v2/common/logger#common/logger#g'
```

Then grep v2 adapter call sites that log milliseconds under `FieldDuration` and switch them to `FieldDurationMs` (they previously resolved to `"duration_ms"`):
`grep -rn 'FieldDuration' apps/crawler-v2/adapters` — change ms-integer sites to `FieldDurationMs`.

- [ ] **Step 3: Confirm telemetry has zero importers, then delete v2 common/**

Run: `grep -rl 'apps/crawler-v2/common' --include=*.go .` — expect only files inside `apps/crawler-v2/common/` itself.

```bash
git rm -r apps/crawler-v2/common
```

- [ ] **Step 4: Build + test v2 adapters green**

Run: `go build ./... && go test -short ./apps/crawler-v2/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "refactor(crawler-v2): unify onto shared common/, drop dead telemetry/errors"
```

### Task A4: Fix Workday/Eightfold listing `LocationKnown`; lock Meta

**Files:**

- Modify: `apps/crawler-v2/adapters/ats/workday/mapper.go`
- Modify (if v1-parity trace confirms): `apps/crawler-v2/adapters/ats/eightfold/mapper.go`
- Test: `apps/crawler-v2/adapters/ats/workday/mapper_test.go`, `.../meta/mapper_test.go`, `.../eightfold/mapper_test.go`

- [ ] **Step 1: Write/adjust failing mapper tests**

In `workday/mapper_test.go`, assert listing summary has `LocationKnown == false` and `AllLocations == nil`. In `meta/mapper_test.go`, assert `LocationKnown == false` (regression-lock). For eightfold, first read `eightfold/mapper.go:25` + the v1 `apps/crawler/ats/eightfold` to confirm v1 parity; if v1 left it false, add the same assertion.

- [ ] **Step 2: Run, verify Workday test fails**

Run: `go test ./apps/crawler-v2/adapters/ats/workday/... -run Summary -v`
Expected: FAIL (currently `LocationKnown=true`).

- [ ] **Step 3: Flip the mapper**

In `workday/mapper.go` (~line 75) remove `LocationKnown: true` (let it default false). Same for eightfold if confirmed.

- [ ] **Step 4: Run green + coverage**

Run: `go test ./apps/crawler-v2/adapters/ats/workday/... ./apps/crawler-v2/adapters/ats/meta/... ./apps/crawler-v2/adapters/ats/eightfold/... -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "fix(crawler-v2): defer Workday/Eightfold location to enrich-time country gate"
```

**Phase A gate:** `go build ./... && go test -short ./...` green.

---

# Phase B — Producer rewire: orchestrator DTOs + crawler-node executor

### Task B1: Collapse orchestrator DTOs into `core.JobSummary`

**Files:**

- Modify: `service/crawlernode/dto/ResultsRequest.go`, `service/crawlernode/dto/WorkUnit.go`
- Test: `service/crawlernode/services/tests/` (DTO marshalling)

- [ ] **Step 1: Write failing wire-shape test**

Add a test asserting `json.Marshal(core.JobSummary{...with JobContent...})` produces keys `id, company_id, adapter, title, location, url, employ_type, job_content{description,requirements,compensation,workplace_type,department,published_date}, location_known, all_locations`.

- [ ] **Step 2: Run, verify fail**

Run: `go test ./service/crawlernode/... -run Wire -v` → FAIL.

- [ ] **Step 3: Rewrite DTOs**

`ResultsRequest.go`: replace `Jobs []CrawledJob` with `Jobs []core.JobSummary`; remove `PreviousKnownIDs`; remove `CrawledJob`/`CrawledJobContent`. `WorkUnit.go`: remove `KnownIDs`. (Final shapes in spec §P3.)

- [ ] **Step 4: Run green**

Run: `go test ./service/crawlernode/... -run Wire -v` → PASS.

- [ ] **Step 5: Commit** (`refactor(orchestrator): adopt core.JobSummary in crawlernode DTOs`)

### Task B2: Orchestrator publishes `core.JobSummary`; server-side new/removed diff

**Files:**

- Modify: `service/crawlernode/services/CrawlerNodeImplService.go`
- Test: `service/crawlernode/services/tests/CrawlerNodeImplService_test.go`

- [ ] **Step 1: Write failing tests** for: (a) `SubmitResults` publishes only `!HasSeenBatch` jobs to `jobs.raw.<adapter>` with the `job_content` JSON; (b) removed-job detection runs only when `req.Final==true`, diffing `GetActiveJobIDsByCompany` ∖ submitted IDs; (c) the `fetch` ledger row is written before publish.

- [ ] **Step 2: Run red.**

- [ ] **Step 3: Implement** — delete `rawJobWire`/`fullContentWire`/`marshalJob`/`jsonMarshal`; `payload, _ := json.Marshal(summary)`; drop `KnownIDs` from `GetWork`; load previous active set in `SubmitResults` for removed diff gated on `Final`. Preserve ledger-before-publish ordering verbatim.

- [ ] **Step 4: Run green + coverage ≥95% for the package.**

- [ ] **Step 5: Commit** (`feat(orchestrator): marshal core.JobSummary, server-side new/removed diff`)

### Task B3: Rewrite crawler-node executor for v2 two-phase

**Files:**

- Modify: `apps/crawler-node/executor.go`, `types.go`, `main.go`, `config.go`
- Test: `apps/crawler-node/executor_test.go`

- [ ] **Step 1: Write failing executor tests** — using `adapters.Registry()` doubles: (a) thin adapter → `FetchJobListing` returns summaries, node submits `[]core.JobSummary` with `Final=true`, no `FetchJobPost` called; (b) configurable adapter with `AdapterConfig` → `FetchJobListingWithConfig`; (c) configurable adapter missing config → error reported, no partial submit; (d) empty listing → submit `Jobs: []`. Remove old `StreamPublisher`/`KnownJobsProvider`/`SeenChecker` tests.

- [ ] **Step 2: Run red.**

- [ ] **Step 3: Implement** — read current `executor.go`; replace the v1 `ats.Adapter` lookup with `adapters.Registry()` returning `AdapterBundle`; call `Listing.FetchJobListing` (or `ConfigListing.FetchJobListingWithConfig` when `AdapterConfig != nil`); map `JobListing.Jobs` → submit DTO; delete optional-interface wiring + `toIDSet`; `main.go` builds a configured `*http.Client` (timeout from config) and passes it to the registry constructors; `config.go` drops `GOOGLE_SCRAPER_*`.

- [ ] **Step 4: Run green + race + coverage.**

- [ ] **Step 5: Commit** (`feat(crawler-node): two-phase listing via crawler-v2 adapters`)

**Phase B gate:** `go build ./... && go test -short ./...` green.

---

# Phase C — Consumer rewire: filter, analyzer, job DTO/row

### Task C1: Filter deserializes `core.JobSummary`

**Files:**

- Modify: `apps/filter/worker.go`, `apps/filter/helpers.go`
- Test: `apps/filter/worker_test.go`, `worker_location_test.go`

- [ ] **Step 1: Write failing tests** — (a) a `jobs.raw.*` message with `core.JobSummary` JSON deserializes and processes; (b) `mergedLocations(core.JobSummary)` returns `AllLocations` when present, else `[Location]`; (c) **parity test:** `core.JobSummary{Adapter:"workday", LocationKnown:false, Location:"Bengaluru, India"}` is NOT routed to the non-USA bucket at filter stage.

- [ ] **Step 2: Run red.**

- [ ] **Step 3: Implement** — `var job core.JobSummary`; retype `chunkItem`, `publishDetail`, `mergedLocations`. Re-marshal preserves `job_content`.

- [ ] **Step 4: Run green + race + coverage.**

- [ ] **Step 5: Commit** (`feat(filter): consume core.JobSummary`)

### Task C2: Analyzer two-phase enrichment via `FetchJobPost`

**Files:**

- Modify: `apps/analyzer/fetcher.go`, `apps/analyzer/worker.go`, `apps/analyzer/enricher.go`
- Test: `apps/analyzer/fetcher_test.go`, `worker_test.go`

- [ ] **Step 1: Write failing tests** — (a) thin summary (`JobContent==nil`) triggers exactly one `FetchJobPost(summary)` (URL preserved), per-adapter rate-limit wait wraps it; (b) fat summary (`JobContent!=nil`) builds `core.JobPost` inline, no registry/PostFetcher call; (c) `JobContent==nil && bundle.Post==nil` routes to `handleFailure` (retry/DLQ), no empty enrich; (d) one job's `FetchJobPost` failure DLQs that job while others enrich.

- [ ] **Step 2: Run red.**

- [ ] **Step 3: Implement** — read current `fetcher.go`; replace `fetchContent` with registry-based `FetchJobPost`; **delete `fetchPage` + the HTTP-GET/`StripHTML` fallback**; use v2 `ats.StripHTML` where needed; retype `analyzerMergedLocations`.

- [ ] **Step 4: Run green + race + coverage.**

- [ ] **Step 5: Commit** (`feat(analyzer): two-phase FetchJobPost enrichment, drop HTML fallback`)

### Task C3: `EnrichedJobInput` + `buildJobRow` authority

**Files:**

- Modify: `service/job/dto/EnrichedJobInput.go`, `service/job/services/JobPipelineImplService.go`, `apps/analyzer/enricher.go` (`toEnrichedInput`)
- Test: `service/job/tests/JobPipelineImplService_test.go`

- [ ] **Step 1: Write failing test** — thin-adapter case: `Summary.Title==""`, `Post.Title=="Senior Engineer"` → built `jobs` row has `Title=="Senior Engineer"`; `EmployType`/`Location` follow the same JobPost-when-non-empty precedence; `URL`/`JobPostID`/`CompanyID` from summary; `Country`/`State` from the LLM result.

- [ ] **Step 2: Run red.**

- [ ] **Step 3: Implement** — `EnrichedJobInput{ Summary core.JobSummary; Post core.JobPost; Enriched *llmdto.JobPostEnrichResult }`; `buildJobRow` uses `coalesce(Post.X, Summary.X)` (TrimSpace) for Title/Location/EmployType; `toEnrichedInput` passes the fetched `core.JobPost`.

- [ ] **Step 4: Run green + coverage.**

- [ ] **Step 5: Commit** (`feat(job): enriched JobPost authority in buildJobRow`)

**Phase C gate:** `go build ./... && go test -short ./...` green; full race pass `go test -race -short ./...`.

---

# Phase D — Delete v1 + build/deploy/doc cleanup

### Task D1: Delete the v1 `apps/crawler/` tree

- [ ] **Step 1: Prove zero remaining importers**

Run: `grep -rl "jobradar/apps/crawler/" --include=*.go .`
Expected: nothing outside `apps/crawler/` itself. If any remain, fix before deleting.

- [ ] **Step 2: Delete + build**

```bash
git rm -r apps/crawler
go build ./... && go test -short ./...
```

Expected: PASS.

- [ ] **Step 3: Commit** (`refactor: remove v1 monolith crawler (apps/crawler)`)

### Task D2: Remove Dockerfiles, Helm, CI, Makefile crawler refs

**Files:** `infra/Dockerfile.crawler`, `infra/Dockerfile.crawler.ci`, `k8s/helm/jobradar/templates/crawler/`, `crawler:`/`images.crawler` blocks in all `values*.yaml`, `.github/workflows/release.yml` (crawler job/filter/tag-set/needs), `Makefile`.

Note: `.github/workflows/backend.yml` no longer exists (replaced by `ci.yml` + `release.yml`). The `test-crawler` job referenced in the original plan was removed when `backend.yml` was deleted.

- [ ] **Step 1: Remove Dockerfiles + Helm**

```bash
git rm infra/Dockerfile.crawler infra/Dockerfile.crawler.ci
git rm -r k8s/helm/jobradar/templates/crawler
```

Edit all `values*.yaml` to delete `crawler:` and `images.crawler` blocks.

- [ ] **Step 2: Edit CI** — in `release.yml` remove `crawler_changed` from `detect-changes`, the `crawler:`/`Dockerfile.crawler.ci` paths-filter entries, `go build ./apps/crawler` from the compile job, and the `build-crawler` job + its `needs` entries.

- [ ] **Step 3: Edit Makefile (must-not-miss)** — repoint `Makefile:111` integration path and `Makefile:188`/`:134`/`:146` digest paths to `apps/crawler-v2/adapters/ats/...`; remove `run-crawler`, `k8s-build-crawler`, `k8s-logs-crawler` + `.PHONY` entries.

- [ ] **Step 4: Validate** — `helm template k8s/helm/jobradar -f k8s/helm/jobradar/values.prod-app.yaml >/dev/null` (no crawler refs); `make k8s-build-crawler-node` digest still resolves; `yamllint`/`actionlint` on workflows if available.

- [ ] **Step 5: Commit** (`chore: remove crawler from infra/CI/Helm/Makefile`)

### Task D3: Docs

**Files:** `CLAUDE.md`, `infra/CLAUDE.md`, `docs/ARCHITECTURE.md`, delete `apps/crawler/CLAUDE.md` (gone with tree), `apps/crawler-v2/CLAUDE.md`.

- [ ] **Step 1:** Update root `CLAUDE.md` pipeline diagram + key rules (the crawler-image "pure Go" rule referencing `Dockerfile.crawler`); update `infra/CLAUDE.md` (drop crawler image/Deployment rows); update `docs/ARCHITECTURE.md`; set `apps/crawler-v2/CLAUDE.md` status to "wired in (canonical adapter library)".
- [ ] **Step 2: Commit** (`docs: reflect crawler-v2 as canonical adapter library`)

**Phase D gate:** `go build ./... && go test -short ./... && go test -race -short ./...` green; `golangci-lint run` clean.

---

# Phase E — Cutover (ops runbook, executed at deploy time)

Not code — follow the spec's "NATS cutover runbook" exactly:

- [ ] Quiesce producers (scheduler → orchestrator → crawler-node to 0 replicas).
- [ ] Drain consumers (filter → analyzer to 0).
- [ ] Purge JetStream `JOBS_RAW`, `JOBS_FILTERED`, `jobs.dlq` (purge, not delete).
- [ ] `DELETE FROM job_pipeline_ledger WHERE reason IN ('fetch','retry','filtered')` (verify `HasSeenBatch` treats absence as not-seen so re-discovery works).
- [ ] Deploy all new images in one Helm upgrade (unique tags; confirm pod image digests). Standalone `jobradar-crawler` Deployment disappears.
- [ ] Scale up consumer-first: analyzer → filter → orchestrator → crawler-node → scheduler.
- [ ] Verify one end-to-end job: scan → `jobs.raw` → `jobs.filtered` → enriched → `jobs` row.

---

## Self-review checklist (run before execution)

- **Spec coverage:** every spec component-change + decision (P1 core relocation A1; P3 logger A2/A3; telemetry/errors A3; mapper fixes A4; orchestrator B1/B2; crawler-node B3; filter C1; analyzer C2; job DTO/row C3; deletion D1–D3; cutover E) maps to a task. ✓
- **Placeholder scan:** rewrite tasks name exact files + the new code shape + concrete test assertions; no "handle edge cases" hand-waves. Where full code depends on the current file, the step instructs reading it and gives the precise transformation.
- **Type consistency:** `core.JobSummary`/`core.JobPost`/`AdapterBundle`/`EnrichedJobInput{Summary,Post,Enriched}` used consistently across B/C tasks.
- **Parallelization note:** Phase A is sequential (A1→A2→A3→A4). Within Phase B and C, tasks share files in `service/crawlernode` and `apps/analyzer`, so serialize B1→B2→B3 and C1, C2→C3. Phases must run in order (B depends on A; C depends on B's wire shape; D depends on C).
