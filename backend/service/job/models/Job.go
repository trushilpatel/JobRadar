package jobmodel

import (
	"time"

	companymodel "github.com/trushilpatel/jobradar/service/company/models"
	"gorm.io/datatypes"
)

type Job struct {
	ID        string `gorm:"primaryKey"  json:"id"`
	JobPostID string `gorm:"not null"    json:"job_post_id"`
	CompanyID string `gorm:"not null"    json:"company_id"`

	Title      string  `gorm:"not null"               json:"title"`
	Location   *string `json:"location"`
	Country    *string `json:"country"`
	State      *string `json:"state"`
	Remote     bool    `gorm:"not null;default:false" json:"remote"`
	URL        string  `gorm:"not null"               json:"url"`
	Department *string `json:"department"`
	EmployType *string `json:"employ_type"`

	ExperienceYears *int           `json:"experience_years"`
	TechStack       datatypes.JSON `json:"tech_stack"`
	SalaryRange     datatypes.JSON `json:"salary_range"`
	LLMSummary      *string        `json:"llm_summary"`

	Sponsorship       bool           `gorm:"not null;default:false" json:"sponsorship"`
	RequiresCitizen   bool           `gorm:"not null;default:false" json:"requires_citizen"`
	SecurityClearance bool           `gorm:"not null;default:false" json:"security_clearance"`
	RoleBrief         *string        `json:"role_brief"`
	Responsibilities  datatypes.JSON `json:"responsibilities"`

	Status string  `gorm:"not null;default:'new'" json:"status"`
	Saved  bool    `gorm:"not null;default:false" json:"saved"`
	Notes  *string `json:"notes"`

	AddedAt time.Time `gorm:"not null;autoCreateTime" json:"added_at"`

	Company companymodel.Company `gorm:"foreignKey:CompanyID" json:"company"`
}

func (Job) TableName() string {
	return "job_feed"
}
