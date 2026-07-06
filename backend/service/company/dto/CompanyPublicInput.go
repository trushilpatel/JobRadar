package dto

// CompanyPublicInput is the validated public-field payload for create and
// update of the `companies` row. Used by both POST /admin/companies (paired
// with CompanyScanSettingsInput in Create) and PATCH /admin/companies/{id}
// (UpdatePublic). Pointer fields are partial-patch optional on update;
// required fields (ID/Name/Slug) are validated as non-empty on create.
// required:"false" on every field matches the pre-Huma handler's plain
// json.Decode: Create's required-field enforcement and UpdatePublic's
// at-least-one-field enforcement both happen downstream in
// AdminValidationService, never at the framework layer — an absent key
// must reach validation as a zero value, not fail Huma's own binding.
type CompanyPublicInput struct {
	ID         string  `json:"id" required:"false"`
	Name       string  `json:"name" required:"false"`
	Slug       string  `json:"slug" required:"false"`
	Website    *string `json:"website" required:"false"`
	CareersURL *string `json:"careers_url" required:"false"`
	LogoURL    *string `json:"logo_url" required:"false"`
}
