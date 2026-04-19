package dto

// CreateCompanyInput is the validated input for creating a new company.
type CreateCompanyInput struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	URL         string `json:"url"`
	AdapterType string `json:"adapter_type"`
	Active      *bool  `json:"active"`
}
