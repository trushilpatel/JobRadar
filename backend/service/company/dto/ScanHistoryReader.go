package dto

import "context"

// ScanHistoryReader is the narrow interface the company service needs to populate
// per-company scan history in list responses. Implemented by scan.ScanLogStore.
type ScanHistoryReader interface {
	GetScanHistories(ctx context.Context, companyIDs []string) (map[string][]bool, error)
}
