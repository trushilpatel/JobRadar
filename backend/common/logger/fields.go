package logger

import "time"

// Field is a typed key-value pair attached to a log event.
// Use the constructor functions below — never construct Field directly.
type Field struct {
	Key   string
	Value any
}

// Standardized field key constants. Always use these — never raw strings.
// Consistent keys are required for Loki queries to work across services.
const (
	FieldService       = "service"
	FieldCompany       = "company_id"
	FieldJobID         = "job_id"
	FieldDuration      = "duration"
	FieldFetched       = "fetched"
	FieldNew           = "new"
	FieldError         = "error"
	FieldBatch         = "batch"
	FieldRelevant      = "relevant"
	FieldMethod        = "method"
	FieldPath          = "path"
	FieldStatus        = "status"
	FieldURL           = "url"
	FieldQueueID       = "queue_id"
	FieldEnqueued      = "enqueued"
	FieldFetchID       = "fetch_id"
	FieldChunkIndex    = "chunk_index"
	FieldChunkCount    = "chunk_count"
	FieldRetryCount    = "retry_count"
	FieldRetryTier     = "retry_tier"
	FieldNextAttemptAt = "next_attempt_at"
	FieldPartition     = "partition"
	FieldOffset        = "offset"
	FieldUserID        = "user_id"
	FieldEmailDomain   = "email_domain"
	FieldResumeID      = "resume_id"
	FieldStorageKey    = "storage_key"

	// Worker / pipeline fields
	FieldAdapterType         = "adapter_type"
	FieldAdapter             = "adapter"
	FieldStack               = "stack"
	FieldSleep               = "sleep"
	FieldSource              = "source"
	FieldLocation            = "location"
	FieldLocationKnown       = "location_known"
	FieldCountry             = "country"
	FieldCountryCode         = "country_code"
	FieldScanID              = "scan_id"
	FieldStage               = "stage"
	FieldCause               = "cause"
	FieldMsgs                = "msgs"
	FieldBatches             = "batches"
	FieldFetchConcurrency    = "fetch_concurrency"
	FieldFetchDuration       = "fetch_duration"
	FieldEnrichDuration      = "enrich_duration"
	FieldLLMBatchSize        = "llm_batch_size"
	FieldLLMCallInterval     = "llm_call_interval"
	FieldRejectedBy          = "rejected_by"
	FieldDescriptionBytes    = "description_bytes"
	FieldJobIDs              = "job_ids"
	FieldTitle               = "title"
	FieldDropReason          = "drop_reason"
	FieldResultCount         = "result_count"
	FieldRoleMatch           = "role_match"
	FieldTechStackCount      = "tech_stack_count"
	FieldRecords             = "records"
	FieldNonUSACount         = "non_usa_count"
	FieldPassThroughCount    = "pass_through_count"
	FieldEntriesCount        = "entries_count"
	FieldRetriedCount        = "retried_count"
	FieldExhaustedCount      = "exhausted_count"
	FieldPublishFailedCount  = "publish_failed_count"
	FieldDLQedCount          = "dlqed_count"
	FieldRetryAfter          = "retry_after"
	FieldRetryIn             = "retry_in"
	FieldSlotsDetail         = "slots_detail"
	FieldIrrelevant          = "irrelevant"
	FieldScanIntervalMinutes = "scan_interval_minutes"
	FieldCheckInterval       = "check_interval"
	FieldStuckTimeout        = "stuck_timeout"
	FieldJitter              = "jitter"
	FieldReason              = "reason"
	FieldAttempt             = "attempt"
	FieldMaxAttempts         = "max_attempts"
	FieldPurged              = "purged"
	FieldSkippedNoSettings   = "skipped_no_settings"
	FieldStillActive         = "still_active"
	FieldRemoved             = "removed"
	FieldConfigBytes         = "config_bytes"
	FieldCount               = "count"
	FieldHasSettings         = "has_settings"
	FieldActive              = "active"
	FieldSweeperEnabled      = "sweeper_enabled"

	// Crawler-v2 adapter fields (unified from apps/crawler-v2/common/logger)
	FieldDepartment = "department"
	FieldJobCount   = "job_count"
	FieldPage       = "page"
	FieldSpanID     = "span_id"
	FieldStatusCode = "status_code"
	FieldTotal      = "total"
	FieldTotalPages = "total_pages"
	FieldTraceID    = "trace_id"
	FieldDurationMs = "duration_ms"
)

// Str creates a string field.
func Str(key, val string) Field { return Field{Key: key, Value: val} }

// Int creates an integer field.
func Int(key string, val int) Field { return Field{Key: key, Value: val} }

// Dur creates a duration field.
func Dur(key string, d time.Duration) Field { return Field{Key: key, Value: d} }

// Bool creates a boolean field.
func Bool(key string, val bool) Field { return Field{Key: key, Value: val} }

// Err creates an error field using FieldError as the key.
// Use this to attach an error at a non-Error log level (e.g. Warn).
// At Error level, pass err directly to Logger.Error() instead.
func Err(err error) Field { return Field{Key: FieldError, Value: err} }
