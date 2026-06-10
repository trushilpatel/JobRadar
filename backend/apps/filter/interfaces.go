package main

import (
	"context"
	"time"

	js "github.com/trushilpatel/jobradar/common/jetstream"
	jobcommon "github.com/trushilpatel/jobradar/service/job/common"
	jobsvc "github.com/trushilpatel/jobradar/service/job/services"
)

// FilterLedger is the narrow ledger interface the filter worker needs.
type FilterLedger interface {
	UpdateSeenReasonBatch(ctx context.Context, jobIDs []string, reason jobcommon.SeenReason) error
	UpdateRetryBatch(ctx context.Context, entries []jobsvc.RetryLedgerEntry) error
	// UpdateRetryBatchWithPayload sets reason='retry' along with payload and adapter.
	// Used by the LLM exhaustion path so the sweeper can re-publish without re-fetching.
	UpdateRetryBatchWithPayload(ctx context.Context, entries []jobsvc.RetryLedgerEntryWithPayload) error
}

// MessageFetcher is the narrow interface the filter worker needs from the NATS pull consumer.
type MessageFetcher interface {
	Fetch(n int, maxWait time.Duration) ([]*js.Msg, error)
}
