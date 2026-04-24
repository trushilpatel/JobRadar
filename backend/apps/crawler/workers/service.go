package workers

import "context"

// AdapterCrawlerService is implemented by each per-adapter crawler service.
// Register all implementations with Registry.StartAll to run them concurrently.
type AdapterCrawlerService interface {
	AdapterType() string
	Run(ctx context.Context)
}

// Scanner is the narrow interface workers use to trigger a single-company scan.
// Engine in package main satisfies this via its RunForCompany method.
type Scanner interface {
	RunForCompany(ctx context.Context, companyID string)
}

// CrawlerQueueService is the narrow queue interface workers depend on.
// Satisfied by scansvc.CrawlerQueueService from the service layer.
type CrawlerQueueService interface {
	ClaimNext(ctx context.Context, adapterType string) (*ScanQueueItem, error)
	Complete(ctx context.Context, id string) error
	UpdateHeartbeat(ctx context.Context, id string) error
}

// ScanQueueItem carries the data a worker needs after claiming a queue entry.
type ScanQueueItem struct {
	ID          string
	CompanyID   string
	AdapterType string
	Source      string
}
