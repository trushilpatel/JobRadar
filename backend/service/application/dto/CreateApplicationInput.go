package dto

// CreateApplicationInput is the validated input for creating a job application entry.
type CreateApplicationInput struct {
	Stage string `json:"stage"`
	Note  string `json:"note"`
}
