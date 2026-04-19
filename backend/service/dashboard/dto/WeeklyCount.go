package dto

// WeeklyCount holds job count for a single calendar week in the heatmap.
type WeeklyCount struct {
	WeekStart string `json:"week_start"` // "YYYY-MM-DD"
	JobCount  int    `json:"job_count"`
}
