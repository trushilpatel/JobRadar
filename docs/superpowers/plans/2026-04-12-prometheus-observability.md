# Prometheus Observability Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Prometheus metrics and `/health`+`/ready` endpoints to all 5 JobRadar services via a centralized `internal/metrics` package, with Prometheus and a second Grafana datasource wired into docker-compose.

**Architecture:** A new `internal/metrics` package holds all metric definitions and a `Serve(addr, readyFn)` helper. Workers call `go metrics.Serve(cfg.MetricsAddr, ...)` from `main.go`. The API adds `/metrics`, `/health`, `/ready` routes to its existing chi router. Prometheus scrapes all 5 services; Grafana auto-provisions Prometheus as a second datasource alongside Loki.

**Tech Stack:** `github.com/prometheus/client_golang`, chi v5, GORM, Docker Compose.

---

## File Map

| Action | Path | Responsibility |
|---|---|---|
| Modify | `backend/config/config.go` | Add `MetricsAddr string` field |
| Modify | `backend/.env` | Add `METRICS_ADDR` default |
| Modify | `backend/internal/db/db.go` | Add `Ping(*gorm.DB) error` helper |
| Create | `backend/internal/metrics/metrics.go` | All metric vars + `Serve()` |
| Create | `backend/internal/api/middleware/metrics.go` | HTTP metrics middleware |
| Modify | `backend/internal/api/router.go` | Add `/metrics`, `/health`, `/ready` routes + metrics middleware |
| Modify | `backend/internal/worker/job_listing_crawler/engine.go` | Instrument `scanCompany` |
| Modify | `backend/internal/worker/job_listing_relevance_filter/worker.go` | Instrument `flushBatch` |
| Modify | `backend/internal/worker/job_post_analyzer/worker.go` | Instrument `enrich` |
| Modify | `backend/cmd/company_scan_scheduler/main.go` | Add `metrics.Serve` + instrument `enqueuedue` |
| Modify | `backend/cmd/job_listing_crawler/main.go` | Add `metrics.Serve` |
| Modify | `backend/cmd/job_listing_relevance_filter/main.go` | Add `metrics.Serve` |
| Modify | `backend/cmd/job_post_analyzer/main.go` | Add `metrics.Serve` |
| Create | `backend/deploy/prometheus/prometheus.yml` | Prometheus scrape config |
| Create | `backend/deploy/prometheus/grafana-prometheus-datasource.yml` | Grafana provisioning |
| Modify | `docker-compose.yml` | Add prometheus service, METRICS_ADDR per worker, mount datasource |

---

## Task 1: Add prometheus dependency and config

**Files:**
- Modify: `backend/config/config.go`
- Modify: `backend/.env`

- [ ] **Step 1: Install the prometheus client library**

```bash
cd backend && go get github.com/prometheus/client_golang@latest
```

Expected: `go.mod` and `go.sum` updated with `github.com/prometheus/client_golang`.

- [ ] **Step 2: Add MetricsAddr to Config struct**

In `backend/config/config.go`, add `MetricsAddr` to the `Config` struct after the existing fields:

```go
type Config struct {
	DBHost        string
	DBPort        string
	DBUser        string
	DBPassword    string
	DBName        string
	DBSSLMode     string
	KafkaBrokers  string
	KafkaUser     string
	KafkaPassword string
	LLMProvider   string
	LLMAPIKey     string
	LLMModel      string
	LLMRateLimits LLMRateLimits

	ScanIntervalMinutes       int
	ScanCompanyDelaySeconds   int
	ScorerBatchSize           int
	ScorerFlushTimeoutSeconds int

	TargetRoles []string
	YearsExp    int
	MaxExpYears int

	MetricsAddr string // address for metrics/health HTTP server (workers); API uses :8080
}
```

- [ ] **Step 3: Add MetricsAddr to Load()**

In `backend/config/config.go`, add this line inside the `Load()` return block after `MaxExpYears`:

```go
MetricsAddr: getEnv("METRICS_ADDR", ":9090"),
```

- [ ] **Step 4: Add METRICS_ADDR to .env**

In `backend/.env`, add this section at the end:

```dotenv

# Observability
# Metrics/health server address for background workers (api uses :8080).
# Each worker service overrides this in docker-compose.
METRICS_ADDR=:9090
```

- [ ] **Step 5: Verify it compiles**

```bash
cd backend && go build ./...
```

Expected: no errors.

- [ ] **Step 6: Commit**

```bash
git add backend/go.mod backend/go.sum backend/config/config.go backend/.env
git commit -m "feat(observability): add prometheus dependency and MetricsAddr config"
```

---

## Task 2: Add db.Ping helper

**Why:** Each service's readiness check needs to ping the database. Rather than calling `database.DB()` + `.Ping()` in every `main.go`, we add a single `db.Ping()` helper that follows the existing `db` package pattern.

**Files:**
- Modify: `backend/internal/db/db.go`

- [ ] **Step 1: Add Ping function**

Replace the full contents of `backend/internal/db/db.go` with:

```go
package db

import (
	"github.com/trushilpatel/jobradar/config"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func Open(cfg *config.Config) (*gorm.DB, error) {
	database, err := gorm.Open(postgres.Open(cfg.DSN()), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		return nil, err
	}

	sqlDB, err := database.DB()
	if err != nil {
		return nil, err
	}

	sqlDB.SetMaxOpenConns(10)
	sqlDB.SetMaxIdleConns(5)

	return database, nil
}

// Ping checks that the database connection is alive.
// Used by readiness handlers.
func Ping(database *gorm.DB) error {
	sqlDB, err := database.DB()
	if err != nil {
		return err
	}
	return sqlDB.Ping()
}
```

- [ ] **Step 2: Verify it compiles**

```bash
cd backend && go build ./...
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add backend/internal/db/db.go
git commit -m "feat(observability): add db.Ping helper for readiness checks"
```

---

## Task 3: Create the centralized metrics package

**Files:**
- Create: `backend/internal/metrics/metrics.go`

- [ ] **Step 1: Create the metrics package**

Create `backend/internal/metrics/metrics.go`:

```go
package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// ── HTTP (recorded by api middleware) ─────────────────────────────────────────

// HTTPRequestsTotal counts requests by HTTP method, chi route pattern, and status code.
var HTTPRequestsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: "http_requests_total",
	Help: "Total HTTP requests by method, route, and status code.",
}, []string{"method", "route", "status"})

// HTTPRequestDuration records request latency by method and chi route pattern.
var HTTPRequestDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
	Name:    "http_request_duration_seconds",
	Help:    "HTTP request latency by method and route.",
	Buckets: prometheus.DefBuckets,
}, []string{"method", "route"})

// ── Crawler ───────────────────────────────────────────────────────────────────

// CrawlerScansTotal counts completed company scans. Label "status": "success" | "error".
var CrawlerScansTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: "crawler_scans_total",
	Help: "Total company scans completed.",
}, []string{"status"})

// CrawlerJobsFetchedTotal counts job listings fetched across all scans.
var CrawlerJobsFetchedTotal = prometheus.NewCounter(prometheus.CounterOpts{
	Name: "crawler_jobs_fetched_total",
	Help: "Total job listings fetched across all company scans.",
})

// CrawlerScanDuration records how long a single company scan takes.
var CrawlerScanDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
	Name:    "crawler_scan_duration_seconds",
	Help:    "Duration of a single company scan.",
	Buckets: []float64{0.5, 1, 2, 5, 10, 30, 60},
})

// ── Filter ────────────────────────────────────────────────────────────────────

// FilterJobsTotal counts jobs processed by the relevance filter.
// Label "outcome": "relevant" | "irrelevant" | "error".
var FilterJobsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: "filter_jobs_total",
	Help: "Total jobs processed by the relevance filter by outcome.",
}, []string{"outcome"})

// FilterLLMDuration records how long LLM ScoreBatch calls take.
var FilterLLMDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
	Name:    "filter_llm_duration_seconds",
	Help:    "Duration of LLM ScoreBatch calls.",
	Buckets: []float64{0.5, 1, 2, 5, 10, 30},
})

// ── Enricher ──────────────────────────────────────────────────────────────────

// EnricherJobsTotal counts jobs processed by the enricher.
// Label "outcome": "saved" | "dropped" | "error".
var EnricherJobsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: "enricher_jobs_total",
	Help: "Total jobs processed by the enricher by outcome.",
}, []string{"outcome"})

// EnricherLLMDuration records how long LLM Enrich calls take.
var EnricherLLMDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
	Name:    "enricher_llm_duration_seconds",
	Help:    "Duration of LLM Enrich calls.",
	Buckets: []float64{0.5, 1, 2, 5, 10, 30},
})

// ── Scheduler ─────────────────────────────────────────────────────────────────

// SchedulerCompaniesEnqueuedTotal counts companies enqueued for scanning.
var SchedulerCompaniesEnqueuedTotal = prometheus.NewCounter(prometheus.CounterOpts{
	Name: "scheduler_companies_enqueued_total",
	Help: "Total companies enqueued for scanning by the scheduler.",
})

// ── Registration ──────────────────────────────────────────────────────────────

func init() {
	prometheus.MustRegister(
		HTTPRequestsTotal,
		HTTPRequestDuration,
		CrawlerScansTotal,
		CrawlerJobsFetchedTotal,
		CrawlerScanDuration,
		FilterJobsTotal,
		FilterLLMDuration,
		EnricherJobsTotal,
		EnricherLLMDuration,
		SchedulerCompaniesEnqueuedTotal,
	)
}

// Serve starts a minimal HTTP server on addr exposing three endpoints:
//
//	GET /metrics — Prometheus scrape target
//	GET /health  — liveness probe, always 200
//	GET /ready   — readiness probe, calls readyFn; returns 503 on error
//
// Call from main() in a goroutine before starting the worker:
//
//	go metrics.Serve(cfg.MetricsAddr, func() error { return db.Ping(database) })
func Serve(addr string, readyFn func() error) {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/ready", func(w http.ResponseWriter, _ *http.Request) {
		if err := readyFn(); err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	http.ListenAndServe(addr, mux) //nolint:errcheck
}
```

- [ ] **Step 2: Verify it compiles**

```bash
cd backend && go build ./...
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add backend/internal/metrics/metrics.go
git commit -m "feat(observability): add centralized metrics package with Serve helper"
```

---

## Task 4: Instrument the API

**Files:**
- Create: `backend/internal/api/middleware/metrics.go`
- Modify: `backend/internal/api/router.go`

- [ ] **Step 1: Create the HTTP metrics middleware**

Create `backend/internal/api/middleware/metrics.go`:

```go
package middleware

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/trushilpatel/jobradar/internal/metrics"
)

// Metrics returns a middleware that records Prometheus HTTP metrics after each request.
// It uses chi's route pattern (e.g. "/jobs/{id}") as the label — not the raw path —
// to keep label cardinality bounded.
func Metrics() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := &responseWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(ww, r)

			route := chi.RouteContext(r.Context()).RoutePattern()
			if route == "" {
				route = r.URL.Path
			}
			elapsed := time.Since(start)
			metrics.HTTPRequestsTotal.WithLabelValues(r.Method, route, strconv.Itoa(ww.status)).Inc()
			metrics.HTTPRequestDuration.WithLabelValues(r.Method, route).Observe(elapsed.Seconds())
		})
	}
}
```

- [ ] **Step 2: Add routes and middleware to the router**

Replace the full contents of `backend/internal/api/router.go` with:

```go
package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/trushilpatel/jobradar/internal/api/handler"
	"github.com/trushilpatel/jobradar/internal/api/middleware"
	"github.com/trushilpatel/jobradar/internal/db"
	"github.com/trushilpatel/jobradar/internal/logger"
	"gorm.io/gorm"
)

func NewRouter(
	log logger.Logger,
	database *gorm.DB,
	jobs *handler.JobHandler,
	apps *handler.ApplicationHandler,
	companies *handler.CompanyHandler,
	dashboard *handler.DashboardHandler,
	insights *handler.InsightsHandler,
) *chi.Mux {
	r := chi.NewRouter()

	// Global middleware — runs on every request
	r.Use(middleware.Logger(log))
	r.Use(middleware.Metrics())
	r.Use(chimiddleware.Recoverer)

	// Operational endpoints
	r.Handle("/metrics", promhttp.Handler())
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	r.Get("/ready", func(w http.ResponseWriter, _ *http.Request) {
		if err := db.Ping(database); err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	r.Get("/jobs", jobs.List)
	r.Get("/jobs/{id}", jobs.Get)
	r.Delete("/jobs/{id}", jobs.Dismiss)
	r.Patch("/jobs/{id}/save", jobs.SetSaved)
	r.Patch("/jobs/{id}/status", jobs.SetStatus)
	r.Patch("/jobs/{id}/notes", jobs.SetNotes)
	r.Get("/jobs/{id}/applications", apps.List)
	r.Post("/jobs/{id}/applications", apps.Create)
	r.Patch("/jobs/{id}/applications/{app_id}/stage", apps.UpdateStage)
	r.Patch("/jobs/{id}/applications/{app_id}/note", apps.UpdateNote)

	r.Get("/applications", apps.ListAll)

	r.Get("/companies", companies.List)
	r.Get("/companies/queue", companies.ListQueue)
	r.Delete("/companies/queue/{entry_id}", companies.RemoveFromQueue)
	r.Post("/companies/queue/{entry_id}/move-up", companies.MoveQueueItemUp)
	r.Post("/companies/queue/{entry_id}/move-down", companies.MoveQueueItemDown)
	r.Post("/companies", companies.Create)
	r.Patch("/companies/{id}", companies.Update)
	r.Delete("/companies/{id}", companies.Delete)
	r.Get("/companies/{id}/scan-stats", companies.ScanStats)
	r.Post("/companies/{id}/scan", companies.TriggerScan)

	r.Get("/dashboard/stats", dashboard.Stats)
	r.Get("/dashboard/activity", dashboard.Activity)

	r.Get("/insights/tech-stack", insights.TechStack)
	r.Get("/insights/company-activity", insights.CompanyActivity)
	r.Get("/insights/funnel", insights.Funnel)
	r.Get("/insights/location", insights.Location)
	r.Get("/insights/sponsorship", insights.Sponsorship)
	r.Get("/insights/velocity", insights.Velocity)
	r.Get("/insights/role-categories", insights.RoleCategories)

	return r
}
```

- [ ] **Step 3: Update api/main.go to pass database to NewRouter**

Replace the full contents of `backend/cmd/api/main.go` with:

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

	router := api.NewRouter(log, database, jobHandler, appHandler, companyHandler, dashboardHandler, insightsHandler)

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

- [ ] **Step 4: Verify it compiles**

```bash
cd backend && go build ./...
```

Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/api/middleware/metrics.go backend/internal/api/router.go backend/cmd/api/main.go
git commit -m "feat(observability): add metrics, health, ready endpoints to API"
```

---

## Task 5: Instrument the crawler

**Files:**
- Modify: `backend/internal/worker/job_listing_crawler/engine.go`

- [ ] **Step 1: Add metrics calls to scanCompany**

In `backend/internal/worker/job_listing_crawler/engine.go`, replace the `scanCompany` method with:

```go
func (e *Engine) scanCompany(ctx context.Context, company model.Company) {
	start := time.Now()
	companyLog := e.log.With(
		logger.Str(logger.FieldCompany, company.ID),
		logger.Str("name", company.Name),
	)

	adp, err := ats_fetcher.Registry(company.AdapterType)
	if err != nil {
		errMsg := fmt.Sprintf("no adapter for type %q: %v", company.AdapterType, err)
		companyLog.Error("no adapter found", err)
		metrics.CrawlerScansTotal.WithLabelValues("error").Inc()
		e.recordResult(ctx, company.ID, 0, 0, time.Since(start), &errMsg)
		return
	}

	result, err := adp.FetchJobs(ctx, company.ID)
	if err != nil {
		errMsg := fmt.Sprintf("fetch failed: %v", err)
		companyLog.Error("fetch jobs failed", err)
		metrics.CrawlerScansTotal.WithLabelValues("error").Inc()
		e.recordResult(ctx, company.ID, 0, 0, time.Since(start), &errMsg)
		return
	}

	fetched := len(result.Jobs)
	newCount := 0

	for _, item := range result.Jobs {
		seen, err := e.ledger.HasSeen(item.ID)
		if err != nil {
			companyLog.Error("seen_ledger check failed", err, logger.Str(logger.FieldJobID, item.ID))
			continue
		}
		if seen {
			continue
		}

		if err := e.ledger.MarkSeen(item.ID, company.ID, model.SeenReasonFetch); err != nil {
			companyLog.Error("failed to mark seen", err, logger.Str(logger.FieldJobID, item.ID))
			continue
		}

		rawJob := toRawJob(item, company.ID, company.AdapterType)
		if err := e.publish(ctx, rawJob); err != nil {
			companyLog.Error("publish failed", err, logger.Str(logger.FieldJobID, item.ID))
			continue
		}

		newCount++
	}

	elapsed := time.Since(start)
	metrics.CrawlerScansTotal.WithLabelValues("success").Inc()
	metrics.CrawlerJobsFetchedTotal.Add(float64(fetched))
	metrics.CrawlerScanDuration.Observe(elapsed.Seconds())

	companyLog.Info("scan complete",
		logger.Int(logger.FieldFetched, fetched),
		logger.Int(logger.FieldNew, newCount),
		logger.Dur(logger.FieldDuration, elapsed.Round(time.Millisecond)),
	)
	e.recordResult(ctx, company.ID, fetched, newCount, elapsed, nil)

	if e.companyDelay > 0 {
		companyLog.Debug("sleeping before next company", logger.Dur("delay", e.companyDelay))
		select {
		case <-time.After(e.companyDelay):
		case <-ctx.Done():
		}
	}
}
```

Also add the metrics import to the import block in `engine.go`:

```go
import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/trushilpatel/jobradar/internal/ats_fetcher"
	"github.com/trushilpatel/jobradar/internal/kafka"
	"github.com/trushilpatel/jobradar/internal/logger"
	"github.com/trushilpatel/jobradar/internal/metrics"
	"github.com/trushilpatel/jobradar/internal/model"
	"github.com/trushilpatel/jobradar/internal/store"
)
```

- [ ] **Step 2: Verify it compiles**

```bash
cd backend && go build ./...
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add backend/internal/worker/job_listing_crawler/engine.go
git commit -m "feat(observability): instrument crawler with scan metrics"
```

---

## Task 6: Instrument the filter

**Files:**
- Modify: `backend/internal/worker/job_listing_relevance_filter/worker.go`

- [ ] **Step 1: Add metrics calls to flushBatch**

In `backend/internal/worker/job_listing_relevance_filter/worker.go`, replace the `flushBatch` method with:

```go
func (w *Worker) flushBatch(ctx context.Context, batch []ats_fetcher.RawJob) {
	inputs := make([]llm.ScorerInput, len(batch))
	for i, job := range batch {
		inputs[i] = llm.ScorerInput{ID: job.ID, Title: job.Title}
	}

	start := time.Now()
	relevantIDs, err := w.scorer.ScoreBatch(ctx, inputs)
	metrics.FilterLLMDuration.Observe(time.Since(start).Seconds())

	if err != nil {
		w.log.Error("ScoreBatch failed, marking batch as error", err, logger.Int(logger.FieldBatch, len(batch)))
		ids := make([]string, len(batch))
		for i, job := range batch {
			ids[i] = job.ID
		}
		if merr := w.ledger.UpdateSeenReasonBatch(ids, model.SeenReasonError); merr != nil {
			w.log.Error("mark error batch failed", merr)
		}
		metrics.FilterJobsTotal.WithLabelValues("error").Add(float64(len(batch)))
		return
	}

	relevant := make(map[string]bool, len(relevantIDs))
	for _, id := range relevantIDs {
		relevant[id] = true
	}

	var irrelevantIDs []string
	for _, job := range batch {
		if relevant[job.ID] {
			w.publishDetail(ctx, job)
		} else {
			irrelevantIDs = append(irrelevantIDs, job.ID)
		}
	}

	if len(irrelevantIDs) > 0 {
		if err := w.ledger.UpdateSeenReasonBatch(irrelevantIDs, model.SeenReasonLLMIrrelevant); err != nil {
			w.log.Error("mark irrelevant failed", err)
		}
	}

	metrics.FilterJobsTotal.WithLabelValues("relevant").Add(float64(len(relevantIDs)))
	metrics.FilterJobsTotal.WithLabelValues("irrelevant").Add(float64(len(irrelevantIDs)))

	w.log.Info("batch scored",
		logger.Int(logger.FieldBatch, len(batch)),
		logger.Int(logger.FieldRelevant, len(relevantIDs)),
		logger.Int("irrelevant", len(batch)-len(relevantIDs)),
	)
}
```

Also add the metrics import to the import block in `worker.go`:

```go
import (
	"context"
	"encoding/json"
	"time"

	"github.com/trushilpatel/jobradar/internal/ats_fetcher"
	"github.com/trushilpatel/jobradar/internal/kafka"
	"github.com/trushilpatel/jobradar/internal/llm"
	"github.com/trushilpatel/jobradar/internal/logger"
	"github.com/trushilpatel/jobradar/internal/metrics"
	"github.com/trushilpatel/jobradar/internal/model"
	"github.com/trushilpatel/jobradar/internal/store"
)
```

- [ ] **Step 2: Verify it compiles**

```bash
cd backend && go build ./...
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add backend/internal/worker/job_listing_relevance_filter/worker.go
git commit -m "feat(observability): instrument filter with LLM duration and job outcome metrics"
```

---

## Task 7: Instrument the enricher

**Files:**
- Modify: `backend/internal/worker/job_post_analyzer/worker.go`

- [ ] **Step 1: Add metrics calls to enrich**

In `backend/internal/worker/job_post_analyzer/worker.go`, replace the `enrich` method with:

```go
func (w *Worker) enrich(ctx context.Context, job ats_fetcher.RawJob) {
	jobLog := w.log.With(
		logger.Str(logger.FieldJobID, job.ID),
		logger.Str("title", job.Title),
	)

	var content ats_fetcher.FetchJobContentResponse
	if job.FullContent != nil {
		content = fullContentToResponse(job)
	} else {
		var err error
		content, err = w.fetchContent(ctx, job)
		if err != nil {
			jobLog.Error("fetch content failed", err)
			w.markError(job.ID)
			metrics.EnricherJobsTotal.WithLabelValues("error").Inc()
			return
		}
	}

	rawText, err := json.Marshal(content)
	if err != nil {
		jobLog.Error("marshal content failed", err)
		w.markError(job.ID)
		metrics.EnricherJobsTotal.WithLabelValues("error").Inc()
		return
	}

	start := time.Now()
	result, err := w.scorer.Enrich(ctx, llm.EnrichInput{
		ID:      job.ID,
		RawText: string(rawText),
	})
	metrics.EnricherLLMDuration.Observe(time.Since(start).Seconds())

	if err != nil {
		jobLog.Error("LLM enrich failed", err)
		w.markError(job.ID)
		metrics.EnricherJobsTotal.WithLabelValues("error").Inc()
		return
	}

	if reason := w.dropJob(result); reason != "" {
		jobLog.Info("job dropped", logger.Str("reason", reason))
		w.markError(job.ID)
		metrics.EnricherJobsTotal.WithLabelValues("dropped").Inc()
		return
	}

	row, err := buildJob(job, result)
	if err != nil {
		jobLog.Error("build job failed", err)
		w.markError(job.ID)
		metrics.EnricherJobsTotal.WithLabelValues("error").Inc()
		return
	}

	if err := w.jobs.InsertJob(row); err != nil {
		jobLog.Error("insert job failed", err)
		w.markError(job.ID)
		metrics.EnricherJobsTotal.WithLabelValues("error").Inc()
		return
	}

	techBytes, _ := json.Marshal(result.TechStack)
	if err := w.snapshot.InsertSnapshot(store.SnapshotInput{
		ID:                row.ID,
		CompanyID:         job.CompanyID,
		Country:           result.Country,
		State:             result.State,
		Remote:            result.Remote,
		Sponsorship:       result.Sponsorship,
		RequiresCitizen:   result.RequiresCitizen,
		SecurityClearance: result.SecurityClearance,
		EmployType:        job.EmployType,
		ExperienceYears:   result.ExperienceYears,
		TechStack:         techBytes,
		RoleCategory:      result.RoleCategory,
	}); err != nil {
		jobLog.Warn("write snapshot failed (non-fatal)", logger.Err(err))
	}

	metrics.EnricherJobsTotal.WithLabelValues("saved").Inc()
	jobLog.Info("enriched and saved")
}
```

Also add the metrics import to the import block in `worker.go`:

```go
import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/trushilpatel/jobradar/internal/ats_fetcher"
	"github.com/trushilpatel/jobradar/internal/kafka"
	"github.com/trushilpatel/jobradar/internal/llm"
	"github.com/trushilpatel/jobradar/internal/logger"
	"github.com/trushilpatel/jobradar/internal/metrics"
	"github.com/trushilpatel/jobradar/internal/model"
	"github.com/trushilpatel/jobradar/internal/store"
	"gorm.io/datatypes"
)
```

- [ ] **Step 2: Verify it compiles**

```bash
cd backend && go build ./...
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add backend/internal/worker/job_post_analyzer/worker.go
git commit -m "feat(observability): instrument enricher with LLM duration and job outcome metrics"
```

---

## Task 8: Wire metrics.Serve into all worker main.go files

**Files:**
- Modify: `backend/cmd/company_scan_scheduler/main.go`
- Modify: `backend/cmd/job_listing_crawler/main.go`
- Modify: `backend/cmd/job_listing_relevance_filter/main.go`
- Modify: `backend/cmd/job_post_analyzer/main.go`

- [ ] **Step 1: Update scheduler main.go**

Replace the full contents of `backend/cmd/company_scan_scheduler/main.go` with:

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
	"github.com/trushilpatel/jobradar/internal/metrics"
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

	go metrics.Serve(cfg.MetricsAddr, func() error { return db.Ping(database) })

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
		metrics.SchedulerCompaniesEnqueuedTotal.Inc()
		log.Debug("queued company", logger.Str(logger.FieldCompany, c.ID), logger.Str("name", c.Name))
	}
}
```

- [ ] **Step 2: Update crawler main.go**

Replace the full contents of `backend/cmd/job_listing_crawler/main.go` with:

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
	"github.com/trushilpatel/jobradar/internal/metrics"
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

	go metrics.Serve(cfg.MetricsAddr, func() error { return db.Ping(database) })

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

	if err := queue.ResetStuck(stuckTimeout); err != nil {
		log.Error("failed to reset stuck queue items", err)
	}

	log.Info("worker started", logger.Dur("poll_interval", pollInterval))

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

- [ ] **Step 3: Update filter main.go**

Replace the full contents of `backend/cmd/job_listing_relevance_filter/main.go` with:

```go
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/trushilpatel/jobradar/config"
	"github.com/trushilpatel/jobradar/internal/db"
	"github.com/trushilpatel/jobradar/internal/kafka"
	"github.com/trushilpatel/jobradar/internal/llm"
	"github.com/trushilpatel/jobradar/internal/logger"
	"github.com/trushilpatel/jobradar/internal/metrics"
	"github.com/trushilpatel/jobradar/internal/store"
	"github.com/trushilpatel/jobradar/internal/worker/job_listing_relevance_filter"
)

func main() {
	cfg := config.Load()
	log := logger.Init("filter")

	database, err := db.Open(cfg)
	if err != nil {
		log.Error("db open failed", err)
		os.Exit(1)
	}

	go metrics.Serve(cfg.MetricsAddr, func() error { return db.Ping(database) })

	if err := kafka.EnsureTopics(context.Background(), cfg, kafka.TopicJobListingsRaw, kafka.TopicJobListingsFiltered); err != nil {
		log.Error("kafka topics failed", err)
		os.Exit(1)
	}

	consumer, err := kafka.NewConsumer(cfg, log, "job-listing-relevance-filter", kafka.TopicJobListingsRaw)
	if err != nil {
		log.Error("kafka consumer failed", err)
		os.Exit(1)
	}
	defer consumer.Close()

	producer, err := kafka.NewProducer(cfg)
	if err != nil {
		log.Error("kafka producer failed", err)
		os.Exit(1)
	}
	defer producer.Close()

	scorer, err := llm.NewScorer(llm.ScorerOptions{
		Provider:    cfg.LLMProvider,
		APIKey:      cfg.LLMAPIKey,
		Model:       cfg.LLMModel,
		TargetRoles: cfg.TargetRoles,
		YearsExp:    cfg.YearsExp,
		Log:         log,
		Limits: llm.RateLimits{
			RPM:   cfg.LLMRateLimits.RPM,
			RPD:   cfg.LLMRateLimits.RPD,
			TPMIn: cfg.LLMRateLimits.TPMIn,
		},
	})
	if err != nil {
		log.Error("llm init failed", err)
		os.Exit(1)
	}

	ledger := store.NewSeenLedgerStore(database)
	worker := job_listing_relevance_filter.NewWorker(consumer, producer, scorer, ledger,
		cfg.ScorerBatchSize, cfg.ScorerFlushTimeoutSeconds, log)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := worker.Run(ctx); err != nil && ctx.Err() == nil {
		log.Error("worker exited with error", err)
		os.Exit(1)
	}
	log.Info("shutting down")
}
```

- [ ] **Step 4: Update enricher main.go**

Replace the full contents of `backend/cmd/job_post_analyzer/main.go` with:

```go
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/trushilpatel/jobradar/config"
	"github.com/trushilpatel/jobradar/internal/db"
	"github.com/trushilpatel/jobradar/internal/kafka"
	"github.com/trushilpatel/jobradar/internal/llm"
	"github.com/trushilpatel/jobradar/internal/logger"
	"github.com/trushilpatel/jobradar/internal/metrics"
	"github.com/trushilpatel/jobradar/internal/store"
	"github.com/trushilpatel/jobradar/internal/worker/job_post_analyzer"
)

func main() {
	cfg := config.Load()
	log := logger.Init("enricher")

	database, err := db.Open(cfg)
	if err != nil {
		log.Error("db open failed", err)
		os.Exit(1)
	}

	go metrics.Serve(cfg.MetricsAddr, func() error { return db.Ping(database) })

	consumer, err := kafka.NewConsumer(cfg, log, "job-post-analyzer", kafka.TopicJobListingsFiltered)
	if err != nil {
		log.Error("kafka consumer failed", err)
		os.Exit(1)
	}
	defer consumer.Close()

	scorer, err := llm.NewScorer(llm.ScorerOptions{
		Provider:    cfg.LLMProvider,
		APIKey:      cfg.LLMAPIKey,
		Model:       cfg.LLMModel,
		TargetRoles: cfg.TargetRoles,
		YearsExp:    cfg.YearsExp,
		Log:         log,
		Limits: llm.RateLimits{
			RPM:   cfg.LLMRateLimits.RPM,
			RPD:   cfg.LLMRateLimits.RPD,
			TPMIn: cfg.LLMRateLimits.TPMIn,
		},
	})
	if err != nil {
		log.Error("llm init failed", err)
		os.Exit(1)
	}

	jobs := store.NewJobStore(database)
	ledger := store.NewSeenLedgerStore(database)
	snapshot := store.NewSnapshotStore(database)
	worker := job_post_analyzer.NewWorker(consumer, scorer, jobs, ledger, snapshot, log, cfg.MaxExpYears)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := worker.Run(ctx); err != nil && ctx.Err() == nil {
		log.Error("worker exited with error", err)
		os.Exit(1)
	}
	log.Info("shutting down")
}
```

- [ ] **Step 5: Verify all binaries compile**

```bash
cd backend && go build ./...
```

Expected: no errors.

- [ ] **Step 6: Run tests**

```bash
cd backend && go test ./...
```

Expected: all tests pass.

- [ ] **Step 7: Commit**

```bash
git add backend/cmd/company_scan_scheduler/main.go \
        backend/cmd/job_listing_crawler/main.go \
        backend/cmd/job_listing_relevance_filter/main.go \
        backend/cmd/job_post_analyzer/main.go
git commit -m "feat(observability): wire metrics.Serve into all worker binaries"
```

---

## Task 9: Add Prometheus to docker-compose

**Files:**
- Create: `backend/deploy/prometheus/prometheus.yml`
- Create: `backend/deploy/prometheus/grafana-prometheus-datasource.yml`
- Modify: `docker-compose.yml`

- [ ] **Step 1: Create Prometheus scrape config**

Create `backend/deploy/prometheus/prometheus.yml`:

```yaml
global:
  scrape_interval: 15s
  evaluation_interval: 15s

scrape_configs:
  - job_name: jobradar_api
    static_configs:
      - targets: ['api:8080']

  - job_name: jobradar_scheduler
    static_configs:
      - targets: ['scheduler:9101']

  - job_name: jobradar_crawler
    static_configs:
      - targets: ['crawler:9102']

  - job_name: jobradar_filter
    static_configs:
      - targets: ['filter:9103']

  - job_name: jobradar_enricher
    static_configs:
      - targets: ['enricher:9104']
```

- [ ] **Step 2: Create Grafana Prometheus datasource provisioning**

Create `backend/deploy/prometheus/grafana-prometheus-datasource.yml`:

```yaml
apiVersion: 1

datasources:
  - name: Prometheus
    type: prometheus
    access: proxy
    url: http://prometheus:9090
    isDefault: false
    editable: true
```

- [ ] **Step 3: Add Prometheus service to docker-compose.yml**

In `docker-compose.yml`, add the following inside the `# ── Observability` section, after the `grafana` service block:

```yaml
  prometheus:
    image: prom/prometheus:v2.51.0
    container_name: jobradar_prometheus
    ports:
      - "9090:9090"
    volumes:
      - ./backend/deploy/prometheus/prometheus.yml:/etc/prometheus/prometheus.yml
      - prometheusdata:/prometheus
    command:
      - '--config.file=/etc/prometheus/prometheus.yml'
      - '--storage.tsdb.retention.time=15d'
    restart: unless-stopped

```

- [ ] **Step 4: Add METRICS_ADDR overrides to each worker service**

In `docker-compose.yml`, add `METRICS_ADDR` to the `environment` block of each worker service:

In the `scheduler` service environment block, add:
```yaml
      METRICS_ADDR: ":9101"
```

In the `crawler` service environment block, add:
```yaml
      METRICS_ADDR: ":9102"
```

In the `filter` service environment block, add:
```yaml
      METRICS_ADDR: ":9103"
```

In the `enricher` service environment block, add:
```yaml
      METRICS_ADDR: ":9104"
```

- [ ] **Step 5: Expose metrics ports on each worker service**

In `docker-compose.yml`, add `ports` to each worker service so Prometheus (inside Docker) can reach them:

In the `scheduler` service, add:
```yaml
    ports:
      - "9101:9101"
```

In the `crawler` service, add:
```yaml
    ports:
      - "9102:9102"
```

In the `filter` service, add:
```yaml
    ports:
      - "9103:9103"
```

In the `enricher` service, add:
```yaml
    ports:
      - "9104:9104"
```

- [ ] **Step 6: Mount the Prometheus datasource into the Grafana service**

In `docker-compose.yml`, find the `grafana` service `volumes` block and add the Prometheus datasource file:

```yaml
    volumes:
      - ./backend/deploy/loki/grafana-datasources.yml:/etc/grafana/provisioning/datasources/loki.yml
      - ./backend/deploy/prometheus/grafana-prometheus-datasource.yml:/etc/grafana/provisioning/datasources/prometheus.yml
      - grafanadata:/var/lib/grafana
```

- [ ] **Step 7: Add prometheusdata volume**

In `docker-compose.yml`, add to the `volumes` block at the bottom:

```yaml
volumes:
  pgdata:
  kafkadata:
  lokidata:
  grafanadata:
  prometheusdata:
```

- [ ] **Step 8: Validate docker-compose syntax**

```bash
docker compose config --quiet
```

Expected: no output (silent = valid).

- [ ] **Step 9: Commit**

```bash
git add backend/deploy/prometheus/prometheus.yml \
        backend/deploy/prometheus/grafana-prometheus-datasource.yml \
        docker-compose.yml
git commit -m "feat(observability): add Prometheus to docker-compose with per-worker metrics ports"
```

---

## Task 10: Build, deploy, and verify

- [ ] **Step 1: Rebuild the backend image**

```bash
docker compose build api
```

Expected: image `jobradar_backend` rebuilt with the new metrics code.

- [ ] **Step 2: Pull the Prometheus image**

```bash
docker compose pull prometheus
```

- [ ] **Step 3: Restart all services to pick up changes**

```bash
docker compose up -d
```

Expected: all containers start, including `jobradar_prometheus`.

- [ ] **Step 4: Verify Prometheus is up**

```bash
curl -s localhost:9090/-/healthy
```

Expected: `Prometheus Server is Healthy.`

- [ ] **Step 5: Verify the API metrics endpoint**

```bash
curl -s localhost:8080/metrics | grep http_requests_total
```

Expected: lines like:
```
# HELP http_requests_total Total HTTP requests by method, route, and status code.
# TYPE http_requests_total counter
```

- [ ] **Step 6: Verify the API health and ready endpoints**

```bash
curl -s -o /dev/null -w "%{http_code}" localhost:8080/health
curl -s -o /dev/null -w "%{http_code}" localhost:8080/ready
```

Expected: `200` for both.

- [ ] **Step 7: Generate some API traffic to create metric data**

```bash
curl -s localhost:8080/jobs > /dev/null
curl -s localhost:8080/dashboard/stats > /dev/null
curl -s localhost:8080/companies > /dev/null
```

- [ ] **Step 8: Check Prometheus scraped the API**

Open `http://localhost:9090/targets` in your browser.

Expected: `jobradar_api` shows state `UP`. Other workers may show `DOWN` if not running — that's fine, Prometheus will scrape them when they start.

- [ ] **Step 9: Query a metric in Prometheus**

Open `http://localhost:9090` → click **Graph** tab → enter:

```promql
http_requests_total
```

Click **Execute**. Expected: a table of label combinations with request counts.

- [ ] **Step 10: Verify Prometheus appears in Grafana**

Open `http://localhost:3001` → Connections → Data sources.

Expected: both **Loki** and **Prometheus** listed.

- [ ] **Step 11: Query in Grafana Explore**

In Grafana → Explore → switch data source to **Prometheus** → enter:

```promql
rate(http_requests_total[5m])
```

Expected: a time-series graph showing request rate per route.

- [ ] **Step 12: Run final tests**

```bash
cd backend && go test ./...
```

Expected: all tests pass.

- [ ] **Step 13: Commit**

```bash
git add .
git commit -m "feat(observability): Prometheus metrics and health endpoints complete"
```
