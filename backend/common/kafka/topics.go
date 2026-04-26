package kafka

// Primary pipeline topics.
const (
	TopicJobListingsRaw      = "job_listings.raw"
	TopicJobListingsFiltered = "job_listings.filtered"
)

// Retry + DLQ topics for the filter worker (consumes .raw).
const (
	TopicJobListingsRawRetry1m  = "job_listings.raw.retry.1m"
	TopicJobListingsRawRetry10m = "job_listings.raw.retry.10m"
	TopicJobListingsRawRetry1h  = "job_listings.raw.retry.1h"
	TopicJobListingsRawDLQ      = "job_listings.raw.dlq"
)

// Retry + DLQ topics for the analyzer worker (consumes .filtered).
const (
	TopicJobListingsFilteredRetry1m  = "job_listings.filtered.retry.1m"
	TopicJobListingsFilteredRetry10m = "job_listings.filtered.retry.10m"
	TopicJobListingsFilteredRetry1h  = "job_listings.filtered.retry.1h"
	TopicJobListingsFilteredDLQ      = "job_listings.filtered.dlq"
)

// Retry metadata carried in Kafka headers.
const (
	HeaderRetryCount    = "retry_count"     // int as decimal string; 0 = fresh message
	HeaderNextAttemptAt = "next_attempt_at" // unix milliseconds as decimal string
	HeaderLastError     = "last_error"      // last error message — set on DLQ only
	HeaderReason        = "reason"          // routing reason (e.g. "malformed") — set on DLQ only
)
