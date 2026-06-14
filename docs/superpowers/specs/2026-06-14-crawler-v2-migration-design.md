# Crawler v2 Migration — Design

Date: 2026-06-14
Status: Approved (design)

## Goal

Make `apps/crawler-v2` the single ATS adapter library for the whole pipeline, rewire
the runtime (crawler-node + orchestrator) and consumers (filter, analyzer) to v2's
`core` types and two-phase contract, and delete the entire v1 `apps/crawler/` tree
(monolith engine, workers, and the v1 `ats` library).

## Background

Three crawler generations currently coexist:

- **v1 monolith** — `apps/crawler/` (engine + workers + `ats/` adapters). Runs as the
  standalone `jobradar-crawler` Deployment. The `ats/` library is still shared.
- **distributed runtime** — `apps/crawler-node/` + orchestrator (`service/crawlernode/`).
  Live, but reuses v1's `apps/crawler/ats` adapters.
- **v2 adapter library** — `apps/crawler-v2/`. Clean-room rewrite (own `core/` interfaces,
  own `common/`, cleaner per-adapter layout). Fully built with ≥95% tests, **not wired in**.

v2's CLAUDE.md states it is the intended adapter library for the orchestration-based crawler.

## Key decisions (settled)

1. **Wire contract:** adopt v2 `core.JobSummary` (listing/wire) and `core.JobPost`
   (enrichment) as canonical types end-to-end. The NATS JSON shape changes
   (`full_content` → `job_content`; adds `workplace_type`, `published_date`). Acceptable
   because the deployment is still being brought up — streams will be purged at cutover.
2. **Crawl model:** drop v1's crawl-time `StreamPublisher` (per-page streaming) and
   `KnownJobsProvider`/`SeenChecker` (known-jobs skip). Rely on v2's two-phase model:
   cheap `FetchJobListing` → summaries; expensive `FetchJobPost` only for jobs that are
   new (orchestrator ledger diff) AND passed the filter (in the analyzer). Fat adapters
   (Gem, Uber, Deel) still return content at listing time.
3. **common/ unification:** repoint v2 adapters at the shared backend `common/logger`;
   **delete** v2's `common/telemetry` outright (zero importers — shared `common/tracing`
   already covers it) and v2's `common/errors` (only `ErrInvalidAdapter` is live → moves
   registry-local into `apps/crawler-v2/adapters`). Single source of truth. Requires
   additive changes to the shared logger (see Logger section).
4. **`core` relocation:** move `apps/crawler-v2/core` → `backend/common/ats/core` (neutral
   leaf, package `core`). Fixes the `service/ → apps/` layering inversion and lets the
   orchestrator import `core` directly and delete its hand-rolled `rawJobWire`/`marshalJob`.

## Target data flow

```
scheduler → company_scan_queue → orchestrator ⇄ crawler-node
   crawler-node: adapters.Registry() → core.ListingFetcher.FetchJobListing → []core.JobSummary
                 (ConfigurableListingFetcher for Workday/Eightfold)
   orchestrator: ledger diff (publish new only) → jobs.raw.* (core.JobSummary JSON)
   filter:       reads core.JobSummary → jobs.filtered.*
   analyzer:     reads core.JobSummary; if JobContent==nil → core.PostFetcher.FetchJobPost(summary)
                 → core.JobPost → enrich → jobs
```

## Component changes (file-level)

- **crawler-node** — `apps/crawler-node/executor.go`, `types.go`, `main.go`, `config.go`:
  rewrite executor against `adapters.Registry()` + `core.ListingFetcher` /
  `core.ConfigurableListingFetcher`; submit `core.JobSummary`; remove the optional-interface
  wiring (`StreamPublisher`/`KnownJobsProvider`/`SeenChecker`/`ConfigurableAdapter`) and
  `toIDSet`; replace `google.Config` plumbing with a configured `*http.Client` (drop
  `GOOGLE_SCRAPER_*` runtime knobs — v2 google uses internal delay defaults + client timeout).
  Delete `WorkUnit.KnownIDs` and `ResultsRequest.PreviousKnownIDs` from the node round-trip.

- **orchestrator** — `service/crawlernode/dto/{WorkUnit,ResultsRequest}.go`,
  `service/crawlernode/services/CrawlerNodeImplService.go`: collapse `CrawledJob`/`CrawledJobContent`
  into `core.JobSummary`/`core.JobContent`; **delete** `rawJobWire`/`fullContentWire`/`marshalJob`/
  `jsonMarshal` and `json.Marshal(core.JobSummary)` directly (`job_content` key). Drop `KnownIDs`
  from `GetWork`; compute new jobs via `HasSeenBatch` and removed jobs via
  `GetActiveJobIDsByCompany ∖ submitted-IDs` **server-side, gated on `Final==true`**. The
  "ledger `fetch` row written BEFORE publish" ordering invariant is preserved verbatim.

- **filter** — `apps/filter/helpers.go`, `worker.go`: deserialize `core.JobSummary` from
  `jobs.raw.*`; `mergedLocations` retyped to `core.JobSummary`. The non-USA gate (`IsNonUSA`,
  `service/job/location`) is string-based and unchanged; only the plumbing feeding it changes.

- **analyzer** — `apps/analyzer/fetcher.go`, `enricher.go`, `worker.go`: deserialize
  `core.JobSummary`; enrichment uses `adapters.Registry()` → `core.PostFetcher.FetchJobPost(summary)`
  when `JobContent==nil`, else builds `core.JobPost` from inline content; use v2 `ats.StripHTML`;
  `analyzerMergedLocations` retyped. **Delete the generic HTTP-GET+StripHTML fallback**
  (`fetchPage`, fetcher.go ~210-247): `JobContent==nil` with no usable `PostFetcher`, or any
  `FetchJobPost` error, routes through `handleFailure` → retry/DLQ (never a phantom/empty enrich).
  Per-job failure isolation (per-goroutine `fetchOne`) unchanged.

- **service/job** — `service/job/dto/EnrichedJobInput.go` carries `Summary core.JobSummary` +
  `Post core.JobPost` + the LLM `JobPostEnrichResult`. `JobPipelineImplService.buildJobRow`
  authority: `Title`/`Location`/`EmployType` = JobPost when non-empty (TrimSpace) else summary;
  `URL`/`JobPostID`/`CompanyID` from summary; `Country`/`State`/derived fields from the LLM result.
  Fixes empty-title rows for Meta/Workday (whose summaries lack title/location).

- **logger/common unification** — add `WithContext(ctx) Logger` to shared
  `common/logger/logger.go` + implement in **all 6 implementers** (`zerolog.go`, `noop.go`,
  and the 3 test doubles in `common/tracing/tracing_test.go`, `apps/api/middleware/logger_test.go`,
  `apps/analyzer/extra_test.go`; the v2 zerolog impl is deleted). Add the 8 missing field
  constants to `common/logger/fields.go` (`FieldDepartment`, `FieldJobCount`, `FieldPage`,
  `FieldSpanID`, `FieldStatusCode`, `FieldTotal`, `FieldTotalPages`, `FieldTraceID`) **plus
  `FieldDurationMs = "duration_ms"`** (distinct from existing `FieldDuration = "duration"` — the
  v2 ms-integer sites map to `FieldDurationMs`). Do these as ONE atomic commit, then repoint v2
  adapter imports, then delete `apps/crawler-v2/common/`.

## Adapter mapper fixes (non-USA gate parity)

- **Workday** `apps/crawler-v2/adapters/ats/workday/mapper.go`: set listing `LocationKnown=false`
  (currently `true` with empty `AllLocations`, which wrongly enables filter-stage string rejection;
  v1 deferred Workday to the enrich-time `CountryCode` gate). Lock with mapper unit test +
  a filter-package test asserting Workday listings bypass the filter-stage non-USA gate.
- **Meta** already correct (`LocationKnown=false` at listing) — add a regression-lock test only.
- **Eightfold** `apps/crawler-v2/adapters/ats/eightfold/mapper.go:25` — trace v1 parity during
  edge-case discovery; flip to `false` if it has the same premature-`LocationKnown` issue.

## NATS cutover runbook (atomic, single Helm release)

Schema change (`full_content`→`job_content` + new fields) is breaking for in-flight messages.
Ordered cutover:

1. Quiesce producers (scale to 0): scheduler → orchestrator → crawler-node.
2. Drain consumers (scale to 0): filter → analyzer.
3. Purge JetStream streams: `JOBS_RAW`, `JOBS_FILTERED`, **and `jobs.dlq`** (DLQ carries old-shape
   payloads too). Purge, don't delete — keep stream config/consumers.
4. Clear in-flight ledger rows: `DELETE FROM job_pipeline_ledger WHERE reason IN
('fetch','retry','filtered')` so the next scan re-discovers them cleanly. (Terminal rows —
   `saved`/`llm_irrelevant`/`error`/`non_usa_location` — untouched. Implementation verifies
   `HasSeenBatch` treats these as not-seen so re-publish works.) Runs strictly between step 1
   and step 6, never with old code still writing (per the cleanup-migration deploy-gap rule).
5. Deploy all new images together (single Helm upgrade) — removes the standalone `jobradar-crawler`
   Deployment. Verify pod image digests (unique tag, not same-tag rebuild).
6. Scale up consumer-first: analyzer → filter → orchestrator → crawler-node → scheduler.
7. Verify one end-to-end job: scan → raw → filtered → enriched → `jobs` row.

## Deletion + build/deploy cleanup

- Delete the entire `apps/crawler/` tree (engine, workers, interfaces, main, v1 `ats/`).
- Remove `infra/Dockerfile.crawler` and `infra/Dockerfile.crawler.ci`.
- Remove Helm `k8s/helm/jobradar/templates/crawler/` and all `crawler:` value blocks
  (`values.yaml`, `values.local.yaml`, `values.prod-app.yaml`, `values.prod-infra.yaml`).
- `.github/workflows/deploy.yml`: remove `crawler_changed` output, the `crawler:` +
  `Dockerfile.crawler.ci` paths-filter entries, the two `steps.filter.outputs.crawler` `if`
  conditions, `go build ./apps/crawler`, the whole `build-crawler` job, its `needs` entry, the
  `crawler=$(get_tag …)` line, and `--set images.crawler.tag=…`.
- `.github/workflows/backend.yml`: delete the `test-crawler` job (crawler-v2 job covers it).
- **Makefile (must-not-miss silent-fail repoints):** `Makefile:111` integration path
  `./apps/crawler/ats/...` → `apps/crawler-v2/adapters/ats/...`; `Makefile:188` (and `:134`/`:146`)
  crawler-node/image digest paths → `apps/crawler-v2/adapters/ats/*`. Remove `run-crawler`,
  `k8s-build-crawler`, `k8s-logs-crawler`, and their `.PHONY` entries.
- No new image: crawler-node and backend images already compile their apps and will pull in
  `apps/crawler-v2` (same Go module). The standalone `jobradar-crawler` Deployment is gone.
- **Deletion gate:** `grep -rl "jobradar/apps/crawler/" --include=*.go` must return nothing
  outside `apps/crawler/` before deleting the tree.
- Update docs: `CLAUDE.md` (pipeline + key rules + crawler-image "pure Go" rule referencing
  `Dockerfile.crawler`), `infra/CLAUDE.md`, `docs/ARCHITECTURE.md`, remove `apps/crawler/CLAUDE.md`,
  update `apps/crawler-v2/CLAUDE.md` status to "wired in".

## Edge cases (to enumerate fully in the edge-case discovery phase)

- Empty listing; adapter returns zero jobs.
- Fat adapter (content inline, no `PostFetcher`) vs thin adapter (`FetchJobPost` required).
- `JobContent==nil` AND `bundle.Post==nil` (registry/config bug) → must route to retry/DLQ.
- Configurable adapter (Workday/Eightfold) with missing/invalid per-company config at listing;
  and Workday `FetchJobPost` deriving tenant from URL with `adapter_config=nil`.
- `FetchJobPost` failure isolation — one job's detail failure must not stop the batch.
- Non-USA gate parity: Workday `LocationKnown=false` bypasses filter gate → enrich `CountryCode`
  gate; fat-adapter jobs (no country code) string-match only; Eightfold v1-parity trace.
- Employment-type / location / country-code field parity v1 → v2 (`buildJobRow` authority).
- Removed-job detection on `Final==true` with full listing (server-side diff).
- Logger `WithContext` under no active span, and under a remote span extracted from NATS headers.
- `FieldDurationMs` vs `FieldDuration` — correct key per call site.
- JSON round-trip parity for `core.JobSummary` (incl. `job_content`, `workplace_type`,
  `published_date`, `location_known`/`all_locations` omitempty) across all 4 hops.
- `CompanyID`/`Adapter` stamped by the orchestrator before NATS marshal (adapters leave empty).
- Cutover: old-shape (`full_content`) message after incomplete purge → `JobContent` nil →
  misclassified as thin; the purge (runbook step 3) must prevent this.

## Testing strategy

- Keep v2 adapter unit + integration tests (already ≥95%).
- Rewrite/port tests: crawler-node executor (v2 registry, two-phase), orchestrator wire
  marshalling (`core.JobSummary` JSON), filter deserialization + location helpers, analyzer
  `FetchJobPost` enrichment path.
- Every enumerated edge case gets a test (workflow rule).
- Gates: per-package ≥95% coverage, `go test -race`, golangci-lint — all must pass.

## Out of scope

- OCI deployment stabilization (separate in-flight work).
- Adapter behavior changes beyond what the v1→v2 contract difference already implies.
