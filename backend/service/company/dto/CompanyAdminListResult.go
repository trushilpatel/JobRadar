package dto

// CompanyAdminListResult is returned by CompanyAdminService.List. Carries
// admin list views (public fields + scan_settings without adapter_config) plus
// aggregate counts useful for the admin dashboard header. Use the detail
// endpoint (Get) for the full CompanyAdminView that includes adapter_config.
type CompanyAdminListResult struct {
	Items       []CompanyAdminListView
	Total       int64   // total matching the current filter
	TotalActive int64   // total companies with scan_settings.active = true (always unfiltered)
	NextCursor  *string // nil when no next page exists
}
