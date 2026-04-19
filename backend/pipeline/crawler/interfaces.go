package crawler

import (
	"context"

	companymodel "github.com/trushilpatel/jobradar/service/company/models"
	"github.com/trushilpatel/jobradar/service/scan/dto"
)

// CompanyReader is the narrow company interface the crawler engine depends on.
type CompanyReader interface {
	GetByID(ctx context.Context, id string) (companymodel.Company, error)
	GetActive(ctx context.Context) ([]companymodel.Company, error)
}

// PipelineLedger is the narrow ledger interface the crawler engine depends on.
type PipelineLedger interface {
	HasSeenBatch(ctx context.Context, jobIDs []string) (map[string]bool, error)
	MarkSeenBatch(ctx context.Context, jobIDs []string, companyID, reason string) error
	GetActiveJobIDsByCompany(ctx context.Context, companyID string) ([]string, error)
	MarkRemovedBatch(ctx context.Context, jobIDs []string) error
}

// ScanRecorder is the interface the crawler engine uses to record scan outcomes.
type ScanRecorder interface {
	RecordScanResult(ctx context.Context, companyID string, result dto.ScanResult) error
}
