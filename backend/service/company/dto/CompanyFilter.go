package dto

// CompanyFilter holds optional filter criteria for listing companies.
type CompanyFilter struct {
	Active         *bool
	AdapterTypes   []string // filter by adapter_type IN (...) — OR across values
	Search         string   // ILIKE match on name
	LastScanStatus string   // filter by last_scan_status (e.g. "error")
	SortBy         string   // "name" | "adapter_type" | "last_scan_at" — defaults to "name"
	SortDir        string   // "asc" | "desc" — defaults to "asc"
}
