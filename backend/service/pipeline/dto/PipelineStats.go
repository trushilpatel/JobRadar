// Package dto holds the data transfer objects for the pipeline domain.
package dto

// RetryTierCounts breaks a worker's pending-redelivery backlog down by NATS
// redelivery tier. Always zero today — NATS JetStream's BackOff/MaxDeliver
// redelivers automatically, so there is no manual per-tier queue to count.
// The shape is kept so the frontend's tier-breakdown UI has a stable
// contract if per-tier introspection is added later.
type RetryTierCounts struct {
	OneMin  int64 `json:"1m"`
	TenMin  int64 `json:"10m"`
	OneHour int64 `json:"1h"`
	DLQ     int64 `json:"dlq"`
}

// PipelineStats is the JSON-serialisable snapshot of pipeline health served
// to the admin Pipeline page: scan-queue depth, filter/enrichment backlog,
// and recent throughput/error counts.
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

// TriggerRetryInput identifies which worker/tier an admin wants to nudge.
// NATS JetStream's BackOff/MaxDeliver handles actual redelivery
// automatically — validating the pair is all this input drives today.
type TriggerRetryInput struct {
	Worker string `json:"worker"`
	Tier   string `json:"tier"`
}

// TriggerRetryResult reports how many messages were drained. Always zero
// today (see TriggerRetryInput) — kept as a stable response shape for when
// manual draining is added.
type TriggerRetryResult struct {
	Drained int64 `json:"drained"`
}
