package jobmodel

import "time"

// UserJobCounters is the materialized per-user count of jobs in each bucket.
// One row per user. The application layer updates this row atomically with
// every user_job_state mutation so reads can hit a single PK lookup instead
// of a GROUP BY scan.
//
// The "saved" count is orthogonal — a saved job is one with saved_at IS NOT
// NULL on user_job_state, independent of any workflow status the row may
// also carry.
type UserJobCounters struct {
	UserID            string    `gorm:"primaryKey;column:user_id" json:"user_id"`
	SavedCount        int64     `gorm:"not null;column:saved_count"        json:"saved_count"`
	AppliedCount      int64     `gorm:"not null;column:applied_count"      json:"applied_count"`
	InterviewingCount int64     `gorm:"not null;column:interviewing_count" json:"interviewing_count"`
	OfferCount        int64     `gorm:"not null;column:offer_count"        json:"offer_count"`
	RejectedCount     int64     `gorm:"not null;column:rejected_count"     json:"rejected_count"`
	DismissedCount    int64     `gorm:"not null;column:dismissed_count"    json:"dismissed_count"`
	UpdatedAt         time.Time `gorm:"not null;autoUpdateTime;column:updated_at" json:"updated_at"`
}

// TableName pins UserJobCounters to the user_job_counters table.
func (UserJobCounters) TableName() string {
	return "user_job_counters"
}
