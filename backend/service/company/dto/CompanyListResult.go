package dto

// CompanyListResult is returned by Service.List.
type CompanyListResult struct {
	Items       []CompanyView
	Total       int64 // total matching the current filter
	TotalActive int64 // total active companies (always unfiltered)
}
