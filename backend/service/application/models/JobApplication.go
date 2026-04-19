package appmodel

import "time"

type JobApplication struct {
	ID          int64     `gorm:"primaryKey;autoIncrement"      json:"id"`
	JobResultID string    `gorm:"column:job_result_id;not null" json:"job_result_id"`
	Stage       string    `gorm:"not null"                      json:"stage"`
	StageAt     time.Time `gorm:"not null;autoCreateTime"       json:"stage_at"`
	Note        *string   `json:"note"`
}

func (JobApplication) TableName() string {
	return "job_application_tracker"
}
