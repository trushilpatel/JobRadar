package lever

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/trushilpatel/jobradar/apps/crawler-v2/common/logger"
	"github.com/trushilpatel/jobradar/apps/crawler-v2/core"
)

// newTestAdapter builds a Lever adapter pointed at the given test server URL.
func newTestAdapter(serverURL string) *Lever {
	l := New(logger.NewNoop(), http.DefaultClient)
	l.baseURL = serverURL
	return l
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

func TestLever_FetchJobListing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(loadFixture(t, "listing.json"))
	}))
	defer srv.Close()

	result, err := newTestAdapter(srv.URL).FetchJobListing(context.Background(), "netlify")
	if err != nil {
		t.Fatalf("FetchJobListing error: %v", err)
	}
	if len(result.Jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(result.Jobs))
	}
	if result.Jobs[0].ID != "lever-abc-111-def" {
		t.Errorf("jobs[0].ID = %q, want lever-abc-111-def", result.Jobs[0].ID)
	}
	if result.Jobs[0].Location != "Remote" {
		t.Errorf("jobs[0].Location = %q, want Remote", result.Jobs[0].Location)
	}
	if result.Jobs[1].ID != "lever-xyz-222-ghi" {
		t.Errorf("jobs[1].ID = %q, want lever-xyz-222-ghi", result.Jobs[1].ID)
	}
	if result.Jobs[0].EmployType != core.EmployFullTime {
		t.Errorf("jobs[0].EmployType = %q, want %q", result.Jobs[0].EmployType, core.EmployFullTime)
	}
	if result.Jobs[1].EmployType != core.EmployInternship {
		t.Errorf("jobs[1].EmployType = %q, want %q", result.Jobs[1].EmployType, core.EmployInternship)
	}
	if !result.Jobs[0].LocationKnown {
		t.Error("jobs[0].LocationKnown should be true")
	}
	if len(result.Jobs[0].AllLocations) != 2 {
		t.Errorf("jobs[0].AllLocations length = %d, want 2", len(result.Jobs[0].AllLocations))
	}
}

func TestLever_FetchJobListing_UsesCorrectPath(t *testing.T) {
	var capturedPath string
	var capturedQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	_, _ = newTestAdapter(srv.URL).FetchJobListing(context.Background(), "netlify")
	if capturedPath != "/v0/postings/netlify" {
		t.Errorf("API path = %q, want /v0/postings/netlify", capturedPath)
	}
	if capturedQuery != "mode=json" {
		t.Errorf("query = %q, want mode=json", capturedQuery)
	}
}

func TestLever_FetchJobListing_Errors(t *testing.T) {
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

			_, err := newTestAdapter(srv.URL).FetchJobListing(context.Background(), "netlify")
			if err == nil {
				t.Fatalf("expected error for %s, got nil", tt.name)
			}
		})
	}
}

// ── FetchJobPost ──────────────────────────────────────────────────────────────

func TestLever_FetchJobPost(t *testing.T) {
	var capturedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(loadFixture(t, "job_post.json"))
	}))
	defer srv.Close()

	summary := core.JobSummary{
		ID:  "lever-abc-111-def",
		URL: "https://jobs.lever.co/netlify/abc-111-def",
	}
	post, err := newTestAdapter(srv.URL).FetchJobPost(context.Background(), summary)
	if err != nil {
		t.Fatalf("FetchJobPost error: %v", err)
	}
	if capturedPath != "/v0/postings/netlify/abc-111-def" {
		t.Errorf("API path = %q, want /v0/postings/netlify/abc-111-def", capturedPath)
	}
	if post.ID != "lever-abc-111-def" {
		t.Errorf("post.ID = %q, want lever-abc-111-def", post.ID)
	}
	if post.Title != "Backend Engineer" {
		t.Errorf("post.Title = %q, want Backend Engineer", post.Title)
	}
	if post.Description == "" {
		t.Error("post.Description is empty")
	}
	if !strings.Contains(post.Description, "We are building") {
		t.Errorf("post.Description = %q, want it to contain 'We are building'", post.Description)
	}
	if post.Compensation == "" {
		t.Error("post.Compensation is empty")
	}
	if post.Department == "" {
		t.Error("post.Department is empty")
	}
}

func TestLever_FetchJobPost_URLParsing(t *testing.T) {
	tests := []struct {
		name        string
		url         string
		wantPathSfx string
	}{
		{
			name:        "standard lever URL",
			url:         "https://jobs.lever.co/netlify/abc-111-def",
			wantPathSfx: "/v0/postings/netlify/abc-111-def",
		},
		{
			name:        "different company",
			url:         "https://jobs.lever.co/stripe/xyz-999-abc",
			wantPathSfx: "/v0/postings/stripe/xyz-999-abc",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedPath string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedPath = r.URL.Path
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(loadFixture(t, "job_post.json"))
			}))
			defer srv.Close()

			summary := core.JobSummary{URL: tt.url}
			_, err := newTestAdapter(srv.URL).FetchJobPost(context.Background(), summary)
			if err != nil {
				t.Fatalf("FetchJobPost error: %v", err)
			}
			if capturedPath != tt.wantPathSfx {
				t.Errorf("path = %q, want %q", capturedPath, tt.wantPathSfx)
			}
		})
	}
}

func TestLever_FetchJobPost_InvalidURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(loadFixture(t, "job_post.json"))
	}))
	defer srv.Close()

	invalidURLs := []struct {
		name string
		url  string
	}{
		{"no uuid", "https://jobs.lever.co/netlify"},
		{"too many parts", "https://jobs.lever.co/netlify/uuid/extra"},
		{"empty path", "https://jobs.lever.co/"},
		{"invalid scheme", "://bad-url"},
	}
	for _, tt := range invalidURLs {
		t.Run(tt.name, func(t *testing.T) {
			summary := core.JobSummary{URL: tt.url}
			_, err := newTestAdapter(srv.URL).FetchJobPost(context.Background(), summary)
			if err == nil {
				t.Fatalf("expected error for URL %q, got nil", tt.url)
			}
		})
	}
}

func TestLever_FetchJobPost_Errors(t *testing.T) {
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

			summary := core.JobSummary{URL: "https://jobs.lever.co/netlify/abc-111-def"}
			_, err := newTestAdapter(srv.URL).FetchJobPost(context.Background(), summary)
			if err == nil {
				t.Fatalf("expected error for %s, got nil", tt.name)
			}
		})
	}
}

// ── parseJobURL ───────────────────────────────────────────────────────────────

func TestParseJobURL(t *testing.T) {
	tests := []struct {
		name        string
		url         string
		wantCompany string
		wantUUID    string
		wantErr     bool
	}{
		{
			name:        "valid lever URL",
			url:         "https://jobs.lever.co/netlify/abc-111-def",
			wantCompany: "netlify",
			wantUUID:    "abc-111-def",
		},
		{
			name:        "different company",
			url:         "https://jobs.lever.co/stripe/xyz-999-abc",
			wantCompany: "stripe",
			wantUUID:    "xyz-999-abc",
		},
		{
			name:        "UUID with many hyphens",
			url:         "https://jobs.lever.co/acme/a1b2c3d4-e5f6-7890-abcd-ef1234567890",
			wantCompany: "acme",
			wantUUID:    "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
		},
		{
			name:    "missing uuid",
			url:     "https://jobs.lever.co/netlify",
			wantErr: true,
		},
		{
			name:    "too many path segments",
			url:     "https://jobs.lever.co/netlify/uuid/extra",
			wantErr: true,
		},
		{
			name:    "empty path",
			url:     "https://jobs.lever.co/",
			wantErr: true,
		},
		{
			name:    "invalid url",
			url:     "://not-a-url",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			company, uuid, err := parseJobURL(tt.url)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (company=%q uuid=%q)", company, uuid)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if company != tt.wantCompany {
				t.Errorf("company = %q, want %q", company, tt.wantCompany)
			}
			if uuid != tt.wantUUID {
				t.Errorf("uuid = %q, want %q", uuid, tt.wantUUID)
			}
		})
	}
}
