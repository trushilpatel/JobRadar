package crawler

import (
	"github.com/trushilpatel/jobradar/model"
	scansvc "github.com/trushilpatel/jobradar/service/scan"
)

// CompanyReader is the narrow read interface the crawler needs for companies.
type CompanyReader interface {
	GetActiveCompanies() ([]model.Company, error)
	GetCompanyByID(id string) (model.Company, error)
}

// PipelineLedger is the narrow ledger interface the crawler needs.
type PipelineLedger interface {
	HasSeen(jobID string) (bool, error)
	MarkSeen(jobID, companyID, reason string) error
	GetActiveJobIDsByCompany(companyID string) ([]string, error)
	MarkRemovedBatch(jobIDs []string) error
}

// ScanRecorder is the interface the crawler uses to record scan outcomes.
// This replaces the old separate scanLog + companyScan fields.
type ScanRecorder interface {
	RecordScanResult(companyID string, result scansvc.ScanResult) error
}
