package main

import (
	"context"
	"time"

	js "github.com/trushilpatel/jobradar/common/jetstream"
	jobsvc "github.com/trushilpatel/jobradar/service/job/services"
)

// EnricherLedger is the narrow ledger contract the worker depends on.
// Keeps the worker off the broad PipelineService for per-job ledger writes,
// matching the pattern established by apps/filter/interfaces.go (FilterLedger).
type EnricherLedger interface {
	UpdateRetryBatch(ctx context.Context, entries []jobsvc.RetryLedgerEntry) error
	// UpdateRetryBatchWithPayload sets reason='retry' along with payload and adapter.
	// Used by the LLM exhaustion path so the sweeper can re-publish without re-fetching.
	UpdateRetryBatchWithPayload(ctx context.Context, entries []jobsvc.RetryLedgerEntryWithPayload) error
}

// MessageFetcher is the narrow interface the analyzer worker needs from the NATS pull consumer.
type MessageFetcher interface {
	Fetch(n int, maxWait time.Duration) ([]*js.Msg, error)
}

// RateLimiter is the narrow interface the analyzer worker needs for per-adapter rate limiting.
type RateLimiter interface {
	Wait(ctx context.Context, adapter string) error
}
