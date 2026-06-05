package gem

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/trushilpatel/jobradar/jobingest/common/logger"
)

// newTestAdapter builds a Gem adapter pointed at the given test server URL.
// Goes through New() so tests exercise the same construction path as production.
func newTestAdapter(serverURL string) *Gem {
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

func TestGem_FetchJobListing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(loadFixture(t, "listing.json"))
	}))
	defer srv.Close()

	result, err := newTestAdapter(srv.URL).FetchJobListing(context.Background(), "inception")
	if err != nil {
		t.Fatalf("FetchJobListing error: %v", err)
	}
	if len(result.Jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(result.Jobs))
	}

	// Both jobs must have JobContent populated (fat adapter).
	for i, j := range result.Jobs {
		if j.JobContent == nil {
			t.Errorf("jobs[%d].JobContent is nil — fat adapter must populate at listing time", i)
		}
	}

	j0 := result.Jobs[0]
	if j0.ID != "gem-abc123" {
		t.Errorf("jobs[0].ID = %q, want gem-abc123", j0.ID)
	}
	if j0.Title != "Software Engineer" {
		t.Errorf("jobs[0].Title = %q, want Software Engineer", j0.Title)
	}
	if j0.Location != "San Francisco, CA" {
		t.Errorf("jobs[0].Location = %q, want San Francisco, CA", j0.Location)
	}

	j1 := result.Jobs[1]
	if j1.ID != "gem-def456" {
		t.Errorf("jobs[1].ID = %q, want gem-def456", j1.ID)
	}
}

func TestGem_FetchJobListing_UsesCorrectPath(t *testing.T) {
	var capturedPath string
	var capturedHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedHeader = r.Header.Get("Accept")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	_, _ = newTestAdapter(srv.URL).FetchJobListing(context.Background(), "inception")
	if capturedPath != "/job_board/v0/inception/job_posts/" {
		t.Errorf("API path = %q, want /job_board/v0/inception/job_posts/", capturedPath)
	}
	if capturedHeader != "application/json" {
		t.Errorf("Accept header = %q, want application/json", capturedHeader)
	}
}

func TestGem_FetchJobListing_EmptySlug(t *testing.T) {
	g := New(logger.NewNoop(), http.DefaultClient)
	_, err := g.FetchJobListing(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty slug, got nil")
	}
	_, err = g.FetchJobListing(context.Background(), "   ")
	if err == nil {
		t.Fatal("expected error for whitespace-only slug, got nil")
	}
}

func TestGem_FetchJobListing_Errors(t *testing.T) {
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

			_, err := newTestAdapter(srv.URL).FetchJobListing(context.Background(), "inception")
			if err == nil {
				t.Fatalf("expected error for %s, got nil", tt.name)
			}
		})
	}
}

func TestGem_FetchJobListing_ContentPlainPreferred(t *testing.T) {
	// jobs[0] in listing.json has both content and content_plain;
	// content_plain must win.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(loadFixture(t, "listing.json"))
	}))
	defer srv.Close()

	result, err := newTestAdapter(srv.URL).FetchJobListing(context.Background(), "inception")
	if err != nil {
		t.Fatalf("FetchJobListing error: %v", err)
	}
	if len(result.Jobs) == 0 {
		t.Fatal("expected at least one job")
	}

	j := result.Jobs[0]
	if j.JobContent == nil {
		t.Fatal("JobContent is nil")
	}
	want := "Full plain-text job description for Software Engineer."
	if j.JobContent.Description != want {
		t.Errorf("Description = %q, want %q", j.JobContent.Description, want)
	}
}

func TestGem_FetchJobListing_HTMLFallback(t *testing.T) {
	// jobs[1] in listing.json has empty content_plain; StripHTML must be applied.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(loadFixture(t, "listing.json"))
	}))
	defer srv.Close()

	result, err := newTestAdapter(srv.URL).FetchJobListing(context.Background(), "inception")
	if err != nil {
		t.Fatalf("FetchJobListing error: %v", err)
	}
	if len(result.Jobs) < 2 {
		t.Fatalf("expected at least 2 jobs, got %d", len(result.Jobs))
	}

	j := result.Jobs[1]
	if j.JobContent == nil {
		t.Fatal("JobContent is nil for job with HTML-only content")
	}
	// Verify HTML tags are stripped and entity decoded.
	desc := j.JobContent.Description
	if desc == "" {
		t.Fatal("Description is empty after HTML strip")
	}
	if containsHTMLTag(desc) {
		t.Errorf("Description still contains HTML tags: %q", desc)
	}
	// &amp; should be decoded to &
	want := "Join our product team as an intern & grow."
	if desc != want {
		t.Errorf("Description = %q, want %q", desc, want)
	}
}

// ── resolveBaseURL ────────────────────────────────────────────────────────────

func TestGem_ResolveBaseURL_DefaultWhenEmpty(t *testing.T) {
	g := New(logger.NewNoop(), http.DefaultClient)
	// baseURL is empty by default; resolveBaseURL must return the production URL.
	got := g.resolveBaseURL()
	if got != defaultBaseURL {
		t.Errorf("resolveBaseURL() = %q, want %q", got, defaultBaseURL)
	}
}

func TestGem_ResolveBaseURL_UsesOverride(t *testing.T) {
	g := New(logger.NewNoop(), http.DefaultClient)
	g.baseURL = "http://localhost:9999"
	got := g.resolveBaseURL()
	if got != "http://localhost:9999" {
		t.Errorf("resolveBaseURL() = %q, want http://localhost:9999", got)
	}
}

// containsHTMLTag is a simple check for angle brackets.
func containsHTMLTag(s string) bool {
	for _, c := range s {
		if c == '<' || c == '>' {
			return true
		}
	}
	return false
}
