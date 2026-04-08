package model

import "time"

type JobSnapshot struct {
	ID        string `gorm:"primaryKey" json:"id"`
	CompanyID string `gorm:"not null"   json:"company_id"`

	SnapshottedAt time.Time `gorm:"not null;type:date;default:CURRENT_DATE" json:"snapshotted_at"`

	// Structured location
	Country *string `json:"country"`
	State   *string `json:"state"`
	Remote  bool    `gorm:"not null;default:false" json:"remote"`

	// Visa / clearance
	Sponsorship       bool `gorm:"not null;default:false" json:"sponsorship"`
	RequiresCitizen   bool `gorm:"not null;default:false" json:"requires_citizen"`
	SecurityClearance bool `gorm:"not null;default:false" json:"security_clearance"`

	// Role shape
	EmployType      *string `json:"employ_type"`
	ExperienceYears *int    `json:"experience_years"`
	TechStack       []byte  `gorm:"type:jsonb"            json:"tech_stack"`
	RoleCategory    *string `json:"role_category"`

	// User outcome (backfilled when user acts)
	UserAction *string    `json:"user_action"`
	ActionedAt *time.Time `json:"actioned_at"`

	Company Company `gorm:"foreignKey:CompanyID" json:"-"`
}

func (JobSnapshot) TableName() string {
	return "job_analytics"
}
