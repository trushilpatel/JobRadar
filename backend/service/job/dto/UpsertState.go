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

// DismissInput is the request payload for dismissing a single job. Both
// fields are optional in the wire format — an absent or empty "reason"
// defaults at the handler (see JobRestService.dismiss); `required:"false"`
// tells Huma's schema validation not to reject a body that omits them,
// matching the pre-Huma json.Decode-into-zero-value behavior exactly.
type DismissInput struct {
	Reason string `json:"reason" required:"false"`
	Note   string `json:"note" required:"false"`
}

// BulkDismissInput is the request payload for dismissing multiple jobs. All
// fields optional in the wire format — see DismissInput's comment on why.
type BulkDismissInput struct {
	IDs    []string `json:"ids" required:"false"`
	Reason string   `json:"reason" required:"false"`
	Note   string   `json:"note" required:"false"`
}

// SetStatusInput is the body for PATCH /api/jobs/{id}/status.
type SetStatusInput struct {
	Status string `json:"status" required:"false"`
}

// SetNotesInput is the body for PATCH /api/jobs/{id}/notes.
type SetNotesInput struct {
	Notes string `json:"notes" required:"false"`
}
