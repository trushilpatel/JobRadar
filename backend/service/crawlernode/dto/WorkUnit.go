package dto

import "time"

// WorkUnit is the assignment handed to a crawler node by the orchestrator.
type WorkUnit struct {
	AssignmentID  string  `json:"id"`
	CompanyID     string  `json:"company_id"`
	AdapterType   string  `json:"adapter_type"`
	AdapterConfig *string `json:"adapter_config"` // JSON blob for ConfigurableAdapter; nil for simple adapters
	Source        string  `json:"source"`
}

// WorkResponse is returned by GET /node/work.
type WorkResponse struct {
	Assignment      *WorkUnit `json:"assignment"`       // nil when queue is empty
	HeartbeatAcked  []string  `json:"heartbeat_acked"`  // lease IDs successfully extended
	HeartbeatMissed []string  `json:"heartbeat_missed"` // lease IDs that are gone; node must abort
}

// ScanQueueItem is the minimal DB projection of a company_scan_queue row.
type ScanQueueItem struct {
	ID          string     `json:"id"`
	CompanyID   string     `json:"company_id"`
	AdapterType string     `json:"adapter_type"`
	Status      string     `json:"status"`
	Source      string     `json:"source"`
	StartedAt   *time.Time `json:"started_at"`
	HeartbeatAt *time.Time `json:"heartbeat_at"`
}
