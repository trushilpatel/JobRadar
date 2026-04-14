package company

import (
	"time"

	"github.com/trushilpatel/jobradar/model"
)

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

// CompanyView embeds a Company and adds the computed next_scan_at field.
type CompanyView struct {
	model.Company
	NextScanAt *time.Time `json:"next_scan_at"`
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
