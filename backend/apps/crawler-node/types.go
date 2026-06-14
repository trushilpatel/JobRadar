package main

import "github.com/trushilpatel/jobradar/common/ats/core"

// WorkUnit is the assignment handed to this node by the orchestrator.
// It mirrors service/crawlernode/dto.WorkUnit but is local to the binary
// so the node has no dependency on the orchestrator's service packages.
type WorkUnit struct {
	AssignmentID  string  `json:"id"`
	CompanyID     string  `json:"company_id"`
	AdapterType   string  `json:"adapter_type"`
	AdapterConfig *string `json:"adapter_config"`
	Source        string  `json:"source"`
}

// PageBatch is the payload submitted to POST /node/results.
// It mirrors service/crawlernode/dto.ResultsRequest. Set Final to true on the
// only (single) submission for an assignment — crawler-node submits once per
// WorkUnit (listing phase only; no streaming/paging).
type PageBatch struct {
	AssignmentID string            `json:"assignment_id"`
	CompanyID    string            `json:"company_id"`
	AdapterType  string            `json:"adapter_type"`
	Jobs         []core.JobSummary `json:"jobs"`
	FetchedCount int               `json:"fetched_count"`
	DurationMs   int64             `json:"duration_ms"`
	Final        bool              `json:"final"`
}

// WorkError is the payload submitted to POST /node/error.
type WorkError struct {
	AssignmentID string `json:"assignment_id"`
	CompanyID    string `json:"company_id"`
	AdapterType  string `json:"adapter_type"`
	ErrorMessage string `json:"error_message"`
	DurationMs   int64  `json:"duration_ms"`
}

// GetWorkResult carries both the (optional) new assignment and the heartbeat
// outcome. HeartbeatMissed contains assignment IDs the orchestrator no longer
// holds — the PollLoop must remove them from its holding list.
type GetWorkResult struct {
	Assignment      *WorkUnit // nil when queue is empty
	HeartbeatMissed []string  // assignment IDs that expired on the orchestrator
}
