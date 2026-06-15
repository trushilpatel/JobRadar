package uber

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/trushilpatel/jobradar/common/logger"
)

// newTestAdapter builds an Uber pointed at the given test server URL.
// Goes through New() so tests exercise the same construction path as production.
func newTestAdapter(serverURL string) *Uber {
	u := New(logger.NewNoop(), http.DefaultClient)
	u.baseURL = serverURL
	return u
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

// TestUber_FetchJobListing verifies that the adapter correctly:
//   - hits the filter-options endpoint once,
//   - hits the search endpoint once per department,
//   - returns all jobs with JobContent set (fat adapter).
func TestUber_FetchJobListing(t *testing.T) {
	filterFixture := loadFixture(t, "filter_options.json")
	searchFixture := loadFixture(t, "search_jobs.json")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "loadFilterOptions"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(filterFixture)
		case strings.Contains(r.URL.Path, "loadSearchJobsResults"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(searchFixture)
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	result, err := newTestAdapter(srv.URL).FetchJobListing(context.Background(), "ignored")
	if err != nil {
		t.Fatalf("FetchJobListing error: %v", err)
	}

	// filter_options.json has 2 departments; search_jobs.json has 2 jobs per call.
	// Both departments return the same 2 jobs → after dedup 2 unique jobs remain.
	if len(result.Jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(result.Jobs))
	}

	j0 := result.Jobs[0]
	if j0.ID != "uber-157927" {
		t.Errorf("jobs[0].ID = %q, want uber-157927", j0.ID)
	}
	if j0.Title != "Software Engineer II - Discovery & Monetization" {
		t.Errorf("jobs[0].Title = %q", j0.Title)
	}
	if j0.JobContent == nil {
		t.Fatal("jobs[0].JobContent is nil — fat adapter must always set it")
	}
	if j0.JobContent.Description == "" {
		t.Error("jobs[0].JobContent.Description is empty")
	}
	if j0.JobContent.Department != "Engineering" {
		t.Errorf("jobs[0].JobContent.Department = %q, want Engineering", j0.JobContent.Department)
	}
	if j0.JobContent.PublishedDate != "2026-03-10T00:00:00.000Z" {
		t.Errorf("jobs[0].JobContent.PublishedDate = %q", j0.JobContent.PublishedDate)
	}
	if !j0.LocationKnown {
		t.Error("jobs[0].LocationKnown should be true")
	}
	// URL must point to the test server base, not production Uber.
	if !strings.Contains(j0.URL, "/careers/list/157927") {
		t.Errorf("jobs[0].URL = %q, want contains /careers/list/157927", j0.URL)
	}
}

// TestUber_FetchJobListing_Deduplication verifies that a job appearing in
// multiple department responses is returned only once.
func TestUber_FetchJobListing_Deduplication(t *testing.T) {
	// filter_options.json has 2 departments; search_jobs.json returns the same
	// 2 jobs for every department. After dedup only 2 unique jobs should remain.
	filterFixture := loadFixture(t, "filter_options.json")
	searchFixture := loadFixture(t, "search_jobs.json")

	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "loadFilterOptions"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(filterFixture)
		case strings.Contains(r.URL.Path, "loadSearchJobsResults"):
			callCount++
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(searchFixture)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	result, err := newTestAdapter(srv.URL).FetchJobListing(context.Background(), "ignored")
	if err != nil {
		t.Fatalf("FetchJobListing error: %v", err)
	}

	// 2 departments → 2 search calls
	if callCount != 2 {
		t.Errorf("expected 2 search calls, got %d", callCount)
	}
	// Both departments return the same 2 jobs, so dedup should leave 2 unique jobs.
	if len(result.Jobs) != 2 {
		t.Errorf("expected 2 unique jobs after dedup, got %d", len(result.Jobs))
	}

	// Verify IDs are distinct.
	ids := make(map[string]struct{})
	for _, j := range result.Jobs {
		if _, dup := ids[j.ID]; dup {
			t.Errorf("duplicate job ID found: %s", j.ID)
		}
		ids[j.ID] = struct{}{}
	}
}

// TestUber_FetchJobListing_CorrectHeaders verifies that every request carries
// the mandatory x-csrf-token header.
func TestUber_FetchJobListing_CorrectHeaders(t *testing.T) {
	filterFixture := loadFixture(t, "filter_options.json")
	searchFixture := loadFixture(t, "search_jobs.json")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("x-csrf-token")
		if token != "x" {
			t.Errorf("x-csrf-token header = %q, want x", token)
		}
		contentType := r.Header.Get("Content-Type")
		if contentType != "application/json" {
			t.Errorf("Content-Type header = %q, want application/json", contentType)
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "loadFilterOptions"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(filterFixture)
		case strings.Contains(r.URL.Path, "loadSearchJobsResults"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(searchFixture)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	_, err := newTestAdapter(srv.URL).FetchJobListing(context.Background(), "ignored")
	if err != nil {
		t.Fatalf("FetchJobListing error: %v", err)
	}
}

// TestUber_FetchJobListing_FilterOptionsError verifies that a non-200 from the
// filter-options endpoint is propagated as an error.
func TestUber_FetchJobListing_FilterOptionsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := newTestAdapter(srv.URL).FetchJobListing(context.Background(), "ignored")
	if err == nil {
		t.Fatal("expected error from filter-options 500, got nil")
	}
}

// TestUber_FetchJobListing_SearchError verifies that a non-200 from the
// search endpoint is propagated as an error.
func TestUber_FetchJobListing_SearchError(t *testing.T) {
	filterFixture := loadFixture(t, "filter_options.json")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "loadFilterOptions"):
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(filterFixture)
		default:
			// search endpoint returns 500
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	_, err := newTestAdapter(srv.URL).FetchJobListing(context.Background(), "ignored")
	if err == nil {
		t.Fatal("expected error from search 500, got nil")
	}
}

// TestUber_FetchJobListing_Errors covers malformed JSON from the filter-options
// endpoint.
func TestUber_FetchJobListing_Errors(t *testing.T) {
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
				// All requests to filter-options (and search) return the error condition.
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
