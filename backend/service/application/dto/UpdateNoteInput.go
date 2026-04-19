package dto

// UpdateNoteInput is the validated input for updating an application note.
type UpdateNoteInput struct {
	Note string `json:"note"`
}
