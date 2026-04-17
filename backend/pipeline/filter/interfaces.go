package filter

import "context"

// FilterLedger is the narrow ledger interface the filter worker needs.
type FilterLedger interface {
	UpdateSeenReasonBatch(ctx context.Context, jobIDs []string, reason string) error
}
