# Company Scan Queue Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the Kafka `company.scan_request` hop with a persistent `company_scan_queue` DB table that provides dedup, visibility, and crash recovery.

**Architecture:** A new `company_scan_queue` table with a unique partial index enforces at-most-one active entry per company at the DB level. The scheduler and manual API trigger insert into this table (ON CONFLICT DO NOTHING). The crawler polls the table with `FOR UPDATE SKIP LOCKED` to claim items atomically. A new `GET /companies/queue` endpoint exposes queue state.

**Tech Stack:** Go, PostgreSQL (goose migrations), GORM raw SQL, chi router

---

## File Map

| File | Change |
|---|---|
| `backend/migrations/014_company_scan_queue.sql` | Create — new table + unique partial index |
| `backend/internal/model/constants.go` | Modify — add QueueStatus* and QueueSource* constants |
| `backend/internal/store/scan_queue.go` | Create — ScanQueueStore interface + implementation |
| `backend/cmd/company_scan_scheduler/main.go` | Modify — remove Kafka, use ScanQueueStore |
| `backend/cmd/company_scan_scheduler/main_test.go` | Modify — replace fakePublisher with fakeScanQueueStore |
| `backend/cmd/job_listing_crawler/main.go` | Modify — remove Kafka consumer, add DB poll loop |
| `backend/internal/api/handler/companies.go` | Modify — replace kafka.Publisher with ScanQueueStore, add ListQueue |
| `backend/internal/api/handler/fakes_test.go` | Modify — remove fakePublisher, add fakeScanQueueStore |
| `backend/internal/api/handler/companies_test.go` | Modify — update TriggerScan assertions, add TestListQueue |
| `backend/internal/api/router.go` | Modify — add GET /companies/queue route |
| `backend/cmd/api/main.go` | Modify — remove Kafka setup, wire ScanQueueStore |

---

### Task 1: Migration

**Files:**
- Create: `backend/migrations/014_company_scan_queue.sql`

- [ ] **Step 1: Create the migration file**

```sql
-- +goose Up
CREATE TABLE company_scan_queue (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id  TEXT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    status      TEXT NOT NULL DEFAULT 'queued',
    source      TEXT NOT NULL DEFAULT 'scheduler',
    queued_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at  TIMESTAMPTZ
);

-- Enforces at most one active entry per company at the DB level.
-- ON CONFLICT DO NOTHING in Enqueue relies on this index.
CREATE UNIQUE INDEX uq_company_scan_queue_active
    ON company_scan_queue(company_id)
    WHERE status IN ('queued', 'processing');

-- +goose Down
DROP TABLE company_scan_queue;
```

- [ ] **Step 2: Verify migration file is picked up by goose**

Run: `cd backend && go run ./cmd/migrate`  
Expected: migration `014` runs without error, table `company_scan_queue` exists.

---

### Task 2: Model Constants

**Files:**
- Modify: `backend/internal/model/constants.go`

- [ ] **Step 1: Add queue constants**

Add these blocks to the end of `backend/internal/model/constants.go`:

```go
// QueueStatus — values for company_scan_queue.status
const (
	QueueStatusQueued     = "queued"
	QueueStatusProcessing = "processing"
)

// QueueSource — who enqueued the company
const (
	QueueSourceScheduler = "scheduler"
	QueueSourceManual    = "manual"
)
```

---

### Task 3: ScanQueueStore

**Files:**
- Create: `backend/internal/store/scan_queue.go`

- [ ] **Step 1: Create the store file**

```go
package store

import (
	"context"
	"time"

	"gorm.io/gorm"
)

// ScanQueueItem is one row in company_scan_queue.
type ScanQueueItem struct {
	ID        string     `json:"id"`
	CompanyID string     `json:"company_id"`
	Status    string     `json:"status"`
	Source    string     `json:"source"`
	QueuedAt  time.Time  `json:"queued_at"`
	StartedAt *time.Time `json:"started_at"`
}

// ScanQueueStore manages the company scan work queue.
type ScanQueueStore interface {
	// Enqueue inserts a company into the queue. Silently no-ops if the company
	// already has an active entry (queued or processing).
	Enqueue(companyID, source string) error

	// ClaimNext atomically claims the next queued item and marks it processing.
	// Returns nil, nil if the queue is empty.
	ClaimNext(ctx context.Context) (*ScanQueueItem, error)

	// Complete removes the queue entry. Call after scan succeeds or fails.
	Complete(id string) error

	// ResetStuck resets processing rows that started more than staleDur ago back
	// to queued status. Call once on crawler startup to recover from crashes.
	ResetStuck(staleDur time.Duration) error

	// ListQueue returns all active queue entries ordered by queued_at ascending.
	ListQueue() ([]ScanQueueItem, error)
}

type scanQueueStore struct {
	db *gorm.DB
}

func NewScanQueueStore(db *gorm.DB) ScanQueueStore {
	return &scanQueueStore{db: db}
}

func (s *scanQueueStore) Enqueue(companyID, source string) error {
	return s.db.Exec(`
		INSERT INTO company_scan_queue (company_id, source)
		VALUES (?, ?)
		ON CONFLICT DO NOTHING
	`, companyID, source).Error
}

func (s *scanQueueStore) ClaimNext(ctx context.Context) (*ScanQueueItem, error) {
	var item ScanQueueItem
	err := s.db.WithContext(ctx).Raw(`
		UPDATE company_scan_queue
		SET status = 'processing', started_at = NOW()
		WHERE id = (
			SELECT id FROM company_scan_queue
			WHERE status = 'queued'
			ORDER BY queued_at ASC
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		)
		RETURNING id, company_id, status, source, queued_at, started_at
	`).Scan(&item).Error
	if err != nil {
		return nil, err
	}
	if item.ID == "" {
		return nil, nil
	}
	return &item, nil
}

func (s *scanQueueStore) Complete(id string) error {
	return s.db.Exec(`DELETE FROM company_scan_queue WHERE id = ?`, id).Error
}

func (s *scanQueueStore) ResetStuck(staleDur time.Duration) error {
	return s.db.Exec(`
		UPDATE company_scan_queue
		SET status = 'queued', started_at = NULL
		WHERE status = 'processing'
		  AND started_at < NOW() - INTERVAL '1 second' * ?
	`, int(staleDur.Seconds())).Error
}

func (s *scanQueueStore) ListQueue() ([]ScanQueueItem, error) {
	var items []ScanQueueItem
	err := s.db.Raw(`
		SELECT id, company_id, status, source, queued_at, started_at
		FROM company_scan_queue
		ORDER BY queued_at ASC
	`).Scan(&items).Error
	return items, err
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd backend && go build ./internal/store/...`  
Expected: exits 0, no errors.

---

### Task 4: Update Scheduler — Tests First

**Files:**
- Modify: `backend/cmd/company_scan_scheduler/main_test.go`

- [ ] **Step 1: Replace fakePublisher with fakeScanQueueStore**

Replace the entire contents of `backend/cmd/company_scan_scheduler/main_test.go`:

```go
package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/trushilpatel/jobradar/internal/logger"
	"github.com/trushilpatel/jobradar/internal/model"
	"github.com/trushilpatel/jobradar/internal/store"
)

// ── fakes ─────────────────────────────────────────────────────────────────────

type fakeCompanyReader struct {
	due []model.Company
	err error
}

func (f *fakeCompanyReader) GetCompaniesDueForScan(_ int) ([]model.Company, error) {
	return f.due, f.err
}
func (f *fakeCompanyReader) GetActiveCompanies() ([]model.Company, error)   { return nil, nil }
func (f *fakeCompanyReader) GetAllCompanies() ([]model.Company, error)      { return nil, nil }
func (f *fakeCompanyReader) GetCompanyByID(_ string) (model.Company, error) { return model.Company{}, nil }

type fakeScanQueueStore struct {
	enqueued []string // company IDs enqueued
	err      error
}

func (f *fakeScanQueueStore) Enqueue(companyID, _ string) error {
	if f.err != nil {
		return f.err
	}
	f.enqueued = append(f.enqueued, companyID)
	return nil
}
func (f *fakeScanQueueStore) ClaimNext(_ context.Context) (*store.ScanQueueItem, error) {
	return nil, f.err
}
func (f *fakeScanQueueStore) Complete(_ string) error              { return f.err }
func (f *fakeScanQueueStore) ResetStuck(_ time.Duration) error    { return f.err }
func (f *fakeScanQueueStore) ListQueue() ([]store.ScanQueueItem, error) { return nil, f.err }

// ── helpers ───────────────────────────────────────────────────────────────────

func company(id, name string) model.Company {
	return model.Company{ID: id, Name: name}
}

// ── tests ─────────────────────────────────────────────────────────────────────

func TestEnqueueDue_NoCompaniesDue(t *testing.T) {
	reader := &fakeCompanyReader{due: []model.Company{}}
	q := &fakeScanQueueStore{}

	enqueuedue(context.Background(), logger.NewNoop(), reader, q, 1440)

	if len(q.enqueued) != 0 {
		t.Errorf("expected no enqueues, got %d", len(q.enqueued))
	}
}

func TestEnqueueDue_EnqueuesAllDueCompanies(t *testing.T) {
	reader := &fakeCompanyReader{due: []model.Company{
		company("company-1", "Acme"),
		company("company-2", "Globex"),
		company("company-3", "Initech"),
	}}
	q := &fakeScanQueueStore{}

	enqueuedue(context.Background(), logger.NewNoop(), reader, q, 1440)

	if len(q.enqueued) != 3 {
		t.Fatalf("expected 3 enqueues, got %d", len(q.enqueued))
	}
	want := map[string]bool{"company-1": true, "company-2": true, "company-3": true}
	for _, id := range q.enqueued {
		if !want[id] {
			t.Errorf("unexpected company enqueued: %q", id)
		}
	}
}

func TestEnqueueDue_StoreError_EnqueuesNothing(t *testing.T) {
	reader := &fakeCompanyReader{err: errors.New("db down")}
	q := &fakeScanQueueStore{}

	enqueuedue(context.Background(), logger.NewNoop(), reader, q, 1440)

	if len(q.enqueued) != 0 {
		t.Errorf("expected no enqueues on store error, got %d", len(q.enqueued))
	}
}

func TestEnqueueDue_EnqueueError_ContinuesToNext(t *testing.T) {
	reader := &fakeCompanyReader{due: []model.Company{
		company("company-1", "Acme"),
		company("company-2", "Globex"),
	}}
	q := &fakeScanQueueStore{err: errors.New("queue full")}

	// Should not panic — logs and continues
	enqueuedue(context.Background(), logger.NewNoop(), reader, q, 1440)

	if len(q.enqueued) != 0 {
		t.Errorf("expected no successful enqueues, got %d", len(q.enqueued))
	}
}
```

- [ ] **Step 2: Run tests to confirm they fail (function signature mismatch)**

Run: `cd backend && go test ./cmd/company_scan_scheduler/...`  
Expected: compile error — `enqueuedue` takes `kafka.Publisher`, not `store.ScanQueueStore`.

---

### Task 5: Update Scheduler — Implementation

**Files:**
- Modify: `backend/cmd/company_scan_scheduler/main.go`

- [ ] **Step 1: Replace Kafka with ScanQueueStore in main.go**

Replace the entire contents of `backend/cmd/company_scan_scheduler/main.go`:

```go
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/trushilpatel/jobradar/config"
	"github.com/trushilpatel/jobradar/internal/db"
	"github.com/trushilpatel/jobradar/internal/logger"
	"github.com/trushilpatel/jobradar/internal/model"
	"github.com/trushilpatel/jobradar/internal/store"
)

const checkInterval = 1 * time.Minute

func main() {
	cfg := config.Load()
	log := logger.Init("scheduler")

	database, err := db.Open(cfg)
	if err != nil {
		log.Error("db open failed", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	companies := store.NewCompanyStore(database)
	queue := store.NewScanQueueStore(database)

	log.Info("started",
		logger.Int("scan_interval_minutes", cfg.ScanIntervalMinutes),
		logger.Dur("check_interval", checkInterval),
	)

	enqueuedue(ctx, log, companies, queue, cfg.ScanIntervalMinutes)

	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			enqueuedue(ctx, log, companies, queue, cfg.ScanIntervalMinutes)
		case <-ctx.Done():
			log.Info("shutting down")
			return
		}
	}
}

func enqueuedue(ctx context.Context, log logger.Logger, companies store.CompanyReader, queue store.ScanQueueStore, intervalMinutes int) {
	due, err := companies.GetCompaniesDueForScan(intervalMinutes)
	if err != nil {
		log.Error("failed to query due companies", err)
		return
	}
	if len(due) == 0 {
		return
	}

	log.Info("companies due for scan", logger.Int("count", len(due)))

	for _, c := range due {
		if err := queue.Enqueue(c.ID, model.QueueSourceScheduler); err != nil {
			log.Error("enqueue failed", err, logger.Str(logger.FieldCompany, c.ID))
			continue
		}
		log.Debug("queued company", logger.Str(logger.FieldCompany, c.ID), logger.Str("name", c.Name))
	}
}
```

- [ ] **Step 2: Run tests — they should now pass**

Run: `cd backend && go test ./cmd/company_scan_scheduler/...`  
Expected: all 4 tests pass, 0 failures.

---

### Task 6: Update Crawler

**Files:**
- Modify: `backend/cmd/job_listing_crawler/main.go`

- [ ] **Step 1: Replace Kafka consumer with DB poll loop**

Replace the entire contents of `backend/cmd/job_listing_crawler/main.go`:

```go
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/trushilpatel/jobradar/config"
	"github.com/trushilpatel/jobradar/internal/db"
	"github.com/trushilpatel/jobradar/internal/kafka"
	"github.com/trushilpatel/jobradar/internal/logger"
	"github.com/trushilpatel/jobradar/internal/store"
	"github.com/trushilpatel/jobradar/internal/worker/job_listing_crawler"
)

const pollInterval = 10 * time.Second
const stuckTimeout = 30 * time.Minute

func main() {
	cfg := config.Load()
	log := logger.Init("crawler")

	database, err := db.Open(cfg)
	if err != nil {
		log.Error("db open failed", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := kafka.EnsureTopics(ctx, cfg, kafka.TopicJobListingsRaw); err != nil {
		log.Error("kafka topics failed", err)
		os.Exit(1)
	}

	producer, err := kafka.NewProducer(cfg)
	if err != nil {
		log.Error("kafka producer failed", err)
		os.Exit(1)
	}
	defer producer.Close()

	companies := store.NewCompanyStore(database)
	ledger := store.NewSeenLedgerStore(database)
	scanLog := store.NewCompanyScanLogStore(database)
	queue := store.NewScanQueueStore(database)

	companyDelay := time.Duration(cfg.ScanCompanyDelaySeconds) * time.Second
	engine := job_listing_crawler.NewEngine(companies, ledger, scanLog, producer, companyDelay, log)

	// Reset any rows stuck in processing from a previous crash.
	if err := queue.ResetStuck(stuckTimeout); err != nil {
		log.Error("failed to reset stuck queue items", err)
		// non-fatal: continue
	}

	log.Info("worker started", logger.Dur("poll_interval", pollInterval))

	// Poll once immediately on startup, then on every tick.
	pollAndProcess(ctx, log, queue, engine)

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			pollAndProcess(ctx, log, queue, engine)
		case <-ctx.Done():
			log.Info("shutting down")
			return
		}
	}
}

func pollAndProcess(ctx context.Context, log logger.Logger, queue store.ScanQueueStore, engine *job_listing_crawler.Engine) {
	item, err := queue.ClaimNext(ctx)
	if err != nil {
		log.Error("failed to claim queue item", err)
		return
	}
	if item == nil {
		log.Debug("queue empty")
		return
	}

	log.Info("processing company from queue",
		logger.Str(logger.FieldCompany, item.CompanyID),
		logger.Str("source", item.Source),
	)

	engine.RunForCompany(ctx, item.CompanyID)

	if err := queue.Complete(item.ID); err != nil {
		log.Error("failed to complete queue item", err, logger.Str(logger.FieldCompany, item.CompanyID))
	}
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd backend && go build ./cmd/job_listing_crawler/...`  
Expected: exits 0, no errors.

---

### Task 7: Update CompanyHandler — Tests First

**Files:**
- Modify: `backend/internal/api/handler/fakes_test.go`
- Modify: `backend/internal/api/handler/companies_test.go`

- [ ] **Step 1: Remove fakePublisher, add fakeScanQueueStore in fakes_test.go**

In `backend/internal/api/handler/fakes_test.go`:

Remove the entire `fakePublisher` block (lines 263–278):
```go
// ── fakePublisher ─────────────────────────────────────────────────────────────

type fakePublisher struct {
	payloads [][]byte
	err      error
}

func (f *fakePublisher) Publish(_ context.Context, _ string, value []byte) error {
	if f.err != nil {
		return f.err
	}
	f.payloads = append(f.payloads, value)
	return nil
}
```

Add this block in its place:

```go
// ── fakeScanQueueStore ────────────────────────────────────────────────────────

type fakeScanQueueStore struct {
	items []store.ScanQueueItem
	err   error
}

func (f *fakeScanQueueStore) Enqueue(companyID, source string) error {
	if f.err != nil {
		return f.err
	}
	f.items = append(f.items, store.ScanQueueItem{
		ID:        "fake-id",
		CompanyID: companyID,
		Source:    source,
		Status:    model.QueueStatusQueued,
		QueuedAt:  time.Now(),
	})
	return nil
}
func (f *fakeScanQueueStore) ClaimNext(_ context.Context) (*store.ScanQueueItem, error) {
	return nil, f.err
}
func (f *fakeScanQueueStore) Complete(_ string) error           { return f.err }
func (f *fakeScanQueueStore) ResetStuck(_ time.Duration) error { return f.err }
func (f *fakeScanQueueStore) ListQueue() ([]store.ScanQueueItem, error) {
	return f.items, f.err
}
```

Also add `"time"` to the imports if not already present, and add:
- `"github.com/trushilpatel/jobradar/internal/model"` (already imported)

- [ ] **Step 2: Update companies_test.go**

Replace the entire contents of `backend/internal/api/handler/companies_test.go`:

```go
package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/trushilpatel/jobradar/internal/model"
)

func newCompanyHandler(cs *fakeCompanyStore, q *fakeScanQueueStore) *CompanyHandler {
	return NewCompanyHandler(cs, &fakeScanLogReader{}, q, 1440)
}

func testCompany(id string, lastScanAt *time.Time) model.Company {
	return model.Company{ID: id, Name: "Acme", URL: "https://example.com", AdapterType: model.AdapterGreenhouse, Active: true, LastScanAt: lastScanAt}
}

// ── List ──────────────────────────────────────────────────────────────────────

func TestCompanyList(t *testing.T) {
	cs := newFakeCompanyStore(testCompany("c1", nil), testCompany("c2", nil))
	h := newCompanyHandler(cs, &fakeScanQueueStore{})

	req := httptest.NewRequest(http.MethodGet, "/companies", nil)
	w := httptest.NewRecorder()
	h.List(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200 got %d", w.Code)
	}
}

// ── Create ────────────────────────────────────────────────────────────────────

func TestCompanyCreate_Valid(t *testing.T) {
	cs := newFakeCompanyStore()
	h := newCompanyHandler(cs, &fakeScanQueueStore{})

	body := strings.NewReader(`{"id":"stripe","name":"Stripe","url":"https://stripe.com","adapter_type":"greenhouse"}`)
	req := httptest.NewRequest(http.MethodPost, "/companies", body)
	w := httptest.NewRecorder()
	h.Create(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("want 201 got %d: %s", w.Code, w.Body.String())
	}
}

func TestCompanyCreate_MissingFields(t *testing.T) {
	h := newCompanyHandler(newFakeCompanyStore(), &fakeScanQueueStore{})

	body := strings.NewReader(`{"id":"stripe","adapter_type":"greenhouse"}`)
	req := httptest.NewRequest(http.MethodPost, "/companies", body)
	w := httptest.NewRecorder()
	h.Create(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400 got %d", w.Code)
	}
}

func TestCompanyCreate_InvalidAdapter(t *testing.T) {
	h := newCompanyHandler(newFakeCompanyStore(), &fakeScanQueueStore{})

	body := strings.NewReader(`{"id":"stripe","name":"Stripe","url":"https://stripe.com","adapter_type":"unknown"}`)
	req := httptest.NewRequest(http.MethodPost, "/companies", body)
	w := httptest.NewRecorder()
	h.Create(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400 got %d", w.Code)
	}
}

// ── Delete ────────────────────────────────────────────────────────────────────

func TestCompanyDelete_Found(t *testing.T) {
	cs := newFakeCompanyStore(testCompany("c1", nil))
	h := newCompanyHandler(cs, &fakeScanQueueStore{})

	r := chi.NewRouter()
	r.Delete("/companies/{id}", h.Delete)

	req := httptest.NewRequest(http.MethodDelete, "/companies/c1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200 got %d", w.Code)
	}
}

func TestCompanyDelete_NotFound(t *testing.T) {
	h := newCompanyHandler(newFakeCompanyStore(), &fakeScanQueueStore{})

	r := chi.NewRouter()
	r.Delete("/companies/{id}", h.Delete)

	req := httptest.NewRequest(http.MethodDelete, "/companies/missing", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404 got %d", w.Code)
	}
}

// ── TriggerScan ───────────────────────────────────────────────────────────────

func TestCompanyTriggerScan_Success(t *testing.T) {
	cs := newFakeCompanyStore(testCompany("c1", nil))
	q := &fakeScanQueueStore{}
	h := newCompanyHandler(cs, q)

	r := chi.NewRouter()
	r.Post("/companies/{id}/scan", h.TriggerScan)

	req := httptest.NewRequest(http.MethodPost, "/companies/c1/scan", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("want 202 got %d: %s", w.Code, w.Body.String())
	}
	if len(q.items) != 1 {
		t.Fatalf("expected 1 queue item, got %d", len(q.items))
	}
	if q.items[0].CompanyID != "c1" {
		t.Errorf("expected company_id c1, got %s", q.items[0].CompanyID)
	}
	if q.items[0].Source != model.QueueSourceManual {
		t.Errorf("expected source=%q, got %q", model.QueueSourceManual, q.items[0].Source)
	}
}

func TestCompanyTriggerScan_Cooldown(t *testing.T) {
	recent := time.Now().Add(-10 * time.Minute)
	cs := newFakeCompanyStore(testCompany("c1", &recent))
	h := newCompanyHandler(cs, &fakeScanQueueStore{})

	r := chi.NewRouter()
	r.Post("/companies/{id}/scan", h.TriggerScan)

	req := httptest.NewRequest(http.MethodPost, "/companies/c1/scan", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("want 429 got %d", w.Code)
	}
}

func TestCompanyTriggerScan_NotFound(t *testing.T) {
	h := newCompanyHandler(newFakeCompanyStore(), &fakeScanQueueStore{})

	r := chi.NewRouter()
	r.Post("/companies/{id}/scan", h.TriggerScan)

	req := httptest.NewRequest(http.MethodPost, "/companies/missing/scan", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404 got %d", w.Code)
	}
}

// ── ListQueue ─────────────────────────────────────────────────────────────────

func TestCompanyListQueue_Empty(t *testing.T) {
	h := newCompanyHandler(newFakeCompanyStore(), &fakeScanQueueStore{})

	req := httptest.NewRequest(http.MethodGet, "/companies/queue", nil)
	w := httptest.NewRecorder()
	h.ListQueue(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200 got %d", w.Code)
	}
}

func TestCompanyListQueue_WithItems(t *testing.T) {
	q := &fakeScanQueueStore{}
	_ = q.Enqueue("c1", model.QueueSourceScheduler)
	_ = q.Enqueue("c2", model.QueueSourceManual)
	h := newCompanyHandler(newFakeCompanyStore(), q)

	req := httptest.NewRequest(http.MethodGet, "/companies/queue", nil)
	w := httptest.NewRecorder()
	h.ListQueue(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200 got %d: %s", w.Code, w.Body.String())
	}
}
```

- [ ] **Step 3: Run tests to confirm they fail (CompanyHandler signature mismatch)**

Run: `cd backend && go test ./internal/api/handler/...`  
Expected: compile error — `NewCompanyHandler` still takes `kafka.Publisher`.

---

### Task 8: Update CompanyHandler — Implementation

**Files:**
- Modify: `backend/internal/api/handler/companies.go`

- [ ] **Step 1: Replace kafka.Publisher with ScanQueueStore, add ListQueue**

Replace the entire contents of `backend/internal/api/handler/companies.go`:

```go
package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/trushilpatel/jobradar/internal/model"
	"github.com/trushilpatel/jobradar/internal/store"
)

type CompanyHandler struct {
	companies           store.CompanyStore
	scanLog             store.CompanyScanLogReader
	queue               store.ScanQueueStore
	scanIntervalMinutes int
}

func NewCompanyHandler(companies store.CompanyStore, scanLog store.CompanyScanLogReader, queue store.ScanQueueStore, scanIntervalMinutes int) *CompanyHandler {
	return &CompanyHandler{companies: companies, scanLog: scanLog, queue: queue, scanIntervalMinutes: scanIntervalMinutes}
}

// GET /companies
func (h *CompanyHandler) List(w http.ResponseWriter, r *http.Request) {
	companies, err := h.companies.GetAllCompanies()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to fetch companies")
		return
	}
	respond(w, http.StatusOK, companies)
}

// POST /companies
func (h *CompanyHandler) Create(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		URL         string `json:"url"`
		AdapterType string `json:"adapter_type"`
		Active      *bool  `json:"active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.ID == "" || body.Name == "" || body.URL == "" {
		respondError(w, http.StatusBadRequest, "id, name, and url are required")
		return
	}

	validAdapters := map[string]bool{
		model.AdapterGreenhouse: true,
		model.AdapterLever:      true,
		model.AdapterAshby:      true,
		model.AdapterWorkday:    true,
		model.AdapterWorkable:   true,
		model.AdapterChromedp:   true,
	}
	if !validAdapters[body.AdapterType] {
		respondError(w, http.StatusBadRequest, "invalid adapter_type")
		return
	}

	active := true
	if body.Active != nil {
		active = *body.Active
	}

	company := model.Company{
		ID:          body.ID,
		Name:        body.Name,
		URL:         body.URL,
		AdapterType: body.AdapterType,
		Active:      active,
	}
	if err := h.companies.CreateCompany(company); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to create company")
		return
	}
	respond(w, http.StatusCreated, company)
}

// PATCH /companies/:id
func (h *CompanyHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	allowed := map[string]bool{"name": true, "url": true, "adapter_type": true, "active": true}
	fields := make(map[string]any, len(body))
	for k, v := range body {
		if allowed[k] {
			fields[k] = v
		}
	}
	if len(fields) == 0 {
		respondError(w, http.StatusBadRequest, "no valid fields to update")
		return
	}

	if at, ok := fields["adapter_type"].(string); ok {
		validAdapters := map[string]bool{
			model.AdapterGreenhouse: true,
			model.AdapterLever:      true,
			model.AdapterAshby:      true,
			model.AdapterWorkday:    true,
			model.AdapterWorkable:   true,
			model.AdapterChromedp:   true,
		}
		if !validAdapters[at] {
			respondError(w, http.StatusBadRequest, "invalid adapter_type")
			return
		}
	}

	if err := h.companies.UpdateCompany(id, fields); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to update company")
		return
	}
	respond(w, http.StatusOK, map[string]string{"id": id})
}

// DELETE /companies/:id
func (h *CompanyHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if _, err := h.companies.GetCompanyByID(id); err != nil {
		if store.IsNotFound(err) {
			respondError(w, http.StatusNotFound, "company not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to fetch company")
		return
	}

	if err := h.companies.DeleteCompany(id); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to delete company")
		return
	}
	respond(w, http.StatusOK, map[string]string{"id": id})
}

// GET /companies/:id/scan-stats
func (h *CompanyHandler) ScanStats(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	stats, err := h.scanLog.GetScanStats(id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to fetch scan stats")
		return
	}
	respond(w, http.StatusOK, stats)
}

// POST /companies/:id/scan
func (h *CompanyHandler) TriggerScan(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	company, err := h.companies.GetCompanyByID(id)
	if err != nil {
		if store.IsNotFound(err) {
			respondError(w, http.StatusNotFound, "company not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to fetch company")
		return
	}

	if company.LastScanAt != nil {
		nextAllowed := company.LastScanAt.Add(time.Duration(h.scanIntervalMinutes) * time.Minute)
		if time.Now().Before(nextAllowed) {
			remaining := time.Until(nextAllowed).Round(time.Minute)
			respondError(w, http.StatusTooManyRequests, "scanned too recently, next scan allowed in "+remaining.String())
			return
		}
	}

	if err := h.queue.Enqueue(id, model.QueueSourceManual); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to trigger scan")
		return
	}
	respond(w, http.StatusAccepted, map[string]string{"company_id": id, "status": "scan_queued"})
}

// GET /companies/queue
func (h *CompanyHandler) ListQueue(w http.ResponseWriter, r *http.Request) {
	items, err := h.queue.ListQueue()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to fetch queue")
		return
	}
	respond(w, http.StatusOK, items)
}
```

- [ ] **Step 2: Run handler tests — they should now pass**

Run: `cd backend && go test ./internal/api/handler/...`  
Expected: all tests pass, including the new `TestCompanyListQueue_*` tests.

---

### Task 9: Update Router and API main

**Files:**
- Modify: `backend/internal/api/router.go`
- Modify: `backend/cmd/api/main.go`

- [ ] **Step 1: Add GET /companies/queue route to router.go**

In `backend/internal/api/router.go`, add this line after `r.Get("/companies", companies.List)`:

```go
r.Get("/companies/queue", companies.ListQueue)
```

The relevant section should look like:

```go
r.Get("/companies", companies.List)
r.Get("/companies/queue", companies.ListQueue)
r.Post("/companies", companies.Create)
r.Patch("/companies/{id}", companies.Update)
r.Delete("/companies/{id}", companies.Delete)
r.Get("/companies/{id}/scan-stats", companies.ScanStats)
r.Post("/companies/{id}/scan", companies.TriggerScan)
```

Note: `GET /companies/queue` must be registered before `GET /companies/{id}` so chi matches the literal `queue` segment first.

- [ ] **Step 2: Update cmd/api/main.go — remove Kafka, wire ScanQueueStore**

Replace the entire contents of `backend/cmd/api/main.go`:

```go
package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/trushilpatel/jobradar/config"
	"github.com/trushilpatel/jobradar/internal/api"
	"github.com/trushilpatel/jobradar/internal/api/handler"
	"github.com/trushilpatel/jobradar/internal/db"
	"github.com/trushilpatel/jobradar/internal/logger"
	"github.com/trushilpatel/jobradar/internal/store"
)

func main() {
	cfg := config.Load()
	log := logger.Init("api")

	database, err := db.Open(cfg)
	if err != nil {
		log.Error("db open failed", err)
		os.Exit(1)
	}

	jobStore := store.NewJobStore(database)
	ledger := store.NewSeenLedgerStore(database)
	discardStore := store.NewDiscardStore(database)
	snapshotStore := store.NewSnapshotStore(database)
	appStore := store.NewApplicationStore(database)
	companyStore := store.NewCompanyStore(database)
	scanLogStore := store.NewCompanyScanLogStore(database)
	insightsStore := store.NewInsightsStore(database)
	scanQueueStore := store.NewScanQueueStore(database)

	jobHandler := handler.NewJobHandler(jobStore, ledger, discardStore, snapshotStore)
	appHandler := handler.NewApplicationHandler(appStore, jobStore)
	companyHandler := handler.NewCompanyHandler(companyStore, scanLogStore, scanQueueStore, cfg.ScanIntervalMinutes)
	dashboardHandler := handler.NewDashboardHandler(jobStore, scanLogStore, insightsStore)
	insightsHandler := handler.NewInsightsHandler(insightsStore)

	router := api.NewRouter(log, jobHandler, appHandler, companyHandler, dashboardHandler, insightsHandler)

	srv := &http.Server{
		Addr:         ":8080",
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Info("listening", logger.Str("addr", srv.Addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("server error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	log.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv.Shutdown(shutdownCtx) //nolint:errcheck
}
```

- [ ] **Step 3: Verify full build**

Run: `cd backend && go build ./...`  
Expected: exits 0, no errors across all packages.

---

### Task 10: Final Test Run

- [ ] **Step 1: Run all unit tests**

Run: `cd backend && go test -short ./...`  
Expected: all tests pass. Key packages to check:
- `./cmd/company_scan_scheduler/...` — 4 tests pass
- `./internal/api/handler/...` — all handler tests pass including new queue tests
- No compile errors in any package

- [ ] **Step 2: Remove TopicCompanyScanRequest from kafka/topics.go**

In `backend/internal/kafka/topics.go`, remove line 6 so the file reads:

```go
package kafka

const (
	TopicJobListingsRaw      = "job_listings.raw"
	TopicJobListingsFiltered = "job_listings.filtered"
)
```

- [ ] **Step 3: Verify no stray references to the removed topic**

Run: `cd backend && grep -r "TopicCompanyScanRequest" .`  
Expected: no output.

- [ ] **Step 4: Final build check**

Run: `cd backend && go build ./...`  
Expected: exits 0.
