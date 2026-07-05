// Package dto holds input/output shapes for the apply-queue service that
// are not GORM models.
package dto

// QueueItem is one row in the apply queue — a job the user is actively
// engaging with (saved and/or advanced to applied/interviewing), plus any
// resume currently attached to it.
//
// Status is derived, never a raw passthrough of user_job_state.status:
//   - NULL status (only saved_at set) renders as "saved" (jobcommon.JobStatusSaved).
//   - Any other status (applied, interviewing, and — reachable only via the
//     FilterSaved view — dismissed/offer/rejected) renders as-is.
type QueueItem struct {
	JobID       string  `json:"job_id"`
	Title       string  `json:"title"`
	Company     string  `json:"company"`
	URL         string  `json:"url"`
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
