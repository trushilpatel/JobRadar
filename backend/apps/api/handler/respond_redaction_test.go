package handler

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	apperr "github.com/trushilpatel/jobradar/common/errors"
)

// TestHandleError_InternalMessageRedacted asserts that the descriptive
// internal message attached via WithMessage is never reflected into the
// response body — only the safe external message (or the generic per-code
// message when none set) is returned to clients.
func TestHandleError_InternalMessageRedacted(t *testing.T) {
	const internal = "db connection refused on primary replica"
	err := apperr.ErrInternal.WithMessage(internal)

	w := httptest.NewRecorder()
	HandleError(w, err)

	body := w.Body.String()
	if strings.Contains(body, internal) {
		t.Fatalf("internal message leaked into response body: %s", body)
	}
	if strings.Contains(body, "replica") || strings.Contains(body, "db connection") {
		t.Fatalf("internal detail leaked into body: %s", body)
	}

	var decoded struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("unmarshal: %v; body=%s", err, body)
	}
	if decoded.Error.Code != "INTERNAL" {
		t.Fatalf("code=%q want INTERNAL", decoded.Error.Code)
	}
	if decoded.Error.Message == "" {
		t.Fatal("external message must not be empty")
	}
	if decoded.Error.Message == internal {
		t.Fatalf("external message equals internal (%q) — redaction failed", internal)
	}
}

// TestHandleError_ExternalMessageReturnedWhenSet asserts that when the
// service has explicitly set an external message via WithExternal, that
// message (and only that message) is returned to clients.
func TestHandleError_ExternalMessageReturnedWhenSet(t *testing.T) {
	const internal = "stripe api timed out after 30s"
	const external = "payment service unavailable"
	err := apperr.ErrInternal.WithMessage(internal).WithExternal(external)

	w := httptest.NewRecorder()
	HandleError(w, err)

	body := w.Body.String()
	if strings.Contains(body, internal) {
		t.Fatalf("internal message leaked: %s", body)
	}
	if !strings.Contains(body, external) {
		t.Fatalf("external message missing from body: %s", body)
	}
}

// TestServiceError_PublicMessage_FallsBackToGeneric ensures a ServiceError
// with no explicit external message yields a safe generic string — never an
// empty message that would surface as an empty body key.
func TestServiceError_PublicMessage_FallsBackToGeneric(t *testing.T) {
	e := apperr.ErrNotFound.WithMessage("row not found in companies table")
	msg := e.PublicMessage()
	if msg == "" {
		t.Fatal("PublicMessage() returned empty string")
	}
	if strings.Contains(msg, "companies table") {
		t.Fatalf("generic message leaked internal detail: %s", msg)
	}
}
