package dto

// UpdateCompanyInput is the validated input for updating a company.
// Nil fields are not updated (partial patch semantics).
type UpdateCompanyInput struct {
	Name        *string `json:"name"`
	URL         *string `json:"url"`
	AdapterType *string `json:"adapter_type"`
	Active      *bool   `json:"active"`
}
