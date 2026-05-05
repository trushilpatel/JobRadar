package dto

import (
	"time"

	"gorm.io/datatypes"

	companymodel "github.com/trushilpatel/jobradar/service/company/models"
)

// CompanyAdminView is the admin-facing view returned by /admin/companies.
// It carries the public fields plus the operational scan_settings row when
// one exists. ScanSettings is nil when the company has not yet been onboarded
// for scanning (post-cutover companies start without settings).
type CompanyAdminView struct {
	companymodel.Company
	ScanSettings *CompanyScanSettingsView `json:"scan_settings,omitempty"`
}

// CompanyScanSettingsView mirrors CompanyScanSettings but is the boundary
// type used by services and handlers. Repos return the model; impl/admin
// layers convert. Keeping a separate DTO shields handlers from GORM
// internals (datatypes.JSON, etc.).
type CompanyScanSettingsView struct {
	CompanyID            string         `json:"company_id"`
	AdapterType          string         `json:"adapter_type"`
	AdapterConfig        datatypes.JSON `json:"adapter_config,omitempty"`
	ScanIntervalOverride *int           `json:"scan_interval_override,omitempty"`
	Active               bool           `json:"active"`
	LastScanAt           *time.Time     `json:"last_scan_at,omitempty"`
	LastSuccessAt        *time.Time     `json:"last_success_at,omitempty"`
	LastErrorAt          *time.Time     `json:"last_error_at,omitempty"`
	LastErrorMessage     *string        `json:"last_error_message,omitempty"`
	ConsecutiveErrors    int            `json:"consecutive_errors"`
	CreatedAt            time.Time      `json:"created_at"`
	UpdatedAt            time.Time      `json:"updated_at"`
}

// ScanSettingsViewFromModel projects a CompanyScanSettings model into the
// DTO view. Returns nil when m is nil so callers can pass a LEFT-JOIN
// result straight through.
func ScanSettingsViewFromModel(m *companymodel.CompanyScanSettings) *CompanyScanSettingsView {
	if m == nil {
		return nil
	}
	return &CompanyScanSettingsView{
		CompanyID:            m.CompanyID,
		AdapterType:          m.AdapterType,
		AdapterConfig:        m.AdapterConfig,
		ScanIntervalOverride: m.ScanIntervalOverride,
		Active:               m.Active,
		LastScanAt:           m.LastScanAt,
		LastSuccessAt:        m.LastSuccessAt,
		LastErrorAt:          m.LastErrorAt,
		LastErrorMessage:     m.LastErrorMessage,
		ConsecutiveErrors:    m.ConsecutiveErrors,
		CreatedAt:            m.CreatedAt,
		UpdatedAt:            m.UpdatedAt,
	}
}
