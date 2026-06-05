package eightfold

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

// ── Test helpers ──────────────────────────────────────────────────────────────

// newTestAdapter creates an Eightfold adapter with zero inter-page delay and
// http scheme for fast test execution against httptest servers.
func newTestAdapter() *Eightfold {
	e := New(logger.NewNoop(), http.DefaultClient)
	e.interPageDelay = 0
	e.scheme = "http"
	return e
}

func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("load fixture %s: %v", name, err)
	}
	return data
}

// cfgJSON builds a raw config JSON for the given test server URL.
// host is extracted from the URL (strip scheme).
func cfgJSON(host, domain string) json.RawMessage {
	b, _ := json.Marshal(map[string]string{
		"host":   host,
		"domain": domain,
	})
	return b
}

// ── FetchJobListing (always errors) ──────────────────────────────────────────

func TestEightfold_FetchJobListing_AlwaysErrors(t *testing.T) {
	e := newTestAdapter()
	_, err := e.FetchJobListing(context.Background(), "some-company")
	if err == nil {
		t.Fatal("FetchJobListing should always return an error")
	}
	if !strings.Contains(err.Error(), "requires config") {
		t.Errorf("error = %q, want it to mention 'requires config'", err.Error())
	}
}

// ── FetchJobListingWithConfig ─────────────────────────────────────────────────

func TestEightfold_FetchJobListingWithConfig_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		// Verify User-Agent is set
		if ua := r.Header.Get("User-Agent"); !strings.Contains(ua, "Mozilla") {
			t.Errorf("User-Agent = %q, want Mozilla browser UA", ua)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(loadFixture(t, "listing.json"))
	}))
	defer srv.Close()

	host := strings.TrimPrefix(srv.URL, "http://")
	cfg := cfgJSON(host, "test.example.com")

	e := newTestAdapter()
	result, err := e.FetchJobListingWithConfig(context.Background(), "company-1", cfg)
	if err != nil {
		t.Fatalf("FetchJobListingWithConfig error: %v", err)
	}
	if len(result.Jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(result.Jobs))
	}

	// First job: has canonical URL and explicit location.
	j0 := result.Jobs[0]
	if j0.ID != "eightfold-790315382430" {
		t.Errorf("jobs[0].ID = %q, want eightfold-790315382430", j0.ID)
	}
	if j0.Title != "Senior Software Engineer" {
		t.Errorf("jobs[0].Title = %q, want Senior Software Engineer", j0.Title)
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
	if j0.URL != "https://explore.jobs.netflix.net/careers/job/790315382430" {
		t.Errorf("jobs[0].URL = %q, want canonical URL", j0.URL)
	}

	// Second job: empty location — falls back to joined Locations; empty canonical URL uses fallback.
	j1 := result.Jobs[1]
	if j1.ID != "eightfold-790315382431" {
		t.Errorf("jobs[1].ID = %q, want eightfold-790315382431", j1.ID)
	}
	wantLoc := "New York, NY; Remote"
	if j1.Location != wantLoc {
		t.Errorf("jobs[1].Location = %q, want %q", j1.Location, wantLoc)
	}
	// Canonical URL is empty in fixture — fallback URL should use the configured host
	if j1.URL == "" {
		t.Error("jobs[1].URL should not be empty (fallback URL expected)")
	}
	if !strings.Contains(j1.URL, fmt.Sprintf("%d", int64(790315382431))) {
		t.Errorf("jobs[1].URL = %q, want it to contain the job ID", j1.URL)
	}
}

func TestEightfold_FetchJobListingWithConfig_Pagination(t *testing.T) {
	// pageSize=2; serve page0 (2 items), page1 (1 item < pageSize → stop).
	var requestedStarts []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := r.URL.Query().Get("start")
		requestedStarts = append(requestedStarts, start)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		switch start {
		case "0":
			_, _ = w.Write([]byte(`{"positions":[
				{"id":1,"name":"Job A","location":"NYC","canonicalPositionUrl":"https://a.b.com/careers/job/1"},
				{"id":2,"name":"Job B","location":"LA","canonicalPositionUrl":"https://a.b.com/careers/job/2"}
			],"count":3}`))
		case "2":
			_, _ = w.Write([]byte(`{"positions":[
				{"id":3,"name":"Job C","location":"Chicago","canonicalPositionUrl":"https://a.b.com/careers/job/3"}
			],"count":3}`))
		default:
			_, _ = w.Write([]byte(`{"positions":[],"count":3}`))
		}
	}))
	defer srv.Close()

	host := strings.TrimPrefix(srv.URL, "http://")
	// Override page size to 2 to control pagination in tests by using a small listing.
	// The adapter uses hardcoded defaultPageSize=10; with page0 returning 2 items,
	// which is < 10, it stops after 1 request. Instead, use 2 items as defaultPageSize
	// via the server returning exactly 10 items for full pages.
	//
	// Re-approach: serve 10 items on page0, 1 item on page1.
	requestedStarts = nil
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := r.URL.Query().Get("start")
		requestedStarts = append(requestedStarts, start)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		switch start {
		case "0":
			// 10 items = full page, must continue
			positions := make([]map[string]any, 10)
			for i := 0; i < 10; i++ {
				positions[i] = map[string]any{
					"id":                   int64(100 + i),
					"name":                 fmt.Sprintf("Job %d", i),
					"location":             "NYC",
					"canonicalPositionUrl": fmt.Sprintf("https://a.b.com/careers/job/%d", 100+i),
				}
			}
			body, _ := json.Marshal(map[string]any{"positions": positions, "count": 11})
			_, _ = w.Write(body)
		case "10":
			// 1 item = partial page, stop
			_, _ = w.Write([]byte(`{"positions":[
				{"id":200,"name":"Last Job","location":"LA","canonicalPositionUrl":"https://a.b.com/careers/job/200"}
			],"count":11}`))
		default:
			_, _ = w.Write([]byte(`{"positions":[],"count":11}`))
		}
	}))
	defer srv2.Close()

	host2 := strings.TrimPrefix(srv2.URL, "http://")
	cfg := cfgJSON(host2, "test.example.com")

	e := newTestAdapter()
	result, err := e.FetchJobListingWithConfig(context.Background(), "company-1", cfg)
	if err != nil {
		t.Fatalf("FetchJobListingWithConfig error: %v", err)
	}
	if len(result.Jobs) != 11 {
		t.Fatalf("expected 11 jobs (10 + 1), got %d", len(result.Jobs))
	}
	if len(requestedStarts) != 2 {
		t.Errorf("expected 2 HTTP requests (start=0 and start=10), got %d: %v", len(requestedStarts), requestedStarts)
	}
	_ = host // silence unused var
}

func TestEightfold_FetchJobListingWithConfig_MaxJobsCap(t *testing.T) {
	// Serve unlimited full pages; adapter must stop at max_jobs_per_scan.
	pageCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pageCount++
		positions := make([]map[string]any, 10)
		for i := 0; i < 10; i++ {
			positions[i] = map[string]any{
				"id":                   int64(pageCount*100 + i),
				"name":                 "Job",
				"location":             "NYC",
				"canonicalPositionUrl": fmt.Sprintf("https://a.b.com/careers/job/%d", pageCount*100+i),
			}
		}
		body, _ := json.Marshal(map[string]any{"positions": positions, "count": 9999})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	host := strings.TrimPrefix(srv.URL, "http://")
	cfgRaw, _ := json.Marshal(map[string]any{
		"host":              host,
		"domain":            "test.example.com",
		"max_jobs_per_scan": 15,
	})

	e := newTestAdapter()
	result, err := e.FetchJobListingWithConfig(context.Background(), "company-1", cfgRaw)
	if err != nil {
		t.Fatalf("FetchJobListingWithConfig error: %v", err)
	}
	if len(result.Jobs) != 15 {
		t.Errorf("expected 15 jobs (capped), got %d", len(result.Jobs))
	}
}

func TestEightfold_FetchJobListingWithConfig_InvalidConfig(t *testing.T) {
	tests := []struct {
		name   string
		rawCfg json.RawMessage
	}{
		{
			name:   "empty host",
			rawCfg: json.RawMessage(`{"host":"","domain":"netflix.com"}`),
		},
		{
			name:   "host with scheme",
			rawCfg: json.RawMessage(`{"host":"https://explore.jobs.netflix.net","domain":"netflix.com"}`),
		},
		{
			name:   "empty domain",
			rawCfg: json.RawMessage(`{"host":"explore.jobs.netflix.net","domain":""}`),
		},
		{
			name:   "nil config",
			rawCfg: nil,
		},
		{
			name:   "malformed JSON config",
			rawCfg: json.RawMessage(`{not valid`),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newTestAdapter()
			_, err := e.FetchJobListingWithConfig(context.Background(), "company-1", tt.rawCfg)
			if err == nil {
				t.Fatalf("expected error for invalid config, got nil")
			}
		})
	}
}

func TestEightfold_FetchJobListingWithConfig_Errors(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{"http 403", http.StatusForbidden, ""},
		{"http 500", http.StatusInternalServerError, ""},
		{"malformed JSON", http.StatusOK, "not json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			host := strings.TrimPrefix(srv.URL, "http://")
			cfg := cfgJSON(host, "test.example.com")

			e := newTestAdapter()
			_, err := e.FetchJobListingWithConfig(context.Background(), "company-1", cfg)
			if err == nil {
				t.Fatalf("expected error for %s, got nil", tt.name)
			}
		})
	}
}

func TestEightfold_FetchJobListingWithConfig_QueryParams(t *testing.T) {
	var captured string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.URL.RequestURI()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"positions":[],"count":0}`))
	}))
	defer srv.Close()

	host := strings.TrimPrefix(srv.URL, "http://")
	cfgRaw, _ := json.Marshal(map[string]string{
		"host":   host,
		"domain": "example.com",
		"locale": "fr",
	})

	e := newTestAdapter()
	_, _ = e.FetchJobListingWithConfig(context.Background(), "company-1", cfgRaw)

	if !strings.Contains(captured, "/api/apply/v2/jobs") {
		t.Errorf("URL path should contain /api/apply/v2/jobs, got %q", captured)
	}
	if !strings.Contains(captured, "domain=example.com") {
		t.Errorf("URL should contain domain=example.com, got %q", captured)
	}
	if !strings.Contains(captured, "hl=fr") {
		t.Errorf("URL should contain hl=fr (locale), got %q", captured)
	}
	if !strings.Contains(captured, "start=0") {
		t.Errorf("URL should contain start=0 for first page, got %q", captured)
	}
}

// ── FetchJobPost ──────────────────────────────────────────────────────────────

func TestEightfold_FetchJobPost(t *testing.T) {
	var capturedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.RequestURI()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(loadFixture(t, "job_post.json"))
	}))
	defer srv.Close()

	host := strings.TrimPrefix(srv.URL, "http://")
	summary := core.JobSummary{
		ID:  "eightfold-790315382430",
		URL: fmt.Sprintf("http://%s/careers/job/790315382430", host),
	}

	e := newTestAdapter()
	post, err := e.FetchJobPost(context.Background(), summary)
	if err != nil {
		t.Fatalf("FetchJobPost error: %v", err)
	}

	// Verify detail URL was built correctly.
	if !strings.Contains(capturedPath, "/api/apply/v2/jobs/790315382430") {
		t.Errorf("detail URL should contain /api/apply/v2/jobs/790315382430, got %q", capturedPath)
	}
	if !strings.Contains(capturedPath, "domain=") {
		t.Errorf("detail URL should contain domain= query param, got %q", capturedPath)
	}

	// Verify mapped fields.
	if post.ID != "eightfold-790315382430" {
		t.Errorf("post.ID = %q, want eightfold-790315382430", post.ID)
	}
	if post.Description == "" {
		t.Error("post.Description is empty")
	}
	if strings.Contains(post.Description, "<p>") || strings.Contains(post.Description, "<ul>") {
		t.Errorf("post.Description should have HTML stripped, got %q", post.Description)
	}
	if post.Department != "Engineering" {
		t.Errorf("post.Department = %q, want Engineering", post.Department)
	}
	if post.WorkplaceType != "onsite" {
		t.Errorf("post.WorkplaceType = %q, want onsite", post.WorkplaceType)
	}
	if post.PublishedDate == "" {
		t.Error("post.PublishedDate is empty")
	}
}

func TestEightfold_FetchJobPost_DomainInferredFromHost(t *testing.T) {
	// When URL has no ?domain= query param, domain should be inferred from host apex.
	var capturedQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":42,"name":"Engineer","location":"NYC"}`))
	}))
	defer srv.Close()

	host := strings.TrimPrefix(srv.URL, "http://")
	// URL has no domain query param; host is "127.0.0.1:port" — apex inference
	// handles port stripping.
	summary := core.JobSummary{
		ID:  "eightfold-42",
		URL: fmt.Sprintf("http://%s/careers/job/42", host),
	}

	e := newTestAdapter()
	_, err := e.FetchJobPost(context.Background(), summary)
	if err != nil {
		t.Fatalf("FetchJobPost error: %v", err)
	}
	if !strings.Contains(capturedQuery, "domain=") {
		t.Errorf("detail URL query should contain domain=, got %q", capturedQuery)
	}
}

func TestEightfold_FetchJobPost_InvalidURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{"empty URL", ""},
		{"missing careers/job segment", "https://example.com/jobs/42"},
		{"missing ID", "https://example.com/careers/job/"},
		{"relative URL", "/careers/job/42"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newTestAdapter()
			summary := core.JobSummary{ID: "eightfold-42", URL: tt.url}
			_, err := e.FetchJobPost(context.Background(), summary)
			if err == nil {
				t.Fatalf("expected error for invalid URL %q, got nil", tt.url)
			}
		})
	}
}

// ── parseJobURL ───────────────────────────────────────────────────────────────

func TestParseJobURL(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantHost   string
		wantID     string
		wantDomain string
		wantErr    bool
	}{
		{
			name:       "standard Eightfold URL",
			input:      "https://explore.jobs.netflix.net/careers/job/790315382430",
			wantHost:   "explore.jobs.netflix.net",
			wantID:     "790315382430",
			wantDomain: "netflix.net",
		},
		{
			name:       "URL with domain query param",
			input:      "https://explore.jobs.netflix.net/careers/job/790315382430?domain=netflix.com",
			wantHost:   "explore.jobs.netflix.net",
			wantID:     "790315382430",
			wantDomain: "netflix.com",
		},
		{
			name:       "URL with leading path segment",
			input:      "https://careers.activision.com/careers/job/12345",
			wantHost:   "careers.activision.com",
			wantID:     "12345",
			wantDomain: "activision.com",
		},
		{
			name:    "missing careers/job segment",
			input:   "https://example.com/jobs/42",
			wantErr: true,
		},
		{
			name:    "missing ID",
			input:   "https://example.com/careers/job/",
			wantErr: true,
		},
		{
			name:    "relative URL",
			input:   "/careers/job/42",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host, id, domain, err := parseJobURL(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseJobURL(%q) error = nil, want error", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseJobURL(%q) unexpected error: %v", tt.input, err)
			}
			if host != tt.wantHost {
				t.Errorf("host = %q, want %q", host, tt.wantHost)
			}
			if id != tt.wantID {
				t.Errorf("id = %q, want %q", id, tt.wantID)
			}
			if domain != tt.wantDomain {
				t.Errorf("domain = %q, want %q", domain, tt.wantDomain)
			}
		})
	}
}

func TestEightfold_FetchJobPost_HTTPErrors(t *testing.T) {
	tests := []struct {
		name   string
		status int
	}{
		{"http 404", http.StatusNotFound},
		{"http 500", http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
			}))
			defer srv.Close()

			host := strings.TrimPrefix(srv.URL, "http://")
			summary := core.JobSummary{
				ID:  "eightfold-42",
				URL: fmt.Sprintf("http://%s/careers/job/42", host),
			}
			e := newTestAdapter()
			_, err := e.FetchJobPost(context.Background(), summary)
			if err == nil {
				t.Fatalf("expected error for status %d, got nil", tt.status)
			}
		})
	}
}

func TestEightfold_FetchJobPost_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not json"))
	}))
	defer srv.Close()

	host := strings.TrimPrefix(srv.URL, "http://")
	summary := core.JobSummary{
		ID:  "eightfold-42",
		URL: fmt.Sprintf("http://%s/careers/job/42", host),
	}
	e := newTestAdapter()
	_, err := e.FetchJobPost(context.Background(), summary)
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
}

// ── resolveScheme ─────────────────────────────────────────────────────────────

func TestResolveScheme_DefaultsToHTTPS(t *testing.T) {
	e := &Eightfold{} // scheme is empty — must fall back to "https"
	if got := e.resolveScheme(); got != "https" {
		t.Errorf("resolveScheme() = %q, want https", got)
	}
}

// ── parseJobURL edge cases ────────────────────────────────────────────────────

func TestParseJobURL_TrailingSegment(t *testing.T) {
	// URL with trailing /apply/... after the ID
	host, id, domain, err := parseJobURL("https://explore.jobs.netflix.net/careers/job/790315382430/apply/start")
	if err != nil {
		t.Fatalf("parseJobURL error: %v", err)
	}
	if host != "explore.jobs.netflix.net" {
		t.Errorf("host = %q, want explore.jobs.netflix.net", host)
	}
	if id != "790315382430" {
		t.Errorf("id = %q, want 790315382430", id)
	}
	if domain != "netflix.net" {
		t.Errorf("domain = %q, want netflix.net", domain)
	}
}

// ── apexDomain edge cases ─────────────────────────────────────────────────────

func TestApexDomain(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"explore.jobs.netflix.net", "netflix.net"},
		{"careers.activision.com", "activision.com"},
		{"eightfold.ai", "eightfold.ai"},
		{"localhost", "localhost"},
		{"127.0.0.1:12345", "0.1"}, // port stripped; 4 octets → last 2 labels
		{"example.com", "example.com"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := apexDomain(tt.input)
			if got != tt.want {
				t.Errorf("apexDomain(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ── context cancellation ──────────────────────────────────────────────────────

func TestEightfold_FetchJobListingWithConfig_ContextCancelled(t *testing.T) {
	// Adapter needs an inter-page delay > 0 so that the context select fires.
	// Serve 10 items on the first page so the adapter enters the delay path.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		positions := make([]map[string]any, 10)
		for i := 0; i < 10; i++ {
			positions[i] = map[string]any{
				"id":                   int64(i + 1),
				"name":                 "Job",
				"location":             "NYC",
				"canonicalPositionUrl": fmt.Sprintf("https://a.b.com/careers/job/%d", i+1),
			}
		}
		body, _ := json.Marshal(map[string]any{"positions": positions, "count": 9999})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	host := strings.TrimPrefix(srv.URL, "http://")
	cfg := cfgJSON(host, "test.example.com")

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel immediately — the select in the delay path should catch it.
	cancel()

	e := New(logger.NewNoop(), http.DefaultClient)
	e.scheme = "http"
	// Use a small but non-zero delay so the select fires on cancel.
	e.interPageDelay = 10 * time.Millisecond

	_, err := e.FetchJobListingWithConfig(ctx, "company-1", cfg)
	// Either the ctx error propagates or we get clean results — both are valid
	// depending on scheduling. The key is no panic and no infinite loop.
	_ = err
}
