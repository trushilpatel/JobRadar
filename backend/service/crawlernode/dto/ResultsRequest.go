package dto

import "github.com/trushilpatel/jobradar/common/ats/core"

// ResultsRequest is the body of POST /node/results. required:"false" on
// AssignmentID/CompanyID/AdapterType matches the pre-Huma handler's plain
// json.Decode — an absent field reaches ValidationService's existing
// non-empty checks rather than failing at the framework layer.
type ResultsRequest struct {
	AssignmentID string            `json:"assignment_id" required:"false"`
	CompanyID    string            `json:"company_id" required:"false"`
	AdapterType  string            `json:"adapter_type" required:"false"`
	Jobs         []core.JobSummary `json:"jobs" required:"false"`
	FetchedCount int               `json:"fetched_count" required:"false"`
	DurationMs   int64             `json:"duration_ms" required:"false"`
	Final        bool              `json:"final" required:"false"`
}

// ResultsResponse is returned by POST /node/results.
type ResultsResponse struct {
	Published int  `json:"published"` // jobs successfully published to NATS
	New       int  `json:"new"`       // jobs not previously seen
	Removed   int  `json:"removed"`   // jobs present before but absent from this final submission
	Completed bool `json:"completed"` // true when final:true and queue row deleted
}

// ScanResult carries the outcome summary written to company_scan_log.
type ScanResult struct {
	FetchedCount int
	NewCount     int
	DurationMs   int64
	ErrMsg       string // empty means success
}
