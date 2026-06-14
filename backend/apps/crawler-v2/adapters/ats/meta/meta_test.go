package meta

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/trushilpatel/jobradar/apps/crawler-v2/common/logger"
	"github.com/trushilpatel/jobradar/common/ats/core"
)

// newTestAdapter constructs a Meta adapter pointed at the given base URL.
// Uses the same New() constructor as production code.
func newTestAdapter(baseURL string) *Meta {
	m := New(logger.NewNoop(), http.DefaultClient)
	m.baseURL = baseURL
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

// ── Interface compliance ──────────────────────────────────────────────────────

func TestMeta_ImplementsInterfaces(t *testing.T) {
	var _ core.ListingFetcher = (*Meta)(nil)
	var _ core.PostFetcher = (*Meta)(nil)
}

// ── FetchJobListing ───────────────────────────────────────────────────────────

func TestMeta_FetchJobListing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/jobsearch/sitemap.xml" {
			t.Errorf("unexpected path %q, want /jobsearch/sitemap.xml", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(loadFixture(t, "sitemap.xml"))
	}))
	defer srv.Close()

	listing, err := newTestAdapter(srv.URL).FetchJobListing(context.Background(), "ignored")
	if err != nil {
		t.Fatalf("FetchJobListing error: %v", err)
	}
	// sitemap.xml has 3 <loc> entries but only 2 are /profile/job_details/ paths
	if len(listing.Jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d: %+v", len(listing.Jobs), listing.Jobs)
	}
	if listing.Jobs[0].ID != "meta-1234567890" {
		t.Errorf("jobs[0].ID = %q, want meta-1234567890", listing.Jobs[0].ID)
	}
	if listing.Jobs[1].ID != "meta-9876543210" {
		t.Errorf("jobs[1].ID = %q, want meta-9876543210", listing.Jobs[1].ID)
	}
	if listing.Jobs[0].Title != "" {
		t.Errorf("jobs[0].Title should be empty at listing time, got %q", listing.Jobs[0].Title)
	}
	if listing.Jobs[0].Location != "" {
		t.Errorf("jobs[0].Location should be empty at listing time, got %q", listing.Jobs[0].Location)
	}
	if !strings.Contains(listing.Jobs[0].URL, "1234567890") {
		t.Errorf("jobs[0].URL %q should contain job ID", listing.Jobs[0].URL)
	}
}

func TestMeta_FetchJobListing_SendsBrowserHeaders(t *testing.T) {
	var capturedHeaders http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeaders = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<?xml version="1.0"?><urlset></urlset>`))
	}))
	defer srv.Close()

	_, _ = newTestAdapter(srv.URL).FetchJobListing(context.Background(), "ignored")

	if ua := capturedHeaders.Get("User-Agent"); !strings.Contains(ua, "Chrome") {
		t.Errorf("User-Agent = %q, should be Chrome UA", ua)
	}
	if capturedHeaders.Get("Sec-Fetch-Dest") == "" {
		t.Error("Sec-Fetch-Dest header missing")
	}
	if capturedHeaders.Get("Sec-Fetch-Mode") == "" {
		t.Error("Sec-Fetch-Mode header missing")
	}
}

func TestMeta_FetchJobListing_Errors(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{"http 500", http.StatusInternalServerError, ""},
		{"http 429", http.StatusTooManyRequests, ""},
		{"http 403", http.StatusForbidden, ""},
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

func TestMeta_FetchJobListing_EmptyXML(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<?xml version="1.0"?><urlset></urlset>`))
	}))
	defer srv.Close()

	listing, err := newTestAdapter(srv.URL).FetchJobListing(context.Background(), "ignored")
	if err != nil {
		t.Fatalf("unexpected error for empty XML: %v", err)
	}
	if len(listing.Jobs) != 0 {
		t.Errorf("expected 0 jobs for empty XML, got %d", len(listing.Jobs))
	}
}

// ── FetchJobPost ──────────────────────────────────────────────────────────────

func TestMeta_FetchJobPost(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(loadFixture(t, "job_detail.html"))
	}))
	defer srv.Close()

	summary := core.JobSummary{
		ID:  "meta-1234567890",
		URL: srv.URL + "/detail",
	}
	post, err := newTestAdapter("").FetchJobPost(context.Background(), summary)
	if err != nil {
		t.Fatalf("FetchJobPost error: %v", err)
	}
	if post.ID != "meta-1234567890" {
		t.Errorf("post.ID = %q, want meta-1234567890", post.ID)
	}
	if post.Title != "Software Engineer, Infrastructure" {
		t.Errorf("post.Title = %q", post.Title)
	}
	if post.Description == "" {
		t.Error("post.Description is empty")
	}
	if post.PublishedDate != "2024-01-15" {
		t.Errorf("post.PublishedDate = %q, want 2024-01-15", post.PublishedDate)
	}
	if post.CountryCode != "US" {
		t.Errorf("post.CountryCode = %q, want US", post.CountryCode)
	}
	if post.Department != "Meta" {
		t.Errorf("post.Department = %q, want Meta", post.Department)
	}
}

func TestMeta_FetchJobPost_UsesURLFromSummary(t *testing.T) {
	var capturedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(loadFixture(t, "job_detail.html"))
	}))
	defer srv.Close()

	summary := core.JobSummary{
		ID:  "meta-42",
		URL: srv.URL + "/profile/job_details/42",
	}
	_, _ = newTestAdapter("").FetchJobPost(context.Background(), summary)
	if capturedPath != "/profile/job_details/42" {
		t.Errorf("path = %q, want /profile/job_details/42", capturedPath)
	}
}

func TestMeta_FetchJobPost_ConstructsURLFromID(t *testing.T) {
	// When summary.URL is empty, adapter constructs URL from ID.
	// We can't easily test the full URL construction without network — but we
	// can verify the error comes from the network call (not ID parsing).
	m := New(logger.NewNoop(), http.DefaultClient)
	summary := core.JobSummary{
		ID:  "meta-99999",
		URL: "", // empty — should construct from ID
	}
	ctx := context.Background()
	_, err := m.FetchJobPost(ctx, summary)
	// Will fail with a network error (no such host / dial error), not a panic.
	if err == nil {
		t.Fatal("expected error when hitting real URL without test server")
	}
	if !strings.Contains(err.Error(), "meta: post:") {
		t.Errorf("error prefix mismatch: %v", err)
	}
}

func TestMeta_FetchJobPost_Errors(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{"http 404", http.StatusNotFound, ""},
		{"http 500", http.StatusInternalServerError, ""},
		{"html with no json-ld", http.StatusOK, "<html><body>no schema</body></html>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			summary := core.JobSummary{
				ID:  "meta-1234",
				URL: srv.URL + "/detail",
			}
			_, err := newTestAdapter("").FetchJobPost(context.Background(), summary)
			if err == nil {
				t.Fatalf("expected error for %s, got nil", tt.name)
			}
		})
	}
}

func TestMeta_FetchJobPost_SendsBrowserHeaders(t *testing.T) {
	var capturedHeaders http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeaders = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(loadFixture(t, "job_detail.html"))
	}))
	defer srv.Close()

	summary := core.JobSummary{
		ID:  "meta-1234567890",
		URL: srv.URL + "/detail",
	}
	_, _ = newTestAdapter("").FetchJobPost(context.Background(), summary)

	if ua := capturedHeaders.Get("User-Agent"); !strings.Contains(ua, "Chrome") {
		t.Errorf("User-Agent = %q, should be Chrome UA", ua)
	}
	if capturedHeaders.Get("Sec-Fetch-Dest") == "" {
		t.Error("Sec-Fetch-Dest header missing")
	}
}

// ── resolveDetailURL ──────────────────────────────────────────────────────────

func TestResolveDetailURL_UsesURL(t *testing.T) {
	m := New(logger.NewNoop(), http.DefaultClient)
	summary := core.JobSummary{
		ID:  "meta-123",
		URL: "https://www.metacareers.com/profile/job_details/123",
	}
	got := m.resolveDetailURL(summary)
	if got != "https://www.metacareers.com/profile/job_details/123" {
		t.Errorf("got %q, want canonical URL", got)
	}
}

func TestResolveDetailURL_ConstructsFromID(t *testing.T) {
	m := New(logger.NewNoop(), http.DefaultClient)
	summary := core.JobSummary{ID: "meta-456", URL: ""}
	got := m.resolveDetailURL(summary)
	if got != "https://www.metacareers.com/profile/job_details/456" {
		t.Errorf("got %q, want https://www.metacareers.com/profile/job_details/456", got)
	}
}

// ── resolveSitemapURL ─────────────────────────────────────────────────────────

func TestResolveSitemapURL_ProductionURL(t *testing.T) {
	m := New(logger.NewNoop(), http.DefaultClient)
	// No baseURL set — should return the production sitemapURL.
	got := m.resolveSitemapURL()
	if got != "https://www.metacareers.com/jobsearch/sitemap.xml" {
		t.Errorf("got %q, want production sitemap URL", got)
	}
}

func TestResolveSitemapURL_TestOverride(t *testing.T) {
	m := New(logger.NewNoop(), http.DefaultClient)
	m.baseURL = "http://localhost:9999"
	got := m.resolveSitemapURL()
	want := "http://localhost:9999/jobsearch/sitemap.xml"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
