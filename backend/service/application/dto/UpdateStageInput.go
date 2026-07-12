package dto

// UpdateStageInput is the validated input for updating an application
// stage. required:"false" matches the pre-Huma handler's plain
// json.Decode, which leaves an absent key at its Go zero value.
type UpdateStageInput struct {
	Stage string `json:"stage" required:"false"`
}
