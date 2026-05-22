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
