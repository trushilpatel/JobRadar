package dto

// ActivityView holds per-day scan activity. Reuses scan domain type.
type ActivityView struct {
	Date    string `json:"date"`
	Scanned int    `json:"scanned"`
	NewJobs int    `json:"new_jobs"`
}
