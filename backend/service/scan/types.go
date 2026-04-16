package scan

import (
	"context"
	"time"
)

// Config holds per-service configuration for the scan domain.
type Config struct {
	ScanIntervalMinutes int
}

// QueueFilter holds optional filter criteria for listing the scan queue.
type QueueFilter struct {
	Status string // "queued" | "processing"
	Source string // "manual" | "scheduler"
}

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

// ScanQueueItem is a raw row from company_scan_queue (for pipeline workers).
type ScanQueueItem struct {
	ID        string     `json:"id"`
	CompanyID string     `json:"company_id"`
	Status    string     `json:"status"`
	Source    string     `json:"source"`
	Priority  int        `json:"priority"`
	QueuedAt  time.Time  `json:"queued_at"`
	StartedAt *time.Time `json:"started_at"`
}

// ScanRun is a single row from company_scan_log, as rendered in the UI.
type ScanRun struct {
	CompanyID   string    `json:"company_id"`
	ScannedAt   time.Time `json:"scanned_at"`
	JobsFetched int       `json:"jobs_fetched"`
	JobsNew     int       `json:"jobs_new"`
	Error       *string   `json:"error"`
	DurationMs  *int      `json:"duration_ms"`
}

// ScanStats is a read-only summary of a company's scan history.
type ScanStats struct {
	TotalScans    int64     `json:"total_scans"`
	TotalFetched  int64     `json:"total_fetched"`
	TotalNew      int64     `json:"total_new"`
	ErrorCount    int64     `json:"error_count"`
	AvgDurationMs *float64  `json:"avg_duration_ms"`
	NewToday      int64     `json:"new_today"`
	NewThisMonth  int64     `json:"new_this_month"`
	RecentRuns    []ScanRun `json:"recent_runs"`
}

// DailyScanCount holds aggregated scan totals for a given date.
type DailyScanCount struct {
	Date    string `json:"date"`
	Scanned int    `json:"scanned"`
	NewJobs int    `json:"new_jobs"`
}

// ScanStatsView is an alias for ScanStats.
type ScanStatsView = ScanStats

// ClaimNexter is the minimal interface needed by the crawler to claim queue items.
type ClaimNexter interface {
	ClaimNext(ctx context.Context) (*ScanQueueItem, error)
	Complete(id string) error
	ResetStuck(staleDur time.Duration) error
}
