package dto

import (
	"time"

	companymodel "github.com/trushilpatel/jobradar/service/company/models"
)

// CompanyView embeds a Company and adds computed fields.
type CompanyView struct {
	companymodel.Company
	NextScanAt *time.Time `json:"next_scan_at"`
	// ScanHistory holds the last ≤10 scan outcomes ordered oldest→newest.
	// true=success, false=error. nil when history is unavailable.
	ScanHistory []bool `json:"scan_history,omitempty"`
}
