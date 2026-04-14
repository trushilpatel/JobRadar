package response

import "fmt"

// ValidationError is returned by validators for invalid user input.
type ValidationError struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation error: %s — %s", e.Field, e.Message)
}

// ServiceError is returned by services when a business rule fails.
type ServiceError struct {
	Code    string
	Message string
	Status  int
}

func (e *ServiceError) Error() string { return e.Message }

// WithMessage returns a new ServiceError with the given message, preserving Code and Status.
func (e *ServiceError) WithMessage(msg string) *ServiceError {
	return &ServiceError{Code: e.Code, Message: msg, Status: e.Status}
}

// Sentinel service errors. Use WithMessage to attach a human-readable description.
var (
	ErrNotFound   = &ServiceError{Code: "NOT_FOUND", Status: 404}
	ErrConflict   = &ServiceError{Code: "CONFLICT", Status: 409}
	ErrBadRequest = &ServiceError{Code: "BAD_REQUEST", Status: 400}
	ErrInternal   = &ServiceError{Code: "INTERNAL", Status: 500}
)
