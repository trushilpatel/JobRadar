// Package dto holds input/output shapes for the apply-queue service that
// are not GORM models.
package dto

// QueueItem is one row in the apply queue — a job the user is actively
// engaging with (saved and/or advanced to applied/interviewing), plus any
// resume currently attached to it.
//
// Status is derived, never a raw passthrough of user_job_state.status:
// NULL status (only saved_at set) renders as "saved"
// (jobcommon.JobStatusSaved); otherwise the actual status verbatim. Only
// {saved, applied, interviewing} are ever returned — both List filters
// exclude terminal statuses (dismissed/offer/rejected) even when saved_at
// is still set — matching the frontend's ApplyQueueStatusSchema
// three-value union.
//
// JSON field names for Title/URL are job_title/job_url (not the bare
// title/url used elsewhere, e.g. dto.JobView) — matches the already-shipped
// frontend contract in frontend/src/features/apply-queue/api/apply-queue.api.ts.
type QueueItem struct {
	JobID       string  `json:"job_id"`
	Title       string  `json:"job_title"`
	Company     string  `json:"company"`
	URL         string  `json:"job_url"`
	Status      string  `json:"status"`
	ResumeID    *string `json:"resume_id"`
	ResumeLabel *string `json:"resume_label"`
}

// QueueListResult is the keyset-pagination response shape for
// GET /api/apply-queue.
type QueueListResult struct {
	Items      []QueueItem `json:"items"`
	NextCursor string      `json:"next_cursor"`
}
