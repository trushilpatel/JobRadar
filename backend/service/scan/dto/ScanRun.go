package dto

import "time"

// ScanRun is a single row from company_scan_log, as rendered in the UI.
type ScanRun struct {
	CompanyID   string    `json:"company_id"`
	ScannedAt   time.Time `json:"scanned_at"`
	JobsFetched int       `json:"jobs_fetched"`
	JobsNew     int       `json:"jobs_new"`
	Error       *string   `json:"error"`
	DurationMs  *int      `json:"duration_ms"`
}
