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

// SetStatusInput is the body for PATCH /api/jobs/{id}/status.
type SetStatusInput struct {
	Status string `json:"status" required:"false"`
}

// SetNotesInput is the body for PATCH /api/jobs/{id}/notes.
type SetNotesInput struct {
	Notes string `json:"notes" required:"false"`
}
