package jetstream

// Stream names.
const (
	StreamScanRequests = "SCAN_REQUESTS"
	StreamJobsRaw      = "JOBS_RAW"
	StreamJobsFiltered = "JOBS_FILTERED"
	StreamJobsDLQ      = "JOBS_DLQ"
)

// Subject patterns used when creating streams (> = wildcard) and when publishing.
const (
	SubjectScanRequests         = "scan.requests"
	SubjectJobsRawWildcard      = "jobs.raw.>"       // stream filter
	SubjectJobsRawFmt           = "jobs.raw.%s"      // format with adapter name when publishing
	SubjectJobsFilteredWildcard = "jobs.filtered.>"  // stream filter
	SubjectJobsFilteredFmt      = "jobs.filtered.%s" // format with adapter name when publishing
	SubjectJobsDLQ              = "jobs.dlq"
)

// Durable consumer names.
const (
	ConsumerOrchestratorWorkers = "ORCHESTRATOR_WORKERS"
	ConsumerFilterWorkers       = "FILTER_WORKERS"
	ConsumerAnalyzerWorkers     = "ANALYZER_WORKERS"
)

// NATS message header keys used across the pipeline.
const (
	// HeaderRetryCount is set by the sweeper on re-injected messages to carry
	// the monotonically increasing retry count accumulated across all sweeper
	// re-publishes. The filter reads this header (falling back to
	// DeliveryCount-1) so the ledger retry_count never resets to zero on a
	// sweeper re-inject. Value is a decimal integer string.
	HeaderRetryCount = "X-Retry-Count"
)
