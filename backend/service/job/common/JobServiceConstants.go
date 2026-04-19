package common

// JobStatus values for job_feed.status
const (
	JobStatusNew          = "new"
	JobStatusSaved        = "saved"
	JobStatusApplied      = "applied"
	JobStatusInterviewing = "interviewing"
	JobStatusOffer        = "offer"
	JobStatusDismissed    = "dismissed"
)

// SeenReason values for job_pipeline_ledger.reason
const (
	SeenReasonFetch         = "fetch"
	SeenReasonLLMIrrelevant = "llm_irrelevant"
	SeenReasonUserDismissed = "user_dismissed"
	SeenReasonError         = "error"
)

// DiscardReason values for job_dismissal_log.reason
const (
	DiscardReasonDismissed   = "user_dismissed"
	DiscardReasonNotRelevant = "user_not_relevant"
)

// EmployType values for job_feed.employ_type
const (
	EmployFullTime   = "full_time"
	EmployPartTime   = "part_time"
	EmployContract   = "contract"
	EmployInternship = "internship"
)
