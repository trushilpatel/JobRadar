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
type PipelineDBStats struct {
	TotalCompanies  int64 `gorm:"column:total_companies"`
	ActiveCompanies int64 `gorm:"column:active_companies"`
	Queued          int64 `gorm:"column:queued"`
	Crawling        int64 `gorm:"column:crawling"`
	NewJobs24h      int64 `gorm:"column:new_jobs_24h"`
	Errors24h       int64 `gorm:"column:errors_24h"`
	// PipelinePending: jobs with reason='fetch' not yet in `jobs` — truly awaiting the filter worker.
	PipelinePending int64 `gorm:"column:pipeline_pending"`
	// PipelineFiltered: jobs with reason='filtered' not yet in `jobs` — passed filter, awaiting enricher.
	PipelineFiltered int64 `gorm:"column:pipeline_filtered"`
}
