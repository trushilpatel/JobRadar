package dto

// StatsView is the response for GET /dashboard/stats.
// Per-user job counts (new/saved/applied) were moved to user_job_state in Phase 2
// and are now served exclusively by GET /dashboard/progress (ProgressStats).
type StatsView struct {
	JobsScannedToday int64 `json:"jobs_scanned_today"`
	PipelinePending  int64 `json:"pipeline_pending"`
	PipelineErrors   int64 `json:"pipeline_errors"`
}
