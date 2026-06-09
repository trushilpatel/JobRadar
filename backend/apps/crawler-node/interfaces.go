package main

import "context"

// OrchestratorClient is the node's only external dependency — all
// orchestrator communication goes through this interface.
type OrchestratorClient interface {
	// GetWork polls for a new assignment while piggybacking heartbeats for all
	// holding IDs. The result contains the (optional) new assignment and the
	// list of holding IDs whose leases have expired on the orchestrator.
	GetWork(ctx context.Context, holding []string) (*GetWorkResult, error)

	// SubmitBatch delivers a page of crawled jobs. Set batch.Final = true on
	// the last (or only) batch for this assignment.
	SubmitBatch(ctx context.Context, batch PageBatch) error

	// ReportError marks the assignment as failed and resets it to 'queued'
	// so the scheduler can retry later.
	ReportError(ctx context.Context, e WorkError) error
}

// AdapterExecutor runs one WorkUnit end-to-end. It is responsible for
// wiring the adapter interfaces (SeenChecker, StreamPublisher, etc.) to
// the OrchestratorClient callbacks. Injected in tests via a mock.
type AdapterExecutor interface {
	Execute(ctx context.Context, unit WorkUnit) error
}
