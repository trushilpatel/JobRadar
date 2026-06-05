// Package atsmodel holds the GORM struct for the ats table.
package atsmodel

import "time"

// ATS is the GORM model for the ats table. It mirrors every column in the
// table one-for-one so the repository layer can read and write rows without
// any casting.
type ATS struct {
	Name               string    `gorm:"column:name;primaryKey"`
	DisplayName        string    `gorm:"column:display_name"`
	BaseURL            string    `gorm:"column:base_url"`
	IsActive           bool      `gorm:"column:is_active"`
	IsAdminCreatable   bool      `gorm:"column:is_admin_creatable"`
	ScanIntervalMin    int       `gorm:"column:scan_interval_min"`
	InterCompanyDelayS int       `gorm:"column:inter_company_delay_s"`
	CreatedAt          time.Time `gorm:"column:created_at"`
	UpdatedAt          time.Time `gorm:"column:updated_at"`
}

// TableName tells GORM to use the "ats" table name for this struct.
func (ATS) TableName() string { return "ats" }
