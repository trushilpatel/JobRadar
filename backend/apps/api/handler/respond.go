package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/trushilpatel/jobradar/apps/api/response"
	apperr "github.com/trushilpatel/jobradar/common/errors"
)

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
		RespondError(w, sErr.Status, sErr.Code, sErr.Message)
		return
	}
	RespondError(w, http.StatusInternalServerError, "INTERNAL", "unexpected error")
}
