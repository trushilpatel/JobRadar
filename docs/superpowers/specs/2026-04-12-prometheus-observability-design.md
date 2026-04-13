# Prometheus Observability Design

**Date:** 2026-04-12
**Scope:** Add Prometheus metrics and health/readiness endpoints to all 5 JobRadar services via a centralized `internal/metrics` package. Grafana (already running) gains Prometheus as a second data source.

---

## Goal

Every JobRadar service exposes `/metrics`, `/health`, and `/ready` on a configurable address. Prometheus scrapes all 5. Grafana visualises pipeline throughput, LLM latency, and HTTP performance in one place.

---

## Architecture

```
┌──────────────┐        ┌─────────────────────────────────────┐
│  Prometheus  │─scrape─▶  api        :8080/metrics           │
│   :9090      │        │  scheduler  METRICS_ADDR/metrics    │
└──────┬───────┘        │  crawler    METRICS_ADDR/metrics    │
       │                │  filter     METRICS_ADDR/metrics    │
       ▼                │  enricher   METRICS_ADDR/metrics    │
   Grafana              └─────────────────────────────────────┘
   :3001
(add Prometheus
 as data source)
```

The API serves `/metrics`, `/health`, `/ready` on its existing port 8080 via the chi router. The four workers start a dedicated lightweight HTTP server (3 routes only) on their own `METRICS_ADDR` port.

---

## Config Changes

### `config/config.go` — new field

```go
MetricsAddr string // e.g. ":9101" — address for metrics/health HTTP server (workers only)
```

Loaded via:
```go
MetricsAddr: getEnv("METRICS_ADDR", ":9090"),
```

### `backend/.env` — new section

```dotenv
# Observability — metrics/health server address per worker
# Override per service in docker-compose environment block
# API uses its main :8080 server; these apply to background workers only
METRICS_ADDR=:9090
```

### `docker-compose.yml` — per-service overrides

Each worker service gets:
```yaml
environment:
  METRICS_ADDR: ":9101"   # scheduler
  METRICS_ADDR: ":9102"   # crawler
  METRICS_ADDR: ":9103"   # filter
  METRICS_ADDR: ":9104"   # enricher
```

---

## New Package: `internal/metrics/`

### `internal/metrics/metrics.go`

Single file. Three responsibilities:
1. Register all metrics with Prometheus default registry
2. Export metric variables for workers/handlers to call
3. Export `Serve(addr string, readyFn func() error)` — starts HTTP server

```go
package metrics

import (
    "net/http"
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promhttp"
)

// ── HTTP (api middleware) ──────────────────────────────────────────────────────

var HTTPRequestsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
    Name: "http_requests_total",
    Help: "Total HTTP requests by method, route, and status code",
}, []string{"method", "route", "status"})

var HTTPRequestDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
    Name:    "http_request_duration_seconds",
    Help:    "HTTP request latency by method and route",
    Buckets: prometheus.DefBuckets,
}, []string{"method", "route"})

// ── Crawler ───────────────────────────────────────────────────────────────────

var CrawlerScansTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
    Name: "crawler_scans_total",
    Help: "Total company scans completed",
}, []string{"status"}) // status: success | error

var CrawlerJobsFetchedTotal = prometheus.NewCounter(prometheus.CounterOpts{
    Name: "crawler_jobs_fetched_total",
    Help: "Total job listings fetched across all scans",
})

var CrawlerScanDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
    Name:    "crawler_scan_duration_seconds",
    Help:    "Duration of a single company scan",
    Buckets: []float64{0.5, 1, 2, 5, 10, 30, 60},
})

// ── Filter ────────────────────────────────────────────────────────────────────

var FilterJobsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
    Name: "filter_jobs_total",
    Help: "Total jobs processed by the relevance filter",
}, []string{"outcome"}) // outcome: relevant | irrelevant | error

var FilterLLMDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
    Name:    "filter_llm_duration_seconds",
    Help:    "Duration of LLM ScoreBatch calls",
    Buckets: []float64{0.5, 1, 2, 5, 10, 30},
})

// ── Enricher ──────────────────────────────────────────────────────────────────

var EnricherJobsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
    Name: "enricher_jobs_total",
    Help: "Total jobs processed by the enricher",
}, []string{"outcome"}) // outcome: saved | dropped | error

var EnricherLLMDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
    Name:    "enricher_llm_duration_seconds",
    Help:    "Duration of LLM Enrich calls",
    Buckets: []float64{0.5, 1, 2, 5, 10, 30},
})

// ── Scheduler ─────────────────────────────────────────────────────────────────

var SchedulerCompaniesEnqueuedTotal = prometheus.NewCounter(prometheus.CounterOpts{
    Name: "scheduler_companies_enqueued_total",
    Help: "Total companies enqueued for scanning",
})

// ── Init + Serve ──────────────────────────────────────────────────────────────

func init() {
    prometheus.MustRegister(
        HTTPRequestsTotal, HTTPRequestDuration,
        CrawlerScansTotal, CrawlerJobsFetchedTotal, CrawlerScanDuration,
        FilterJobsTotal, FilterLLMDuration,
        EnricherJobsTotal, EnricherLLMDuration,
        SchedulerCompaniesEnqueuedTotal,
    )
}

// Serve starts a minimal HTTP server on addr with three routes:
//   GET /metrics — Prometheus scrape endpoint
//   GET /health  — liveness: always 200
//   GET /ready   — readiness: calls readyFn, returns 200 or 503
//
// Call this in a goroutine from main():
//
//	go metrics.Serve(cfg.MetricsAddr, func() error { return db.Ping() })
func Serve(addr string, readyFn func() error) {
    mux := http.NewServeMux()
    mux.Handle("/metrics", promhttp.Handler())
    mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
    })
    mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
        if err := readyFn(); err != nil {
            http.Error(w, err.Error(), http.StatusServiceUnavailable)
            return
        }
        w.WriteHeader(http.StatusOK)
    })
    http.ListenAndServe(addr, mux) //nolint:errcheck
}
```

---

## Changes to Existing Code

### `internal/api/router.go`

Add three routes to the chi router:

```go
r.Handle("/metrics", promhttp.Handler())
r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
    w.WriteHeader(http.StatusOK)
})
r.Get("/ready", readinessHandler) // checks DB ping
```

### `internal/api/middleware/logger.go`

After writing the response, record metrics:

```go
metrics.HTTPRequestsTotal.WithLabelValues(r.Method, r.URL.Path, strconv.Itoa(ww.Status())).Inc()
metrics.HTTPRequestDuration.WithLabelValues(r.Method, r.URL.Path).Observe(elapsed.Seconds())
```

### `cmd/scheduler/main.go`

```go
go metrics.Serve(cfg.MetricsAddr, func() error { return db.DB().Ping() })
```

### `cmd/crawler/main.go`

```go
go metrics.Serve(cfg.MetricsAddr, func() error {
    if err := db.DB().Ping(); err != nil { return err }
    return kafkaAdmin.Ping(ctx)
})
```

### `cmd/filter/main.go`

```go
go metrics.Serve(cfg.MetricsAddr, func() error { return kafkaAdmin.Ping(ctx) })
```

### `cmd/enricher/main.go`

```go
go metrics.Serve(cfg.MetricsAddr, func() error {
    if err := db.DB().Ping(); err != nil { return err }
    return kafkaAdmin.Ping(ctx)
})
```

### Worker instrumentation call sites

| File | Where | Call |
|---|---|---|
| `worker/job_listing_crawler/engine.go` | after scan completes | `metrics.CrawlerScansTotal.WithLabelValues("success").Inc()` |
| `worker/job_listing_crawler/engine.go` | on scan error | `metrics.CrawlerScansTotal.WithLabelValues("error").Inc()` |
| `worker/job_listing_crawler/engine.go` | after fetching jobs | `metrics.CrawlerJobsFetchedTotal.Add(float64(fetched))` |
| `worker/job_listing_crawler/engine.go` | around scan | `metrics.CrawlerScanDuration.Observe(elapsed.Seconds())` |
| `worker/job_listing_relevance_filter/worker.go` | in `flushBatch` | `metrics.FilterJobsTotal.WithLabelValues("relevant").Add(...)` etc. |
| `worker/job_listing_relevance_filter/worker.go` | around `ScoreBatch` | `metrics.FilterLLMDuration.Observe(elapsed.Seconds())` |
| `worker/job_post_analyzer/worker.go` | after job saved | `metrics.EnricherJobsTotal.WithLabelValues("saved").Inc()` |
| `worker/job_post_analyzer/worker.go` | after job dropped | `metrics.EnricherJobsTotal.WithLabelValues("dropped").Inc()` |
| `worker/job_post_analyzer/worker.go` | around `Enrich` | `metrics.EnricherLLMDuration.Observe(elapsed.Seconds())` |
| `cmd/company_scan_scheduler/main.go` | after enqueue | `metrics.SchedulerCompaniesEnqueuedTotal.Add(float64(n))` |

---

## New Infra Files

### `deploy/prometheus/prometheus.yml`

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

### `docker-compose.yml` additions

New service:
```yaml
  prometheus:
    image: prometheus/prometheus:v2.51.0
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

New volume: `prometheusdata`

Per-worker `METRICS_ADDR` overrides in existing service blocks.

### `deploy/prometheus/grafana-prometheus-datasource.yml`

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

Mount this into the Grafana container alongside the existing Loki datasource file.

---

## New Go Dependency

```
github.com/prometheus/client_golang
```

Add via: `go get github.com/prometheus/client_golang@latest`

---

## What Is NOT in Scope

- Grafana dashboards (can be built after metrics are flowing)
- Alerting rules
- `/version` endpoint or `build_info` metric
- Tracing (OpenTelemetry)
