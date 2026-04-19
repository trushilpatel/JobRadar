package handler

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apperr "github.com/trushilpatel/jobradar/common/errors"
)

func TestHandleError(t *testing.T) {
	tests := []struct {
		name           string
		err            error
		wantStatus     int
		wantCodeSubstr string
	}{
		{
			name:           "ValidationError returns 400",
			err:            &apperr.ValidationError{Field: "name", Code: "REQUIRED", Message: "name is required"},
			wantStatus:     http.StatusBadRequest,
			wantCodeSubstr: "REQUIRED",
		},
		{
			name:           "ServiceError NOT_FOUND returns 404",
			err:            apperr.ErrNotFound.WithMessage("company not found"),
			wantStatus:     http.StatusNotFound,
			wantCodeSubstr: "NOT_FOUND",
		},
		{
			name:           "ServiceError CONFLICT returns 409",
			err:            apperr.ErrConflict.WithMessage("already exists"),
			wantStatus:     http.StatusConflict,
			wantCodeSubstr: "CONFLICT",
		},
		{
			name:           "ServiceError BAD_REQUEST returns 400",
			err:            apperr.ErrBadRequest.WithMessage("bad input"),
			wantStatus:     http.StatusBadRequest,
			wantCodeSubstr: "BAD_REQUEST",
		},
		{
			name:           "Unknown error returns 500",
			err:            errors.New("something unexpected"),
			wantStatus:     http.StatusInternalServerError,
			wantCodeSubstr: "INTERNAL",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			HandleError(w, tc.err)

			if w.Code != tc.wantStatus {
				t.Errorf("got status %d, want %d", w.Code, tc.wantStatus)
			}
			body := w.Body.String()
			if !strings.Contains(body, tc.wantCodeSubstr) {
				t.Errorf("response body %q does not contain %q", body, tc.wantCodeSubstr)
			}
		})
	}
}

func TestRespond(t *testing.T) {
	w := httptest.NewRecorder()
	Respond(w, http.StatusOK, map[string]string{"id": "123"})

	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("got Content-Type %q, want application/json", ct)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"data"`) {
		t.Errorf("body %q does not contain 'data' envelope", body)
	}
}

func TestRespondPaged(t *testing.T) {
	w := httptest.NewRecorder()
	RespondPaged(w, http.StatusOK, []string{"a", "b"}, 10, 1, 20)

	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want 200", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"total"`) {
		t.Errorf("body %q does not contain 'total'", body)
	}
	if !strings.Contains(body, `"page"`) {
		t.Errorf("body %q does not contain 'page'", body)
	}
}
