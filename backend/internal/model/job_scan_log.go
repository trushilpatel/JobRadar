package model

import "time"

type JobScanLog struct {
	ID          int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	CompanyID   string    `gorm:"not null"                 json:"company_id"`
	ScannedAt   time.Time `gorm:"not null;autoCreateTime"  json:"scanned_at"`
	JobsFetched int       `gorm:"not null;default:0"       json:"jobs_fetched"`
	JobsNew     int       `gorm:"not null;default:0"       json:"jobs_new"`
	Error       *string   `json:"error"`
	DurationMs  *int      `json:"duration_ms"`

	Company Company `gorm:"foreignKey:CompanyID" json:"company"`
}

func (JobScanLog) TableName() string {
	return "job_scan_log"
}
