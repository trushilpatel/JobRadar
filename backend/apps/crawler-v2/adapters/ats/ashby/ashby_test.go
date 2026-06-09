package ashby

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/trushilpatel/jobradar/apps/crawler-v2/common/logger"
	"github.com/trushilpatel/jobradar/apps/crawler-v2/core"
)

// newTestAdapter creates an Ashby adapter pointed at the given test server URL.
func newTestAdapter(serverURL string) *Ashby {
	a := New(logger.NewNoop(), http.DefaultClient)
	a.baseURL = serverURL
	return a
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

func TestAshby_FetchJobListing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		// Verify the request body has the correct operationName.
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if body["operationName"] != "ApiJobBoardWithTeams" {
			t.Errorf("operationName = %q, want ApiJobBoardWithTeams", body["operationName"])
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(loadFixture(t, "listing.json"))
	}))
	defer srv.Close()

	result, err := newTestAdapter(srv.URL).FetchJobListing(context.Background(), "ashby")
	if err != nil {
		t.Fatalf("FetchJobListing error: %v", err)
	}
	if len(result.Jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(result.Jobs))
	}
	if result.Jobs[0].ID != "ashby-abc-123-def-456" {
		t.Errorf("jobs[0].ID = %q, want ashby-abc-123-def-456", result.Jobs[0].ID)
	}
	if result.Jobs[1].ID != "ashby-xyz-789-uvw-012" {
		t.Errorf("jobs[1].ID = %q, want ashby-xyz-789-uvw-012", result.Jobs[1].ID)
	}
	if result.Jobs[0].Title != "Backend Engineer" {
		t.Errorf("jobs[0].Title = %q, want Backend Engineer", result.Jobs[0].Title)
	}
}

func TestAshby_FetchJobListing_UsesCorrectPath(t *testing.T) {
	var capturedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"jobBoardWithTeams":{"jobPostings":[]}}}`))
	}))
	defer srv.Close()

	_, _ = newTestAdapter(srv.URL).FetchJobListing(context.Background(), "ashby")
	if capturedPath != "/api/non-user-graphql" {
		t.Errorf("API path = %q, want /api/non-user-graphql", capturedPath)
	}
}

func TestAshby_FetchJobListing_UsesCorrectQueryParam(t *testing.T) {
	var capturedQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"jobBoardWithTeams":{"jobPostings":[]}}}`))
	}))
	defer srv.Close()

	_, _ = newTestAdapter(srv.URL).FetchJobListing(context.Background(), "ashby")
	if capturedQuery != "op=ApiJobBoardWithTeams" {
		t.Errorf("query = %q, want op=ApiJobBoardWithTeams", capturedQuery)
	}
}

func TestAshby_FetchJobListing_SetsUserAgent(t *testing.T) {
	var capturedUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUA = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"jobBoardWithTeams":{"jobPostings":[]}}}`))
	}))
	defer srv.Close()

	_, _ = newTestAdapter(srv.URL).FetchJobListing(context.Background(), "ashby")
	if !strings.HasPrefix(capturedUA, "Mozilla/5.0") {
		t.Errorf("User-Agent = %q, want Mozilla/5.0 prefix", capturedUA)
	}
}

func TestAshby_FetchJobListing_Errors(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{"http 500", http.StatusInternalServerError, ""},
		{"http 404", http.StatusNotFound, ""},
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

			_, err := newTestAdapter(srv.URL).FetchJobListing(context.Background(), "ashby")
			if err == nil {
				t.Fatalf("expected error for %s, got nil", tt.name)
			}
		})
	}
}

// ── FetchJobPost ──────────────────────────────────────────────────────────────

func TestAshby_FetchJobPost(t *testing.T) {
	var capturedPath string
	var capturedOp string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedOp = r.URL.Query().Get("op")

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if body["operationName"] != "ApiJobPosting" {
			t.Errorf("operationName = %q, want ApiJobPosting", body["operationName"])
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(loadFixture(t, "job_post.json"))
	}))
	defer srv.Close()

	summary := core.JobSummary{
		ID:  "ashby-abc-123-def-456",
		URL: "https://jobs.ashbyhq.com/netlify/abc-123-def-456",
	}
	post, err := newTestAdapter(srv.URL).FetchJobPost(context.Background(), summary)
	if err != nil {
		t.Fatalf("FetchJobPost error: %v", err)
	}
	if capturedPath != "/api/non-user-graphql" {
		t.Errorf("API path = %q, want /api/non-user-graphql", capturedPath)
	}
	if capturedOp != "ApiJobPosting" {
		t.Errorf("op query param = %q, want ApiJobPosting", capturedOp)
	}
	if post.ID != "ashby-abc-123-def-456" {
		t.Errorf("post.ID = %q, want ashby-abc-123-def-456", post.ID)
	}
	if post.Description == "" {
		t.Error("post.Description is empty")
	}
	if post.Title != "Backend Engineer" {
		t.Errorf("post.Title = %q, want Backend Engineer", post.Title)
	}
}

func TestAshby_FetchJobPost_URLParsing(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
		wantCo  string
		wantID  string
	}{
		{
			name:    "valid ashby url",
			url:     "https://jobs.ashbyhq.com/netlify/abc-123-def-456",
			wantErr: false,
			wantCo:  "netlify",
			wantID:  "abc-123-def-456",
		},
		{
			name:    "valid different company",
			url:     "https://jobs.ashbyhq.com/openai/xyz-789",
			wantErr: false,
			wantCo:  "openai",
			wantID:  "xyz-789",
		},
		{
			name:    "missing uuid segment",
			url:     "https://jobs.ashbyhq.com/netlify",
			wantErr: true,
		},
		{
			name:    "too many path segments",
			url:     "https://jobs.ashbyhq.com/netlify/abc/extra",
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
			if tt.wantErr {
				// Feed the URL into FetchJobPost via a stub server that returns a valid response —
				// the URL parse error must be returned before any HTTP call.
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write(loadFixture(t, "job_post.json"))
				}))
				defer srv.Close()
				_, err := newTestAdapter(srv.URL).FetchJobPost(context.Background(), core.JobSummary{URL: tt.url})
				if err == nil {
					t.Fatalf("expected error for URL %q, got nil", tt.url)
				}
				return
			}
			co, id, err := parseJobURL(tt.url)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if co != tt.wantCo {
				t.Errorf("company = %q, want %q", co, tt.wantCo)
			}
			if id != tt.wantID {
				t.Errorf("id = %q, want %q", id, tt.wantID)
			}
		})
	}
}

func TestAshby_FetchJobPost_Errors(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{"http 500", http.StatusInternalServerError, ""},
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

			summary := core.JobSummary{URL: "https://jobs.ashbyhq.com/ashby/abc-123"}
			_, err := newTestAdapter(srv.URL).FetchJobPost(context.Background(), summary)
			if err == nil {
				t.Fatalf("expected error for %s, got nil", tt.name)
			}
		})
	}
}

func TestAshby_FetchJobPost_EmptyPostingID(t *testing.T) {
	// When the GraphQL response returns a posting with an empty ID, we must error.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"jobPosting":{"id":"","title":""}}}`))
	}))
	defer srv.Close()

	summary := core.JobSummary{URL: "https://jobs.ashbyhq.com/ashby/abc-123"}
	_, err := newTestAdapter(srv.URL).FetchJobPost(context.Background(), summary)
	if err == nil {
		t.Fatal("expected error for empty posting ID, got nil")
	}
}

func TestAshby_FetchJobListing_NetworkError(t *testing.T) {
	// Close the server immediately so the HTTP client gets a connection error.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close()

	_, err := newTestAdapter(srv.URL).FetchJobListing(context.Background(), "ashby")
	if err == nil {
		t.Fatal("expected network error, got nil")
	}
}

func TestAshby_FetchJobPost_NetworkError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close()

	summary := core.JobSummary{URL: "https://jobs.ashbyhq.com/ashby/abc-123"}
	_, err := newTestAdapter(srv.URL).FetchJobPost(context.Background(), summary)
	if err == nil {
		t.Fatal("expected network error, got nil")
	}
}

func TestAshby_FetchJobListing_InvalidBaseURL(t *testing.T) {
	// Using an invalid base URL causes http.NewRequestWithContext to fail.
	a := New(logger.NewNoop(), http.DefaultClient)
	a.baseURL = "://invalid-url"
	_, err := a.FetchJobListing(context.Background(), "ashby")
	if err == nil {
		t.Fatal("expected error for invalid base URL, got nil")
	}
}

func TestAshby_FetchJobPost_InvalidBaseURL(t *testing.T) {
	a := New(logger.NewNoop(), http.DefaultClient)
	a.baseURL = "://invalid-url"
	_, err := a.FetchJobPost(context.Background(), core.JobSummary{URL: "https://jobs.ashbyhq.com/ashby/abc-123"})
	if err == nil {
		t.Fatal("expected error for invalid base URL, got nil")
	}
}

func TestAshby_post_MarshalError(t *testing.T) {
	// Pass an unmarshalable value (channel) to trigger the json.Marshal error path.
	a := New(logger.NewNoop(), http.DefaultClient)
	a.baseURL = "http://localhost:1" // won't be reached
	err := a.post(context.Background(), "http://localhost:1/api/non-user-graphql", make(chan int), nil)
	if err == nil {
		t.Fatal("expected marshal error, got nil")
	}
	if !strings.Contains(err.Error(), "marshal payload") {
		t.Errorf("error = %q, want marshal payload error", err.Error())
	}
}

// ── parseJobURL (unit) ────────────────────────────────────────────────────────

func TestParseJobURL(t *testing.T) {
	tests := []struct {
		name        string
		url         string
		wantCompany string
		wantID      string
		wantErr     bool
	}{
		{
			name:        "valid url",
			url:         "https://jobs.ashbyhq.com/netlify/abc-123-def-456",
			wantCompany: "netlify",
			wantID:      "abc-123-def-456",
		},
		{
			name:        "different company",
			url:         "https://jobs.ashbyhq.com/openai/xyz-789",
			wantCompany: "openai",
			wantID:      "xyz-789",
		},
		{
			name:    "root path only",
			url:     "https://jobs.ashbyhq.com/",
			wantErr: true,
		},
		{
			name:    "too many segments",
			url:     "https://jobs.ashbyhq.com/netlify/abc/extra",
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
			company, id, err := parseJobURL(tt.url)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (company=%q id=%q)", company, id)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if company != tt.wantCompany {
				t.Errorf("company = %q, want %q", company, tt.wantCompany)
			}
			if id != tt.wantID {
				t.Errorf("id = %q, want %q", id, tt.wantID)
			}
		})
	}
}
