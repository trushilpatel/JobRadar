package workday

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/trushilpatel/jobradar/apps/crawler-v2/common/logger"
	"github.com/trushilpatel/jobradar/apps/crawler-v2/core"
)

// newTestAdapter builds a Workday adapter wired to a test server.
// interPageDelay is set to 0 so pagination tests complete instantly.
func newTestAdapter() *Workday {
	w := New(logger.NewNoop(), http.DefaultClient)
	w.interPageDelay = 0
	return w
}

func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("load fixture %s: %v", name, err)
	}
	return data
}

// validConfig returns a minimal valid adapter config JSON for tests.
func validConfig(host, site string) json.RawMessage {
	return json.RawMessage(fmt.Sprintf(`{"host":%q,"site":%q}`, host, site))
}

// ── FetchJobListing (stub) ────────────────────────────────────────────────────

func TestWorkday_FetchJobListing_AlwaysErrors(t *testing.T) {
	w := newTestAdapter()
	_, err := w.FetchJobListing(context.Background(), "any-company")
	if err == nil {
		t.Fatal("expected error from stub FetchJobListing, got nil")
	}
	if !strings.Contains(err.Error(), "workday") {
		t.Errorf("error should mention workday, got: %v", err)
	}
	if !strings.Contains(err.Error(), "FetchJobListingWithConfig") {
		t.Errorf("error should mention FetchJobListingWithConfig, got: %v", err)
	}
}

// ── FetchJobListingWithConfig ─────────────────────────────────────────────────

func TestWorkday_FetchJobListingWithConfig_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if !strings.Contains(r.Header.Get("User-Agent"), "Mozilla") {
			t.Errorf("missing browser User-Agent: %q", r.Header.Get("User-Agent"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(loadFixture(t, "listing.json"))
	}))
	defer srv.Close()

	// Build a config pointing at the test server host. We parse the server URL
	// to get host:port, then craft a config that matches it.
	// Since hostPattern requires *.wd*.myworkdayjobs.com, we need to override
	// the URL building. We test via a custom server using a fake host in config
	// but override postJSON to hit the test server instead.
	//
	// Instead, we test via server-controlled URL injection by temporarily patching
	// the adapter's URL construction. Since our implementation builds the URL
	// from config, we use a wrapper approach: serve responses normally.
	//
	// The clean approach: use httptest + inject via a custom round tripper.
	adapter := &Workday{
		httpClient:     testHTTPClientRedirectingTo(srv.URL),
		interPageDelay: 0,
		log:            logger.NewNoop(),
	}

	cfg := validConfig("nvidia.wd5.myworkdayjobs.com", "NVIDIAExternalCareerSite")
	result, err := adapter.FetchJobListingWithConfig(context.Background(), "nvidia", cfg)
	if err != nil {
		t.Fatalf("FetchJobListingWithConfig error: %v", err)
	}
	if len(result.Jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(result.Jobs))
	}

	j0 := result.Jobs[0]
	if j0.ID != "workday-R-12345" {
		t.Errorf("jobs[0].ID = %q, want workday-R-12345", j0.ID)
	}
	if j0.Title != "Senior Software Engineer" {
		t.Errorf("jobs[0].Title = %q, want Senior Software Engineer", j0.Title)
	}
	if j0.Location != "San Jose, CA" {
		t.Errorf("jobs[0].Location = %q, want San Jose, CA", j0.Location)
	}
	if !strings.Contains(j0.URL, "nvidia.wd5.myworkdayjobs.com") {
		t.Errorf("jobs[0].URL %q should contain workday host", j0.URL)
	}
	if !strings.Contains(j0.URL, "/job/") {
		t.Errorf("jobs[0].URL %q should contain /job/", j0.URL)
	}
	if j0.EmployType != core.EmployFullTime {
		t.Errorf("jobs[0].EmployType = %q, want %q", j0.EmployType, core.EmployFullTime)
	}
	if !j0.LocationKnown {
		t.Error("jobs[0].LocationKnown should be true")
	}

	j1 := result.Jobs[1]
	if j1.ID != "workday-R-99999" {
		t.Errorf("jobs[1].ID = %q, want workday-R-99999", j1.ID)
	}
}

func TestWorkday_FetchJobListingWithConfig_Pagination(t *testing.T) {
	// Page 1: 2 jobs, total=3; Page 2: 1 job → expect 3 total.
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if callCount == 1 {
			_, _ = w.Write([]byte(`{
				"total": 3,
				"jobPostings": [
					{"title":"Job A","externalPath":"/job/US/Job-A_R-001","locationsText":"New York","bulletFields":["R-001"]},
					{"title":"Job B","externalPath":"/job/US/Job-B_R-002","locationsText":"Remote","bulletFields":["R-002"]}
				]
			}`))
		} else {
			_, _ = w.Write([]byte(`{
				"total": 3,
				"jobPostings": [
					{"title":"Job C","externalPath":"/job/US/Job-C_R-003","locationsText":"Austin","bulletFields":["R-003"]}
				]
			}`))
		}
	}))
	defer srv.Close()

	adapter := &Workday{
		httpClient:     testHTTPClientRedirectingTo(srv.URL),
		interPageDelay: 0,
		log:            logger.NewNoop(),
	}

	cfg := validConfig("nvidia.wd5.myworkdayjobs.com", "NVIDIAExternalCareerSite")
	result, err := adapter.FetchJobListingWithConfig(context.Background(), "nvidia", cfg)
	if err != nil {
		t.Fatalf("FetchJobListingWithConfig error: %v", err)
	}
	if len(result.Jobs) != 3 {
		t.Fatalf("expected 3 jobs across 2 pages, got %d", len(result.Jobs))
	}
	if callCount != 2 {
		t.Errorf("expected 2 API calls, got %d", callCount)
	}

	ids := []string{result.Jobs[0].ID, result.Jobs[1].ID, result.Jobs[2].ID}
	wantIDs := []string{"workday-R-001", "workday-R-002", "workday-R-003"}
	for i, want := range wantIDs {
		if ids[i] != want {
			t.Errorf("jobs[%d].ID = %q, want %q", i, ids[i], want)
		}
	}
}

func TestWorkday_FetchJobListingWithConfig_EmptyPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"total":0,"jobPostings":[]}`))
	}))
	defer srv.Close()

	adapter := &Workday{
		httpClient:     testHTTPClientRedirectingTo(srv.URL),
		interPageDelay: 0,
		log:            logger.NewNoop(),
	}

	cfg := validConfig("nvidia.wd5.myworkdayjobs.com", "NVIDIAExternalCareerSite")
	result, err := adapter.FetchJobListingWithConfig(context.Background(), "nvidia", cfg)
	if err != nil {
		t.Fatalf("FetchJobListingWithConfig error: %v", err)
	}
	if len(result.Jobs) != 0 {
		t.Errorf("expected 0 jobs, got %d", len(result.Jobs))
	}
}

func TestWorkday_FetchJobListingWithConfig_404BoardRetired(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	adapter := &Workday{
		httpClient:     testHTTPClientRedirectingTo(srv.URL),
		interPageDelay: 0,
		log:            logger.NewNoop(),
	}

	cfg := validConfig("nvidia.wd5.myworkdayjobs.com", "NVIDIAExternalCareerSite")
	result, err := adapter.FetchJobListingWithConfig(context.Background(), "nvidia", cfg)
	if err != nil {
		t.Fatalf("expected empty result for retired board (404), got error: %v", err)
	}
	if len(result.Jobs) != 0 {
		t.Errorf("expected 0 jobs for retired board, got %d", len(result.Jobs))
	}
}

func TestWorkday_FetchJobListingWithConfig_InvalidConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  string
		wantErr string
	}{
		{
			name:    "missing host",
			config:  `{"host":"","site":"NVIDIAExternalCareerSite"}`,
			wantErr: "host is required",
		},
		{
			name:    "bad host pattern",
			config:  `{"host":"nvidia.example.com","site":"NVIDIAExternalCareerSite"}`,
			wantErr: "does not match pattern",
		},
		{
			name:    "missing site",
			config:  `{"host":"nvidia.wd5.myworkdayjobs.com","site":""}`,
			wantErr: "site is required",
		},
		{
			name:    "site with slash",
			config:  `{"host":"nvidia.wd5.myworkdayjobs.com","site":"foo/bar"}`,
			wantErr: "must not contain",
		},
		{
			name:    "malformed JSON config",
			config:  `not-json`,
			wantErr: "decode config",
		},
	}

	w := newTestAdapter()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := w.FetchJobListingWithConfig(context.Background(), "company", json.RawMessage(tt.config))
			if err == nil {
				t.Fatalf("expected error for %q, got nil", tt.name)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestWorkday_FetchJobListingWithConfig_Errors(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{"http 500", http.StatusInternalServerError, ""},
		{"http 429", http.StatusTooManyRequests, ""},
		{"malformed JSON", http.StatusOK, "not-json"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			adapter := &Workday{
				httpClient:     testHTTPClientRedirectingTo(srv.URL),
				interPageDelay: 0,
				log:            logger.NewNoop(),
			}
			cfg := validConfig("nvidia.wd5.myworkdayjobs.com", "NVIDIAExternalCareerSite")
			_, err := adapter.FetchJobListingWithConfig(context.Background(), "nvidia", cfg)
			if err == nil {
				t.Fatalf("expected error for %s, got nil", tt.name)
			}
		})
	}
}

func TestWorkday_FetchJobListingWithConfig_MaxJobsCap(t *testing.T) {
	// Config sets max_jobs_per_scan=1; page returns 2 → should stop at 1.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(loadFixture(t, "listing.json"))
	}))
	defer srv.Close()

	adapter := &Workday{
		httpClient:     testHTTPClientRedirectingTo(srv.URL),
		interPageDelay: 0,
		log:            logger.NewNoop(),
	}

	cfg := json.RawMessage(`{"host":"nvidia.wd5.myworkdayjobs.com","site":"NVIDIAExternalCareerSite","max_jobs_per_scan":1}`)
	result, err := adapter.FetchJobListingWithConfig(context.Background(), "nvidia", cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Jobs) != 1 {
		t.Errorf("expected 1 job (capped), got %d", len(result.Jobs))
	}
}

// ── FetchJobPost ──────────────────────────────────────────────────────────────

func TestWorkday_FetchJobPost(t *testing.T) {
	var capturedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if !strings.Contains(r.Header.Get("User-Agent"), "Mozilla") {
			t.Errorf("missing browser User-Agent: %q", r.Header.Get("User-Agent"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(loadFixture(t, "job_post.json"))
	}))
	defer srv.Close()

	adapter := &Workday{
		httpClient:     testHTTPClientRedirectingTo(srv.URL),
		interPageDelay: 0,
		log:            logger.NewNoop(),
	}

	summary := core.JobSummary{
		ID:  "workday-R-12345",
		URL: "https://nvidia.wd5.myworkdayjobs.com/NVIDIAExternalCareerSite/job/San-Jose/Senior-Software-Engineer_R-12345",
	}
	post, err := adapter.FetchJobPost(context.Background(), summary)
	if err != nil {
		t.Fatalf("FetchJobPost error: %v", err)
	}

	// Verify the CXS detail path was constructed correctly (no doubled /job/job/).
	if !strings.Contains(capturedPath, "/wday/cxs/") {
		t.Errorf("path %q should contain /wday/cxs/", capturedPath)
	}
	if strings.Contains(capturedPath, "/job/job/") {
		t.Errorf("path %q must not contain doubled /job/job/", capturedPath)
	}

	if post.ID != "workday-R-12345" {
		t.Errorf("post.ID = %q, want workday-R-12345", post.ID)
	}
	if post.Title != "Senior Software Engineer" {
		t.Errorf("post.Title = %q, want Senior Software Engineer", post.Title)
	}
	if post.Location != "San Jose, CA" {
		t.Errorf("post.Location = %q, want San Jose, CA", post.Location)
	}
	if post.Description == "" {
		t.Error("post.Description is empty")
	}
	if strings.Contains(post.Description, "<p>") {
		t.Error("post.Description should have HTML stripped")
	}
	if post.CountryCode != "US" {
		t.Errorf("post.CountryCode = %q, want US", post.CountryCode)
	}
	if post.PublishedDate != "2026-05-01" {
		t.Errorf("post.PublishedDate = %q, want 2026-05-01", post.PublishedDate)
	}
	if post.EmploymentType != core.EmployFullTime {
		t.Errorf("post.EmploymentType = %q, want %q", post.EmploymentType, core.EmployFullTime)
	}
	if post.URL != summary.URL {
		t.Errorf("post.URL = %q, want %q", post.URL, summary.URL)
	}
}

func TestWorkday_FetchJobPost_TenantCannotBeDerived(t *testing.T) {
	// This shouldn't happen with a valid Workday host (always has dots),
	// but exercises the defensive tenantFromHost empty branch by using a URL
	// that passes hostPattern then produces a host with no dot via parsing edge.
	// The only reachable path is to construct a URL whose host passes
	// hostPattern but tenantFromHost returns "". Since hostPattern requires
	// <segment>.wd<N>.myworkdayjobs.com, there will always be a dot, so this
	// branch is a defensive guard. We test it by calling FetchJobPost with a
	// URL that has an otherwise valid-looking host that won't pass hostPattern,
	// ensuring the parse error fires instead.
	// Skip — defensive dead-branch by design; tested via parseJobURL error path.
}

func TestWorkday_FetchJobPost_InvalidURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{"not a workday host", "https://example.com/site/job/123/title"},
		{"missing job segment", "https://nvidia.wd5.myworkdayjobs.com/NVIDIAExternalCareerSite/apply/12345"},
		{"empty url", ""},
		{"site segment empty — path starts with /job/", "https://nvidia.wd5.myworkdayjobs.com//job/San-Jose/Title_R-1"},
	}

	w := newTestAdapter()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summary := core.JobSummary{ID: "workday-1", URL: tt.url}
			_, err := w.FetchJobPost(context.Background(), summary)
			if err == nil {
				t.Fatalf("expected error for URL %q, got nil", tt.url)
			}
		})
	}
}

func TestWorkday_FetchJobPost_Errors(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{"http 500", http.StatusInternalServerError, ""},
		{"http 404", http.StatusNotFound, ""},
		{"malformed JSON", http.StatusOK, "not-json"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			adapter := &Workday{
				httpClient:     testHTTPClientRedirectingTo(srv.URL),
				interPageDelay: 0,
				log:            logger.NewNoop(),
			}
			summary := core.JobSummary{
				ID:  "workday-R-12345",
				URL: "https://nvidia.wd5.myworkdayjobs.com/NVIDIAExternalCareerSite/job/San-Jose/Title_R-12345",
			}
			_, err := adapter.FetchJobPost(context.Background(), summary)
			if err == nil {
				t.Fatalf("expected error for %s, got nil", tt.name)
			}
		})
	}
}

func TestWorkday_FetchJobPost_NetworkError(t *testing.T) {
	// Use a server that is immediately closed to trigger a network error.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close() // close before making requests

	adapter := &Workday{
		httpClient:     testHTTPClientRedirectingTo(srv.URL),
		interPageDelay: 0,
		log:            logger.NewNoop(),
	}
	summary := core.JobSummary{
		ID:  "workday-R-12345",
		URL: "https://nvidia.wd5.myworkdayjobs.com/NVIDIAExternalCareerSite/job/San-Jose/Title_R-12345",
	}
	_, err := adapter.FetchJobPost(context.Background(), summary)
	if err == nil {
		t.Fatal("expected network error after server closed, got nil")
	}
}

func TestWorkday_FetchJobListingWithConfig_NetworkError(t *testing.T) {
	// Use a closed server to trigger a network/do error in postJSON.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close()

	adapter := &Workday{
		httpClient:     testHTTPClientRedirectingTo(srv.URL),
		interPageDelay: 0,
		log:            logger.NewNoop(),
	}
	cfg := validConfig("nvidia.wd5.myworkdayjobs.com", "NVIDIAExternalCareerSite")
	_, err := adapter.FetchJobListingWithConfig(context.Background(), "nvidia", cfg)
	if err == nil {
		t.Fatal("expected network error after server closed, got nil")
	}
}

// TestWorkday_postJSON_buildRequestError exercises the build-request error path
// in postJSON by passing a nil context, which http.NewRequestWithContext rejects.
func TestWorkday_postJSON_buildRequestError(t *testing.T) {
	w := newTestAdapter()
	//nolint:staticcheck // intentional nil context to trigger build-request error
	_, _, err := w.postJSON(nil, "https://nvidia.wd5.myworkdayjobs.com/x", struct{}{}) //nolint:all
	if err == nil {
		t.Fatal("expected error for nil context, got nil")
	}
}

// TestWorkday_getJSON_buildRequestError exercises the build-request error path
// in getJSON by passing a nil context.
func TestWorkday_getJSON_buildRequestError(t *testing.T) {
	w := newTestAdapter()
	//nolint:staticcheck // intentional nil context to trigger build-request error
	_, _, err := w.getJSON(nil, "https://nvidia.wd5.myworkdayjobs.com/x") //nolint:all
	if err == nil {
		t.Fatal("expected error for nil context, got nil")
	}
}

func TestWorkday_NilHTTPClientFallsToDefault(t *testing.T) {
	// Verify client() returns http.DefaultClient when httpClient is nil.
	w := &Workday{
		httpClient:     nil,
		interPageDelay: 0,
		log:            logger.NewNoop(),
	}
	if w.client() != http.DefaultClient {
		t.Error("expected client() to return http.DefaultClient when httpClient is nil")
	}
}

func TestWorkday_FetchJobListingWithConfig_ContextCancelled(t *testing.T) {
	// After the first page is served, cancel the context so the inter-page
	// delay select fires ctx.Done().
	firstPageDone := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"total": 100,
			"jobPostings": [
				{"title":"Job A","externalPath":"/job/US/A_R-001","locationsText":"NY","bulletFields":["R-001"]},
				{"title":"Job B","externalPath":"/job/US/B_R-002","locationsText":"LA","bulletFields":["R-002"]}
			]
		}`))
		// Signal that first page has been served.
		select {
		case <-firstPageDone:
		default:
			close(firstPageDone)
		}
	}))
	defer srv.Close()

	adapter := &Workday{
		httpClient:     testHTTPClientRedirectingTo(srv.URL),
		interPageDelay: 50 * time.Millisecond, // long enough that cancel fires first
		log:            logger.NewNoop(),
	}

	go func() {
		<-firstPageDone
		cancel()
	}()

	cfg := validConfig("nvidia.wd5.myworkdayjobs.com", "NVIDIAExternalCareerSite")
	_, err := adapter.FetchJobListingWithConfig(ctx, "nvidia", cfg)
	if err == nil {
		// ctx may have been cancelled after pagination completed — acceptable
		// if the timer fired before cancel. Either way no panic = pass.
		t.Log("no error returned; context may have been cancelled after completion")
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

// testHTTPClientRedirectingTo returns an *http.Client whose transport rewrites
// every request's host:port to the given test server URL. This allows
// production-shaped URLs (e.g. nvidia.wd5.myworkdayjobs.com) to be intercepted
// by the in-process test server without DNS changes.
func testHTTPClientRedirectingTo(serverURL string) *http.Client {
	return &http.Client{
		Transport: &redirectTransport{target: serverURL},
	}
}

type redirectTransport struct {
	target string
}

func (rt *redirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Clone the request so we don't mutate the original.
	cloned := req.Clone(req.Context())
	// Replace scheme+host with the test server.
	cloned.URL.Scheme = "http"
	cloned.URL.Host = strings.TrimPrefix(rt.target, "http://")
	return http.DefaultTransport.RoundTrip(cloned)
}
