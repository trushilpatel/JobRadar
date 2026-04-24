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
// machine-readable code. Services return *ServiceError; handlers call
// HandleError which renders only the external (client-facing) message — the
// internal message is preserved for server logs and never reflected into the
// response body.
type ServiceError struct {
	Code            string
	Status          int
	InternalMessage string
	ExternalMessage string
}

// Error returns the internal message for logging. External consumers must use
// PublicMessage() or HandleError — never err.Error() — so server-internal
// detail cannot leak.
func (e *ServiceError) Error() string {
	if e.InternalMessage != "" {
		return e.InternalMessage
	}
	return e.ExternalMessage
}

// PublicMessage returns the safe-to-return client-facing message for the
// error. Falls back to a generic code-derived message when no external
// message has been explicitly set, so internal detail never leaks by default.
func (e *ServiceError) PublicMessage() string {
	if e.ExternalMessage != "" {
		return e.ExternalMessage
	}
	return genericMessageFor(e.Code)
}

// WithMessage returns a copy of e with a new internal message. Descriptive
// internal detail belongs here; it is logged but never returned to clients.
func (e *ServiceError) WithMessage(msg string) *ServiceError {
	return &ServiceError{Code: e.Code, Status: e.Status, InternalMessage: msg, ExternalMessage: e.ExternalMessage}
}

// WithExternal returns a copy of e with an explicit client-facing message.
// Use this only when the specific text is safe to expose to end users; prefer
// letting the generic per-code message stand.
func (e *ServiceError) WithExternal(msg string) *ServiceError {
	return &ServiceError{Code: e.Code, Status: e.Status, InternalMessage: e.InternalMessage, ExternalMessage: msg}
}

func genericMessageFor(code string) string {
	switch code {
	case "NOT_FOUND":
		return "resource not found"
	case "CONFLICT":
		return "resource already exists"
	case "BAD_REQUEST":
		return "invalid request"
	case "INTERNAL":
		return "internal server error"
	case "LLM_VALIDATION":
		return "request validation failed"
	case "LLM_LOCAL_QUOTA", "LLM_PROVIDER_429":
		return "rate limited"
	case "LLM_EXHAUSTED":
		return "service temporarily unavailable"
	default:
		return "error"
	}
}

var (
	ErrNotFound   = &ServiceError{Code: "NOT_FOUND", Status: 404}
	ErrConflict   = &ServiceError{Code: "CONFLICT", Status: 409}
	ErrBadRequest = &ServiceError{Code: "BAD_REQUEST", Status: 400}
	ErrInternal   = &ServiceError{Code: "INTERNAL", Status: 500}

	ErrLLMValidation          = &ServiceError{Code: "LLM_VALIDATION", Status: 400, InternalMessage: "llm: validation failed"}
	ErrLLMLocalQuotaExceeded  = &ServiceError{Code: "LLM_LOCAL_QUOTA", Status: 429, InternalMessage: "llm: local quota exceeded"}
	ErrLLMProviderRateLimited = &ServiceError{Code: "LLM_PROVIDER_429", Status: 429, InternalMessage: "llm: provider rate limited"}
	ErrLLMAllExhausted        = &ServiceError{Code: "LLM_EXHAUSTED", Status: 503, InternalMessage: "llm: all slots exhausted"}
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

func (e *LLMAllExhaustedError) Error() string { return ErrLLMAllExhausted.InternalMessage }
func (e *LLMAllExhaustedError) Unwrap() error { return ErrLLMAllExhausted }

// IsNotFound reports whether err is a GORM record-not-found error.
func IsNotFound(err error) bool {
	return stderrors.Is(err, gorm.ErrRecordNotFound)
}
