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

var CrawlerJobsNonUSADropped = prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: "crawler_jobs_non_usa_dropped_total",
	Help: "Total non-USA jobs dropped at the crawler (LocationKnown=true gate) by adapter.",
}, []string{"adapter"})

// CrawlerPublishFailures counts per-job NATS publish failures in the crawler.
// The ledger row stays at reason='fetch' (recovered by the next scan); this
// counter is the alert surface for a job that was marked seen but never
// published.
var CrawlerPublishFailures = prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: "crawler_publish_failures_total",
	Help: "Per-job NATS publish failures in the crawler by adapter (ledger row stays at 'fetch').",
}, []string{"adapter"})

// CrawlerScanAbortedReassigned counts scans the crawler abandons because its
// queue row was reassigned by the scheduler's ResetStuck (UpdateHeartbeat
// reported the row missing). Aborting prevents double-publishing.
var CrawlerScanAbortedReassigned = prometheus.NewCounter(prometheus.CounterOpts{
	Name: "crawler_scan_aborted_reassigned_total",
	Help: "Scans aborted because the queue row was reassigned (heartbeat row missing).",
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

var FilterChunkDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
	Name:    "filter_chunk_duration_seconds",
	Help:    "Per-chunk score+route+publish duration by outcome.",
	Buckets: []float64{0.5, 1, 2, 5, 10, 30},
}, []string{"outcome"})

var FilterRetryPublishesTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: "filter_retry_publishes_total",
	Help: "Successful retry-tier publishes by tier.",
}, []string{"tier"})

var FilterRetryPublishFailures = prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: "filter_retry_publish_failures_total",
	Help: "Failed retry-tier publishes by tier (silent-drop alert surface).",
}, []string{"tier"})

var FilterDLQPublishesTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: "filter_dlq_publishes_total",
	Help: "Successful DLQ publishes by cause.",
}, []string{"cause"})

var FilterCommitFailuresTotal = prometheus.NewCounter(prometheus.CounterOpts{
	Name: "filter_commit_failures_total",
	Help: "CommitRecords errors (replay on next poll).",
})

var FilterChunksPerFetch = prometheus.NewHistogram(prometheus.HistogramOpts{
	Name:    "filter_chunks_per_fetch",
	Help:    "Fan-out size (chunks) per Kafka fetch.",
	Buckets: []float64{1, 2, 4, 8, 16, 32, 64},
})

// FilterPublishFailures counts detail-subject publish failures in the filter.
// On failure the job is Nak'd (NATS redelivers) and no ledger write occurs.
var FilterPublishFailures = prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: "filter_publish_failures_total",
	Help: "Filter detail-publish failures by subject (job Nak'd, no ledger write).",
}, []string{"subject"})

// FilterLedgerMissing counts ledger UPDATE operations in the filter that
// reported ErrLedgerRowMissing (the crawler→filter race). The chunk is Nak'd so
// NATS redelivers once the crawler INSERT has committed. Labelled by the reason
// the filter tried to set the row to.
var FilterLedgerMissing = prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: "filter_ledger_missing_total",
	Help: "Filter ledger UPDATEs that hit a missing row by target reason (chunk Nak'd).",
}, []string{"reason"})

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

var EnricherGroupDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
	Name:    "enricher_group_duration_seconds",
	Help:    "Per-group fetch+enrich+publish duration by outcome.",
	Buckets: prometheus.DefBuckets,
}, []string{"outcome"})

var EnricherRetryPublishesTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: "enricher_retry_publishes_total",
	Help: "Successful retry-tier publishes by tier and stage.",
}, []string{"tier", "stage"})

var EnricherRetryPublishFailures = prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: "enricher_retry_publish_failures_total",
	Help: "Failed retry-tier publishes by tier and stage (silent-drop alert surface).",
}, []string{"tier", "stage"})

var EnricherRetryLedgerFailuresTotal = prometheus.NewCounter(prometheus.CounterOpts{
	Name: "enricher_retry_ledger_failures_total",
	Help: "UpdateRetryBatch errors encountered by the enricher.",
})

var EnricherDLQPublishesTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: "enricher_dlq_publishes_total",
	Help: "Successful DLQ publishes by cause.",
}, []string{"cause"})

var EnricherCommitFailuresTotal = prometheus.NewCounter(prometheus.CounterOpts{
	Name: "enricher_commit_failures_total",
	Help: "CommitRecords errors (replay on next poll).",
})

var EnricherGroupsPerFetch = prometheus.NewHistogram(prometheus.HistogramOpts{
	Name:    "enricher_groups_per_fetch",
	Help:    "Fan-out size (groups) per Kafka fetch.",
	Buckets: []float64{1, 2, 4, 8, 16, 32, 64},
})

// EnricherLedgerMissing counts ledger UPDATE operations in the enricher
// (analyzer) that hit a missing row. Non-fatal at this stage — the row may have
// been cleaned out by a maintenance migration — so the worker logs WARN and
// proceeds. Labelled by the reason the enricher tried to set the row to.
var EnricherLedgerMissing = prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: "enricher_ledger_missing_total",
	Help: "Enricher ledger UPDATEs that hit a missing row by target reason (non-fatal).",
}, []string{"reason"})

// EnricherJobsDuplicate counts enriched jobs whose InsertJob hit ON CONFLICT
// DO NOTHING (the job_post_id row already existed). The enricher still advances
// the ledger to 'saved' and skips snapshot + tech-tag writes.
var EnricherJobsDuplicate = prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: "enricher_jobs_duplicate_total",
	Help: "Enriched jobs skipped as duplicate (job_post_id already present) by adapter.",
}, []string{"adapter"})

// ── Scheduler ─────────────────────────────────────────────────────────────────

var SchedulerCompaniesEnqueuedTotal = prometheus.NewCounter(prometheus.CounterOpts{
	Name: "scheduler_companies_enqueued_total",
	Help: "Total companies enqueued for scanning by the scheduler.",
})

// SchedulerRunDuration tracks how long each full enqueue pass takes.
var SchedulerRunDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
	Name:    "scheduler_run_duration_seconds",
	Help:    "Duration of a full scheduler enqueue pass (time to exhaust all due companies).",
	Buckets: []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2, 5},
})

// SchedulerErrorsTotal counts errors encountered during scheduler runs, by reason.
// Reason values: "enqueue_due", "reset_stuck".
var SchedulerErrorsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: "scheduler_errors_total",
	Help: "Total errors encountered by the scheduler, by reason.",
}, []string{"reason"})

// SchedulerStuckChecksTotal counts how many stuck-detection passes completed
// without error. Each increment represents one successful ResetStuck call,
// not one row — use this as a liveness signal for the stuck-detection path.
var SchedulerStuckChecksTotal = prometheus.NewCounter(prometheus.CounterOpts{
	Name: "scheduler_stuck_checks_total",
	Help: "Total stuck-detection passes completed by the scheduler without error.",
})

// ── Registration ──────────────────────────────────────────────────────────────

func init() {
	prometheus.MustRegister(
		HTTPRequestsTotal,
		HTTPRequestDuration,
		CrawlerScansTotal,
		CrawlerJobsFetchedTotal,
		CrawlerScanDuration,
		CrawlerJobsNonUSADropped,
		CrawlerPublishFailures,
		CrawlerScanAbortedReassigned,
		FilterJobsTotal,
		FilterLLMDuration,
		FilterChunkDuration,
		FilterRetryPublishesTotal,
		FilterRetryPublishFailures,
		FilterDLQPublishesTotal,
		FilterCommitFailuresTotal,
		FilterChunksPerFetch,
		FilterPublishFailures,
		FilterLedgerMissing,
		EnricherJobsTotal,
		EnricherLLMDuration,
		EnricherGroupDuration,
		EnricherRetryPublishesTotal,
		EnricherRetryPublishFailures,
		EnricherRetryLedgerFailuresTotal,
		EnricherDLQPublishesTotal,
		EnricherCommitFailuresTotal,
		EnricherGroupsPerFetch,
		EnricherLedgerMissing,
		EnricherJobsDuplicate,
		SchedulerCompaniesEnqueuedTotal,
		SchedulerRunDuration,
		SchedulerErrorsTotal,
		SchedulerStuckChecksTotal,
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
