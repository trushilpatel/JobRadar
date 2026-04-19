package domain

import (
	"errors"
	"fmt"

	"gorm.io/gorm"
)

// ValidationError is returned by validators for malformed or out-of-range input.
type ValidationError struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation error: %s — %s", e.Field, e.Message)
}

// ServiceError is a typed error that carries an HTTP status code and a short
// machine-readable code. Services return *ServiceError; handlers inspect the
// code to decide the HTTP response.
type ServiceError struct {
	Code    string
	Message string
	Status  int
}

func (e *ServiceError) Error() string { return e.Message }

// WithMessage returns a copy of e with a new human-readable message.
func (e *ServiceError) WithMessage(msg string) *ServiceError {
	return &ServiceError{Code: e.Code, Message: msg, Status: e.Status}
}

var (
	ErrNotFound   = &ServiceError{Code: "NOT_FOUND", Status: 404}
	ErrConflict   = &ServiceError{Code: "CONFLICT", Status: 409}
	ErrBadRequest = &ServiceError{Code: "BAD_REQUEST", Status: 400}
	ErrInternal   = &ServiceError{Code: "INTERNAL", Status: 500}
)

// IsNotFound reports whether err is a GORM record-not-found error.
func IsNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}
