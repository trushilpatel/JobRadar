package microsoft

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/trushilpatel/jobradar/jobingest/common/logger"
	"github.com/trushilpatel/jobradar/jobingest/core"
)

// newTestAdapter builds a Microsoft adapter pointed at the given test server URL.
// Goes through New() so tests exercise the same construction path as production.
func newTestAdapter(serverURL string) *Microsoft {
	m := New(logger.NewNoop(), http.DefaultClient)
	m.baseURL = serverURL
	m.domain = "microsoft.com"
	return m
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

func TestMicrosoft_FetchJobListing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(loadFixture(t, "listing.json"))
	}))
	defer srv.Close()

	result, err := newTestAdapter(srv.URL).FetchJobListing(context.Background(), "")
	if err != nil {
		t.Fatalf("FetchJobListing error: %v", err)
	}
	if len(result.Jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(result.Jobs))
	}

	j0 := result.Jobs[0]
	if j0.ID != "microsoft-1812345678" {
		t.Errorf("jobs[0].ID = %q, want microsoft-1812345678", j0.ID)
	}
	if j0.Title != "Senior Software Engineer" {
		t.Errorf("jobs[0].Title = %q, want 'Senior Software Engineer' (req ID stripped)", j0.Title)
	}
	if j0.Location != "Redmond, Washington, United States" {
		t.Errorf("jobs[0].Location = %q, want 'Redmond, Washington, United States'", j0.Location)
	}
	if j0.EmployType != core.EmployFullTime {
		t.Errorf("jobs[0].EmployType = %q, want full_time", j0.EmployType)
	}
	if !j0.LocationKnown {
		t.Error("jobs[0].LocationKnown should be true")
	}

	j1 := result.Jobs[1]
	if j1.ID != "microsoft-1812345679" {
		t.Errorf("jobs[1].ID = %q, want microsoft-1812345679", j1.ID)
	}
	if j1.Title != "Principal Program Manager" {
		t.Errorf("jobs[1].Title = %q, want 'Principal Program Manager' (req ID stripped)", j1.Title)
	}
	if len(j1.AllLocations) != 2 {
		t.Errorf("jobs[1].AllLocations len = %d, want 2", len(j1.AllLocations))
	}
}

func TestMicrosoft_FetchJobListing_Pagination(t *testing.T) {
	// Serve two pages: first has 1 item with count=2, second is empty → stop.
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		startStr := r.URL.Query().Get("start")
		if startStr == "0" || startStr == "" {
			// First page: 1 position, count=2 — pagination should continue
			resp := map[string]interface{}{
				"status": "success",
				"data": map[string]interface{}{
					"count": 2,
					"positions": []interface{}{
						map[string]interface{}{
							"id":           1812345678,
							"displayJobId": "1812345678",
							"name":         "Software Engineer",
							"locations":    []string{"Redmond, WA"},
							"postedTs":     1712678400,
						},
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		} else {
			// Second page: empty positions → stop
			resp := map[string]interface{}{
				"status": "success",
				"data": map[string]interface{}{
					"count":     2,
					"positions": []interface{}{},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		}
	}))
	defer srv.Close()

	adapter := newTestAdapter(srv.URL)
	adapter.pageSize = 1

	result, err := adapter.FetchJobListing(context.Background(), "")
	if err != nil {
		t.Fatalf("FetchJobListing error: %v", err)
	}
	if len(result.Jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(result.Jobs))
	}
	if callCount != 2 {
		t.Errorf("expected 2 API calls (page 1 + empty page 2), got %d", callCount)
	}
}

func TestMicrosoft_FetchJobListing_StopsAtCount(t *testing.T) {
	// Serve one page that fills the count exactly — should not make a second call.
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		resp := map[string]interface{}{
			"status": "success",
			"data": map[string]interface{}{
				"count": 1,
				"positions": []interface{}{
					map[string]interface{}{
						"id":           1,
						"displayJobId": "1",
						"name":         "Engineer",
						"locations":    []string{"Remote"},
						"postedTs":     1712678400,
					},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	adapter := newTestAdapter(srv.URL)
	adapter.pageSize = 20

	result, err := adapter.FetchJobListing(context.Background(), "")
	if err != nil {
		t.Fatalf("FetchJobListing error: %v", err)
	}
	if len(result.Jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(result.Jobs))
	}
	if callCount != 1 {
		t.Errorf("expected 1 API call (count satisfied), got %d", callCount)
	}
}

func TestMicrosoft_FetchJobListing_PathAndQuery(t *testing.T) {
	var capturedURL *url.URL
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedURL = r.URL
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"success","data":{"count":0,"positions":[]}}`))
	}))
	defer srv.Close()

	_, _ = newTestAdapter(srv.URL).FetchJobListing(context.Background(), "")

	if capturedURL == nil {
		t.Fatal("no request captured")
	}
	if capturedURL.Path != "/api/pcsx/search" {
		t.Errorf("path = %q, want /api/pcsx/search", capturedURL.Path)
	}
	if got := capturedURL.Query().Get("domain"); got != "microsoft.com" {
		t.Errorf("domain param = %q, want microsoft.com", got)
	}
	if got := capturedURL.Query().Get("start"); got != "0" {
		t.Errorf("start param = %q, want 0", got)
	}
}

func TestMicrosoft_FetchJobListing_UserAgent(t *testing.T) {
	var capturedUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUA = r.Header.Get("User-Agent")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"success","data":{"count":0,"positions":[]}}`))
	}))
	defer srv.Close()

	_, _ = newTestAdapter(srv.URL).FetchJobListing(context.Background(), "")

	if !strings.Contains(capturedUA, "Mozilla") {
		t.Errorf("User-Agent = %q, want a browser-like User-Agent", capturedUA)
	}
}

func TestMicrosoft_FetchJobListing_Errors(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{"http 500", http.StatusInternalServerError, ""},
		{"http 429", http.StatusTooManyRequests, ""},
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

			_, err := newTestAdapter(srv.URL).FetchJobListing(context.Background(), "")
			if err == nil {
				t.Fatalf("expected error for %s, got nil", tt.name)
			}
		})
	}
}

// ── FetchJobPost ──────────────────────────────────────────────────────────────

func TestMicrosoft_FetchJobPost(t *testing.T) {
	var capturedURL *url.URL
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedURL = r.URL
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(loadFixture(t, "job_post.json"))
	}))
	defer srv.Close()

	summary := core.JobSummary{
		ID:  "microsoft-1812345678",
		URL: fmt.Sprintf("%s/careers/job/1812345678", srv.URL),
	}
	post, err := newTestAdapter(srv.URL).FetchJobPost(context.Background(), summary)
	if err != nil {
		t.Fatalf("FetchJobPost error: %v", err)
	}

	// Verify path hits position_details
	if capturedURL.Path != "/api/pcsx/position_details" {
		t.Errorf("path = %q, want /api/pcsx/position_details", capturedURL.Path)
	}
	// Verify position_id is in the query
	if got := capturedURL.Query().Get("position_id"); got != "1812345678" {
		t.Errorf("position_id = %q, want 1812345678", got)
	}

	if post.ID != "microsoft-1812345678" {
		t.Errorf("post.ID = %q, want microsoft-1812345678", post.ID)
	}
	if post.Title != "Senior Software Engineer" {
		t.Errorf("post.Title = %q, want 'Senior Software Engineer' (req ID stripped)", post.Title)
	}
	if post.Description == "" {
		t.Error("post.Description is empty — HTML should have been stripped to non-empty text")
	}
	if post.Department != "Engineering" {
		t.Errorf("post.Department = %q, want Engineering", post.Department)
	}
	if post.EmploymentType != core.EmployFullTime {
		t.Errorf("post.EmploymentType = %q, want full_time", post.EmploymentType)
	}
	if post.PublishedDate == "" {
		t.Error("post.PublishedDate is empty")
	}
}

func TestMicrosoft_FetchJobPost_ExtractPositionID(t *testing.T) {
	var capturedPositionID string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPositionID = r.URL.Query().Get("position_id")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(loadFixture(t, "job_post.json"))
	}))
	defer srv.Close()

	// The URL contains a long int64 ID — verify extraction
	summary := core.JobSummary{
		ID:  "microsoft-1812345678",
		URL: "https://apply.careers.microsoft.com/careers/job/1812345678",
	}
	// Point to test server for actual request
	adapter := newTestAdapter(srv.URL)
	// We need the request to go to test server, so build an internal URL
	summary.URL = fmt.Sprintf("%s/careers/job/1812345678", srv.URL)

	_, err := adapter.FetchJobPost(context.Background(), summary)
	if err != nil {
		t.Fatalf("FetchJobPost error: %v", err)
	}
	if capturedPositionID != "1812345678" {
		t.Errorf("position_id = %q, want 1812345678", capturedPositionID)
	}
}

func TestMicrosoft_FetchJobPost_InvalidURL(t *testing.T) {
	adapter := newTestAdapter("http://localhost:0") // unreachable server, won't matter
	summary := core.JobSummary{
		ID:  "microsoft-abc",
		URL: "https://apply.careers.microsoft.com/careers/job/not-a-number",
	}
	_, err := adapter.FetchJobPost(context.Background(), summary)
	if err == nil {
		t.Fatal("expected error for URL with non-numeric ID segment, got nil")
	}
	if !strings.Contains(err.Error(), "could not extract position ID") {
		t.Errorf("error = %q, want to contain 'could not extract position ID'", err.Error())
	}
}

// TestMicrosoft_DefaultResolvers exercises the production fallback paths in
// resolveBaseURL and resolveDomain — i.e. when no override is set.
func TestMicrosoft_DefaultResolvers(t *testing.T) {
	var capturedURL *url.URL
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedURL = r.URL
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"success","data":{"count":0,"positions":[]}}`))
	}))
	defer srv.Close()

	// Build adapter via New() then manually set only baseURL (keep domain empty so
	// resolveDomain() falls through to the default).
	m := New(logger.NewNoop(), http.DefaultClient)
	m.baseURL = srv.URL // override host so request goes to test server
	// Leave m.domain empty → resolveDomain returns defaultDomain ("microsoft.com")

	_, err := m.FetchJobListing(context.Background(), "")
	if err != nil {
		t.Fatalf("FetchJobListing error: %v", err)
	}
	if capturedURL == nil {
		t.Fatal("no request captured")
	}
	// Verify default domain was used
	if got := capturedURL.Query().Get("domain"); got != defaultDomain {
		t.Errorf("domain = %q, want %q", got, defaultDomain)
	}
}

// TestMicrosoft_ResolveBaseURL_Default verifies that a zero-value baseURL falls
// back to the production constant.
func TestMicrosoft_ResolveBaseURL_Default(t *testing.T) {
	m := &Microsoft{}
	if got := m.resolveBaseURL(); got != defaultBaseURL {
		t.Errorf("resolveBaseURL() = %q, want %q", got, defaultBaseURL)
	}
}

// TestMicrosoft_GetBuildRequestError covers the http.NewRequestWithContext error
// path inside get() by using an invalid URL scheme.
func TestMicrosoft_GetBuildRequestError(t *testing.T) {
	m := New(logger.NewNoop(), http.DefaultClient)
	// Use an invalid baseURL that will cause url.Parse to succeed but request build to fail.
	// An empty scheme causes NewRequestWithContext to error.
	m.baseURL = "://invalid"

	_, err := m.FetchJobListing(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for invalid URL, got nil")
	}
}

func TestMicrosoft_FetchJobPost_Errors(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{"http 404", http.StatusNotFound, ""},
		{"http 500", http.StatusInternalServerError, ""},
		{"malformed json", http.StatusOK, "not json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			summary := core.JobSummary{
				ID:  "microsoft-1812345678",
				URL: fmt.Sprintf("%s/careers/job/1812345678", srv.URL),
			}
			_, err := newTestAdapter(srv.URL).FetchJobPost(context.Background(), summary)
			if err == nil {
				t.Fatalf("expected error for %s, got nil", tt.name)
			}
		})
	}
}
