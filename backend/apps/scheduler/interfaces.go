package main

import (
	"context"
	"time"

	scansvc "github.com/trushilpatel/jobradar/service/scan/services"
)

// SchedulerService is the narrow interface the scheduler worker depends on.
// It mirrors scansvc.SchedulerService so the app never couples directly to
// the service package's exported type.
type SchedulerService interface {
	EnqueueDue(ctx context.Context, intervalMinutes int) (scansvc.SchedulerCycleResult, error)
	// ResetStuck resets queue rows stuck in 'running' state whose heartbeat has
	// expired. Returns the count of rows recovered — zero is not an error.
	ResetStuck(ctx context.Context, staleDur time.Duration) (int, error)
	PurgeInactive(ctx context.Context) (int, error)
	// RetryDueJobs fetches ledger rows where reason='retry' and
	// next_attempt_at <= now() with a non-null payload, re-publishes them to
	// jobs.raw.{adapter} (leaving the row at reason='retry' while in-flight),
	// or escalates to reason='error' when retry_count >= max.
	RetryDueJobs(ctx context.Context) error
	// EscalateStuckRetryJobs promotes reason='retry' rows that have not been
	// re-published within 4 hours of their next_attempt_at to reason='error'.
	// Returns the number of rows escalated — zero is not an error.
	EscalateStuckRetryJobs(ctx context.Context) (int64, error)
	// ReclaimStuckFetchRows DELETEs reason='fetch' ledger rows older than a
	// safety threshold, clearing the dedup block left when a job entered the
	// filter but never reached a terminal verdict (e.g. NATS restart mid-flight).
	// The next crawl re-discovers and re-drives the job. Returns the number of
	// rows reclaimed — zero is not an error.
	ReclaimStuckFetchRows(ctx context.Context) (int64, error)
	// ReclaimStuckFilteredRows DELETEs reason='filtered' ledger rows older than
	// a safety threshold, clearing the dedup block left when a job was published
	// to jobs.filtered.* but the analyzer never reached a terminal verdict.
	// The next crawl re-discovers and re-drives the job. Returns the number of
	// rows reclaimed — zero is not an error.
	ReclaimStuckFilteredRows(ctx context.Context) (int64, error)
	// AutoPauseChronicFailures sets company_scan_settings.auto_paused=true for
	// every company whose consecutive scan errors crossed the threshold and is
	// not already paused, writing one admin_audit_log row per newly paused
	// company. Idempotent. Returns the number of companies newly paused — zero
	// is not an error.
	AutoPauseChronicFailures(ctx context.Context) (int, error)
	// RefreshGaugeMetrics recomputes the current-state gauge metrics for the
	// Overview dashboard stat strip. Always returns a non-nil error only when
	// at least one of the 5 underlying reads failed — partial failure never
	// blocks the others.
	RefreshGaugeMetrics(ctx context.Context) error
}

// RetryPublisher is the narrow NATS publishing contract the sweeper needs.
// Declared here (app-level interfaces.go) so the scheduler binary never
// imports the jetstream package directly.
type RetryPublisher interface {
	Publish(ctx context.Context, subject string, payload []byte) error
}
