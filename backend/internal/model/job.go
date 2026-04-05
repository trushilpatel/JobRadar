package model

import (
	"time"

	"gorm.io/datatypes"
)

type Job struct {
	ID        string `gorm:"primaryKey"`
	JobPostID     string `gorm:"not null"`
	CompanyID string `gorm:"not null"`

	Title      string  `gorm:"not null"`
	Location   *string
	Remote     bool    `gorm:"not null;default:false"`
	URL        string  `gorm:"not null"`
	Department *string
	EmployType *string

	YearsExpMin   *int
	YearsExpMax   *int
	TechStack     datatypes.JSON
	SalaryRange   datatypes.JSON
	LLMSummary    *string
	OtherCriteria datatypes.JSON

	Status string `gorm:"not null;default:new"`
	Saved  bool   `gorm:"not null;default:false"`
	Notes  *string

	AddedAt time.Time `gorm:"not null;autoCreateTime"`

	Company Company `gorm:"foreignKey:CompanyID"`
}

func (Job) TableName() string {
	return "job"
}
