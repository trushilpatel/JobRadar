package dto

// UpdateNoteInput is the validated input for updating an application note.
// required:"false" matches the pre-Huma handler's plain json.Decode, which
// leaves an absent key at its Go zero value.
type UpdateNoteInput struct {
	Note string `json:"note" required:"false"`
}
