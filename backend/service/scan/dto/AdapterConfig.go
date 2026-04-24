package dto

import "time"

// AdapterConfigDTO is the response shape for an adapter_configs row.
type AdapterConfigDTO struct {
	AdapterType         string     `json:"adapter_type"`
	ScanIntervalMinutes int        `json:"scan_interval_minutes"`
	InterCompanyDelayS  int        `json:"inter_company_delay_s"`
	LastScheduledAt     *time.Time `json:"last_scheduled_at"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

// UpdateAdapterConfigInput is the PATCH body for updating an adapter config.
// Only provided fields are applied (all fields are optional).
type UpdateAdapterConfigInput struct {
	ScanIntervalMinutes *int `json:"scan_interval_minutes"`
	InterCompanyDelayS  *int `json:"inter_company_delay_s"`
}

// EffectiveScanConfigDTO is the merged config for a single company:
// the adapter default plus any company-level override.
type EffectiveScanConfigDTO struct {
	CompanyID                    string `json:"company_id"`
	AdapterType                  string `json:"adapter_type"`
	AdapterScanIntervalMinutes   int    `json:"adapter_scan_interval_minutes"`
	EffectiveScanIntervalMinutes int    `json:"effective_scan_interval_minutes"`
	ScanIntervalMinutesOverride  *int   `json:"scan_interval_minutes_override"`
	InterCompanyDelayS           int    `json:"inter_company_delay_s"`
}
