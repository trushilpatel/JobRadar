package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/trushilpatel/jobradar/apps/api/response"
	"github.com/trushilpatel/jobradar/common/domain"
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

// HandleError inspects err and writes the appropriate error response.
// ValidationError → 400, ServiceError → its Status, anything else → 500.
func HandleError(w http.ResponseWriter, err error) {
	var vErr *domain.ValidationError
	if errors.As(err, &vErr) {
		RespondError(w, http.StatusBadRequest, vErr.Code, vErr.Message)
		return
	}
	var sErr *domain.ServiceError
	if errors.As(err, &sErr) {
		RespondError(w, sErr.Status, sErr.Code, sErr.Message)
		return
	}
	RespondError(w, http.StatusInternalServerError, "INTERNAL", "unexpected error")
}
