package job

// JobFilter holds all filter criteria for listing jobs.
// Every field is optional — zero value means "no filter on this field".
type JobFilter struct {
	Status           string   // "new" | "saved" | "applied" — required
	Companies        []string // filter by company_id (exact match, OR across values)
	Remote           *bool    // remote = true/false
	EmployTypes      []string // employ_type IN (...) — OR across values
	Sponsorship      *bool    // sponsorship = true/false
	NoRequirements   bool     // requires_citizen = false AND security_clearance = false
	RequiresCitizen  *bool    // requires_citizen = true/false
	ExperienceRanges []string // "0-2" | "3-5" | "5+" — OR across values
	DaysAdded        *int     // added_at >= NOW() - N days
}

// DismissInput is the input for dismissing a job.
type DismissInput struct {
	Reason string `json:"reason"`
	Note   string `json:"note"`
}

// SetStatusInput is the input for changing job status.
type SetStatusInput struct {
	Status string `json:"status"`
}

// SetNotesInput is the input for updating job notes.
type SetNotesInput struct {
	Notes string `json:"notes"`
}
