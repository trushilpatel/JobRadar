package apple

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

// newTestAdapter builds an Apple adapter pointed at the given test server URL.
// Goes through New() so tests exercise the same construction path as production.
func newTestAdapter(serverURL string) *Apple {
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

// ── extractHydrationData ──────────────────────────────────────────────────────

func TestExtractHydrationData(t *testing.T) {
	const jsonObj = `{"loaderData":{"search":{"searchResults":[],"totalRecords":0}}}`

	tests := []struct {
		name    string
		body    string
		want    string
		wantErr bool
	}{
		{
			name: "direct object",
			body: `<script>window.__staticRouterHydrationData = ` + jsonObj + `;</script>`,
			want: jsonObj,
		},
		{
			name: "JSON.parse double-quote",
			// The JSON body needs to have inner quotes escaped when placed inside a JSON string.
			body: `<script>window.__staticRouterHydrationData = JSON.parse("{\"loaderData\":{\"search\":{\"searchResults\":[],\"totalRecords\":0}}}");</script>`,
			want: jsonObj,
		},
		{
			name: "JSON.parse single-quote",
			body: `<script>window.__staticRouterHydrationData = JSON.parse('{"loaderData":{"search":{"searchResults":[],"totalRecords":0}}}');</script>`,
			want: jsonObj,
		},
		{
			name:    "marker not found",
			body:    `<html><body>no hydration data here</body></html>`,
			wantErr: true,
		},
		{
			name:    "missing closing script tag for direct object",
			body:    `<script>window.__staticRouterHydrationData = ` + jsonObj,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractHydrationData(tt.body)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (result=%q)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("extractHydrationData:\n got  %q\n want %q", got, tt.want)
			}
		})
	}
}

// ── FetchJobListing ───────────────────────────────────────────────────────────

func TestApple_FetchJobListing(t *testing.T) {
	listingHTML := loadFixture(t, "listing.html")
	callCount := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		// Verify User-Agent header is set
		if r.Header.Get("User-Agent") == "" {
			t.Error("User-Agent header not set")
		}
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		if callCount == 1 {
			// Page 1: return 3 jobs
			_, _ = w.Write(listingHTML)
		} else {
			// Page 2: empty results → stop pagination
			_, _ = w.Write([]byte(`<!DOCTYPE html><html><body><script>window.__staticRouterHydrationData = {"loaderData":{"search":{"searchResults":[],"totalRecords":0}}};</script></body></html>`))
		}
	}))
	defer srv.Close()

	result, err := newTestAdapter(srv.URL).FetchJobListing(context.Background(), "ignored")
	if err != nil {
		t.Fatalf("FetchJobListing error: %v", err)
	}

	// The fixture has 3 jobs total: 2 US + 1 UK (non-US).
	// The adapter fetches ALL jobs in the listing (no server-side US filter in this path —
	// the listing endpoint itself is already filtered by location=united-states-USA).
	// All 3 jobs returned by the mock are included in the listing.
	if len(result.Jobs) != 3 {
		t.Fatalf("expected 3 jobs from listing, got %d", len(result.Jobs))
	}

	// Verify first job fields
	j0 := result.Jobs[0]
	if j0.ID != "apple-200647207" {
		t.Errorf("jobs[0].ID = %q, want apple-200647207", j0.ID)
	}
	if j0.Title != "Software Engineer" {
		t.Errorf("jobs[0].Title = %q, want Software Engineer", j0.Title)
	}
	if !strings.Contains(j0.URL, "200647207") {
		t.Errorf("jobs[0].URL %q should contain position ID", j0.URL)
	}
	if !strings.Contains(j0.URL, "software-engineer") {
		t.Errorf("jobs[0].URL %q should contain transformedPostingTitle", j0.URL)
	}
	if j0.EmployType != core.EmployFullTime {
		t.Errorf("jobs[0].EmployType = %q, want %q", j0.EmployType, core.EmployFullTime)
	}
	if j0.Location != "Cupertino, California" {
		t.Errorf("jobs[0].Location = %q, want Cupertino, California", j0.Location)
	}
	if !j0.LocationKnown {
		t.Error("jobs[0].LocationKnown should be true for US location")
	}
	if len(j0.AllLocations) == 0 {
		t.Error("jobs[0].AllLocations should not be empty")
	}

	// Verify second job
	j1 := result.Jobs[1]
	if j1.ID != "apple-200647208" {
		t.Errorf("jobs[1].ID = %q, want apple-200647208", j1.ID)
	}
	if j1.Title != "Product Manager" {
		t.Errorf("jobs[1].Title = %q, want Product Manager", j1.Title)
	}
}

func TestApple_FetchJobListing_Pagination(t *testing.T) {
	listingHTML := loadFixture(t, "listing.html")

	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		if callCount == 1 {
			// Page 1: return jobs
			_, _ = w.Write(listingHTML)
		} else {
			// Page 2: return empty results → stop pagination
			_, _ = w.Write([]byte(`<!DOCTYPE html><html><body><script>window.__staticRouterHydrationData = {"loaderData":{"search":{"searchResults":[],"totalRecords":0}}};</script></body></html>`))
		}
	}))
	defer srv.Close()

	result, err := newTestAdapter(srv.URL).FetchJobListing(context.Background(), "ignored")
	if err != nil {
		t.Fatalf("FetchJobListing error: %v", err)
	}
	if callCount != 2 {
		t.Errorf("expected 2 HTTP calls (page 1 + empty page 2), got %d", callCount)
	}
	if len(result.Jobs) != 3 {
		t.Errorf("expected 3 jobs from page 1, got %d", len(result.Jobs))
	}
}

func TestApple_FetchJobListing_Errors(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{"http 500", http.StatusInternalServerError, ""},
		{"no hydration data", http.StatusOK, "<html><body>no hydration here</body></html>"},
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

func TestApple_FetchJobPost(t *testing.T) {
	jobPostHTML := loadFixture(t, "job_post.html")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(jobPostHTML)
	}))
	defer srv.Close()

	summary := core.JobSummary{
		ID:    "apple-200647207",
		Title: "Software Engineer",
		URL:   fmt.Sprintf("%s/en-us/details/200647207/software-engineer", srv.URL),
	}
	post, err := newTestAdapter(srv.URL).FetchJobPost(context.Background(), summary)
	if err != nil {
		t.Fatalf("FetchJobPost error: %v", err)
	}

	if post.ID != "apple-200647207" {
		t.Errorf("post.ID = %q, want apple-200647207", post.ID)
	}
	if post.Description == "" {
		t.Error("post.Description is empty")
	}
	// Description should contain labeled sections
	if !strings.Contains(post.Description, "Summary:") {
		t.Errorf("post.Description missing Summary section: %q", post.Description)
	}
	if !strings.Contains(post.Description, "Description:") {
		t.Errorf("post.Description missing Description section: %q", post.Description)
	}
	if !strings.Contains(post.Description, "Responsibilities:") {
		t.Errorf("post.Description missing Responsibilities section: %q", post.Description)
	}
	if !strings.Contains(post.Description, "Requirements:") {
		t.Errorf("post.Description missing Requirements section: %q", post.Description)
	}
	if !strings.Contains(post.Description, "Preferred:") {
		t.Errorf("post.Description missing Preferred section: %q", post.Description)
	}

	// Pay range should be extracted
	if post.Compensation == "" {
		t.Error("post.Compensation is empty, expected pay range")
	}
	if !strings.Contains(post.Compensation, "$150,000") {
		t.Errorf("post.Compensation %q should contain $150,000", post.Compensation)
	}

	// Location and country code
	if post.Location == "" {
		t.Error("post.Location is empty")
	}
	if post.CountryCode != "US" {
		t.Errorf("post.CountryCode = %q, want US", post.CountryCode)
	}
}

func TestApple_FetchJobPost_Errors(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{"http 404", http.StatusNotFound, ""},
		{"no hydration data", http.StatusOK, "<html><body>no hydration here</body></html>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			summary := core.JobSummary{
				ID:  "apple-200647207",
				URL: fmt.Sprintf("%s/en-us/details/200647207/software-engineer", srv.URL),
			}
			_, err := newTestAdapter(srv.URL).FetchJobPost(context.Background(), summary)
			if err == nil {
				t.Fatalf("expected error for %s, got nil", tt.name)
			}
		})
	}
}

// ── extractHydrationData edge cases ──────────────────────────────────────────

func TestExtractHydrationData_EdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr bool
	}{
		{
			name:    "JSON.parse empty arg",
			body:    `<script>window.__staticRouterHydrationData = JSON.parse(</script>`,
			wantErr: true,
		},
		{
			name:    "JSON.parse unexpected quote char",
			body:    `<script>window.__staticRouterHydrationData = JSON.parse(` + "`" + `{}` + "`" + `);</script>`,
			wantErr: true,
		},
		{
			name:    "JSON.parse double-quote unclosed string",
			body:    `<script>window.__staticRouterHydrationData = JSON.parse("unclosed</script>`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := extractHydrationData(tt.body)
			if tt.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// ── Default base URL (production path) ───────────────────────────────────────

func TestNew_DefaultBaseURL(t *testing.T) {
	a := New(logger.NewNoop(), http.DefaultClient)
	// baseURL is empty for production adapters; resolveBaseURL returns default.
	if a.resolveBaseURL() != defaultBaseURL {
		t.Errorf("resolveBaseURL() = %q, want %q", a.resolveBaseURL(), defaultBaseURL)
	}
}

// ── Context cancellation ──────────────────────────────────────────────────────

func TestApple_FetchJobListing_ContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return 3 jobs so pagination loop would request another page
		listingHTML := loadFixture(t, "listing.html")
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(listingHTML)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := newTestAdapter(srv.URL).FetchJobListing(ctx, "ignored")
	// Should return context.Canceled
	if err == nil {
		t.Fatal("expected context error, got nil")
	}
}
