package dto

// ErrorRequest is the body of POST /node/error. required:"false" on every
// field matches the pre-Huma handler's plain json.Decode — an absent field
// reaches ValidationService's existing non-empty check on AssignmentID
// rather than failing at the framework layer.
type ErrorRequest struct {
	AssignmentID string `json:"assignment_id" required:"false"`
	CompanyID    string `json:"company_id" required:"false"`
	AdapterType  string `json:"adapter_type" required:"false"`
	ErrorMessage string `json:"error_message" required:"false"`
	DurationMs   int64  `json:"duration_ms" required:"false"`
}
