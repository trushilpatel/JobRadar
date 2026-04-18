package crawler

import (
	"context"

	"github.com/trushilpatel/jobradar/model"
	scansvc "github.com/trushilpatel/jobradar/service/scan"
)

// CompanyReader is the narrow read interface the crawler needs for companies.
type CompanyReader interface {
	GetActiveCompanies(ctx context.Context) ([]model.Company, error)
	GetCompanyByID(ctx context.Context, id string) (model.Company, error)
}

// PipelineLedger is the narrow ledger interface the crawler needs.
type PipelineLedger interface {
	HasSeen(ctx context.Context, jobID string) (bool, error)
	HasSeenBatch(ctx context.Context, jobIDs []string) (map[string]bool, error)
	MarkSeen(ctx context.Context, jobID, companyID, reason string) error
	MarkSeenBatch(ctx context.Context, jobIDs []string, companyID, reason string) error
	GetActiveJobIDsByCompany(ctx context.Context, companyID string) ([]string, error)
	MarkRemovedBatch(ctx context.Context, jobIDs []string) error
}

// ScanRecorder is the interface the crawler uses to record scan outcomes.
// This replaces the old separate scanLog + companyScan fields.
type ScanRecorder interface {
	RecordScanResult(ctx context.Context, companyID string, result scansvc.ScanResult) error
}
