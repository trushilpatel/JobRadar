package model

import "time"

type JobApplication struct {
	ID          int64     `gorm:"primaryKey;autoIncrement"`
	JobResultID string    `gorm:"column:job_result_id;not null"`
	Stage       string    `gorm:"not null"`
	StageAt     time.Time `gorm:"not null;autoCreateTime"`
	Note        *string
}

func (JobApplication) TableName() string {
	return "job_application"
}
