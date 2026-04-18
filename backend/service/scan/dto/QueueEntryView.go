package dto

import "time"

// QueueEntryView is the enriched representation of a scan queue entry.
type QueueEntryView struct {
	ID          string     `json:"id"`
	CompanyID   string     `json:"company_id"`
	CompanyName string     `json:"company_name"`
	AdapterType string     `json:"adapter_type"`
	Status      string     `json:"status"`
	Source      string     `json:"source"`
	Priority    int        `json:"priority"`
	QueuedAt    time.Time  `json:"queued_at"`
	StartedAt   *time.Time `json:"started_at"`
	NextScanAt  *time.Time `json:"next_scan_at"`
}
