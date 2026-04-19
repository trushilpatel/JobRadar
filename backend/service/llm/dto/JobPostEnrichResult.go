package dto

// JobPostEnrichResult is the structured data the LLM extracts from a full job page.
type JobPostEnrichResult struct {
	JobID             string   `json:"-"`
	ExperienceYears   *int     `json:"experience_years"`
	TechStack         []string `json:"tech_stack"`
	SalaryMin         *int     `json:"salary_min"`
	SalaryMax         *int     `json:"salary_max"`
	SalaryCurrency    string   `json:"salary_currency"`
	Summary           string   `json:"summary"`
	Remote            bool     `json:"remote"`
	Country           string   `json:"country"`
	State             string   `json:"state"`
	Sponsorship       bool     `json:"sponsorship"`
	RequiresCitizen   bool     `json:"requires_citizen"`
	SecurityClearance bool     `json:"security_clearance"`
	RoleBrief         string   `json:"role_brief"`
	Responsibilities  []string `json:"responsibilities"`
	RoleCategory      string   `json:"role_category"`
	RoleMatch         bool     `json:"role_match"`
}
