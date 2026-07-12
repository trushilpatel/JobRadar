package dto

// CreateApplicationInput is the validated input for creating a job
// application entry. required:"false" on both fields matches the pre-Huma
// handler's plain json.Decode, which leaves an absent key at its Go zero
// value rather than erroring.
type CreateApplicationInput struct {
	Stage string `json:"stage" required:"false"`
	Note  string `json:"note" required:"false"`
}
