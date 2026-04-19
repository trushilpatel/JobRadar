package dto

// DismissInput is the input for dismissing a job.
type DismissInput struct {
	Reason string `json:"reason"`
	Note   string `json:"note"`
}
