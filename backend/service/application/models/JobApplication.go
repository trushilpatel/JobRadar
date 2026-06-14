package appmodel

import "time"

// JobApplication is one row in the job_application_tracker table.
//
// Phase 2: per-user. UserID is required (NOT NULL FK to users.id, ON DELETE
// CASCADE). The composite index (user_id, job_result_id) backs the per-user
// list-by-job query path. Old rows were wiped during the Phase 2 cutover so
// the column comes in NOT NULL with no backfill.
type JobApplication struct {
	ID          int64     `gorm:"primaryKey;autoIncrement"                                json:"id"`
	UserID      string    `gorm:"type:text;not null;index;index:idx_user_job,priority:1"  json:"user_id"`
	JobResultID string    `gorm:"column:job_result_id;not null;index:idx_user_job,priority:2" json:"job_result_id"`
	Stage       string    `gorm:"not null"                                                json:"stage"`
	StageAt     time.Time `gorm:"not null;autoCreateTime"                                 json:"stage_at"`
	Note        *string   `json:"note"`
}

func (JobApplication) TableName() string {
	return "job_application_tracker"
}
