package dto

// ErrorRequest is the body of POST /node/error.
type ErrorRequest struct {
	AssignmentID string `json:"assignment_id"`
	CompanyID    string `json:"company_id"`
	AdapterType  string `json:"adapter_type"`
	ErrorMessage string `json:"error_message"`
	DurationMs   int64  `json:"duration_ms"`
}
