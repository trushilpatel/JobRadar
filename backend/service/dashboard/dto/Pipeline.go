package dto

// PipelineStats is the full response returned by GET /dashboard/pipeline.
type PipelineStats struct {
	TotalCompanies     int64 `json:"total_companies"`
	ActiveCompanies    int64 `json:"active_companies"`
	Queued             int64 `json:"queued"`
	Crawling           int64 `json:"crawling"`
	AwaitingFilter     int64 `json:"awaiting_filter"`
	AwaitingEnrichment int64 `json:"awaiting_enrichment"`
	NewJobs24h         int64 `json:"new_jobs_24h"`
	Errors24h          int64 `json:"errors_24h"`
}

// PipelineDBStats holds the DB-sourced counts for the pipeline strip.
type PipelineDBStats struct {
	TotalCompanies  int64 `gorm:"column:total_companies"`
	ActiveCompanies int64 `gorm:"column:active_companies"`
	Queued          int64 `gorm:"column:queued"`
	Crawling        int64 `gorm:"column:crawling"`
	NewJobs24h      int64 `gorm:"column:new_jobs_24h"`
	Errors24h       int64 `gorm:"column:errors_24h"`
	// PipelinePending: jobs crawled (reason='fetch') but not yet written to `jobs`.
	// Used as the DB-based proxy for awaiting_filter when Kafka lag is unavailable.
	PipelinePending int64 `gorm:"column:pipeline_pending"`
}
