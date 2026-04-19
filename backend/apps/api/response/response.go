package response

// Response wraps a single resource in a standard envelope.
type Response[T any] struct {
	Data T `json:"data"`
}

// PagedResponse wraps a paginated list of resources.
type PagedResponse[T any] struct {
	Data  []T `json:"data"`
	Total int `json:"total"`
	Page  int `json:"page"`
	Limit int `json:"limit"`
}

// ErrorDetail holds the error code and human-readable message sent to clients.
type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ErrorResponse is the envelope sent on error responses.
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

// ValidationFieldError is one field-level error in a validation failure response.
type ValidationFieldError struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ValidationErrorResponse is the envelope sent for validation failures (HTTP 400).
type ValidationErrorResponse struct {
	Errors []ValidationFieldError `json:"errors"`
}
