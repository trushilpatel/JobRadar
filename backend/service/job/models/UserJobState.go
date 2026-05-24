package jobmodel

import "time"

// UserJobState records a single user's interaction with a single job. The
// composite primary key (UserID, JobID) is enforced by the user_job_state
// table; absence of a row means status="new" for that user-job pair.
//
// Status is nullable — NULL means "no workflow status". The orthogonal
// "saved" flag lives on SavedAt (saved_at IS NOT NULL means saved). The
// constants below mirror the workflow values allowed by the
// user_job_state.status CHECK constraint; StatusSaved is a filter-only
// sentinel — it is NEVER written to the status column.
type UserJobState struct {
	UserID string `gorm:"primaryKey;column:user_id" json:"user_id"`
	JobID  string `gorm:"primaryKey;column:job_id"  json:"job_id"`

	Status *string `gorm:"column:status" json:"status"`

	Notes           *string `gorm:"column:notes"             json:"notes"`
	DismissalReason *string `gorm:"column:dismissal_reason"  json:"dismissal_reason"`
	DismissalNote   *string `gorm:"column:dismissal_note"    json:"dismissal_note"`

	SavedAt     *time.Time `gorm:"column:saved_at"     json:"saved_at"`
	AppliedAt   *time.Time `gorm:"column:applied_at"   json:"applied_at"`
	DismissedAt *time.Time `gorm:"column:dismissed_at" json:"dismissed_at"`

	CreatedAt time.Time `gorm:"not null;autoCreateTime;column:created_at" json:"created_at"`
	UpdatedAt time.Time `gorm:"not null;autoUpdateTime;column:updated_at" json:"updated_at"`
}

// TableName pins UserJobState to the user_job_state table.
func (UserJobState) TableName() string {
	return "user_job_state"
}

// StatusOrEmpty returns the dereferenced status, or "" if NULL. Use this at
// every read site to avoid nil-deref panics when comparing the status
// column against workflow values.
func (s UserJobState) StatusOrEmpty() string {
	if s.Status == nil {
		return ""
	}
	return *s.Status
}

// Per-user job status values. The status column is constrained to one of
// the workflow values below (or NULL). StatusSaved is a FILTER-ONLY
// sentinel — it is never persisted to the status column; saved_at IS NOT
// NULL is the source of truth for "this job is saved".
const (
	StatusSaved        = "saved" // FILTER-ONLY — never stored in status column; saved_at IS NOT NULL is the source of truth
	StatusApplied      = "applied"
	StatusDismissed    = "dismissed"
	StatusInterviewing = "interviewing"
	StatusOffer        = "offer"
	StatusRejected     = "rejected"
)
