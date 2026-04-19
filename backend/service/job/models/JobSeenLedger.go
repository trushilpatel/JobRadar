package jobmodel

import (
	"time"

	companymodel "github.com/trushilpatel/jobradar/service/company/models"
)

type JobSeenLedger struct {
	JobID       string     `gorm:"primaryKey"`
	CompanyID   string     `gorm:"not null"`
	FirstSeenAt time.Time  `gorm:"not null;autoCreateTime"`
	Reason      string     `gorm:"not null"`
	RemovedAt   *time.Time `gorm:"column:removed_at"`

	Company companymodel.Company `gorm:"foreignKey:CompanyID"`
}

func (JobSeenLedger) TableName() string {
	return "job_pipeline_ledger"
}
