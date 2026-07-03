package dto

// JobFilter holds all filter criteria for listing jobs.
// Every field is optional — zero value means "no filter on this field".
type JobFilter struct {
	Status           string            `json:"status"`    // "new" | "saved" | "applied" | "interviewing" | "offer"
	Statuses         []string          `json:"statuses"`  // combined multi-status view (e.g. the Tracking tab); when non-empty, GetJobsByStatus uses this instead of Status
	Companies        []string          `json:"companies"` // filter by company name (exact match, OR across values)
	Search           string            // ILIKE match on title, location, or company name
	Remote           *bool             // remote = true/false
	OnSite           bool              // remote = false (on-site only)
	Hybrid           bool              // employ_type ILIKE '%hybrid%'
	EmployTypes      []string          // employ_type IN (...) — OR across values
	Sponsorship      *bool             // sponsorship = true/false
	NoRequirements   bool              // requires_citizen = false AND security_clearance = false
	RequiresCitizen  *bool             // requires_citizen = true/false
	ExperienceRanges []ExperienceRange // numeric range bands — OR across values
	DaysAdded        *int              // added_at >= NOW() - N days
	SortBy           string            // "title" | "company" | "added_at" — defaults to "added_at"
	SortDir          string            // "asc" | "desc" — defaults to "desc"
}
