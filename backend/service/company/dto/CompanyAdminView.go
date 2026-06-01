package dto

import (
	"time"

	"gorm.io/datatypes"

	companymodel "github.com/trushilpatel/jobradar/service/company/models"
)

// CompanyAdminCommon holds the fields shared by both admin views (list +
// detail). Embedded in CompanyAdminView and CompanyAdminListView so the two
// shapes stay in lock-step except for their ScanSettings field type. Embedded
// fields promote flat in JSON, so the wire shape is identical to declaring
// each field inline. Exported so the services layer (and tests) can construct
// the views via the embedded literal.
type CompanyAdminCommon struct {
	companymodel.Company
	QueueStatus *string `json:"queue_status,omitempty"`
	ScanHistory []bool  `json:"scan_history,omitempty"`
}

// CompanyAdminView is the admin-facing view returned by GET /admin/companies/{id}.
// It carries the public fields plus the operational scan_settings row (with
// adapter_config) when one exists. ScanSettings is nil when the company has not
// yet been onboarded for scanning (post-cutover companies start without
// settings).
type CompanyAdminView struct {
	CompanyAdminCommon
	ScanSettings *CompanyScanSettingsView `json:"scan_settings,omitempty"`
}

// CompanyScanSettingsListView is the credential-free base scan-settings view.
// It intentionally omits adapter_config to prevent bulk exposure of sensitive
// credentials in paginated list responses. The detail view embeds this and
// adds adapter_config (see CompanyScanSettingsView).
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

// CompanyScanSettingsView is the detail scan-settings view. It embeds the
// credential-free base and adds only adapter_config — the one field the list
// view must never expose. Embedding keeps the two views from drifting: a new
// base field is automatically visible in both.
type CompanyScanSettingsView struct {
	CompanyScanSettingsListView
	AdapterConfig datatypes.JSON `json:"adapter_config,omitempty"`
}

// ScanSettingsListViewFromModel projects a CompanyScanSettings model into the
// list-safe DTO view (omits adapter_config). Returns nil when m is nil so
// callers can pass a LEFT-JOIN result straight through.
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

// ScanSettingsViewFromModel projects a CompanyScanSettings model into the full
// detail DTO view (includes adapter_config). Returns nil when m is nil. Built
// on ScanSettingsListViewFromModel so the shared field projection lives in one
// place.
func ScanSettingsViewFromModel(m *companymodel.CompanyScanSettings) *CompanyScanSettingsView {
	base := ScanSettingsListViewFromModel(m)
	if base == nil {
		return nil
	}
	return &CompanyScanSettingsView{
		CompanyScanSettingsListView: *base,
		AdapterConfig:               m.AdapterConfig,
	}
}

// CompanyAdminListView is the per-item shape in the admin list response. It
// uses CompanyScanSettingsListView (no adapter_config) rather than the full
// CompanyScanSettingsView returned by the detail endpoint.
type CompanyAdminListView struct {
	CompanyAdminCommon
	ScanSettings *CompanyScanSettingsListView `json:"scan_settings,omitempty"`
}
