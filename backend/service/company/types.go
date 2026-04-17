package company

import (
	"context"
	"time"

	"github.com/trushilpatel/jobradar/model"
)

// ScanHistoryReader is the narrow interface the company service needs to populate
// per-company scan history in list responses. Implemented by scan.ScanLogStore.
type ScanHistoryReader interface {
	GetScanHistories(ctx context.Context, companyIDs []string) (map[string][]bool, error)
}

// Config holds per-service configuration for the company domain.
type Config struct {
	ScanIntervalMinutes int
}

// CompanyFilter holds optional filter criteria for listing companies.
type CompanyFilter struct {
	Active      *bool
	AdapterType string
	Search      string // ILIKE match on name
}

// CompanyListResult is returned by Service.List.
type CompanyListResult struct {
	Items       []CompanyView
	Total       int64 // total matching the current filter
	TotalActive int64 // total active companies (always unfiltered)
}

// CompanyView embeds a Company and adds computed fields.
type CompanyView struct {
	model.Company
	NextScanAt *time.Time `json:"next_scan_at"`
	// ScanHistory holds the last ≤10 scan outcomes ordered oldest→newest.
	// true=success, false=error. nil when history is unavailable.
	ScanHistory []bool `json:"scan_history,omitempty"`
}

// CreateCompanyInput is the validated input for creating a new company.
type CreateCompanyInput struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	URL         string `json:"url"`
	AdapterType string `json:"adapter_type"`
	Active      *bool  `json:"active"`
}

// UpdateCompanyInput is the validated input for updating a company.
// Nil fields are not updated (partial patch semantics).
type UpdateCompanyInput struct {
	Name        *string `json:"name"`
	URL         *string `json:"url"`
	AdapterType *string `json:"adapter_type"`
	Active      *bool   `json:"active"`
}
