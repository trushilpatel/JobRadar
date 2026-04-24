package dto

// JobFilter holds all filter criteria for listing jobs.
// Every field is optional — zero value means "no filter on this field".
type JobFilter struct {
	Status           string            `json:"status"`    // "new" | "saved" | "applied" | "interviewing" | "offer"
	Companies        []string          `json:"companies"` // filter by company_id (exact match, OR across values)
	Remote           *bool             // remote = true/false
	OnSite           bool              // remote = false (on-site only)
	EmployTypes      []string          // employ_type IN (...) — OR across values
	Sponsorship      *bool             // sponsorship = true/false
	NoRequirements   bool              // requires_citizen = false AND security_clearance = false
	RequiresCitizen  *bool             // requires_citizen = true/false
	ExperienceRanges []ExperienceRange // numeric range bands — OR across values
	DaysAdded        *int              // added_at >= NOW() - N days
	SortBy           string            // "title" | "company" | "added_at" — defaults to "added_at"
	SortDir          string            // "asc" | "desc" — defaults to "desc"
}
