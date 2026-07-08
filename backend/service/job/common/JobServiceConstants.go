package common

// JobStatus values for the per-user job status. Workflow values (applied /
// interviewing / offer / rejected) mirror the CHECK constraint on
// user_job_state.status. JobStatusNew and JobStatusSaved are NOT stored:
//
//   - JobStatusNew is synthetic — absence of a workflow status; only
//     returned via the overlay or accepted as a filter/tab key.
//   - JobStatusSaved is a filter/tab key — `saved_at IS NOT NULL` is the
//     source of truth for the saved flag; it is orthogonal to the workflow
//     status column.
//
// JobStatusDismissed is no longer writable (the dismiss feature has been
// removed — see migration 091) but the constant remains: existing
// 'dismissed' rows stay in the DB permanently (never deleted, to avoid
// resurfacing the non-USA jobs migration 084 hid), and code that reads
// historical rows — the apply-queue's terminal-status exclusion, the
// per-user counters, the "New" tab count math — still needs to recognize
// the value.
const (
	JobStatusNew          = "new"   // synthetic — never written; overlay/filter only
	JobStatusSaved        = "saved" // filter/tab key — never written to status column; saved_at IS NOT NULL is the source of truth
	JobStatusApplied      = "applied"
	JobStatusDismissed    = "dismissed" // historical only — no longer writable via any API
	JobStatusInterviewing = "interviewing"
	JobStatusOffer        = "offer"
	JobStatusRejected     = "rejected"
)

// SeenReason is the type-safe alias for job_pipeline_ledger.reason values.
// Using SeenReason instead of string in function signatures makes the
// compiler reject raw-string literals and catch stale constant references.
type SeenReason string

// SeenReason values for job_pipeline_ledger.reason. Mirrors the CHECK
// constraint installed by migration 059. Adding a new value here must be
// paired with a new migration that widens the constraint.
const (
	SeenReasonFetch         SeenReason = "fetch"
	SeenReasonFiltered      SeenReason = "filtered" // passed filter, published to jobs.filtered.*
	SeenReasonLLMIrrelevant SeenReason = "llm_irrelevant"
	SeenReasonRetry         SeenReason = "retry"
	SeenReasonError         SeenReason = "error"
	SeenReasonNonUSA        SeenReason = "non_usa_location"
	SeenReasonSaved         SeenReason = "saved" // job successfully written to the jobs table
)

// EmployType values for jobs.employ_type
const (
	EmployFullTime   = "full_time"
	EmployPartTime   = "part_time"
	EmployContract   = "contract"
	EmployInternship = "internship"
)
