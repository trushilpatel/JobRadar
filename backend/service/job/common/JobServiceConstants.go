package common

// JobStatus values for the per-user UserJobState.status column. Mirrors the
// CHECK constraint on user_job_state.status. Note: "new" is intentionally
// NOT a valid stored status — absence of a row implies "new".
const (
	JobStatusNew          = "new" // synthetic — never written, only returned via overlay
	JobStatusSaved        = "saved"
	JobStatusApplied      = "applied"
	JobStatusDismissed    = "dismissed"
	JobStatusInterviewing = "interviewing"
	JobStatusOffer        = "offer"
	JobStatusRejected     = "rejected"
)

// SeenReason values for job_pipeline_ledger.reason. Mirrors the CHECK
// constraint installed by migration 057. Adding a new value here must be
// paired with a new migration that widens the constraint.
const (
	SeenReasonFetch         = "fetch"
	SeenReasonLLMIrrelevant = "llm_irrelevant"
	SeenReasonRetry         = "retry"
	SeenReasonUserDismissed = "user_dismissed"
	SeenReasonError         = "error"
	SeenReasonNonUSA        = "non_usa_location"
)

// DismissalReason values stored on user_job_state.dismissal_reason. The
// allowed set is enforced at the application layer (the table itself does
// not constrain it) so all dismissal flows write one of these values.
const (
	DismissalReasonUserDismissed = "user_dismissed"
	DismissalReasonNotRelevant   = "user_not_relevant"
)

// EmployType values for jobs.employ_type
const (
	EmployFullTime   = "full_time"
	EmployPartTime   = "part_time"
	EmployContract   = "contract"
	EmployInternship = "internship"
)
