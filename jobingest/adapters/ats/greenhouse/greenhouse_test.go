package greenhouse

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/trushilpatel/jobradar/jobingest/common/logger"
	"github.com/trushilpatel/jobradar/jobingest/core"
)

// newTestAdapter builds a Greenhouse pointed at the given test server URL.
// Goes through New() so tests exercise the same construction path as production.
func newTestAdapter(serverURL string) *Greenhouse {
	g := New(logger.NewNoop(), http.DefaultClient)
	g.baseURL = serverURL
	return g
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

func TestGreenhouse_FetchJobListing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(loadFixture(t, "listing.json"))
	}))
	defer srv.Close()

	result, err := newTestAdapter(srv.URL).FetchJobListing(context.Background(), "stripe")
	if err != nil {
		t.Fatalf("FetchJobListing error: %v", err)
	}
	if len(result.Jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(result.Jobs))
	}
	if result.Jobs[0].ID != "greenhouse-12345" {
		t.Errorf("jobs[0].ID = %q, want greenhouse-12345", result.Jobs[0].ID)
	}
	if result.Jobs[0].Location != "San Francisco, CA" {
		t.Errorf("jobs[0].Location = %q, want San Francisco, CA", result.Jobs[0].Location)
	}
	if result.Jobs[1].ID != "greenhouse-67890" {
		t.Errorf("jobs[1].ID = %q, want greenhouse-67890", result.Jobs[1].ID)
	}
}

func TestGreenhouse_FetchJobListing_UsesJobListingURL(t *testing.T) {
	var capturedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"jobs":[]}`))
	}))
	defer srv.Close()

	_, _ = newTestAdapter(srv.URL).FetchJobListing(context.Background(), "stripe")
	if capturedPath != "/v1/boards/stripe/jobs" {
		t.Errorf("API path = %q, want /v1/boards/stripe/jobs", capturedPath)
	}
}

func TestGreenhouse_FetchJobListing_Errors(t *testing.T) {
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

			_, err := newTestAdapter(srv.URL).FetchJobListing(context.Background(), "stripe")
			if err == nil {
				t.Fatalf("expected error for %s, got nil", tt.name)
			}
		})
	}
}

// ── FetchJobPost ──────────────────────────────────────────────────────────────

func TestGreenhouse_FetchJobPost(t *testing.T) {
	var capturedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(loadFixture(t, "job_post.json"))
	}))
	defer srv.Close()

	summary := core.JobSummary{
		ID:  "greenhouse-12345",
		URL: "https://job-boards.greenhouse.io/stripe/jobs/12345",
	}
	post, err := newTestAdapter(srv.URL).FetchJobPost(context.Background(), summary)
	if err != nil {
		t.Fatalf("FetchJobPost error: %v", err)
	}
	if capturedPath != "/v1/boards/stripe/jobs/12345" {
		t.Errorf("API path = %q, want /v1/boards/stripe/jobs/12345", capturedPath)
	}
	if post.ID != "greenhouse-12345" {
		t.Errorf("post.ID = %q, want greenhouse-12345", post.ID)
	}
	if post.Description == "" {
		t.Error("post.Description is empty")
	}
}

func TestGreenhouse_FetchJobPost_Errors(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{"http 404", http.StatusNotFound, ""},
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

			summary := core.JobSummary{URL: "https://job-boards.greenhouse.io/stripe/jobs/12345"}
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
		name     string
		url      string
		wantSlug string
		wantID   string
		wantErr  bool
	}{
		{
			name:     "valid url",
			url:      "https://job-boards.greenhouse.io/stripe/jobs/12345",
			wantSlug: "stripe",
			wantID:   "12345",
		},
		{
			name:     "different company",
			url:      "https://job-boards.greenhouse.io/reddit/jobs/67890",
			wantSlug: "reddit",
			wantID:   "67890",
		},
		{
			name:    "missing job id",
			url:     "https://job-boards.greenhouse.io/stripe",
			wantErr: true,
		},
		{
			name:    "wrong path segment",
			url:     "https://job-boards.greenhouse.io/stripe/roles/12345",
			wantErr: true,
		},
		{
			name:    "invalid url",
			url:     "://not-a-url",
			wantErr: true,
		},
		{
			name:    "empty url",
			url:     "",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			slug, id, err := parseJobURL(tt.url)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (slug=%q id=%q)", slug, id)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if slug != tt.wantSlug {
				t.Errorf("slug = %q, want %q", slug, tt.wantSlug)
			}
			if id != tt.wantID {
				t.Errorf("id = %q, want %q", id, tt.wantID)
			}
		})
	}
}

// ── resolveBaseURL / default URL ──────────────────────────────────────────────

func TestGreenhouse_ResolveBaseURL_Default(t *testing.T) {
	g := New(logger.NewNoop(), http.DefaultClient)
	if g.resolveBaseURL() != defaultBaseURL {
		t.Errorf("resolveBaseURL() = %q, want %q", g.resolveBaseURL(), defaultBaseURL)
	}
}

func TestGreenhouse_Get_RequestBuildError(t *testing.T) {
	// Passing an invalid URL causes http.NewRequestWithContext to error.
	g := New(logger.NewNoop(), http.DefaultClient)
	var dst any
	err := g.get(context.Background(), "://invalid-url", &dst)
	if err == nil {
		t.Fatal("expected error for invalid URL, got nil")
	}
}
