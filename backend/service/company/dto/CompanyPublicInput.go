package dto

// CompanyPublicInput is the validated public-field payload for create and
// update of the `companies` row. Used by both POST /admin/companies (paired
// with CompanyScanSettingsInput in Create) and PATCH /admin/companies/{id}
// (UpdatePublic). Pointer fields are partial-patch optional on update;
// required fields (ID/Name/Slug) are validated as non-empty on create.
type CompanyPublicInput struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	Slug       string  `json:"slug"`
	Website    *string `json:"website"`
	CareersURL *string `json:"careers_url"`
	LogoURL    *string `json:"logo_url"`
}
