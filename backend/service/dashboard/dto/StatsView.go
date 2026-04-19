package dto

// StatsView holds the data returned by GET /dashboard/stats.
type StatsView struct {
	JobsScannedToday int64         `json:"jobs_scanned_today"`
	JobsNew          int64         `json:"jobs_new"`
	JobsSaved        int64         `json:"jobs_saved"`
	JobsApplied      int64         `json:"jobs_applied"`
	PipelinePending  int64         `json:"pipeline_pending"`
	PipelineErrors   int64         `json:"pipeline_errors"`
	Funnel           FunnelSummary `json:"funnel"`
}

// FunnelSummary is a simplified funnel for the dashboard stats card.
type FunnelSummary struct {
	Scanned  int64 `json:"scanned"`
	Relevant int64 `json:"relevant"`
	Saved    int64 `json:"saved"`
	Applied  int64 `json:"applied"`
}
