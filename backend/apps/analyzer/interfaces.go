package main

import (
	"context"

	jobsvc "github.com/trushilpatel/jobradar/service/job/services"
)

// EnricherLedger is the narrow ledger contract the worker depends on.
// Keeps the worker off the broad PipelineService for per-job ledger writes,
// matching the pattern established by apps/filter/interfaces.go (FilterLedger).
type EnricherLedger interface {
	UpdateRetryBatch(ctx context.Context, entries []jobsvc.RetryLedgerEntry) error
}
