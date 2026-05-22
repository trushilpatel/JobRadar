package dto

// CompanyFilter holds optional filter criteria for listing companies. Used
// for both the public list endpoint (operational fields ignored) and the
// admin list endpoint (operational fields honoured against scan_settings).
type CompanyFilter struct {
	Search       string   // ILIKE match on name (public + admin)
	AdapterTypes []string // admin-only: scan_settings.adapter_type IN (...)
	Active       *bool    // admin-only: scan_settings.active = ?
	HasSettings  *bool    // admin-only: filter by presence/absence of scan_settings row
	SortBy       string   // "name" — defaults to "name"
	SortDir      string   // "asc" | "desc" — defaults to "asc"
	ScanStatus   string   // "ok" | "error" | "queued" | "running"; empty = all
	Cursor       string   // opaque keyset cursor; empty = first page
}
