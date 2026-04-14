package application

// CreateApplicationInput is the validated input for creating a job application entry.
type CreateApplicationInput struct {
	Stage string `json:"stage"`
	Note  string `json:"note"`
}

// UpdateStageInput is the validated input for updating an application stage.
type UpdateStageInput struct {
	Stage string `json:"stage"`
}

// UpdateNoteInput is the validated input for updating an application note.
type UpdateNoteInput struct {
	Note string `json:"note"`
}
