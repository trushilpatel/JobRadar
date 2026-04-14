package job

// JobFilter holds optional filter criteria for listing jobs.
type JobFilter struct {
	Status  string // "new" | "saved" | "applied"
	Company string // company_id substring match
	Saved   *bool
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
