package errors

import (
	stderrors "errors"

	"fmt"
	"strings"
	"time"

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

// ValidationErrors is returned by validators that collect all failures instead of stopping at the first.
type ValidationErrors []ValidationError

func (e ValidationErrors) Error() string {
	msgs := make([]string, len(e))
	for i, v := range e {
		msgs[i] = fmt.Sprintf("%s: %s", v.Field, v.Message)
	}
	return strings.Join(msgs, "; ")
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

	ErrLLMValidation          = &ServiceError{Code: "LLM_VALIDATION", Status: 400, Message: "llm: validation failed"}
	ErrLLMLocalQuotaExceeded  = &ServiceError{Code: "LLM_LOCAL_QUOTA", Status: 429, Message: "llm: local quota exceeded"}
	ErrLLMProviderRateLimited = &ServiceError{Code: "LLM_PROVIDER_429", Status: 429, Message: "llm: provider rate limited"}
	ErrLLMAllExhausted        = &ServiceError{Code: "LLM_EXHAUSTED", Status: 503, Message: "llm: all slots exhausted"}
)

// LLMSlotStatus is the public shape of slot state at exhaustion time.
type LLMSlotStatus struct {
	SlotID        string
	Model         string
	CooldownUntil time.Time
	FailureCount  int
	LastDenial    string // "rpm" | "rpd" | "tpm" | "cooldown" | "provider_429"
}

// LLMAllExhaustedError carries per-slot exhaustion detail. Unwraps to ErrLLMAllExhausted.
type LLMAllExhaustedError struct {
	Slots      []LLMSlotStatus
	RetryAfter time.Time
}

func (e *LLMAllExhaustedError) Error() string { return ErrLLMAllExhausted.Message }
func (e *LLMAllExhaustedError) Unwrap() error { return ErrLLMAllExhausted }

// IsNotFound reports whether err is a GORM record-not-found error.
func IsNotFound(err error) bool {
	return stderrors.Is(err, gorm.ErrRecordNotFound)
}
