package dto

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

// ScanStatsView is an alias for ScanStats.
type ScanStatsView = ScanStats
