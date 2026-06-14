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
}

// RetryPublisher is the narrow NATS publishing contract the sweeper needs.
// Declared here (app-level interfaces.go) so the scheduler binary never
// imports the jetstream package directly.
type RetryPublisher interface {
	Publish(ctx context.Context, subject string, payload []byte) error
}
