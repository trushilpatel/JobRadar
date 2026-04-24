package dto

import "time"

// ScanQueueItem is a raw row from company_scan_queue (for pipeline workers).
type ScanQueueItem struct {
	ID          string     `json:"id"`
	CompanyID   string     `json:"company_id"`
	AdapterType string     `json:"adapter_type"`
	Status      string     `json:"status"`
	Source      string     `json:"source"`
	Priority    int        `json:"priority"`
	QueuedAt    time.Time  `json:"queued_at"`
	StartedAt   *time.Time `json:"started_at"`
	HeartbeatAt *time.Time `json:"heartbeat_at"`
}
