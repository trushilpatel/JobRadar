package dto

import "github.com/trushilpatel/jobradar/common/ats/core"

// ResultsRequest is the body of POST /node/results.
type ResultsRequest struct {
	AssignmentID string            `json:"assignment_id"`
	CompanyID    string            `json:"company_id"`
	AdapterType  string            `json:"adapter_type"`
	Jobs         []core.JobSummary `json:"jobs"`
	FetchedCount int               `json:"fetched_count"`
	DurationMs   int64             `json:"duration_ms"`
	Final        bool              `json:"final"`
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
