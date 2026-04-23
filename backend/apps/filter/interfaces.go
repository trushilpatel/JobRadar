package main

import (
	"context"

	jobsvc "github.com/trushilpatel/jobradar/service/job/services"
)

// FilterLedger is the narrow ledger interface the filter worker needs.
type FilterLedger interface {
	UpdateSeenReasonBatch(ctx context.Context, jobIDs []string, reason string) error
	UpdateRetryBatch(ctx context.Context, entries []jobsvc.RetryLedgerEntry) error
}
