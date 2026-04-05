package model

import (
	"time"

	"gorm.io/datatypes"
)

type Job struct {
	ID        string `gorm:"primaryKey"  json:"id"`
	JobPostID string `gorm:"not null"    json:"job_post_id"`
	CompanyID string `gorm:"not null"    json:"company_id"`

	Title      string  `gorm:"not null"            json:"title"`
	Location   *string `json:"location"`
	Remote     bool    `gorm:"not null;default:false" json:"remote"`
	URL        string  `gorm:"not null"            json:"url"`
	Department *string `json:"department"`
	EmployType *string `json:"employ_type"`

	YearsExpMin   *int           `json:"years_exp_min"`
	YearsExpMax   *int           `json:"years_exp_max"`
	TechStack     datatypes.JSON `json:"tech_stack"`
	SalaryRange   datatypes.JSON `json:"salary_range"`
	LLMSummary    *string        `json:"llm_summary"`
	OtherCriteria datatypes.JSON `json:"other_criteria"`

	Status string  `gorm:"not null;default:new" json:"status"`
	Saved  bool    `gorm:"not null;default:false" json:"saved"`
	Notes  *string `json:"notes"`

	AddedAt time.Time `gorm:"not null;autoCreateTime" json:"added_at"`

	Company Company `gorm:"foreignKey:CompanyID" json:"company"`
}

func (Job) TableName() string {
	return "job"
}
