package job

// ExperienceRange is a parsed experience band with a lower bound and optional upper bound.
type ExperienceRange struct {
	Min int  // inclusive lower bound (years)
	Max *int // inclusive upper bound; nil means unbounded
}

// JobFilter holds all filter criteria for listing jobs.
// Every field is optional — zero value means "no filter on this field".
type JobFilter struct {
	Status           string            // "new" | "saved" | "applied" | "interviewing" | "offer" — required
	Companies        []string          // filter by company_id (exact match, OR across values)
	Remote           *bool             // remote = true/false
	OnSite           bool              // remote = false (on-site only)
	EmployTypes      []string          // employ_type IN (...) — OR across values
	Sponsorship      *bool             // sponsorship = true/false
	NoRequirements   bool              // requires_citizen = false AND security_clearance = false
	RequiresCitizen  *bool             // requires_citizen = true/false
	ExperienceRanges []ExperienceRange // numeric range bands — OR across values
	DaysAdded        *int              // added_at >= NOW() - N days
}

// JobCounts holds the count of jobs per active status.
type JobCounts struct {
	New          int64 `json:"new"`
	Saved        int64 `json:"saved"`
	Applied      int64 `json:"applied"`
	Interviewing int64 `json:"interviewing"`
	Offer        int64 `json:"offer"`
}

// CountsResult is the store result for GetCounts.
type CountsResult struct {
	Status string
	Count  int64
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
