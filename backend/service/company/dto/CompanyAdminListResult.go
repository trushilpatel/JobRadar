package dto

// CompanyAdminListResult is returned by CompanyAdminService.List. Carries
// admin views (public fields + scan_settings) plus aggregate counts useful
// for the admin dashboard header.
type CompanyAdminListResult struct {
	Items       []CompanyAdminView
	Total       int64 // total matching the current filter
	TotalActive int64 // total companies with scan_settings.active = true (always unfiltered)
}
