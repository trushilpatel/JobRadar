package model

import "time"

type SeenLedger struct {
	JobID       string    `gorm:"primaryKey"`
	CompanyID   string    `gorm:"not null"`
	FirstSeenAt time.Time `gorm:"not null;autoCreateTime"`
	Reason      string    `gorm:"not null"`

	Company Company `gorm:"foreignKey:CompanyID"`
}

func (SeenLedger) TableName() string {
	return "job_seen_ledger"
}
