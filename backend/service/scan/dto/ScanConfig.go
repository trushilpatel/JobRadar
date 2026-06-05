package dto

// EffectiveScanConfigDTO is the merged scan config for a single company:
// the adapter default (from the ats table) plus any company-level override.
type EffectiveScanConfigDTO struct {
	CompanyID                    string `json:"company_id"`
	AdapterType                  string `json:"adapter_type"`
	AdapterScanIntervalMinutes   int    `json:"adapter_scan_interval_minutes"`
	EffectiveScanIntervalMinutes int    `json:"effective_scan_interval_minutes"`
	ScanIntervalMinutesOverride  *int   `json:"scan_interval_minutes_override"`
	InterCompanyDelayS           int    `json:"inter_company_delay_s"`
}
