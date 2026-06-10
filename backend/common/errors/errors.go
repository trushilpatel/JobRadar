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

// Is reports whether target is the same logical sentinel as e, comparing by
// Code. This lets callers use errors.Is(err, ErrLedgerRowMissing) even when the
// returned error was produced via WithMessage/WithExternal (which yield a new
// pointer with the same Code). Matching by Code is intentional: the Code is the
// stable machine-readable identity of the error; the messages are detail.
func (e *ServiceError) Is(target error) bool {
	t, ok := target.(*ServiceError)
	if !ok {
		return false
	}
	return e.Code == t.Code
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
	case "LEDGER_ROW_MISSING":
		return "resource not found"
	case "QUEUE_ROW_MISSING":
		return "resource not found"
	case "JOB_DUPLICATE":
		return "resource already exists"
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

	// ErrLedgerRowMissing is returned by JobSeenLedger UPDATE operations when no
	// ledger row matched (RowsAffected fell short of the expected count). It
	// signals the crawler→filter race (message consumed before the ledger INSERT
	// committed) or a row cleaned out by maintenance. Callers decide: the filter
	// Naks the source message (NATS redelivers, by which time the INSERT lands);
	// the analyzer logs+metrics and proceeds (a cleaned-out row is non-fatal at
	// that stage). Status 409 (conflict) — never serialised to a user-facing API.
	ErrLedgerRowMissing = &ServiceError{Code: "LEDGER_ROW_MISSING", Status: 409, InternalMessage: "ledger row missing"}

	// ErrQueueRowMissing is returned by ScanQueue UPDATE operations (Complete,
	// Remove, UpdateHeartbeat) when no queue row matched. The admin Remove path
	// maps it to HTTP 404 via HandleError (Status 404). The crawler treats a
	// missing-row heartbeat as a reassignment signal and aborts the scan.
	ErrQueueRowMissing = &ServiceError{Code: "QUEUE_ROW_MISSING", Status: 404, InternalMessage: "scan queue row missing"}

	// ErrJobAlreadyExists is returned by InsertJob when the ON CONFLICT DO NOTHING
	// clause matched an existing job_post_id (RowsAffected == 0). It is an
	// internal control-flow signal for idempotent inserts — the enricher treats
	// it as non-fatal — and is never serialised to a user-facing API. Status 200
	// reflects "no error from the client's perspective".
	ErrJobAlreadyExists = &ServiceError{Code: "JOB_DUPLICATE", Status: 200, InternalMessage: "job already exists for job_post_id"}

	// ErrNodeUnauthorized is returned by RequireNodeToken when the Bearer token
	// does not match any active crawler node (missing, malformed, or HMAC mismatch).
	ErrNodeUnauthorized = &ServiceError{Code: "NODE_UNAUTHORIZED", Status: 401, ExternalMessage: "invalid or missing node token"}

	// ErrNodeDisabled is returned by RequireNodeToken when the node's token is
	// valid but the node status is not 'active' (disabled or retired).
	ErrNodeDisabled = &ServiceError{Code: "NODE_DISABLED", Status: 403, ExternalMessage: "node is not active"}
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
