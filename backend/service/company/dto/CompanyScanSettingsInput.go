package dto

import "gorm.io/datatypes"

// CompanyScanSettingsInput is the validated payload for creating or updating
// a `company_scan_settings` row. Used by Create (alongside CompanyPublicInput)
// and by PATCH /admin/companies/{id}/scan-settings.
//
// All fields are optional pointers so partial-patch semantics work cleanly:
// a nil field is left untouched on update. On Create, the admin chain treats
// a fully-zero input as "no scan_settings row yet — admin will set it later".
type CompanyScanSettingsInput struct {
	AdapterType          *string         `json:"adapter_type"`
	AdapterConfig        *datatypes.JSON `json:"adapter_config"`
	ScanIntervalOverride *int            `json:"scan_interval_override"`
	Active               *bool           `json:"active"`
}

// IsEmpty reports whether every field of the input is nil. Used by Create
// to decide whether to insert a scan_settings row at all.
func (i CompanyScanSettingsInput) IsEmpty() bool {
	return i.AdapterType == nil &&
		i.AdapterConfig == nil &&
		i.ScanIntervalOverride == nil &&
		i.Active == nil
}
