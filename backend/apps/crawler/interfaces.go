package main

import (
	"context"

	companymodel "github.com/trushilpatel/jobradar/service/company/models"
	"github.com/trushilpatel/jobradar/service/scan/dto"
)

// ScanSettingsReader is the narrow scan-settings interface the crawler
// engine depends on. Phase 2: per-company adapter selection and active flag
// live on `company_scan_settings`, not `companies`. The engine fetches a
// company's settings on every scan; if no row exists the company is treated
// as un-onboarded and the queue entry is logged-and-completed without
// invoking any adapter.
//
// Writes to scan state (last_scan_at / last_success_at / last_error_*) are
// performed atomically with the company_scan_log insert by ScanRecorder, not
// here — the reader is read-only from the engine's perspective.
type ScanSettingsReader interface {
	Get(ctx context.Context, companyID string) (*companymodel.CompanyScanSettings, error)
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
