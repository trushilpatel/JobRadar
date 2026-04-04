package model

import "time"

type JobScanLog struct {
	ID          int64     `gorm:"primaryKey;autoIncrement"`
	CompanyID   string    `gorm:"not null"`
	ScannedAt   time.Time `gorm:"not null;autoCreateTime"`
	JobsFetched int       `gorm:"not null;default:0"`
	JobsNew     int       `gorm:"not null;default:0"`
	Error       *string
	DurationMs  *int

	Company Company `gorm:"foreignKey:CompanyID"`
}

func (JobScanLog) TableName() string {
	return "job_scan_log"
}
