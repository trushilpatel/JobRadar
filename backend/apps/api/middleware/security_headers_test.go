package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSecurityHeaders_SetsExpectedHeaders(t *testing.T) {
	mw := SecurityHeaders()
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	cases := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"Referrer-Policy":        "strict-origin-when-cross-origin",
		"Permissions-Policy":     "geolocation=(), camera=(), microphone=(), interest-cohort=()",
	}
	for k, want := range cases {
		if got := w.Header().Get(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
}

// TestSecurityHeaders_NoHSTS ensures HSTS is NOT set here. HSTS is owned by
// Caddy at the edge; duplicating it in the app layer creates header-merge
// surprises when Caddy adds `includeSubDomains` later.
func TestSecurityHeaders_NoHSTS(t *testing.T) {
	mw := SecurityHeaders()
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if got := w.Header().Get("Strict-Transport-Security"); got != "" {
		t.Errorf("Strict-Transport-Security = %q, expected empty (edge owns HSTS)", got)
	}
	if got := w.Header().Get("Content-Security-Policy"); got != "" {
		t.Errorf("Content-Security-Policy = %q, expected empty (edge owns CSP)", got)
	}
}
