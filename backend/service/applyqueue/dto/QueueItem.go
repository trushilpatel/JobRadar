// Package dto holds input/output shapes for the apply-queue service that
// are not GORM models.
package dto

import (
	"time"

	"gorm.io/datatypes"
)

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
//
// JobKeywords is the job's canonical (techtag.Clean-cleaned) job_tech_tags,
// attached on-read per page (see loadJobTechTags in
// ApplyQueueRepositoryService.go) — plain data, no scoring. The frontend
// computes any resume-match against it client-side (see
// docs/superpowers/specs/2026-07-05-resume-job-keyword-match-design.md);
// the backend no longer recommends a resume.
//
// Location/Remote/SalaryRange/AddedAt are a verbatim pass-through of
// jobmodel.Job's fields of the same name — same JSON tags, same Go types —
// so the apply-queue row can render the same summary info as the Jobs page
// and back a detail panel, unlike Title/URL above which deliberately diverge
// from dto.JobView's naming. Location and SalaryRange are nullable at the DB
// level (a job may have no parsed location or salary) and surface as nil /
// JSON null respectively rather than panicking or omitting the field.
type QueueItem struct {
	JobID       string         `json:"job_id"`
	Title       string         `json:"job_title"`
	Company     string         `json:"company"`
	URL         string         `json:"job_url"`
	Status      string         `json:"status"`
	ResumeID    *string        `json:"resume_id"`
	ResumeLabel *string        `json:"resume_label"`
	JobKeywords []string       `json:"job_keywords"`
	Location    *string        `json:"location"`
	Remote      bool           `json:"remote"`
	SalaryRange datatypes.JSON `json:"salary_range"`
	AddedAt     time.Time      `json:"added_at"`
}

// QueueListResult is the keyset-pagination response shape for
// GET /api/apply-queue.
type QueueListResult struct {
	Items      []QueueItem `json:"items"`
	NextCursor string      `json:"next_cursor"`
}
