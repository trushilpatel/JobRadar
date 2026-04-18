package dto

// DailyScanCount holds aggregated scan totals for a given date.
type DailyScanCount struct {
	Date    string `json:"date"`
	Scanned int    `json:"scanned"`
	NewJobs int    `json:"new_jobs"`
}
