package netflix

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/trushilpatel/jobradar/apps/crawler-v2/common/logger"
	"github.com/trushilpatel/jobradar/common/ats/core"
)

// newTestAdapter builds a Netflix adapter pointed at the given test server URL.
// Goes through New() so tests exercise the same construction path as production.
func newTestAdapter(serverURL string) *Netflix {
	n := New(logger.NewNoop(), http.DefaultClient)
	n.baseURL = serverURL
	n.domain = "test.example.com"
	n.pageSize = 10
	return n
}

func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("load fixture %s: %v", name, err)
	}
	return data
}

// ── FetchJobListing ───────────────────────────────────────────────────────────

func TestNetflix_FetchJobListing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(loadFixture(t, "listing.json"))
	}))
	defer srv.Close()

	result, err := newTestAdapter(srv.URL).FetchJobListing(context.Background(), "ignored")
	if err != nil {
		t.Fatalf("FetchJobListing error: %v", err)
	}
	if len(result.Jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(result.Jobs))
	}

	// First job: has explicit location
	j0 := result.Jobs[0]
	if j0.ID != "netflix-790315382430" {
		t.Errorf("jobs[0].ID = %q, want netflix-790315382430", j0.ID)
	}
	if j0.Location != "Los Angeles, California, United States" {
		t.Errorf("jobs[0].Location = %q, want Los Angeles, California, United States", j0.Location)
	}
	if j0.EmployType != core.EmployFullTime {
		t.Errorf("jobs[0].EmployType = %q, want %q", j0.EmployType, core.EmployFullTime)
	}
	if !j0.LocationKnown {
		t.Error("jobs[0].LocationKnown should be true")
	}

	// Second job: empty location — falls back to joined Locations
	j1 := result.Jobs[1]
	if j1.ID != "netflix-790315382431" {
		t.Errorf("jobs[1].ID = %q, want netflix-790315382431", j1.ID)
	}
	wantLoc1 := "New York, NY; Remote"
	if j1.Location != wantLoc1 {
		t.Errorf("jobs[1].Location = %q, want %q", j1.Location, wantLoc1)
	}
}

func TestNetflix_FetchJobListing_Pagination(t *testing.T) {
	// pageSize=2; serve page0 (2 items), page1 (1 item < pageSize → stop)
	var requestedStarts []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := r.URL.Query().Get("start")
		requestedStarts = append(requestedStarts, start)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		switch start {
		case "0":
			_, _ = w.Write([]byte(`{"positions":[
				{"id":1,"name":"Job A","location":"NYC"},
				{"id":2,"name":"Job B","location":"LA"}
			],"count":3}`))
		case "2":
			_, _ = w.Write([]byte(`{"positions":[
				{"id":3,"name":"Job C","location":"Chicago"}
			],"count":3}`))
		default:
			_, _ = w.Write([]byte(`{"positions":[],"count":3}`))
		}
	}))
	defer srv.Close()

	n := New(logger.NewNoop(), http.DefaultClient)
	n.baseURL = srv.URL
	n.domain = "test.example.com"
	n.pageSize = 2

	result, err := n.FetchJobListing(context.Background(), "ignored")
	if err != nil {
		t.Fatalf("FetchJobListing error: %v", err)
	}
	if len(result.Jobs) != 3 {
		t.Fatalf("expected 3 jobs across 2 pages, got %d", len(result.Jobs))
	}
	if len(requestedStarts) != 2 {
		t.Errorf("expected 2 HTTP requests (start=0 and start=2), got %d: %v", len(requestedStarts), requestedStarts)
	}
	if requestedStarts[0] != "0" {
		t.Errorf("first request start = %q, want 0", requestedStarts[0])
	}
	if requestedStarts[1] != "2" {
		t.Errorf("second request start = %q, want 2", requestedStarts[1])
	}
}

func TestNetflix_FetchJobListing_PathAndQuery(t *testing.T) {
	var capturedURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedURL = r.URL.RequestURI()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"positions":[],"count":0}`))
	}))
	defer srv.Close()

	n := New(logger.NewNoop(), http.DefaultClient)
	n.baseURL = srv.URL
	n.domain = "example.com"
	n.pageSize = 10

	_, _ = n.FetchJobListing(context.Background(), "ignored")

	if !strings.Contains(capturedURL, "/api/apply/v2/jobs") {
		t.Errorf("URL path should contain /api/apply/v2/jobs, got %q", capturedURL)
	}
	if !strings.Contains(capturedURL, "domain=example.com") {
		t.Errorf("URL should contain domain=example.com, got %q", capturedURL)
	}
	if !strings.Contains(capturedURL, "start=0") {
		t.Errorf("URL should contain start=0, got %q", capturedURL)
	}
}

func TestNetflix_FetchJobListing_Errors(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{"http 500", http.StatusInternalServerError, ""},
		{"http 429", http.StatusTooManyRequests, ""},
		{"malformed json", http.StatusOK, "not json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			_, err := newTestAdapter(srv.URL).FetchJobListing(context.Background(), "ignored")
			if err == nil {
				t.Fatalf("expected error for %s, got nil", tt.name)
			}
		})
	}
}

// ── FetchJobPost ──────────────────────────────────────────────────────────────

func TestNetflix_FetchJobPost(t *testing.T) {
	var capturedURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedURL = r.URL.RequestURI()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(loadFixture(t, "job_post.json"))
	}))
	defer srv.Close()

	summary := core.JobSummary{
		ID:  "netflix-790315382430",
		URL: fmt.Sprintf("%s/careers/job/790315382430", srv.URL),
	}
	post, err := newTestAdapter(srv.URL).FetchJobPost(context.Background(), summary)
	if err != nil {
		t.Fatalf("FetchJobPost error: %v", err)
	}

	// Verify the detail URL was built with the stripped ID
	if !strings.Contains(capturedURL, "/api/apply/v2/jobs/790315382430") {
		t.Errorf("detail URL should contain /api/apply/v2/jobs/790315382430, got %q", capturedURL)
	}
	if post.ID != "netflix-790315382430" {
		t.Errorf("post.ID = %q, want netflix-790315382430", post.ID)
	}
	if post.Description == "" {
		t.Error("post.Description is empty")
	}
	// Verify HTML was stripped
	if strings.Contains(post.Description, "<p>") || strings.Contains(post.Description, "<ul>") {
		t.Errorf("post.Description should have HTML stripped, got %q", post.Description)
	}
	if post.Department != "Engineering" {
		t.Errorf("post.Department = %q, want Engineering", post.Department)
	}
	if post.PublishedDate == "" {
		t.Error("post.PublishedDate is empty")
	}
}

func TestNetflix_FetchJobPost_IDExtraction(t *testing.T) {
	var capturedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":42,"name":"Engineer","location":"Remote"}`))
	}))
	defer srv.Close()

	summary := core.JobSummary{ID: "netflix-42"}
	_, err := newTestAdapter(srv.URL).FetchJobPost(context.Background(), summary)
	if err != nil {
		t.Fatalf("FetchJobPost error: %v", err)
	}
	if capturedPath != "/api/apply/v2/jobs/42" {
		t.Errorf("detail path = %q, want /api/apply/v2/jobs/42", capturedPath)
	}
}

func TestNetflix_FetchJobPost_Errors(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{"http 404", http.StatusNotFound, ""},
		{"malformed json", http.StatusOK, "not json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			summary := core.JobSummary{ID: "netflix-12345"}
			_, err := newTestAdapter(srv.URL).FetchJobPost(context.Background(), summary)
			if err == nil {
				t.Fatalf("expected error for %s, got nil", tt.name)
			}
		})
	}
}

// ── get: build-request error ──────────────────────────────────────────────────

func TestNetflix_FetchJobListing_BuildRequestError(t *testing.T) {
	// A baseURL with a control character causes http.NewRequestWithContext to fail.
	n := New(logger.NewNoop(), http.DefaultClient)
	n.baseURL = "http://\x00invalid"
	n.domain = "test.example.com"
	n.pageSize = 10
	_, err := n.FetchJobListing(context.Background(), "ignored")
	if err == nil {
		t.Fatal("expected error for invalid URL, got nil")
	}
}

// ── FetchJobPost empty ID ─────────────────────────────────────────────────────

func TestNetflix_FetchJobPost_EmptyIDAfterStrip(t *testing.T) {
	n := newTestAdapter("http://127.0.0.1:0") // never reached
	// ID "netflix-" strips to "", triggering the empty-ID guard
	summary := core.JobSummary{ID: "netflix-"}
	_, err := n.FetchJobPost(context.Background(), summary)
	if err == nil {
		t.Fatal("expected error for empty ID after stripping prefix, got nil")
	}
}

// ── resolve* fallback paths ───────────────────────────────────────────────────

func TestNetflix_New_Defaults(t *testing.T) {
	// New() with no overrides should use the production defaults.
	// Verify resolveBaseURL, resolveDomain, resolvePageSize all return defaults.
	n := New(logger.NewNoop(), http.DefaultClient)

	if got := n.resolveBaseURL(); got != defaultBaseURL {
		t.Errorf("resolveBaseURL() = %q, want %q", got, defaultBaseURL)
	}
	if got := n.resolveDomain(); got != defaultDomain {
		t.Errorf("resolveDomain() = %q, want %q", got, defaultDomain)
	}
	if got := n.resolvePageSize(); got != defaultPageSize {
		t.Errorf("resolvePageSize() = %d, want %d", got, defaultPageSize)
	}
}

func TestNetflix_ResolvePageSize_Zero(t *testing.T) {
	// A zero pageSize (e.g. after zeroing out the field) falls back to defaultPageSize.
	n := New(logger.NewNoop(), http.DefaultClient)
	n.pageSize = 0
	if got := n.resolvePageSize(); got != defaultPageSize {
		t.Errorf("resolvePageSize() with zero = %d, want %d", got, defaultPageSize)
	}
}

// ── get: network error ────────────────────────────────────────────────────────

func TestNetflix_FetchJobListing_NetworkError(t *testing.T) {
	// Point the adapter at a server that immediately closes the connection.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Hijack and close to force a network error on the client side.
		hj, ok := w.(http.Hijacker)
		if !ok {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		conn, _, _ := hj.Hijack()
		_ = conn.Close()
	}))
	defer srv.Close()

	_, err := newTestAdapter(srv.URL).FetchJobListing(context.Background(), "ignored")
	if err == nil {
		t.Fatal("expected network error, got nil")
	}
}

// ── stripPrefix ───────────────────────────────────────────────────────────────

func TestStripPrefix(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"with prefix", "netflix-790315382430", "790315382430"},
		{"without prefix", "790315382430", "790315382430"},
		{"prefix only", "netflix-", ""},
		{"empty string", "", ""},
		{"other prefix", "greenhouse-12345", "greenhouse-12345"},
		{"large id", "netflix-9999999999999", "9999999999999"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripPrefix(tt.input)
			if got != tt.want {
				t.Errorf("stripPrefix(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ── unixToRFC3339 ─────────────────────────────────────────────────────────────

func TestUnixToRFC3339(t *testing.T) {
	t.Run("zero returns empty", func(t *testing.T) {
		if got := unixToRFC3339(0); got != "" {
			t.Errorf("unixToRFC3339(0) = %q, want empty string", got)
		}
	})
	t.Run("non-zero returns RFC3339", func(t *testing.T) {
		// 1775779200 = 2026-04-07T16:00:00Z
		got := unixToRFC3339(1775779200)
		if got == "" {
			t.Fatal("unixToRFC3339(1775779200) returned empty string")
		}
		// Must be valid RFC3339
		if !strings.Contains(got, "T") || !strings.HasSuffix(got, "Z") {
			t.Errorf("unixToRFC3339(1775779200) = %q, does not look like RFC3339", got)
		}
	})
	t.Run("known timestamp", func(t *testing.T) {
		// Unix 0 = 1970-01-01T00:00:00Z; use 1 to verify format
		got := unixToRFC3339(1)
		want := "1970-01-01T00:00:01Z"
		if got != want {
			t.Errorf("unixToRFC3339(1) = %q, want %q", got, want)
		}
	})
}
