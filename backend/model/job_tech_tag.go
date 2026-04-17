package model

import "time"

// JobTechTag is a flattened, normalized technology tag extracted from a job posting.
// One row per (job, tech) pair. Written by the analyzer after validation.
type JobTechTag struct {
	JobID     string    `gorm:"primaryKey"`
	Tech      string    `gorm:"primaryKey"`
	CompanyID string    `gorm:"not null"`
	TaggedAt  time.Time `gorm:"not null;autoCreateTime"`
}

func (JobTechTag) TableName() string {
	return "job_tech_tags"
}
