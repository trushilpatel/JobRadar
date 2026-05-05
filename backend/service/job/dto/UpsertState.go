package dto

// UpsertState carries the field set the UserJobStateRepository.Upsert
// method writes. Status is required; the optional pointer fields allow
// callers to clear or set a single column without touching the others.
type UpsertState struct {
	Status          string
	Notes           *string
	DismissalReason *string
	DismissalNote   *string
}

// DismissInput is the request payload for dismissing a single job.
type DismissInput struct {
	Reason string `json:"reason"`
	Note   string `json:"note"`
}

// BulkDismissInput is the request payload for dismissing multiple jobs.
type BulkDismissInput struct {
	IDs    []string `json:"ids"`
	Reason string   `json:"reason"`
	Note   string   `json:"note"`
}

// SetStatusInput is the body for PATCH /api/jobs/{id}/status.
type SetStatusInput struct {
	Status string `json:"status"`
}

// SetNotesInput is the body for PATCH /api/jobs/{id}/notes.
type SetNotesInput struct {
	Notes string `json:"notes"`
}
