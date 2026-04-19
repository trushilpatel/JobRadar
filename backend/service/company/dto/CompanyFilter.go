package dto

// CompanyFilter holds optional filter criteria for listing companies.
type CompanyFilter struct {
	Active      *bool
	AdapterType string
	Search      string // ILIKE match on name
}
