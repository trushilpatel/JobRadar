package common

// JobStatus values for the per-user job status. Workflow values (applied /
// dismissed / interviewing / offer / rejected) mirror the CHECK constraint
// on user_job_state.status. JobStatusNew and JobStatusSaved are NOT stored:
//
//   - JobStatusNew is synthetic — absence of a workflow status; only
//     returned via the overlay or accepted as a filter/tab key.
//   - JobStatusSaved is a filter/tab key — `saved_at IS NOT NULL` is the
//     source of truth for the saved flag; it is orthogonal to the workflow
//     status column.
const (
	JobStatusNew          = "new"   // synthetic — never written; overlay/filter only
	JobStatusSaved        = "saved" // filter/tab key — never written to status column; saved_at IS NOT NULL is the source of truth
	JobStatusApplied      = "applied"
	JobStatusDismissed    = "dismissed"
	JobStatusInterviewing = "interviewing"
	JobStatusOffer        = "offer"
	JobStatusRejected     = "rejected"
)

// SeenReason values for job_pipeline_ledger.reason. Mirrors the CHECK
// constraint installed by migration 059. Adding a new value here must be
// paired with a new migration that widens the constraint.
const (
	SeenReasonFetch         = "fetch"
	SeenReasonFiltered      = "filtered" // passed filter, published to jobs.filtered.*
	SeenReasonLLMIrrelevant = "llm_irrelevant"
	SeenReasonRetry         = "retry"
	SeenReasonUserDismissed = "user_dismissed"
	SeenReasonError         = "error"
	SeenReasonNonUSA        = "non_usa_location"
	SeenReasonSaved         = "saved" // job successfully written to the jobs table
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
