package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/trushilpatel/jobradar/apps/api/response"
	apperr "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/logger"
)

// pkgLogger is the logger used by HandleError when an unmapped error falls
// through to the 500 path. Defaults to a noop so test binaries that never
// call SetLogger stay silent; main.go installs the process logger at boot.
var pkgLogger logger.Logger = logger.NewNoop()

// SetLogger installs the logger used by HandleError for unmapped 500s. Call
// once during process init from main.go before any request is served. Safe
// to omit — the default noop logger silences output, which is the right
// behaviour for unit tests.
func SetLogger(l logger.Logger) {
	if l == nil {
		pkgLogger = logger.NewNoop()
		return
	}
	pkgLogger = l
}

// Respond writes a JSON envelope response with a typed data payload.
func Respond[T any](w http.ResponseWriter, status int, data T) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(response.Response[T]{Data: data}) //nolint:errcheck
}

// RespondPaged writes a JSON paged envelope response.
func RespondPaged[T any](w http.ResponseWriter, status int, items []T, total, page, limit int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(response.PagedResponse[T]{ //nolint:errcheck
		Data:  items,
		Total: total,
		Page:  page,
		Limit: limit,
	})
}

// RespondError writes a JSON error envelope.
func RespondError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(response.ErrorResponse{ //nolint:errcheck
		Error: response.ErrorDetail{Code: code, Message: message},
	})
}

// RespondValidationErrors writes { "errors": [...] } for validation failures.
func RespondValidationErrors(w http.ResponseWriter, errs apperr.ValidationErrors) {
	fields := make([]response.ValidationFieldError, len(errs))
	for i, e := range errs {
		fields[i] = response.ValidationFieldError{Field: e.Field, Code: e.Code, Message: e.Message}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(response.ValidationErrorResponse{Errors: fields}) //nolint:errcheck
}

// RespondNoContent writes a 204 No Content response with no body.
func RespondNoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}

// DecodeBody decodes r.Body as JSON into T. On failure (malformed JSON, an
// empty body, or a body that doesn't match T's shape) it writes a 400
// BAD_REQUEST response itself and returns the zero value with ok=false —
// callers just check ok and return:
//
//	input, ok := handler.DecodeBody[dto.CreateApplicationInput](w, r)
//	if !ok {
//		return
//	}
func DecodeBody[T any](w http.ResponseWriter, r *http.Request) (_ T, ok bool) {
	var body T
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		RespondError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return body, false
	}
	return body, true
}

// HandleError inspects err and writes the appropriate error response.
// ValidationErrors → 400 with field list, *ValidationError → 400 (single field),
// ServiceError → its Status, anything else → 500.
func HandleError(w http.ResponseWriter, err error) {
	var vErrs apperr.ValidationErrors
	if errors.As(err, &vErrs) {
		RespondValidationErrors(w, vErrs)
		return
	}
	var vErr *apperr.ValidationError
	if errors.As(err, &vErr) {
		RespondValidationErrors(w, apperr.ValidationErrors{*vErr})
		return
	}
	var sErr *apperr.ServiceError
	if errors.As(err, &sErr) {
		// 5xx ServiceErrors carry a deliberately generic PublicMessage() — the
		// InternalMessage set via .WithMessage() is the only record of what
		// actually failed, so it must be logged here or it's gone forever.
		// 4xx (NOT_FOUND, CONFLICT, BAD_REQUEST, ...) are routine and already
		// visible in the access log; logging those at Error level would just
		// be noise.
		if sErr.Status >= http.StatusInternalServerError {
			pkgLogger.Error("handler: service error returned 5xx", sErr)
		}
		RespondError(w, sErr.Status, sErr.Code, sErr.PublicMessage())
		return
	}
	// Unmapped error → generic 500. Log the underlying cause so the next
	// mystery 500 in production is one grep away, not a code reading session.
	// The public response stays opaque ("internal server error") — the log
	// line is the operator-facing diagnostic.
	pkgLogger.Error("handler: unmapped error returned 500", err)
	RespondError(w, http.StatusInternalServerError, "INTERNAL", "internal server error")
}
