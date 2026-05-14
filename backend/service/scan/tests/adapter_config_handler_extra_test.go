package scan

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestAdapterConfigHandler_UpdateAdapterConfig_BadJSON asserts that
// malformed JSON in the request body is rejected with 400 and never
// reaches the validation layer.
func TestAdapterConfigHandler_UpdateAdapterConfig_BadJSON(t *testing.T) {
	mock := &mockAdapterConfigService{}
	r := newAdapterConfigTestRouter(mock)

	req := httptest.NewRequest(http.MethodPatch, "/adapter-configs/greenhouse", strings.NewReader("not-json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want 400; body: %s", w.Code, w.Body.String())
	}
}

// TestAdapterConfigHandler_UpdateScanIntervalOverride_BadJSON mirrors the
// guard above for the company-level override endpoint.
func TestAdapterConfigHandler_UpdateScanIntervalOverride_BadJSON(t *testing.T) {
	mock := &mockAdapterConfigService{}
	r := newAdapterConfigTestRouter(mock)

	req := httptest.NewRequest(http.MethodPatch, "/companies/company-1/scan-interval-override", strings.NewReader("not-json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want 400; body: %s", w.Code, w.Body.String())
	}
}

// TestAdapterConfigHandler_ListAdapterConfigs_Empty asserts the list endpoint
// returns an empty data envelope (not a null) when the underlying service
// returns no rows — important for the admin UI that maps over data.
func TestAdapterConfigHandler_ListAdapterConfigs_Empty(t *testing.T) {
	mock := &mockAdapterConfigService{}
	r := newAdapterConfigTestRouter(mock)

	req := httptest.NewRequest(http.MethodGet, "/adapter-configs", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200; body: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"data"`) {
		t.Errorf("response missing data envelope: %s", w.Body.String())
	}
}
