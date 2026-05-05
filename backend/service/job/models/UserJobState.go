package jobmodel

import "time"

// UserJobState records a single user's interaction with a single job. The
// composite primary key (UserID, JobID) is enforced by the user_job_state
// table; absence of a row means status="new" for that user-job pair.
//
// Status is constrained at the DB layer to one of the six stored values below —
// the constants mirror the user_job_state.status CHECK constraint.
type UserJobState struct {
	UserID string `gorm:"primaryKey;column:user_id" json:"user_id"`
	JobID  string `gorm:"primaryKey;column:job_id"  json:"job_id"`

	Status string `gorm:"not null;column:status" json:"status"`

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

// Per-user job status values — mirror the user_job_state.status CHECK
// constraint. Absence of a row implies "new"; "new" is intentionally NOT a
// valid stored status.
const (
	StatusSaved        = "saved"
	StatusApplied      = "applied"
	StatusDismissed    = "dismissed"
	StatusInterviewing = "interviewing"
	StatusOffer        = "offer"
	StatusRejected     = "rejected"
)
