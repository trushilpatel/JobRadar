# Grafana Overview Dashboard — Phase 1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire the orchestrator's missing metrics endpoint, add 5 new Prometheus gauges for current-state pipeline health (queue depth, oldest-pending age, online crawler nodes, auto-paused companies, retry-ladder backlog), and ship the Grafana Cloud "Pipeline Overview" dashboard + alert rules as git-tracked JSON/YAML.

**Architecture:** Every stage already emits Prometheus counters/histograms except current-state snapshots (queue depth, node count, etc.), which don't exist as any metric type today. This plan adds a small `RefreshGaugeMetrics` step to the scheduler's existing 1-minute tick — it re-reads 5 cheap COUNT/MIN queries from Postgres (via new store methods, no new tables) and sets 5 new `prometheus.Gauge` vars. The orchestrator gets `metrics.Serve()` (already used by every other binary) plus the existing `apimiddleware.Metrics()` chi middleware (already used by `api`, never wired into orchestrator) so its `/node/*` and `/admin/crawler-nodes/*` routes get free RED-method HTTP metrics. The dashboard and alert rules are hand-authored JSON/YAML files under `infra/observability/`, referencing metrics that now exist end-to-end, pushed to Grafana Cloud manually via the Grafana MCP tools once available.

**Tech Stack:** Go 1.26, GORM (Postgres in prod, in-memory SQLite in unit tests), `prometheus/client_golang`, chi router, Grafana Cloud (Mimir + unified alerting), Grafana dashboard JSON schema v39.

## Global Constraints

- Backend per-package coverage floor: 95% (`scripts/check-coverage.sh`), excluding `main.go`/`wire.go`/`wire_gen.go`/`/dto/`/`/models/`/`/testutil/` per project convention — `main.go` changes in this plan need no new unit test.
- Race detector clean: `go test -race -short ./...`.
- No raw GORM/SQL outside the store layer (`{Domain}RepositoryService.go` files) — every new query goes through an existing or new store method.
- No magic values — reused existing constants (`crawlernodecommon.NodeStatusActive`, `jobcommon.SeenReasonRetry`) wherever one exists; new SQL literals match the exact string literal style already used in the same file (this codebase inlines `'queued'`/`'running'` status literals directly in raw SQL rather than parameter-binding Go constants — match that, don't introduce a new convention in an isolated spot).
- Log only at top-level boundaries (`platform/logger.Logger`) — service-layer methods wrap errors with `fmt.Errorf("context: %w", err)` and return; only `apps/scheduler/scheduler.go` (`runCycle`, the worker-loop boundary) calls `log.Error`.
- Every store UPDATE/DELETE already in this codebase checks `RowsAffected`; the new methods added here are read-only `COUNT`/`MIN` queries, so this doesn't apply to them.
- Formatter (`gofmt`/`goimports`) and `golangci-lint run --config ../.github/.golangci.yml --timeout 5m` must pass before every commit — this is enforced by the pre-commit hook, not optional.

---

### Task 1: Orchestrator — wire metrics endpoint + HTTP RED middleware

**Files:**

- Modify: `backend/apps/orchestrator/main.go`

**Interfaces:**

- Consumes: `metrics.Serve(addr string, readyFn func() error)` (`backend/common/metrics/metrics.go:382`, already exists), `apimiddleware.Metrics() func(http.Handler) http.Handler` (`backend/apps/api/middleware/metrics.go`, already exists and already imported as `apimiddleware` in this file), `db.PoolReady(database *gorm.DB) error` (`backend/common/db/db.go:126`, already exists), `cfg.Observability.MetricsAddr` (already resolves to `:9104` via the existing `METRICS_ADDR` env var set in Helm).
- Produces: nothing new consumed by later tasks — this task is self-contained. After this task, `http_requests_total{job="jobradar_orchestrator"}` and `http_request_duration_seconds{job="jobradar_orchestrator"}` exist in Mimir, which Task 9 (dashboard JSON) queries.

- [ ] **Step 1: Add the metrics import and start the metrics server**

Edit `backend/apps/orchestrator/main.go`. Add `"github.com/trushilpatel/jobradar/common/metrics"` to the import block (alongside the existing `"github.com/trushilpatel/jobradar/common/logger"` import), then insert the server start right after `database` is opened:

```go
	database, err := db.Open(cfg.DB, 25)
	if err != nil {
		log.Error("db open failed", err)
		os.Exit(1)
	}

	go metrics.Serve(cfg.Observability.MetricsAddr, func() error { return db.PoolReady(database) })

	nc, njs, err := js.Connect(js.Config{URL: cfg.NATS.URL, CredsFile: cfg.NATS.CredsFile})
```

This mirrors the exact pattern already used in `backend/apps/scheduler/main.go:45`.

- [ ] **Step 2: Wire the RED-method HTTP middleware**

In the same file, add `r.Use(apimiddleware.Metrics())` immediately after the existing logger middleware:

```go
	r := chi.NewRouter()
	r.Use(chimiddleware.Recoverer)
	r.Use(chimiddleware.RequestID)
	r.Use(apimiddleware.Logger(log))
	r.Use(apimiddleware.Metrics())
```

This is the same middleware already wired into `apps/api/router.go:39` (`r.Use(middleware.Metrics())`) — it records `http_requests_total{method,route,status}` and `http_request_duration_seconds{method,route}` using chi's route pattern as the label, keyed apart from `api`'s own series by the `job="jobradar_orchestrator"` label Alloy attaches at scrape time.

- [ ] **Step 3: Build and verify**

Run: `cd backend && go build ./apps/orchestrator/...`
Expected: no errors.

Run: `cd backend && golangci-lint run --config ../.github/.golangci.yml --timeout 5m ./apps/orchestrator/...`
Expected: no new lint findings.

- [ ] **Step 4: Commit**

```bash
git add backend/apps/orchestrator/main.go
git commit -m "feat(orchestrator): wire metrics endpoint and HTTP RED middleware"
```

---

### Task 2: Add 5 new gauge metrics to the shared metrics package

**Files:**

- Modify: `backend/common/metrics/metrics.go`

**Interfaces:**

- Produces: 5 new package-level `prometheus.Gauge` vars, consumed by Task 7 (`RefreshGaugeMetrics`) and Task 9 (dashboard JSON):
  - `metrics.SchedulerQueuePending` — gauge, no labels
  - `metrics.SchedulerQueueOldestPendingAgeSeconds` — gauge, no labels
  - `metrics.CrawlerNodesOnline` — gauge, no labels
  - `metrics.SchedulerCompaniesAutoPausedCurrent` — gauge, no labels
  - `metrics.JobPipelineLedgerRetryCount` — gauge, no labels

- [ ] **Step 1: Add the gauge declarations**

Edit `backend/common/metrics/metrics.go`. Insert a new section after the existing `// ── LLM Exhaustion + Sweeper ──` block (before `// ── Registration ──`):

```go
// ── Current-state gauges (refreshed once per scheduler tick) ─────────────────
//
// Every metric above this section is a Counter or Histogram — cumulative
// totals, not current state. These 5 gauges are the exception: they answer
// "how many right now", refreshed by schedulerService.RefreshGaugeMetrics on
// every scheduler tick (default every 60s, see CheckIntervalSeconds). They
// lag real-time by at most one tick interval — acceptable for an Overview
// dashboard stat strip, not for anything requiring sub-minute freshness.

// SchedulerQueuePending is the current number of company_scan_queue rows in
// status='queued'. Refreshed by schedulerService.RefreshGaugeMetrics.
var SchedulerQueuePending = prometheus.NewGauge(prometheus.GaugeOpts{
	Name: "jobradar_scheduler_queue_pending",
	Help: "Current number of company_scan_queue rows in status='queued'.",
})

// SchedulerQueueOldestPendingAgeSeconds is the age in seconds of the oldest
// status='queued' row, or 0 when the queue is empty. Refreshed by
// schedulerService.RefreshGaugeMetrics.
var SchedulerQueueOldestPendingAgeSeconds = prometheus.NewGauge(prometheus.GaugeOpts{
	Name: "jobradar_scheduler_queue_oldest_pending_age_seconds",
	Help: "Age in seconds of the oldest status='queued' company_scan_queue row, 0 when empty.",
})

// CrawlerNodesOnline is the current number of crawler nodes with
// status='active' and a heartbeat within crawlernodecommon.OnlineThreshold.
// Refreshed by schedulerService.RefreshGaugeMetrics.
var CrawlerNodesOnline = prometheus.NewGauge(prometheus.GaugeOpts{
	Name: "jobradar_crawler_nodes_online",
	Help: "Current number of crawler nodes with status=active and a recent heartbeat.",
})

// SchedulerCompaniesAutoPausedCurrent is the current number of companies with
// company_scan_settings.auto_paused=true. Distinct from
// SchedulerCompaniesAutoPausedTotal, which counts pause *events* — this is a
// point-in-time snapshot. Refreshed by schedulerService.RefreshGaugeMetrics.
var SchedulerCompaniesAutoPausedCurrent = prometheus.NewGauge(prometheus.GaugeOpts{
	Name: "jobradar_scheduler_companies_auto_paused_current",
	Help: "Current number of companies with company_scan_settings.auto_paused=true.",
})

// JobPipelineLedgerRetryCount is the current number of job_pipeline_ledger
// rows at reason='retry' (removed_at IS NULL) — the sweeper's backlog size.
// Refreshed by schedulerService.RefreshGaugeMetrics.
var JobPipelineLedgerRetryCount = prometheus.NewGauge(prometheus.GaugeOpts{
	Name: "jobradar_job_pipeline_ledger_retry_count",
	Help: "Current number of job_pipeline_ledger rows at reason='retry' (removed_at IS NULL).",
})
```

- [ ] **Step 2: Register the 5 gauges**

In the same file, add all 5 vars to the `init()` function's `prometheus.MustRegister(...)` call list (append after `LLMCooldownMissTotal,`):

```go
		LLMCooldownMissTotal,
		SchedulerQueuePending,
		SchedulerQueueOldestPendingAgeSeconds,
		CrawlerNodesOnline,
		SchedulerCompaniesAutoPausedCurrent,
		JobPipelineLedgerRetryCount,
	)
}
```

- [ ] **Step 3: Build and verify**

Run: `cd backend && go build ./common/metrics/...`
Expected: no errors — package-level `var` + `init()` registration compiles even with zero callers yet.

- [ ] **Step 4: Commit**

```bash
git add backend/common/metrics/metrics.go
git commit -m "feat(metrics): add 5 current-state gauges for pipeline backlog signals"
```

---

### Task 3: Scan queue store — CountQueued + OldestQueuedAgeSeconds

**Files:**

- Modify: `backend/service/scan/services/ScanRepositoryService.go`
- Test: `backend/service/scan/tests/scan_queue_store_test.go`

**Interfaces:**

- Produces: 2 new methods on the `ScanQueueStore` interface, consumed by Task 6 (`schedulerService.RefreshGaugeMetrics`):
  - `CountQueued(ctx context.Context) (int, error)`
  - `OldestQueuedAgeSeconds(ctx context.Context) (float64, error)`

- [ ] **Step 1: Write the failing tests**

Add to `backend/service/scan/tests/scan_queue_store_test.go` (same package `scan`, reuses `newScanDryRunDB`/`scanSQLCapture` from `scan_log_store_test.go` and `newClosedScanQueueDB` already in this file):

```go
func TestScanQueueStore_CountQueued_EmitsStatusFilter(t *testing.T) {
	db, cap := newScanDryRunDB(t)
	store := svc.NewQueueStore(db)

	if _, err := store.CountQueued(context.Background()); err != nil {
		t.Fatalf("CountQueued() error = %v", err)
	}

	joined := strings.Join(cap.statements(), "\n")
	for _, want := range []string{"SELECT COUNT(*)", "FROM company_scan_queue", "status = 'queued'"} {
		if !strings.Contains(joined, want) {
			t.Errorf("expected SQL to contain %q; got:\n%s", want, joined)
		}
	}
}

func TestScanQueueStore_CountQueued_DBError(t *testing.T) {
	store := svc.NewQueueStore(newClosedScanQueueDB(t))
	if _, err := store.CountQueued(context.Background()); err == nil {
		t.Fatal("expected error on closed DB")
	}
}

func TestScanQueueStore_OldestQueuedAgeSeconds_EmitsAgeQuery(t *testing.T) {
	db, cap := newScanDryRunDB(t)
	store := svc.NewQueueStore(db)

	if _, err := store.OldestQueuedAgeSeconds(context.Background()); err != nil {
		t.Fatalf("OldestQueuedAgeSeconds() error = %v", err)
	}

	joined := strings.Join(cap.statements(), "\n")
	for _, want := range []string{"EXTRACT(EPOCH", "MIN(queued_at)", "status = 'queued'"} {
		if !strings.Contains(joined, want) {
			t.Errorf("expected SQL to contain %q; got:\n%s", want, joined)
		}
	}
}

func TestScanQueueStore_OldestQueuedAgeSeconds_DBError(t *testing.T) {
	store := svc.NewQueueStore(newClosedScanQueueDB(t))
	if _, err := store.OldestQueuedAgeSeconds(context.Background()); err == nil {
		t.Fatal("expected error on closed DB")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd backend && go test ./service/scan/tests/... -run 'TestScanQueueStore_CountQueued|TestScanQueueStore_OldestQueuedAgeSeconds' -v`
Expected: FAIL — `store.CountQueued undefined (type svc.ScanQueueStore has no field or method CountQueued)`.

- [ ] **Step 3: Add the interface methods and implementation**

Edit `backend/service/scan/services/ScanRepositoryService.go`. Add to the `ScanQueueStore` interface (after `ListQueue`):

```go
	// ListQueue returns queue entries matching filter, ordered by priority DESC, queued_at ASC.
	ListQueue(ctx context.Context, filter dto.QueueFilter) ([]dto.ScanQueueItem, error)

	// CountQueued returns the current number of company_scan_queue rows in
	// status='queued'. Read-only snapshot — used by the scheduler's gauge
	// refresh, not the enqueue/claim path.
	CountQueued(ctx context.Context) (int, error)

	// OldestQueuedAgeSeconds returns the age in seconds of the oldest
	// status='queued' row, or 0 when the queue is empty.
	OldestQueuedAgeSeconds(ctx context.Context) (float64, error)
}
```

Add the implementation after `ListQueue`'s implementation (before `// --- repositoryService: terminal Service implementation ---`):

```go
func (s *scanQueueStore) CountQueued(ctx context.Context) (_ int, err error) {
	ctx, span := tracing.Start(ctx, "scan.store.CountQueued")
	defer tracing.End(span, &err)
	var count int64
	err = s.db.WithContext(ctx).Raw(`SELECT COUNT(*) FROM company_scan_queue WHERE status = 'queued'`).Scan(&count).Error
	return int(count), err
}

func (s *scanQueueStore) OldestQueuedAgeSeconds(ctx context.Context) (_ float64, err error) {
	ctx, span := tracing.Start(ctx, "scan.store.OldestQueuedAgeSeconds")
	defer tracing.End(span, &err)
	var seconds float64
	err = s.db.WithContext(ctx).Raw(`
		SELECT COALESCE(EXTRACT(EPOCH FROM (NOW() - MIN(queued_at))), 0)
		FROM company_scan_queue WHERE status = 'queued'
	`).Scan(&seconds).Error
	return seconds, err
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd backend && go test ./service/scan/tests/... -run 'TestScanQueueStore_CountQueued|TestScanQueueStore_OldestQueuedAgeSeconds' -v`
Expected: PASS (4 tests).

- [ ] **Step 5: Run the full scan package test suite and fix every broken fake (interface change ripple)**

Run: `cd backend && go test ./service/scan/... -race`
Expected: FAIL at compile time — this project's pre-commit hook runs the full test suite, so this task cannot commit while any fake fails to satisfy `svc.ScanQueueStore`. Find every fake implementing `ScanQueueStore` with `grep -rn "ScanQueueStore = (\*" backend/service/scan/tests/*.go` — there are 4: `schedulerFakeQueue` in `scheduler_service_test.go`, `fakeQueueStore` in `service_test.go`, `sweeperFakeQueue` in `sweeper_service_test.go`, and `crawlerQueueFakeStore` in `crawler_queue_service_test.go` (confirm the grep finds exactly these 4 and no others — the interface may grow further fakes before this task runs). Add a trivial stub to each:

```go
func (f *schedulerFakeQueue) CountQueued(context.Context) (int, error)                { return 0, nil }
func (f *schedulerFakeQueue) OldestQueuedAgeSeconds(context.Context) (float64, error)  { return 0, nil }
```

(adjust the receiver name per fake — `f`, `q`, etc. — matching each file's existing convention). These are intentionally trivial no-op stubs, not real test behavior — Task 7 later replaces `schedulerFakeQueue`'s stubs with fully configurable fields for its own gauge-refresh tests; `fakeQueueStore`, `sweeperFakeQueue`, and `crawlerQueueFakeStore` never need more than a trivial stub since no task exercises gauge behavior through them.

Run `cd backend && go test ./service/scan/... -race` again.
Expected: PASS — full package, zero regressions.

- [ ] **Step 6: Commit**

```bash
git add backend/service/scan/services/ScanRepositoryService.go backend/service/scan/tests/scan_queue_store_test.go backend/service/scan/tests/service_test.go backend/service/scan/tests/sweeper_service_test.go backend/service/scan/tests/scheduler_service_test.go backend/service/scan/tests/crawler_queue_service_test.go
git commit -m "feat(scan): add CountQueued and OldestQueuedAgeSeconds to ScanQueueStore"
```

---

### Task 4: Crawler node store — CountOnline

**Files:**

- Modify: `backend/service/crawlernode/services/CrawlerNodeService.go`
- Modify: `backend/service/crawlernode/services/CrawlerNodeRepositoryService.go`
- Test: `backend/service/crawlernode/services/tests/CrawlerNodeRepositoryServiceDB_test.go`

**Interfaces:**

- Produces: 1 new method on the `NodeStore` interface, consumed by Task 6:
  - `CountOnline(ctx context.Context) (int, error)`

- [ ] **Step 1: Write the failing tests**

Add to `backend/service/crawlernode/services/tests/CrawlerNodeRepositoryServiceDB_test.go` (reuses `newNodeTestDB` already in this file):

```go
func TestNodeRepositoryService_CountOnline_CountsActiveWithRecentHeartbeat(t *testing.T) {
	db := newNodeTestDB(t)
	store := services.NewNodeRepositoryService(db)
	now := time.Now().UTC()

	seedNodeWithHeartbeat(t, db, "n1", "node-1", "active", now)                    // online
	seedNodeWithHeartbeat(t, db, "n2", "node-2", "active", now.Add(-30*time.Second)) // online (within 90s)
	seedNodeWithHeartbeat(t, db, "n3", "node-3", "active", now.Add(-5*time.Minute))  // offline (stale heartbeat)
	seedNodeWithHeartbeat(t, db, "n4", "node-4", "disabled", now)                    // offline (not active status)
	seedNodeWithHeartbeat(t, db, "n5", "node-5", "active", now.Add(120*time.Second)) // online (future heartbeat, still >= cutoff)

	count, err := store.CountOnline(context.Background())
	if err != nil {
		t.Fatalf("CountOnline() error = %v", err)
	}
	if count != 3 {
		t.Errorf("CountOnline() = %d, want 3", count)
	}
}

func TestNodeRepositoryService_CountOnline_NilLastSeenExcluded(t *testing.T) {
	db := newNodeTestDB(t)
	store := services.NewNodeRepositoryService(db)
	hash := "hash-never-seen"
	if err := db.Create(&crawlernodemodel.CrawlerNode{
		ID: "n-never", Name: "node-never", TokenPrefix: "n-never0", TokenHash: &hash, Status: "active",
	}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	count, err := store.CountOnline(context.Background())
	if err != nil {
		t.Fatalf("CountOnline() error = %v", err)
	}
	if count != 0 {
		t.Errorf("CountOnline() = %d, want 0 (nil last_seen_at never counts as online)", count)
	}
}

// seedNodeWithHeartbeat extends seedNode with an explicit last_seen_at.
func seedNodeWithHeartbeat(t *testing.T, db *gorm.DB, id, name, status string, lastSeen time.Time) {
	t.Helper()
	hash := "hash-" + id
	if err := db.Create(&crawlernodemodel.CrawlerNode{
		ID:          id,
		Name:        name,
		TokenPrefix: id[:8],
		TokenHash:   &hash,
		Status:      status,
		LastSeenAt:  &lastSeen,
	}).Error; err != nil {
		t.Fatalf("seed node %s: %v", id, err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd backend && go test ./service/crawlernode/services/tests/... -run TestNodeRepositoryService_CountOnline -v`
Expected: FAIL — `store.CountOnline undefined`.

- [ ] **Step 3: Add the interface method and implementation**

Edit `backend/service/crawlernode/services/CrawlerNodeService.go`. Add to the `NodeStore` interface (after `Retire`):

```go
	// Retire sets status='retired' and clears token_hash atomically.
	Retire(ctx context.Context, nodeID string) error

	// CountOnline returns the number of nodes with status=active whose
	// last_seen_at is within crawlernodecommon.OnlineThreshold of now. The
	// cutoff is computed inside the store (not passed by the caller) so the
	// "what counts as online" threshold stays encapsulated where
	// OnlineThreshold is already defined and used by ListNodes.
	CountOnline(ctx context.Context) (int, error)
}
```

Edit `backend/service/crawlernode/services/CrawlerNodeRepositoryService.go`. Add the import:

```go
import (
	"context"
	stderrors "errors"
	"strings"
	"time"

	"github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/tracing"
	crawlernodecommon "github.com/trushilpatel/jobradar/service/crawlernode/common"
	crawlernodemodel "github.com/trushilpatel/jobradar/service/crawlernode/models"
	"gorm.io/gorm"
)
```

Add the implementation after `List` (before `Retire`):

```go
// CountOnline returns the number of status=active nodes whose last_seen_at
// is at or after (now - OnlineThreshold). Mirrors the online/offline
// computation ListNodes already does in Go for the admin UI, pushed down to
// SQL here since only a count is needed.
func (s *nodeRepositoryService) CountOnline(ctx context.Context) (_ int, err error) {
	ctx, span := tracing.Start(ctx, "crawlernode.store.CountOnline")
	defer tracing.End(span, &err)

	cutoff := time.Now().UTC().Add(-crawlernodecommon.OnlineThreshold)
	var count int64
	result := s.db.WithContext(ctx).
		Model(&crawlernodemodel.CrawlerNode{}).
		Where("status = ? AND last_seen_at >= ?", crawlernodecommon.NodeStatusActive, cutoff).
		Count(&count)
	if result.Error != nil {
		return 0, errors.ErrInternal.WithMessage("count online nodes: " + result.Error.Error())
	}
	return int(count), nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd backend && go test ./service/crawlernode/services/tests/... -run TestNodeRepositoryService_CountOnline -v`
Expected: PASS (2 tests).

- [ ] **Step 5: Run the full crawlernode package test suite and fix the broken fake**

Run: `cd backend && go test ./service/crawlernode/... -race`
Expected: FAIL at compile time — `mockNodeStore` in `backend/service/crawlernode/services/tests/CrawlerNodeRepository_test.go` (`var _ services.NodeStore = (*mockNodeStore)(nil)`) no longer satisfies `NodeStore`. This project's pre-commit hook runs the full test suite, so this task cannot commit while that fake fails to compile. Add a trivial stub to it:

```go
func (m *mockNodeStore) CountOnline(context.Context) (int, error) { return 0, nil }
```

(adjust the receiver name to match the file's existing convention if different from `m`). Run `cd backend && go test ./service/crawlernode/... -race` again.
Expected: PASS — full package, zero regressions.

- [ ] **Step 6: Commit**

```bash
git add backend/service/crawlernode/services/CrawlerNodeService.go backend/service/crawlernode/services/CrawlerNodeRepositoryService.go backend/service/crawlernode/services/tests/CrawlerNodeRepositoryServiceDB_test.go backend/service/crawlernode/services/tests/CrawlerNodeRepository_test.go
git commit -m "feat(crawlernode): add CountOnline to NodeStore"
```

---

### Task 5: Company scan settings store — CountAutoPaused

**Files:**

- Modify: `backend/service/company/services/CompanyService.go`
- Modify: `backend/service/company/services/CompanyScanSettingsRepositoryService.go`
- Test: `backend/service/company/tests/CompanyScanSettingsRepository_test.go`

**Interfaces:**

- Produces: 1 new method on the `ScanSettingsRepository` interface, consumed by Task 6:
  - `CountAutoPaused(ctx context.Context) (int, error)`

- [ ] **Step 1: Write the failing test**

Add to `backend/service/company/tests/CompanyScanSettingsRepository_test.go` (reuses `newScanSettingsDB` already in this file):

```go
func TestScanSettingsRepo_CountAutoPaused_CountsOnlyPausedRows(t *testing.T) {
	db := newScanSettingsDB(t)
	now := time.Now()
	seed := func(id string, paused bool) {
		if err := db.Exec(`INSERT INTO company_scan_settings (company_id, adapter_type, active, auto_paused, consecutive_errors, created_at, updated_at) VALUES (?, ?, true, ?, 0, ?, ?)`,
			id, companycommon.AdapterGreenhouse, paused, now, now).Error; err != nil {
			t.Fatalf("seed %s: %v", id, err)
		}
	}
	seed("a", true)
	seed("b", false)
	seed("c", true)

	repo := companysvc.NewScanSettingsRepositoryService(db)
	count, err := repo.CountAutoPaused(context.Background())
	if err != nil {
		t.Fatalf("CountAutoPaused() error = %v", err)
	}
	if count != 2 {
		t.Errorf("CountAutoPaused() = %d, want 2", count)
	}
}

func TestScanSettingsRepo_CountAutoPaused_EmptyTable(t *testing.T) {
	db := newScanSettingsDB(t)
	repo := companysvc.NewScanSettingsRepositoryService(db)
	count, err := repo.CountAutoPaused(context.Background())
	if err != nil {
		t.Fatalf("CountAutoPaused() error = %v", err)
	}
	if count != 0 {
		t.Errorf("CountAutoPaused() = %d, want 0", count)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd backend && go test ./service/company/tests/... -run TestScanSettingsRepo_CountAutoPaused -v`
Expected: FAIL — `repo.CountAutoPaused undefined`.

- [ ] **Step 3: Add the interface method and implementation**

Edit `backend/service/company/services/CompanyService.go`. Add to the `ScanSettingsRepository` interface (after `Delete`):

```go
type ScanSettingsRepository interface {
	Get(ctx context.Context, companyID string) (*companymodel.CompanyScanSettings, error)
	GetMissing(ctx context.Context) ([]string, error)
	Create(ctx context.Context, s companymodel.CompanyScanSettings) error
	Update(ctx context.Context, companyID string, p companydto.CompanyScanSettingsInput) error
	UpdateScanState(ctx context.Context, companyID string, st companydto.CompanyScanStateUpdate) error
	Delete(ctx context.Context, companyID string) error
	// CountAutoPaused returns the current number of company_scan_settings
	// rows with auto_paused=true — a point-in-time snapshot, distinct from
	// the scheduler's pause-event counter.
	CountAutoPaused(ctx context.Context) (int, error)
}
```

Edit `backend/service/company/services/CompanyScanSettingsRepositoryService.go`. Add the implementation after `Delete` (before `scanSettingsInputToFields`):

```go
// CountAutoPaused returns the current number of company_scan_settings rows
// with auto_paused=true. Read-only snapshot for the scheduler's gauge
// refresh — distinct from AutoPauseChronicFailures, which is the write path
// that sets auto_paused=true.
func (r *scanSettingsRepository) CountAutoPaused(ctx context.Context) (_ int, err error) {
	ctx, span := tracing.Start(ctx, "company.scan_settings.repo.CountAutoPaused")
	defer tracing.End(span, &err)

	var count int64
	result := r.db.WithContext(ctx).
		Model(&companymodel.CompanyScanSettings{}).
		Where("auto_paused = ?", true).
		Count(&count)
	if result.Error != nil {
		return 0, apperr.ErrInternal.WithMessage("count auto-paused companies: " + result.Error.Error())
	}
	return int(count), nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd backend && go test ./service/company/tests/... -run TestScanSettingsRepo_CountAutoPaused -v`
Expected: PASS (2 tests).

- [ ] **Step 5: Run the full company AND scan package test suites, fix the broken fake**

`ScanSettingsRepository` is consumed cross-domain — `service/scan/tests/service_test.go` has its own fake, `fakeScanSettingsRepo` (`var _ companysvc.ScanSettingsRepository = (*fakeScanSettingsRepo)(nil)`), separate from anything in `service/company/`.

Run: `cd backend && go test ./service/company/... ./service/scan/... -race`
Expected: FAIL at compile time — `fakeScanSettingsRepo` in `backend/service/scan/tests/service_test.go` no longer satisfies `ScanSettingsRepository`. This project's pre-commit hook runs the full test suite, so this task cannot commit while that fake fails to compile. Add a trivial stub to it:

```go
func (f *fakeScanSettingsRepo) CountAutoPaused(context.Context) (int, error) { return 0, nil }
```

(adjust the receiver name to match the file's existing convention if different from `f`). Run `cd backend && go test ./service/company/... ./service/scan/... -race` again.
Expected: PASS — both packages, zero regressions.

- [ ] **Step 6: Commit**

```bash
git add backend/service/company/services/CompanyService.go backend/service/company/services/CompanyScanSettingsRepositoryService.go backend/service/company/tests/CompanyScanSettingsRepository_test.go backend/service/scan/tests/service_test.go
git commit -m "feat(company): add CountAutoPaused to ScanSettingsRepository"
```

---

### Task 6: Job ledger store — CountByReason

**Files:**

- Modify: `backend/service/job/services/JobService.go`
- Modify: `backend/service/job/services/JobSeenLedgerRepository.go`
- Test: `backend/service/job/tests/` (new test alongside the existing `JobSeenLedgerRepository` test file — locate the exact file with `find backend/service/job/tests -iname "*ledger*"` before editing, since this plan was authored without reading that directory; use the same in-memory-DB helper pattern already established there)

**Interfaces:**

- Produces: 1 new method on the `SeenLedgerReader` interface, consumed by Task 7:
  - `CountByReason(ctx context.Context, reason jobcommon.SeenReason) (int, error)`

- [ ] **Step 1: Locate the existing ledger test file and its DB helper**

Run: `cd backend && find service/job/tests -iname "*ledger*" -o -iname "*seen*"`

Read the matched file(s) to find the existing sqlite/dryrun test-DB helper (same pattern as Tasks 3–5: either a real in-memory sqlite DB if the query is portable, or a DryRun capture if it uses Postgres-only syntax). `CountByReason`'s query (`WHERE reason = ? AND removed_at IS NULL`) is plain SQL with no Postgres-only functions, so it should round-trip on real sqlite — prefer the real-execution helper over DryRun if one exists for this store.

- [ ] **Step 2: Write the failing test**

Using the DB helper found in Step 1 (adjust table/column setup to match `job_pipeline_ledger`'s actual schema as reflected in that helper), add:

```go
func TestSeenLedgerStore_CountByReason_CountsMatchingNonRemovedRows(t *testing.T) {
	db := /* existing helper, e.g. newLedgerTestDB(t) */
	store := services.NewSeenLedgerStore(db)

	// seed via the helper's existing seed function or a direct Exec matching
	// the table's real column set — mirror however this file already seeds rows.
	seedLedgerRow(t, db, "job-1", "company-a", string(jobcommon.SeenReasonRetry), nil)
	seedLedgerRow(t, db, "job-2", "company-a", string(jobcommon.SeenReasonRetry), nil)
	removedAt := time.Now()
	seedLedgerRow(t, db, "job-3", "company-a", string(jobcommon.SeenReasonRetry), &removedAt) // removed — excluded
	seedLedgerRow(t, db, "job-4", "company-a", string(jobcommon.SeenReasonFetch), nil)        // wrong reason — excluded

	count, err := store.CountByReason(context.Background(), jobcommon.SeenReasonRetry)
	if err != nil {
		t.Fatalf("CountByReason() error = %v", err)
	}
	if count != 2 {
		t.Errorf("CountByReason() = %d, want 2", count)
	}
}
```

If the file's seeding helper has a different signature than `seedLedgerRow(t, db, jobID, companyID, reason, removedAt)`, adapt the call to match — the important assertions are: 2 non-removed `retry` rows counted, 1 removed `retry` row excluded, 1 non-`retry` row excluded.

- [ ] **Step 3: Run test to verify it fails**

Run: `cd backend && go test ./service/job/tests/... -run TestSeenLedgerStore_CountByReason -v`
Expected: FAIL — `store.CountByReason undefined`.

- [ ] **Step 4: Add the interface method and implementation**

Edit `backend/service/job/services/JobService.go`. Add to the `SeenLedgerReader` interface (after `HasSeenBatch`):

```go
type SeenLedgerReader interface {
	HasSeen(ctx context.Context, jobID string) (bool, error)
	HasSeenBatch(ctx context.Context, jobIDs []string) (map[string]bool, error)
	// CountByReason returns the current number of non-removed
	// job_pipeline_ledger rows at the given reason. Read-only snapshot used
	// by the scheduler's gauge refresh.
	CountByReason(ctx context.Context, reason jobcommon.SeenReason) (int, error)
}
```

Edit `backend/service/job/services/JobSeenLedgerRepository.go`. Add the implementation near the other ledger-reading methods:

```go
// CountByReason returns the current number of job_pipeline_ledger rows at
// the given reason with removed_at IS NULL — e.g. the sweeper's
// reason='retry' backlog size.
func (s *seenLedgerStore) CountByReason(ctx context.Context, reason jobcommon.SeenReason) (_ int, err error) {
	ctx, span := tracing.Start(ctx, "job.ledger.store.CountByReason")
	defer tracing.End(span, &err)
	var count int64
	err = s.db.WithContext(ctx).Raw(`
		SELECT COUNT(*) FROM job_pipeline_ledger WHERE reason = ? AND removed_at IS NULL
	`, string(reason)).Scan(&count).Error
	return int(count), err
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `cd backend && go test ./service/job/tests/... -run TestSeenLedgerStore_CountByReason -v`
Expected: PASS.

- [ ] **Step 6: Run the full job AND scan package test suites, fix every broken fake**

`SeenLedgerStore`/`SeenLedgerReader` is consumed cross-domain — `service/scan/tests/sweeper_service_test.go` has 2 fakes implementing the full `jobsvc.SeenLedgerStore`: `fakeSweeperLedger` and `conditionalSweeperLedger` (both asserted via `var _ jobsvc.SeenLedgerStore = (*...)(nil)`).

Run: `cd backend && go test ./service/job/... ./service/scan/... -race`
Expected: FAIL at compile time — both fakes above no longer satisfy `SeenLedgerStore`. This project's pre-commit hook runs the full test suite, so this task cannot commit while either fake fails to compile. Add a trivial stub to each:

```go
func (f *fakeSweeperLedger) CountByReason(context.Context, jobcommon.SeenReason) (int, error) { return 0, nil }
func (c *conditionalSweeperLedger) CountByReason(context.Context, jobcommon.SeenReason) (int, error) { return 0, nil }
```

(adjust receiver names to match each type's existing convention if different from `f`/`c`; confirm there are no other fakes with `grep -rn "SeenLedgerStore = (\*\|SeenLedgerReader = (\*" backend/service/job/tests/*.go backend/service/scan/tests/*.go` before moving on). Run `cd backend && go test ./service/job/... ./service/scan/... -race` again.
Expected: PASS — both packages, zero regressions.

- [ ] **Step 7: Commit**

```bash
git add backend/service/job/services/JobService.go backend/service/job/services/JobSeenLedgerRepository.go backend/service/job/tests/ backend/service/scan/tests/sweeper_service_test.go
git commit -m "feat(job): add CountByReason to SeenLedgerReader"
```

---

### Task 7: Scheduler service — RefreshGaugeMetrics

**Files:**

- Modify: `backend/service/scan/services/ScanSchedulerService.go`
- Modify: `backend/service/scan/services/ScanService.go`
- Modify: `backend/service/scan/services/ScanRepositoryService.go`
- Modify: `backend/service/scan/tests/scheduler_service_test.go`
- Modify: `backend/service/scan/tests/handler_test.go`
- Modify: `backend/service/scan/tests/scan_config_service_test.go`
- Modify: `backend/service/scan/tests/validator_test.go`
- Modify: `backend/service/scan/tests/base_service_test.go`

**Interfaces:**

- Consumes: `ScanQueueStore.CountQueued`/`OldestQueuedAgeSeconds` (Task 3), `NodeStore.CountOnline` (Task 4, via a narrow local interface), `ScanSettingsRepository.CountAutoPaused` (Task 5, via a narrow local interface), `jobsvc.SeenLedgerStore.CountByReason` (Task 6, already a field on `schedulerService` via `s.ledger`), the 5 gauges from Task 2.
- Produces: `SchedulerService.RefreshGaugeMetrics(ctx context.Context) error`, consumed by Task 8 (`runCycle` + `apps/scheduler/interfaces.go`).

- [ ] **Step 1: Upgrade schedulerFakeQueue's trivial CountQueued/OldestQueuedAgeSeconds stubs into configurable test hooks**

Task 3 already added trivial `CountQueued`/`OldestQueuedAgeSeconds` stub methods to `schedulerFakeQueue` (returning `(0, nil)` unconditionally) so the `scan` package would compile after the `ScanQueueStore` interface grew — this task's tests need those methods to actually return configurable values, so replace the stub bodies and add the backing fields.

Edit `backend/service/scan/tests/scheduler_service_test.go`. Add to `schedulerFakeQueue` (fields + methods):

```go
type schedulerFakeQueue struct {
	enqueueDueRes svc.EnqueueDueResult
	purgeCount    int
	resetCount    int
	err           error

	enqueueDueArgs struct {
		intervalMinutes int
		source          string
	}
	resetStuckArg time.Duration

	autoPauseIDs       []string
	autoPauseErr       error
	autoPauseThreshold int
	autoPauseCalls     int

	// Gauge-refresh test hooks.
	countQueuedRes  int
	countQueuedErr  error
	oldestAgeRes    float64
	oldestAgeErr    error
}
```

```go
func (f *schedulerFakeQueue) CountQueued(context.Context) (int, error) {
	return f.countQueuedRes, f.countQueuedErr
}
func (f *schedulerFakeQueue) OldestQueuedAgeSeconds(context.Context) (float64, error) {
	return f.oldestAgeRes, f.oldestAgeErr
}
```

- [ ] **Step 2: Verify the upgrade compiles clean**

Run: `cd backend && go build ./service/scan/...`
Expected: builds clean (the `var _ svc.ScanQueueStore = (*schedulerFakeQueue)(nil)` assertion still passes — the method set didn't change, only the bodies and struct fields).

- [ ] **Step 3: Write the failing tests for RefreshGaugeMetrics**

Add to `backend/service/scan/tests/scheduler_service_test.go`:

```go
// fakeGaugeNodeReader is a test double for svc.GaugeNodeReader.
type fakeGaugeNodeReader struct {
	count int
	err   error
	calls int
}

func (f *fakeGaugeNodeReader) CountOnline(context.Context) (int, error) {
	f.calls++
	return f.count, f.err
}

// fakeGaugeSettingsReader is a test double for svc.GaugeSettingsReader.
type fakeGaugeSettingsReader struct {
	count int
	err   error
	calls int
}

func (f *fakeGaugeSettingsReader) CountAutoPaused(context.Context) (int, error) {
	f.calls++
	return f.count, f.err
}

func TestSchedulerService_RefreshGaugeMetrics_SetsAllFiveGauges(t *testing.T) {
	q := &schedulerFakeQueue{countQueuedRes: 7, oldestAgeRes: 42.5}
	nodeReader := &fakeGaugeNodeReader{count: 3}
	settingsReader := &fakeGaugeSettingsReader{count: 2}
	ledger := &fakeSeenLedgerStore{countByReasonRes: 5}

	s := svc.NewSchedulerServiceWithSweeper(q, svc.SchedulerServiceOptions{
		Ledger:         ledger,
		NodeReader:     nodeReader,
		SettingsReader: settingsReader,
		Log:            logger.NewNoop(),
	})

	if err := s.RefreshGaugeMetrics(context.Background()); err != nil {
		t.Fatalf("RefreshGaugeMetrics() error = %v", err)
	}

	if got := testutil.ToFloat64(metrics.SchedulerQueuePending); got != 7 {
		t.Errorf("SchedulerQueuePending = %v, want 7", got)
	}
	if got := testutil.ToFloat64(metrics.SchedulerQueueOldestPendingAgeSeconds); got != 42.5 {
		t.Errorf("SchedulerQueueOldestPendingAgeSeconds = %v, want 42.5", got)
	}
	if got := testutil.ToFloat64(metrics.CrawlerNodesOnline); got != 3 {
		t.Errorf("CrawlerNodesOnline = %v, want 3", got)
	}
	if got := testutil.ToFloat64(metrics.SchedulerCompaniesAutoPausedCurrent); got != 2 {
		t.Errorf("SchedulerCompaniesAutoPausedCurrent = %v, want 2", got)
	}
	if got := testutil.ToFloat64(metrics.JobPipelineLedgerRetryCount); got != 5 {
		t.Errorf("JobPipelineLedgerRetryCount = %v, want 5", got)
	}
	if nodeReader.calls != 1 || settingsReader.calls != 1 {
		t.Errorf("expected exactly 1 call each; nodeReader=%d settingsReader=%d", nodeReader.calls, settingsReader.calls)
	}
}

func TestSchedulerService_RefreshGaugeMetrics_NilOptionalReaders_NoPanic(t *testing.T) {
	q := &schedulerFakeQueue{countQueuedRes: 1}
	s := svc.NewSchedulerServiceWithSweeper(q, svc.SchedulerServiceOptions{Log: logger.NewNoop()})

	// nodeReader/settingsReader/ledger all nil — must not panic, must not error
	// (their gauges are simply not refreshed this tick).
	if err := s.RefreshGaugeMetrics(context.Background()); err != nil {
		t.Fatalf("RefreshGaugeMetrics() error = %v, want nil (nil readers are skipped, not errors)", err)
	}
}

func TestSchedulerService_RefreshGaugeMetrics_PartialFailure_JoinsErrorsAndSetsSucceeded(t *testing.T) {
	q := &schedulerFakeQueue{countQueuedErr: errors.New("db down"), oldestAgeRes: 10}
	nodeReader := &fakeGaugeNodeReader{err: errors.New("node store down")}
	settingsReader := &fakeGaugeSettingsReader{count: 4}

	s := svc.NewSchedulerServiceWithSweeper(q, svc.SchedulerServiceOptions{
		NodeReader:     nodeReader,
		SettingsReader: settingsReader,
		Log:            logger.NewNoop(),
	})

	err := s.RefreshGaugeMetrics(context.Background())
	if err == nil {
		t.Fatal("expected a joined error from the 2 failing reads")
	}
	if !strings.Contains(err.Error(), "count queued") || !strings.Contains(err.Error(), "count online nodes") {
		t.Errorf("expected joined error to mention both failures, got: %v", err)
	}
	// The 2 succeeding reads (oldest age, auto-paused) must still have set
	// their gauges — a partial failure never blocks the other gauges.
	if got := testutil.ToFloat64(metrics.SchedulerQueueOldestPendingAgeSeconds); got != 10 {
		t.Errorf("SchedulerQueueOldestPendingAgeSeconds = %v, want 10 (should still be set despite the other failure)", got)
	}
	if got := testutil.ToFloat64(metrics.SchedulerCompaniesAutoPausedCurrent); got != 4 {
		t.Errorf("SchedulerCompaniesAutoPausedCurrent = %v, want 4", got)
	}
}
```

This test file needs a `fakeSeenLedgerStore` implementing `jobsvc.SeenLedgerStore` with a `countByReasonRes int` field and `CountByReason` method — check whether one already exists in this file (used by the existing `RetryDueJobs`/`EscalateStuckRetryJobs` tests); if so, add the `countByReasonRes` field and method to it instead of creating a duplicate. If none exists, add a minimal one implementing only the methods `schedulerService` actually calls on `s.ledger` (`FetchDueRetryJobsWithPayload`, `EscalateRetryToError`, `UpdateRetryBatchWithPayload`, `EscalateStuckRetryJobs`, `ReclaimStuckFetchRows`, `CountByReason` — mirror whatever the existing sweeper tests already stub).

- [ ] **Step 4: Run tests to verify they fail**

Run: `cd backend && go test ./service/scan/tests/... -run TestSchedulerService_RefreshGaugeMetrics -v`
Expected: FAIL — `s.RefreshGaugeMetrics undefined`, `svc.GaugeNodeReader undefined`, `SchedulerServiceOptions.NodeReader undefined`, etc.

- [ ] **Step 5: Add the narrow reader interfaces and SchedulerServiceOptions fields**

Edit `backend/service/scan/services/ScanSchedulerService.go`. Add near the top, after the existing `AutoPauseAuditor` interface:

```go
// GaugeNodeReader is the narrow crawler_nodes read contract the gauge
// refresher needs — declared here (not the full crawlernode NodeStore) so
// the scheduler depends on one method, not the admin CRUD surface it never
// calls. Satisfied structurally by crawlernodesvc.NodeStore.
type GaugeNodeReader interface {
	CountOnline(ctx context.Context) (int, error)
}

// GaugeSettingsReader is the narrow company_scan_settings read contract the
// gauge refresher needs. Satisfied structurally by
// companysvc.ScanSettingsRepository.
type GaugeSettingsReader interface {
	CountAutoPaused(ctx context.Context) (int, error)
}
```

Add 2 fields to `schedulerService`:

```go
type schedulerService struct {
	queue          ScanQueueStore
	ledger         jobsvc.SeenLedgerStore
	publisher      SweeperPublisher
	auditor        AutoPauseAuditor
	nodeReader     GaugeNodeReader
	settingsReader GaugeSettingsReader
	maxRetries     int
	log            logger.Logger
}
```

Add 2 fields to `SchedulerServiceOptions`:

```go
type SchedulerServiceOptions struct {
	Ledger         jobsvc.SeenLedgerStore
	Publisher      SweeperPublisher
	Auditor        AutoPauseAuditor
	NodeReader     GaugeNodeReader
	SettingsReader GaugeSettingsReader
	MaxRetries     int
	Log            logger.Logger
}
```

Wire both new fields in `NewSchedulerServiceWithSweeper`:

```go
	return &schedulerService{
		queue:          queue,
		ledger:         opts.Ledger,
		publisher:      opts.Publisher,
		auditor:        opts.Auditor,
		nodeReader:     opts.NodeReader,
		settingsReader: opts.SettingsReader,
		maxRetries:     maxRetries,
		log:            log,
	}
```

- [ ] **Step 6: Add the RefreshGaugeMetrics method**

Add `"errors"` (aliased `stderrors`, matching this codebase's convention seen in `ScanRepositoryService.go`) to the import block of `ScanSchedulerService.go`, then add the method after `AutoPauseChronicFailures`:

```go
// RefreshGaugeMetrics recomputes the 5 current-state gauge metrics (queue
// depth, oldest-pending age, online node count, currently-paused company
// count, retry-ladder backlog) from their source-of-truth tables and sets
// them on the package-level Prometheus gauges. Called once per scheduler
// tick (runCycle).
//
// Each of the 5 reads is independent — a failure on one is joined into the
// returned error but never prevents the others from running or setting
// their gauge. NodeReader and SettingsReader are optional (nil-safe,
// mirroring the Ledger/Publisher optional-deps pattern); when nil, that
// gauge is simply not refreshed this tick rather than treated as an error.
func (s *schedulerService) RefreshGaugeMetrics(ctx context.Context) (err error) {
	ctx, span := tracing.Start(ctx, "scan.service.RefreshGaugeMetrics")
	defer tracing.End(span, &err)

	var errs []error

	if n, qerr := s.queue.CountQueued(ctx); qerr != nil {
		errs = append(errs, fmt.Errorf("count queued: %w", qerr))
	} else {
		metrics.SchedulerQueuePending.Set(float64(n))
	}

	if age, aerr := s.queue.OldestQueuedAgeSeconds(ctx); aerr != nil {
		errs = append(errs, fmt.Errorf("oldest queued age: %w", aerr))
	} else {
		metrics.SchedulerQueueOldestPendingAgeSeconds.Set(age)
	}

	if s.nodeReader != nil {
		if n, nerr := s.nodeReader.CountOnline(ctx); nerr != nil {
			errs = append(errs, fmt.Errorf("count online nodes: %w", nerr))
		} else {
			metrics.CrawlerNodesOnline.Set(float64(n))
		}
	}

	if s.settingsReader != nil {
		if n, serr := s.settingsReader.CountAutoPaused(ctx); serr != nil {
			errs = append(errs, fmt.Errorf("count auto-paused: %w", serr))
		} else {
			metrics.SchedulerCompaniesAutoPausedCurrent.Set(float64(n))
		}
	}

	if s.ledger != nil {
		if n, lerr := s.ledger.CountByReason(ctx, jobcommon.SeenReasonRetry); lerr != nil {
			errs = append(errs, fmt.Errorf("count retry ledger: %w", lerr))
		} else {
			metrics.JobPipelineLedgerRetryCount.Set(float64(n))
		}
	}

	return stderrors.Join(errs...)
}
```

Add `jobcommon "github.com/trushilpatel/jobradar/service/job/common"` to the import block if not already present (check first — `jobsvc` is already imported as `"github.com/trushilpatel/jobradar/service/job/services"`; `jobcommon` is a sibling package under `service/job/common`, likely not yet imported here).

- [ ] **Step 7: Wire RefreshGaugeMetrics through the interface chain**

Edit `backend/service/scan/services/ScanService.go`. Add to the `SchedulerService` interface (after `AutoPauseChronicFailures`):

```go
	AutoPauseChronicFailures(ctx context.Context) (int, error)
	// RefreshGaugeMetrics recomputes the current-state gauge metrics (queue
	// depth, oldest-pending age, online node count, auto-paused company
	// count, retry-ladder backlog). Each of the 5 reads is independent —
	// a failure on one is joined into the returned error but never blocks
	// the others.
	RefreshGaugeMetrics(ctx context.Context) error
}
```

Add the `BaseService` passthrough:

```go
func (b *BaseService) RefreshGaugeMetrics(ctx context.Context) error {
	return b.next.RefreshGaugeMetrics(ctx)
}
```

Edit `backend/service/scan/services/ScanRepositoryService.go`. Add the no-op (matching `RetryDueJobs`/`EscalateStuckRetryJobs`/`ReclaimStuckFetchRows`/`AutoPauseChronicFailures`'s existing no-op pattern) after `AutoPauseChronicFailures`:

```go
// RefreshGaugeMetrics is a no-op on the repository service for the same
// reason as the other scheduler-only sweeps — the gauge-refresh path is
// never wired through the REST decorator chain.
func (s *repositoryService) RefreshGaugeMetrics(_ context.Context) error {
	return nil
}
```

- [ ] **Step 8: Fix the 3 other Service fakes broken by the interface addition**

Run: `cd backend && go build ./service/scan/...`
Expected: 3 compile errors — `fakeScanService` (`handler_test.go`), `mockScanService` (`scan_config_service_test.go`), `noopService` (`validator_test.go`) no longer satisfy `svc.Service`.

Add to each of the 3 fakes a trivial passthrough matching that fake's existing style for other no-argument-effect methods (e.g. if the fake has a configurable-error field pattern like `err error`, reuse it; otherwise return `nil`):

```go
func (f *fakeScanService) RefreshGaugeMetrics(context.Context) error { return nil }
```

```go
func (m *mockScanService) RefreshGaugeMetrics(context.Context) error { return nil }
```

```go
func (n *noopService) RefreshGaugeMetrics(context.Context) error { return nil }
```

(Adjust receiver names to match each file's actual receiver — confirm by reading the surrounding methods in each file before adding.)

- [ ] **Step 9: Add the BaseService delegation test**

Add to `backend/service/scan/tests/base_service_test.go`, following the existing `TestBaseService_Delegates*` pattern:

```go
func TestBaseService_DelegatesRefreshGaugeMetrics(t *testing.T) {
	noop := &noopService{}
	base := newBasePassthroughService(noop)

	if err := base.RefreshGaugeMetrics(context.Background()); err != nil {
		t.Errorf("RefreshGaugeMetrics delegate error = %v", err)
	}
}
```

- [ ] **Step 10: Run the full scan package test suite**

Run: `cd backend && go test ./service/scan/... -race -v -run 'RefreshGaugeMetrics|Gauge'`
Expected: PASS (Task 7's 3 new tests + the base-service delegation test).

Run: `cd backend && go test ./service/scan/... -race`
Expected: PASS — full package, no regressions.

- [ ] **Step 11: Run coverage check**

Run: `cd backend && go test ./service/scan/... -coverprofile=/tmp/scan_cov.out && go tool cover -func=/tmp/scan_cov.out | tail -1`
Expected: total coverage stays at or above 95% (the project's per-package floor).

- [ ] **Step 12: Commit**

```bash
git add backend/service/scan/services/ScanSchedulerService.go backend/service/scan/services/ScanService.go backend/service/scan/services/ScanRepositoryService.go backend/service/scan/tests/scheduler_service_test.go backend/service/scan/tests/handler_test.go backend/service/scan/tests/scan_config_service_test.go backend/service/scan/tests/validator_test.go backend/service/scan/tests/base_service_test.go
git commit -m "feat(scan): add RefreshGaugeMetrics to SchedulerService"
```

---

### Task 8: Wire RefreshGaugeMetrics into the scheduler tick + main.go

**Files:**

- Modify: `backend/apps/scheduler/interfaces.go`
- Modify: `backend/apps/scheduler/scheduler.go`
- Modify: `backend/apps/scheduler/main.go`
- Modify: `backend/apps/scheduler/main_test.go`
- Test: `backend/apps/scheduler/scheduler_test.go`

**Interfaces:**

- Consumes: `scansvc.SchedulerService.RefreshGaugeMetrics` (Task 7), `crawlernodesvc.NewNodeRepositoryService` (Task 4, already exists as a constructor), `companysvc.NewScanSettingsRepositoryService` (Task 5, already exists as a constructor).
- Produces: nothing new — this is the final wiring task for the gauge-refresh feature. After this task, `jobradar_scheduler_queue_pending` and the other 4 gauges are live in Mimir every scheduler tick (default 60s).

- [ ] **Step 1: Add RefreshGaugeMetrics to the local SchedulerService interface**

Edit `backend/apps/scheduler/interfaces.go`. Add after `AutoPauseChronicFailures`:

```go
	AutoPauseChronicFailures(ctx context.Context) (int, error)
	// RefreshGaugeMetrics recomputes the current-state gauge metrics for the
	// Overview dashboard stat strip. Always returns a non-nil error only when
	// at least one of the 5 underlying reads failed — partial failure never
	// blocks the others.
	RefreshGaugeMetrics(ctx context.Context) error
}
```

- [ ] **Step 2: Write the failing scheduler.go tests**

Add to `backend/apps/scheduler/scheduler_test.go`. First add a `refreshGaugeErr error` field and `refreshGaugeCalls atomic.Int64` counter to `fakeSchedulerService` (in `main_test.go`, per Step 5 below — write both test files' changes together since they share the fake), then:

```go
func TestRunCycle_RefreshesGaugeMetrics(t *testing.T) {
	svc := &fakeSchedulerService{}
	runCycle(context.Background(), logger.NewNoop(), svc, 1440, 30*time.Minute)

	if svc.refreshGaugeCalls.Load() != 1 {
		t.Errorf("RefreshGaugeMetrics calls = %d, want 1", svc.refreshGaugeCalls.Load())
	}
}

func TestRunCycle_RefreshGaugeMetricsError_ContinuesToEnqueue(t *testing.T) {
	svc := &fakeSchedulerService{refreshGaugeErr: errors.New("db down"), enqueueCount: 2}
	// Must not abort the cycle — enqueue still runs after a gauge-refresh failure.
	runCycle(context.Background(), logger.NewNoop(), svc, 1440, 30*time.Minute)

	if svc.enqueueCalls.Load() != 1 {
		t.Errorf("EnqueueDue calls = %d, want 1 (gauge-refresh failure must not block enqueue)", svc.enqueueCalls.Load())
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `cd backend && go test ./apps/scheduler/... -run 'TestRunCycle_RefreshesGaugeMetrics|TestRunCycle_RefreshGaugeMetricsError' -v`
Expected: FAIL — compile error, `fakeSchedulerService` doesn't implement `RefreshGaugeMetrics` yet and `refreshGaugeCalls` field doesn't exist.

- [ ] **Step 4: Wire RefreshGaugeMetrics into runCycle**

Edit `backend/apps/scheduler/scheduler.go`. Add a call inside `runCycle`, after the `AutoPauseChronicFailures` block and before `RetryDueJobs` (ordering doesn't matter functionally — placed here to group it near the other non-enqueue housekeeping calls):

```go
	if n, err := svc.AutoPauseChronicFailures(ctx); err != nil {
		metrics.SchedulerErrorsTotal.WithLabelValues("auto_pause").Inc()
		log.Error("auto-pause chronic failures failed", err)
	} else if n > 0 {
		log.Info("auto-paused chronic-failing companies", logger.Int(logger.FieldCount, n))
	}

	if err := svc.RefreshGaugeMetrics(ctx); err != nil {
		metrics.SchedulerErrorsTotal.WithLabelValues("refresh_gauge_metrics").Inc()
		log.Error("refresh gauge metrics failed", err)
	}

	if rerr := svc.RetryDueJobs(ctx); rerr != nil {
```

- [ ] **Step 5: Update the fakeSchedulerService test double**

Edit `backend/apps/scheduler/main_test.go`. Add a field and method to `fakeSchedulerService`:

```go
	autoPauseErr     error
	autoPauseN       int
	resetStuckArg    time.Duration
	refreshGaugeErr  error
```

```go
	autoPauseCalls     atomic.Int64
	refreshGaugeCalls  atomic.Int64
```

```go
func (f *fakeSchedulerService) RefreshGaugeMetrics(_ context.Context) error {
	f.mu.Lock()
	err := f.refreshGaugeErr
	f.mu.Unlock()
	f.refreshGaugeCalls.Add(1)
	return err
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `cd backend && go test ./apps/scheduler/... -run 'TestRunCycle_RefreshesGaugeMetrics|TestRunCycle_RefreshGaugeMetricsError' -v`
Expected: PASS (2 tests).

- [ ] **Step 7: Wire NodeReader/SettingsReader into main.go**

Edit `backend/apps/scheduler/main.go`. Add 2 imports:

```go
	companysvc "github.com/trushilpatel/jobradar/service/company/services"
	crawlernodesvc "github.com/trushilpatel/jobradar/service/crawlernode/services"
```

Add the constructions and wire them into `SchedulerServiceOptions`:

```go
	auditWriter := auditsvc.NewImplService(auditsvc.NewRepositoryService(database))

	sweeperOpts := scansvc.SchedulerServiceOptions{
		Ledger:         jobsvc.NewSeenLedgerStore(database),
		Publisher:      lazyPub,
		Auditor:        auditWriter,
		NodeReader:     crawlernodesvc.NewNodeRepositoryService(database),
		SettingsReader: companysvc.NewScanSettingsRepositoryService(database),
		MaxRetries:     cfg.Scheduler.SweeperMaxRetries,
		Log:            log,
	}
```

- [ ] **Step 8: Build and verify**

Run: `cd backend && go build ./apps/scheduler/...`
Expected: no errors.

Run: `cd backend && go test ./apps/scheduler/... -race`
Expected: PASS, full package.

Run: `cd backend && golangci-lint run --config ../.github/.golangci.yml --timeout 5m ./apps/scheduler/...`
Expected: no new lint findings.

- [ ] **Step 9: Run the entire backend test suite as a final ripple check**

Run: `cd backend && go test -race -short ./...`
Expected: PASS across every package — this catches any remaining fake/interface break not covered by the per-package steps above (e.g. an orchestrator-side fake that happens to reference `scansvc.Service` indirectly).

- [ ] **Step 10: Commit**

```bash
git add backend/apps/scheduler/interfaces.go backend/apps/scheduler/scheduler.go backend/apps/scheduler/main.go backend/apps/scheduler/main_test.go backend/apps/scheduler/scheduler_test.go
git commit -m "feat(scheduler): refresh gauge metrics every tick, wire node/settings readers"
```

---

### Task 9: Overview dashboard JSON

**Files:**

- Create: `infra/observability/dashboards/overview.json`

**Interfaces:**

- Consumes: every metric referenced below — all now live in Mimir after Tasks 1–8 (orchestrator HTTP metrics + 5 new gauges) plus the pre-existing counters/histograms.

- [ ] **Step 1: Create the dashboard JSON**

Create `infra/observability/dashboards/overview.json`:

```json
{
  "title": "JobRadar — Pipeline Overview",
  "uid": "jobradar-pipeline-overview",
  "tags": ["jobradar", "pipeline"],
  "timezone": "browser",
  "schemaVersion": 39,
  "version": 1,
  "refresh": "30s",
  "time": { "from": "now-6h", "to": "now" },
  "templating": {
    "list": [
      {
        "name": "datasource",
        "type": "datasource",
        "query": "prometheus",
        "current": {},
        "hide": 0,
        "refresh": 1
      }
    ]
  },
  "panels": [
    {
      "id": 1,
      "type": "stat",
      "title": "Queue Backlog",
      "gridPos": { "x": 0, "y": 0, "w": 6, "h": 4 },
      "datasource": { "type": "prometheus", "uid": "${datasource}" },
      "targets": [
        {
          "refId": "A",
          "expr": "jobradar_scheduler_queue_pending{job=\"jobradar_scheduler\"}",
          "legendFormat": "Pending"
        },
        {
          "refId": "B",
          "expr": "jobradar_scheduler_queue_oldest_pending_age_seconds{job=\"jobradar_scheduler\"}",
          "legendFormat": "Oldest Age"
        }
      ],
      "fieldConfig": {
        "defaults": { "unit": "short" },
        "overrides": [
          {
            "matcher": { "id": "byFrameRefID", "options": "B" },
            "properties": [{ "id": "unit", "value": "s" }]
          }
        ]
      },
      "options": { "reduceOptions": { "calcs": ["lastNotNull"] } }
    },
    {
      "id": 2,
      "type": "stat",
      "title": "DLQ + Retry Ladder",
      "gridPos": { "x": 6, "y": 0, "w": 6, "h": 4 },
      "datasource": { "type": "prometheus", "uid": "${datasource}" },
      "targets": [
        {
          "refId": "A",
          "expr": "sum(rate(filter_dlq_publishes_total{job=\"jobradar_filter\"}[5m])) + sum(rate(enricher_dlq_publishes_total{job=\"jobradar_enricher\"}[5m]))",
          "legendFormat": "DLQ Publish Rate (5m)"
        },
        {
          "refId": "B",
          "expr": "jobradar_job_pipeline_ledger_retry_count{job=\"jobradar_scheduler\"}",
          "legendFormat": "Retry Ladder (current)"
        }
      ],
      "fieldConfig": { "defaults": { "unit": "short" } },
      "options": { "reduceOptions": { "calcs": ["lastNotNull"] } }
    },
    {
      "id": 3,
      "type": "stat",
      "title": "Crawler Capacity",
      "gridPos": { "x": 12, "y": 0, "w": 6, "h": 4 },
      "datasource": { "type": "prometheus", "uid": "${datasource}" },
      "targets": [
        {
          "refId": "A",
          "expr": "jobradar_crawler_nodes_online{job=\"jobradar_scheduler\"}",
          "legendFormat": "Online Nodes"
        },
        {
          "refId": "B",
          "expr": "jobradar_scheduler_companies_auto_paused_current{job=\"jobradar_scheduler\"}",
          "legendFormat": "Auto-Paused Companies"
        }
      ],
      "fieldConfig": { "defaults": { "unit": "short" } },
      "options": { "reduceOptions": { "calcs": ["lastNotNull"] } }
    },
    {
      "id": 4,
      "type": "stat",
      "title": "DB Pooler Health",
      "gridPos": { "x": 18, "y": 0, "w": 6, "h": 4 },
      "datasource": { "type": "prometheus", "uid": "${datasource}" },
      "targets": [
        {
          "refId": "A",
          "expr": "sum(rate(jobradar_db_transient_retries_total[5m]))",
          "legendFormat": "Transient Retry Rate (5m)"
        }
      ],
      "fieldConfig": { "defaults": { "unit": "reqps" } },
      "options": { "reduceOptions": { "calcs": ["lastNotNull"] } }
    },

    {
      "id": 10,
      "type": "timeseries",
      "title": "Scheduler — Rate",
      "gridPos": { "x": 0, "y": 4, "w": 8, "h": 8 },
      "datasource": { "type": "prometheus", "uid": "${datasource}" },
      "targets": [
        {
          "refId": "A",
          "expr": "rate(scheduler_companies_enqueued_total{job=\"jobradar_scheduler\"}[5m])",
          "legendFormat": "Companies Enqueued/s"
        }
      ]
    },
    {
      "id": 11,
      "type": "timeseries",
      "title": "Scheduler — Errors",
      "gridPos": { "x": 8, "y": 4, "w": 8, "h": 8 },
      "datasource": { "type": "prometheus", "uid": "${datasource}" },
      "targets": [
        {
          "refId": "A",
          "expr": "sum(rate(scheduler_errors_total{job=\"jobradar_scheduler\"}[5m])) by (reason)",
          "legendFormat": "{{reason}}"
        }
      ]
    },
    {
      "id": 12,
      "type": "timeseries",
      "title": "Scheduler — Duration",
      "gridPos": { "x": 16, "y": 4, "w": 8, "h": 8 },
      "datasource": { "type": "prometheus", "uid": "${datasource}" },
      "targets": [
        {
          "refId": "A",
          "expr": "histogram_quantile(0.50, sum(rate(scheduler_run_duration_seconds_bucket{job=\"jobradar_scheduler\"}[5m])) by (le))",
          "legendFormat": "p50"
        },
        {
          "refId": "B",
          "expr": "histogram_quantile(0.95, sum(rate(scheduler_run_duration_seconds_bucket{job=\"jobradar_scheduler\"}[5m])) by (le))",
          "legendFormat": "p95"
        }
      ],
      "fieldConfig": { "defaults": { "unit": "s" } }
    },

    {
      "id": 20,
      "type": "timeseries",
      "title": "Orchestrator — Rate",
      "gridPos": { "x": 0, "y": 12, "w": 8, "h": 8 },
      "datasource": { "type": "prometheus", "uid": "${datasource}" },
      "targets": [
        {
          "refId": "A",
          "expr": "sum(rate(http_requests_total{job=\"jobradar_orchestrator\"}[5m]))",
          "legendFormat": "Requests/s"
        }
      ]
    },
    {
      "id": 21,
      "type": "timeseries",
      "title": "Orchestrator — Errors",
      "gridPos": { "x": 8, "y": 12, "w": 8, "h": 8 },
      "datasource": { "type": "prometheus", "uid": "${datasource}" },
      "targets": [
        {
          "refId": "A",
          "expr": "sum(rate(http_requests_total{job=\"jobradar_orchestrator\", status=~\"5..\"}[5m]))",
          "legendFormat": "5xx/s"
        }
      ]
    },
    {
      "id": 22,
      "type": "timeseries",
      "title": "Orchestrator — Duration",
      "gridPos": { "x": 16, "y": 12, "w": 8, "h": 8 },
      "datasource": { "type": "prometheus", "uid": "${datasource}" },
      "targets": [
        {
          "refId": "A",
          "expr": "histogram_quantile(0.50, sum(rate(http_request_duration_seconds_bucket{job=\"jobradar_orchestrator\"}[5m])) by (le))",
          "legendFormat": "p50"
        },
        {
          "refId": "B",
          "expr": "histogram_quantile(0.95, sum(rate(http_request_duration_seconds_bucket{job=\"jobradar_orchestrator\"}[5m])) by (le))",
          "legendFormat": "p95"
        }
      ],
      "fieldConfig": { "defaults": { "unit": "s" } }
    },

    {
      "id": 30,
      "type": "timeseries",
      "title": "Crawler-node — Rate",
      "gridPos": { "x": 0, "y": 20, "w": 8, "h": 8 },
      "datasource": { "type": "prometheus", "uid": "${datasource}" },
      "targets": [
        {
          "refId": "A",
          "expr": "sum(rate(crawler_scans_total{job=\"jobradar_crawler_node\"}[5m])) by (status)",
          "legendFormat": "{{status}}"
        }
      ]
    },
    {
      "id": 31,
      "type": "timeseries",
      "title": "Crawler-node — Errors",
      "gridPos": { "x": 8, "y": 20, "w": 8, "h": 8 },
      "datasource": { "type": "prometheus", "uid": "${datasource}" },
      "targets": [
        {
          "refId": "A",
          "expr": "sum(rate(crawler_publish_failures_total{job=\"jobradar_crawler_node\"}[5m])) by (adapter)",
          "legendFormat": "{{adapter}} publish failures/s"
        }
      ]
    },
    {
      "id": 32,
      "type": "timeseries",
      "title": "Crawler-node — Duration",
      "gridPos": { "x": 16, "y": 20, "w": 8, "h": 8 },
      "datasource": { "type": "prometheus", "uid": "${datasource}" },
      "targets": [
        {
          "refId": "A",
          "expr": "histogram_quantile(0.50, sum(rate(crawler_scan_duration_seconds_bucket{job=\"jobradar_crawler_node\"}[5m])) by (le))",
          "legendFormat": "p50"
        },
        {
          "refId": "B",
          "expr": "histogram_quantile(0.95, sum(rate(crawler_scan_duration_seconds_bucket{job=\"jobradar_crawler_node\"}[5m])) by (le))",
          "legendFormat": "p95"
        }
      ],
      "fieldConfig": { "defaults": { "unit": "s" } }
    },

    {
      "id": 40,
      "type": "timeseries",
      "title": "Filter — Rate",
      "gridPos": { "x": 0, "y": 28, "w": 8, "h": 8 },
      "datasource": { "type": "prometheus", "uid": "${datasource}" },
      "targets": [
        {
          "refId": "A",
          "expr": "sum(rate(filter_jobs_total{job=\"jobradar_filter\"}[5m])) by (outcome)",
          "legendFormat": "{{outcome}}"
        }
      ]
    },
    {
      "id": 41,
      "type": "timeseries",
      "title": "Filter — Errors",
      "gridPos": { "x": 8, "y": 28, "w": 8, "h": 8 },
      "datasource": { "type": "prometheus", "uid": "${datasource}" },
      "targets": [
        {
          "refId": "A",
          "expr": "sum(rate(filter_commit_failures_total{job=\"jobradar_filter\"}[5m])) + sum(rate(filter_retry_publish_failures_total{job=\"jobradar_filter\"}[5m])) + sum(rate(filter_publish_failures_total{job=\"jobradar_filter\"}[5m]))",
          "legendFormat": "Errors/s (commit+retry+publish)"
        }
      ]
    },
    {
      "id": 42,
      "type": "timeseries",
      "title": "Filter — Duration",
      "gridPos": { "x": 16, "y": 28, "w": 8, "h": 8 },
      "datasource": { "type": "prometheus", "uid": "${datasource}" },
      "targets": [
        {
          "refId": "A",
          "expr": "histogram_quantile(0.50, sum(rate(filter_chunk_duration_seconds_bucket{job=\"jobradar_filter\"}[5m])) by (le))",
          "legendFormat": "p50"
        },
        {
          "refId": "B",
          "expr": "histogram_quantile(0.95, sum(rate(filter_chunk_duration_seconds_bucket{job=\"jobradar_filter\"}[5m])) by (le))",
          "legendFormat": "p95"
        }
      ],
      "fieldConfig": { "defaults": { "unit": "s" } }
    },

    {
      "id": 50,
      "type": "timeseries",
      "title": "Analyzer — Rate",
      "gridPos": { "x": 0, "y": 36, "w": 8, "h": 8 },
      "datasource": { "type": "prometheus", "uid": "${datasource}" },
      "targets": [
        {
          "refId": "A",
          "expr": "sum(rate(enricher_jobs_total{job=\"jobradar_enricher\"}[5m])) by (outcome)",
          "legendFormat": "{{outcome}}"
        }
      ]
    },
    {
      "id": 51,
      "type": "timeseries",
      "title": "Analyzer — Errors",
      "gridPos": { "x": 8, "y": 36, "w": 8, "h": 8 },
      "datasource": { "type": "prometheus", "uid": "${datasource}" },
      "targets": [
        {
          "refId": "A",
          "expr": "sum(rate(enricher_commit_failures_total{job=\"jobradar_enricher\"}[5m])) + sum(rate(enricher_retry_publish_failures_total{job=\"jobradar_enricher\"}[5m])) + sum(rate(enricher_retry_ledger_failures_total{job=\"jobradar_enricher\"}[5m]))",
          "legendFormat": "Errors/s"
        }
      ]
    },
    {
      "id": 52,
      "type": "timeseries",
      "title": "Analyzer — Duration",
      "gridPos": { "x": 16, "y": 36, "w": 8, "h": 8 },
      "datasource": { "type": "prometheus", "uid": "${datasource}" },
      "targets": [
        {
          "refId": "A",
          "expr": "histogram_quantile(0.50, sum(rate(enricher_group_duration_seconds_bucket{job=\"jobradar_enricher\"}[5m])) by (le))",
          "legendFormat": "p50"
        },
        {
          "refId": "B",
          "expr": "histogram_quantile(0.95, sum(rate(enricher_group_duration_seconds_bucket{job=\"jobradar_enricher\"}[5m])) by (le))",
          "legendFormat": "p95"
        }
      ],
      "fieldConfig": { "defaults": { "unit": "s" } }
    },

    {
      "id": 60,
      "type": "timeseries",
      "title": "API — Rate",
      "gridPos": { "x": 0, "y": 44, "w": 8, "h": 8 },
      "datasource": { "type": "prometheus", "uid": "${datasource}" },
      "targets": [
        {
          "refId": "A",
          "expr": "sum(rate(http_requests_total{job=\"jobradar_api\"}[5m]))",
          "legendFormat": "Requests/s"
        }
      ]
    },
    {
      "id": 61,
      "type": "timeseries",
      "title": "API — Errors",
      "gridPos": { "x": 8, "y": 44, "w": 8, "h": 8 },
      "datasource": { "type": "prometheus", "uid": "${datasource}" },
      "targets": [
        {
          "refId": "A",
          "expr": "sum(rate(http_requests_total{job=\"jobradar_api\", status=~\"5..\"}[5m]))",
          "legendFormat": "5xx/s"
        }
      ]
    },
    {
      "id": 62,
      "type": "timeseries",
      "title": "API — Duration",
      "gridPos": { "x": 16, "y": 44, "w": 8, "h": 8 },
      "datasource": { "type": "prometheus", "uid": "${datasource}" },
      "targets": [
        {
          "refId": "A",
          "expr": "histogram_quantile(0.50, sum(rate(http_request_duration_seconds_bucket{job=\"jobradar_api\"}[5m])) by (le))",
          "legendFormat": "p50"
        },
        {
          "refId": "B",
          "expr": "histogram_quantile(0.95, sum(rate(http_request_duration_seconds_bucket{job=\"jobradar_api\"}[5m])) by (le))",
          "legendFormat": "p95"
        }
      ],
      "fieldConfig": { "defaults": { "unit": "s" } }
    }
  ]
}
```

- [ ] **Step 2: Validate the JSON is well-formed**

Run: `python3 -m json.tool infra/observability/dashboards/overview.json > /dev/null && echo "valid JSON"`
Expected: `valid JSON` — catches any syntax error before it reaches Grafana Cloud.

- [ ] **Step 3: Commit**

```bash
git add infra/observability/dashboards/overview.json
git commit -m "feat(observability): add Pipeline Overview dashboard JSON"
```

---

### Task 10: Overview alert rules YAML

**Files:**

- Create: `infra/observability/alerts/overview-alerts.yaml`

**Interfaces:**

- Consumes: the same metrics as Task 9. Notification routing (contact point) is intentionally left unconfigured per the approved design — these rules will evaluate and enter `Alerting` state but produce no notification until a contact point is wired in a follow-up pass.

- [ ] **Step 1: Create the alert rules YAML**

Create `infra/observability/alerts/overview-alerts.yaml`:

```yaml
# JobRadar Overview dashboard alert rules — Grafana Cloud unified alerting
# provisioning format. Notification routing is intentionally unconfigured:
# these rules evaluate and enter "Alerting" state but produce no
# notification until a contact point is wired in a follow-up pass (see
# docs/superpowers/specs/2026-07-02-grafana-observability-dashboards-design.md).
#
# datasourceUid below is a placeholder — resolve it via the Grafana MCP
# tools (list datasources, find the Prometheus/Mimir one) before pushing.
# Unlike dashboard JSON, alert rule queries cannot use a templated
# "${datasource}" variable — they evaluate independently of any dashboard.
apiVersion: 1

groups:
  - orgId: 1
    name: jobradar-overview-alerts
    folder: JobRadar
    interval: 1m
    rules:
      - uid: jobradar-queue-backlog
        title: QueueBacklog
        condition: C
        for: 10m
        noDataState: NoData
        execErrState: Error
        annotations:
          summary: 'company_scan_queue oldest-pending row has been waiting longer than 2x the default scan interval'
        data:
          - refId: A
            datasourceUid: REPLACE_WITH_PROMETHEUS_DATASOURCE_UID
            model:
              expr: jobradar_scheduler_queue_oldest_pending_age_seconds{job="jobradar_scheduler"}
              instant: true
              refId: A
          - refId: B
            datasourceUid: __expr__
            model:
              type: reduce
              expression: A
              reducer: last
              refId: B
          - refId: C
            datasourceUid: __expr__
            model:
              type: threshold
              expression: B
              # 172800s = 2 x SCAN_INTERVAL_MINUTES default (1440min = 24h).
              # Update this if SCAN_INTERVAL_MINUTES changes from its default.
              conditions:
                - evaluator: { type: gt, params: [172800] }
              refId: C

      - uid: jobradar-dlq-publish-rate
        title: DLQPublishRateNonZero
        condition: C
        for: 5m
        noDataState: NoData
        execErrState: Error
        annotations:
          summary: 'Filter or analyzer is publishing to the DLQ — jobs are failing terminally'
        data:
          - refId: A
            datasourceUid: REPLACE_WITH_PROMETHEUS_DATASOURCE_UID
            model:
              expr: sum(rate(filter_dlq_publishes_total{job="jobradar_filter"}[5m])) + sum(rate(enricher_dlq_publishes_total{job="jobradar_enricher"}[5m]))
              instant: true
              refId: A
          - refId: B
            datasourceUid: __expr__
            model:
              type: reduce
              expression: A
              reducer: last
              refId: B
          - refId: C
            datasourceUid: __expr__
            model:
              type: threshold
              expression: B
              conditions:
                - evaluator: { type: gt, params: [0] }
              refId: C

      - uid: jobradar-retry-ladder-backlog
        title: RetryLadderBacklog
        condition: C
        for: 30m
        noDataState: NoData
        execErrState: Error
        annotations:
          summary: 'job_pipeline_ledger retry-ladder backlog has stayed above 50 for 30 minutes — sweeper may not be draining it. Initial conservative threshold; tune once real traffic baselines are observed.'
        data:
          - refId: A
            datasourceUid: REPLACE_WITH_PROMETHEUS_DATASOURCE_UID
            model:
              expr: jobradar_job_pipeline_ledger_retry_count{job="jobradar_scheduler"}
              instant: true
              refId: A
          - refId: B
            datasourceUid: __expr__
            model:
              type: reduce
              expression: A
              reducer: last
              refId: B
          - refId: C
            datasourceUid: __expr__
            model:
              type: threshold
              expression: B
              conditions:
                - evaluator: { type: gt, params: [50] }
              refId: C

      - uid: jobradar-db-transient-retry-spike
        title: DBTransientRetrySpike
        condition: C
        for: 10m
        noDataState: NoData
        execErrState: Error
        annotations:
          summary: 'DB transient connection-drop retries have been continuously non-zero for 10 minutes — possible pooler or network instability'
        data:
          - refId: A
            datasourceUid: REPLACE_WITH_PROMETHEUS_DATASOURCE_UID
            model:
              expr: sum(rate(jobradar_db_transient_retries_total[5m]))
              instant: true
              refId: A
          - refId: B
            datasourceUid: __expr__
            model:
              type: reduce
              expression: A
              reducer: last
              refId: B
          - refId: C
            datasourceUid: __expr__
            model:
              type: threshold
              expression: B
              conditions:
                - evaluator: { type: gt, params: [0] }
              refId: C
```

- [ ] **Step 2: Validate the YAML is well-formed**

Run: `python3 -c "import yaml; yaml.safe_load(open('infra/observability/alerts/overview-alerts.yaml'))" && echo "valid YAML"`
Expected: `valid YAML`.

- [ ] **Step 3: Commit**

```bash
git add infra/observability/alerts/overview-alerts.yaml
git commit -m "feat(observability): add Pipeline Overview alert rules YAML"
```

---

### Task 11: Push to Grafana Cloud and verify (manual, MCP-tool-assisted)

**Files:** none — this task pushes the artifacts from Tasks 9–10, it does not create new files.

**Interfaces:**

- Consumes: Grafana MCP tools (list datasources, push dashboard, push alert rules) — only usable once the Grafana MCP server is active in this session (per the user's note that it requires a Claude Code restart).

- [ ] **Step 1: Resolve the datasource UID**

Use the Grafana MCP tools to list datasources in the Grafana Cloud instance and identify the Prometheus/Mimir one (its name will match the pattern `grafanacloud-<stack>-prom`). Note its UID.

- [ ] **Step 2: Replace the alert-rule placeholder**

Edit `infra/observability/alerts/overview-alerts.yaml` — replace all 4 occurrences of `REPLACE_WITH_PROMETHEUS_DATASOURCE_UID` with the real UID from Step 1.

- [ ] **Step 3: Push the dashboard**

Use the Grafana MCP tools to push `infra/observability/dashboards/overview.json` to the Grafana Cloud instance (creating it under a "JobRadar" folder if the tool requires one).

- [ ] **Step 4: Push the alert rules**

Use the Grafana MCP tools to push `infra/observability/alerts/overview-alerts.yaml`.

- [ ] **Step 5: Verify panels render with real data**

Open the pushed dashboard. Confirm:

- The 5 gauge-backed stat-strip panels (Queue Backlog, Crawler Capacity's 2 series, Retry Ladder) show non-error values (0 is fine, "No data" is not) — this confirms the scheduler's `RefreshGaugeMetrics` tick is reaching Mimir.
- The Orchestrator row shows non-zero request-rate data after hitting any orchestrator endpoint (e.g. a crawler-node's next `GetWork` poll) — confirms Task 1's middleware wiring reached Mimir.
- Every other row (scheduler, crawler-node, filter, analyzer, api) shows data consistent with recent pipeline activity.

- [ ] **Step 6: Verify alert rules evaluate without a query error**

In Grafana Cloud's Alerting UI, confirm all 4 rules show a `Normal` or `Pending`/`Alerting` state (not `Error`) after their first evaluation cycle — an `Error` state means the PromQL or datasource UID is wrong.

- [ ] **Step 7: Commit the resolved datasource UID**

```bash
git add infra/observability/alerts/overview-alerts.yaml
git commit -m "chore(observability): resolve Prometheus datasource UID in alert rules"
```
