package companymodel

import (
	"time"

	"gorm.io/datatypes"
)

// CompanyScanSettings is the admin-only operational row backed by the
// `company_scan_settings` table. Workers read this row to decide what
// adapter to invoke and how often; admins write to it via the admin chain.
//
// CompanyID is both PK and FK (1:1 with `companies`) — the row is created
// when an admin onboards a company for scanning, deleted on company delete
// via ON DELETE CASCADE.
type CompanyScanSettings struct {
	CompanyID            string         `gorm:"primaryKey;column:company_id" json:"company_id"`
	AdapterType          string         `gorm:"not null"                     json:"adapter_type"`
	AdapterConfig        datatypes.JSON `gorm:"column:adapter_config"        json:"adapter_config,omitempty"`
	ScanIntervalOverride *int           `gorm:"column:scan_interval_override" json:"scan_interval_override,omitempty"`
	Active               bool           `gorm:"not null;default:true"        json:"active"`
	LastScanAt           *time.Time     `gorm:"column:last_scan_at"          json:"last_scan_at,omitempty"`
	LastSuccessAt        *time.Time     `gorm:"column:last_success_at"       json:"last_success_at,omitempty"`
	LastErrorAt          *time.Time     `gorm:"column:last_error_at"         json:"last_error_at,omitempty"`
	LastErrorMessage     *string        `gorm:"column:last_error_message"    json:"last_error_message,omitempty"`
	ConsecutiveErrors    int            `gorm:"not null;default:0"           json:"consecutive_errors"`
	CreatedAt            time.Time      `gorm:"not null;autoCreateTime"      json:"created_at"`
	UpdatedAt            time.Time      `gorm:"not null;autoUpdateTime"      json:"updated_at"`
}

// TableName pins the GORM table name.
func (CompanyScanSettings) TableName() string { return "company_scan_settings" }
