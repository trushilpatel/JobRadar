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
	QueueStatus  *string                  `json:"queue_status,omitempty"`
	ScanHistory  []bool                   `json:"scan_history,omitempty"`
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

// CompanyScanSettingsListView is the admin list variant of CompanyScanSettingsView.
// It intentionally omits adapter_config to prevent bulk exposure of sensitive
// credentials in paginated list responses. Use CompanyScanSettingsView (via Get)
// to retrieve the full config for a single company.
type CompanyScanSettingsListView struct {
	CompanyID            string     `json:"company_id"`
	AdapterType          string     `json:"adapter_type"`
	ScanIntervalOverride *int       `json:"scan_interval_override,omitempty"`
	Active               bool       `json:"active"`
	LastScanAt           *time.Time `json:"last_scan_at,omitempty"`
	LastSuccessAt        *time.Time `json:"last_success_at,omitempty"`
	LastErrorAt          *time.Time `json:"last_error_at,omitempty"`
	LastErrorMessage     *string    `json:"last_error_message,omitempty"`
	ConsecutiveErrors    int        `json:"consecutive_errors"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

// CompanyAdminListView is the per-item shape in the admin list response.
// It uses CompanyScanSettingsListView (no adapter_config) rather than the
// full CompanyScanSettingsView returned by the detail endpoint.
type CompanyAdminListView struct {
	companymodel.Company
	ScanSettings *CompanyScanSettingsListView `json:"scan_settings,omitempty"`
	QueueStatus  *string                      `json:"queue_status,omitempty"`
	ScanHistory  []bool                       `json:"scan_history,omitempty"`
}

// ScanSettingsListViewFromModel projects a CompanyScanSettings model into the
// list-safe DTO view (omits adapter_config).
func ScanSettingsListViewFromModel(m *companymodel.CompanyScanSettings) *CompanyScanSettingsListView {
	if m == nil {
		return nil
	}
	return &CompanyScanSettingsListView{
		CompanyID:            m.CompanyID,
		AdapterType:          m.AdapterType,
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
