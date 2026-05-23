package dto

// RetryTierCounts holds the per-tier retry backlog counts for one worker.
type RetryTierCounts struct {
	Tier1m  int64 `json:"1m"`
	Tier10m int64 `json:"10m"`
	Tier1h  int64 `json:"1h"`
	DLQ     int64 `json:"dlq"`
}

// TriggerRetryRequest is the body for POST /admin/dashboard/pipeline/retry.
type TriggerRetryRequest struct {
	Worker string `json:"worker"` // "filter" or "enricher"
	Tier   string `json:"tier"`   // "1m", "10m", "1h", "dlq"
}

// TriggerRetryResponse is the response for POST /admin/dashboard/pipeline/retry.
type TriggerRetryResponse struct {
	Drained int64 `json:"drained"`
}

// PipelineStats is the full response returned by GET /dashboard/pipeline.
type PipelineStats struct {
	TotalCompanies     int64           `json:"total_companies"`
	ActiveCompanies    int64           `json:"active_companies"`
	Queued             int64           `json:"queued"`
	Crawling           int64           `json:"crawling"`
	AwaitingFilter     int64           `json:"awaiting_filter"`
	FilterRetries      RetryTierCounts `json:"filter_retries"`
	AwaitingEnrichment int64           `json:"awaiting_enrichment"`
	EnricherRetries    RetryTierCounts `json:"enricher_retries"`
	NewJobs24h         int64           `json:"new_jobs_24h"`
	Errors24h          int64           `json:"errors_24h"`
}

// PipelineDBStats holds the DB-sourced counts for the pipeline strip.
// GORM column scanning is handled internally by the repository via pipelineDBStatsRow.
type PipelineDBStats struct {
	TotalCompanies  int64
	ActiveCompanies int64
	Queued          int64
	Crawling        int64
	NewJobs24h      int64
	Errors24h       int64
	// PipelinePending: jobs with reason='fetch' not yet in `jobs` — truly awaiting the filter worker.
	PipelinePending int64
	// PipelineFiltered: jobs with reason='filtered' not yet in `jobs` — passed filter, awaiting enricher.
	PipelineFiltered int64
}
