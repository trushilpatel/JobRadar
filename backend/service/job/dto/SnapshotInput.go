package dto

// SnapshotInput carries the data the enricher writes at pipeline time.
type SnapshotInput struct {
	ID                string
	CompanyID         string
	Country           string
	State             string
	Remote            bool
	Sponsorship       bool
	RequiresCitizen   bool
	SecurityClearance bool
	EmployType        string
	ExperienceYears   *int
	TechStack         []byte
	RoleCategory      string
}
