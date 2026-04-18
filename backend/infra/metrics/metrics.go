package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// ── HTTP (recorded by api middleware) ─────────────────────────────────────────

var HTTPRequestsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: "http_requests_total",
	Help: "Total HTTP requests by method, route, and status code.",
}, []string{"method", "route", "status"})

var HTTPRequestDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
	Name:    "http_request_duration_seconds",
	Help:    "HTTP request latency by method and route.",
	Buckets: prometheus.DefBuckets,
}, []string{"method", "route"})

// ── Crawler ───────────────────────────────────────────────────────────────────

var CrawlerScansTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: "crawler_scans_total",
	Help: "Total company scans completed.",
}, []string{"status"})

var CrawlerJobsFetchedTotal = prometheus.NewCounter(prometheus.CounterOpts{
	Name: "crawler_jobs_fetched_total",
	Help: "Total job listings fetched across all company scans.",
})

var CrawlerScanDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
	Name:    "crawler_scan_duration_seconds",
	Help:    "Duration of a single company scan.",
	Buckets: []float64{0.5, 1, 2, 5, 10, 30, 60},
})

// ── Filter ────────────────────────────────────────────────────────────────────

var FilterJobsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: "filter_jobs_total",
	Help: "Total jobs processed by the relevance filter by outcome.",
}, []string{"outcome"})

var FilterLLMDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
	Name:    "filter_llm_duration_seconds",
	Help:    "Duration of LLM ScoreBatch calls.",
	Buckets: []float64{0.5, 1, 2, 5, 10, 30},
})

// ── Enricher ──────────────────────────────────────────────────────────────────

var EnricherJobsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: "enricher_jobs_total",
	Help: "Total jobs processed by the enricher by outcome.",
}, []string{"outcome"})

var EnricherLLMDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
	Name:    "enricher_llm_duration_seconds",
	Help:    "Duration of LLM Enrich calls.",
	Buckets: []float64{0.5, 1, 2, 5, 10, 30},
})

// ── Scheduler ─────────────────────────────────────────────────────────────────

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
//	go metrics.Serve(cfg.Observability.MetricsAddr, func() error { return db.Ping(database) })
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
