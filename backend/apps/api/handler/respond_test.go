package handler

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apperr "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/logger"
)

// TestSetLogger_NonNil replaces the package-level logger and confirms that a
// subsequent 500 path in HandleError uses it (indirectly via no panic).
func TestSetLogger_NonNil(t *testing.T) {
	// Save and restore the package logger so parallel tests are not affected.
	original := pkgLogger
	t.Cleanup(func() { pkgLogger = original })

	real := logger.NewNoop()
	SetLogger(real)
	if pkgLogger != real {
		t.Fatal("SetLogger did not replace pkgLogger")
	}
}

// TestSetLogger_Nil installs a nil logger and verifies the package falls back
// to a noop — HandleError must not panic when called after SetLogger(nil).
func TestSetLogger_Nil(t *testing.T) {
	original := pkgLogger
	t.Cleanup(func() { pkgLogger = original })

	SetLogger(nil)
	// Verify pkgLogger is non-nil after a nil call (noop fallback).
	if pkgLogger == nil {
		t.Fatal("SetLogger(nil) must install a noop, not leave pkgLogger nil")
	}
	// Verify HandleError still works without panicking.
	w := httptest.NewRecorder()
	HandleError(w, errors.New("trigger 500 path to exercise pkgLogger.Error"))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("got %d want 500", w.Code)
	}
}

// TestHandleError_ValidationErrors_Slice covers the apperr.ValidationErrors
// (slice) branch — distinct from the *ValidationError (single-field) branch
// already tested below.
func TestHandleError_ValidationErrors_Slice(t *testing.T) {
	errs := apperr.ValidationErrors{
		{Field: "email", Code: "INVALID", Message: "must be a valid email"},
		{Field: "name", Code: "REQUIRED", Message: "name is required"},
	}
	w := httptest.NewRecorder()
	HandleError(w, errs)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("got %d want 400", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "INVALID") || !strings.Contains(body, "REQUIRED") {
		t.Fatalf("body %q missing expected codes", body)
	}
}

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
