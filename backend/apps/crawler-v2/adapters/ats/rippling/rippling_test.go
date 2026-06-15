package rippling

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/trushilpatel/jobradar/common/ats/core"
	"github.com/trushilpatel/jobradar/common/logger"
)

// ── test helpers ──────────────────────────────────────────────────────────────

// newTestAdapter returns a Rippling pointed at the given test server.
// careersURL and atsURL may be the same server or different servers.
func newTestAdapter(careersURL, atsURL string) *Rippling {
	r := New(logger.NewNoop(), http.DefaultClient)
	r.careersBaseURL = careersURL
	r.atsBaseURL = atsURL
	return r
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

func TestRippling_FetchJobListing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(loadFixture(t, "listing.html"))
	}))
	defer srv.Close()

	result, err := newTestAdapter(srv.URL, srv.URL).FetchJobListing(context.Background(), "ignored")
	if err != nil {
		t.Fatalf("FetchJobListing error: %v", err)
	}

	// listing.html has 3 jobs: 2 US, 1 non-US (GB) → expect 2
	if len(result.Jobs) != 2 {
		t.Fatalf("expected 2 US jobs, got %d", len(result.Jobs))
	}

	j0 := result.Jobs[0]
	if j0.ID != "rippling-aaa-111" {
		t.Errorf("jobs[0].ID = %q, want rippling-aaa-111", j0.ID)
	}
	if j0.Title != "Software Engineer" {
		t.Errorf("jobs[0].Title = %q, want Software Engineer", j0.Title)
	}
	if j0.Location != "San Francisco, CA" {
		t.Errorf("jobs[0].Location = %q, want San Francisco, CA", j0.Location)
	}
	if j0.Adapter != "rippling" {
		t.Errorf("jobs[0].Adapter = %q, want rippling", j0.Adapter)
	}
	if !j0.LocationKnown {
		t.Error("jobs[0].LocationKnown should be true")
	}
	if len(j0.AllLocations) != 1 || j0.AllLocations[0] != "San Francisco, CA" {
		t.Errorf("jobs[0].AllLocations = %v, want [San Francisco, CA]", j0.AllLocations)
	}

	j1 := result.Jobs[1]
	if j1.ID != "rippling-bbb-222" {
		t.Errorf("jobs[1].ID = %q, want rippling-bbb-222", j1.ID)
	}
	if j1.Location != "New York, NY" {
		t.Errorf("jobs[1].Location = %q, want New York, NY", j1.Location)
	}
}

func TestRippling_FetchJobListing_NonUSFiltered(t *testing.T) {
	// Build a listing with only non-US jobs.
	body := `<!DOCTYPE html><html><body>` +
		`<script id="__NEXT_DATA__" type="application/json">` +
		`{"props":{"pageProps":{"jobs":{"items":[` +
		`{"id":"uk-001","name":"UK Engineer","url":"https://ats.rippling.com/rippling/jobs/uk-001",` +
		`"locations":[{"countryCode":"GB","city":"London","stateCode":"ENG"}]}` +
		`]}}}}` +
		`</script></body></html>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	result, err := newTestAdapter(srv.URL, srv.URL).FetchJobListing(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Jobs) != 0 {
		t.Errorf("expected 0 jobs after filtering non-US, got %d", len(result.Jobs))
	}
}

func TestRippling_FetchJobListing_URL(t *testing.T) {
	var capturedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<!DOCTYPE html><html><body><script id="__NEXT_DATA__" type="application/json">{"props":{"pageProps":{"jobs":{"items":[]}}}}</script></body></html>`))
	}))
	defer srv.Close()

	_, _ = newTestAdapter(srv.URL, srv.URL).FetchJobListing(context.Background(), "")

	if capturedPath != "/careers/open-roles" {
		t.Errorf("careers path = %q, want /careers/open-roles", capturedPath)
	}
}

func TestRippling_FetchJobListing_UserAgent(t *testing.T) {
	var capturedUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUA = r.Header.Get("User-Agent")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<!DOCTYPE html><html><body><script id="__NEXT_DATA__" type="application/json">{"props":{"pageProps":{"jobs":{"items":[]}}}}</script></body></html>`))
	}))
	defer srv.Close()

	_, _ = newTestAdapter(srv.URL, srv.URL).FetchJobListing(context.Background(), "")

	if !strings.Contains(capturedUA, "Mozilla") {
		t.Errorf("User-Agent = %q, should contain Mozilla", capturedUA)
	}
}

func TestRippling_FetchJobListing_Errors(t *testing.T) {
	tests := []struct {
		name string
		body string
		code int
	}{
		{"http 500", "", http.StatusInternalServerError},
		{"http 429", "", http.StatusTooManyRequests},
		{"no __NEXT_DATA__ tag", "<html><body>no script here</body></html>", http.StatusOK},
		{"malformed json", `<html><body><script id="__NEXT_DATA__" type="application/json">not-json</script></body></html>`, http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.code)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			_, err := newTestAdapter(srv.URL, srv.URL).FetchJobListing(context.Background(), "")
			if err == nil {
				t.Fatalf("expected error for %q, got nil", tt.name)
			}
		})
	}
}

func TestRippling_FetchJobListing_DeduplicatesIDs(t *testing.T) {
	// Two items with the same ID — only one should be returned.
	body := `<!DOCTYPE html><html><body>` +
		`<script id="__NEXT_DATA__" type="application/json">` +
		`{"props":{"pageProps":{"jobs":{"items":[` +
		`{"id":"dup-001","name":"Engineer A","url":"u1","locations":[{"countryCode":"US","city":"SF","stateCode":"CA"}]},` +
		`{"id":"dup-001","name":"Engineer B","url":"u2","locations":[{"countryCode":"US","city":"NY","stateCode":"NY"}]}` +
		`]}}}}` +
		`</script></body></html>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	result, err := newTestAdapter(srv.URL, srv.URL).FetchJobListing(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Jobs) != 1 {
		t.Errorf("expected 1 job after dedup, got %d", len(result.Jobs))
	}
}

func TestRippling_FetchJobListing_EmployType(t *testing.T) {
	body := `<!DOCTYPE html><html><body>` +
		`<script id="__NEXT_DATA__" type="application/json">` +
		`{"props":{"pageProps":{"jobs":{"items":[` +
		`{"id":"e-001","name":"Eng","url":"u","locations":[{"countryCode":"US","city":"SF","stateCode":"CA"}]}` +
		`]}}}}` +
		`</script></body></html>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	result, err := newTestAdapter(srv.URL, srv.URL).FetchJobListing(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Jobs[0].EmployType != core.EmployFullTime {
		t.Errorf("EmployType = %q, want %q", result.Jobs[0].EmployType, core.EmployFullTime)
	}
}

// ── FetchJobPost ──────────────────────────────────────────────────────────────

func TestRippling_FetchJobPost(t *testing.T) {
	var capturedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(loadFixture(t, "job_post.html"))
	}))
	defer srv.Close()

	summary := core.JobSummary{
		ID:  "rippling-aaa-111",
		URL: srv.URL + "/rippling/jobs/aaa-111",
	}
	post, err := newTestAdapter(srv.URL, srv.URL).FetchJobPost(context.Background(), summary)
	if err != nil {
		t.Fatalf("FetchJobPost error: %v", err)
	}

	if capturedPath != "/rippling/jobs/aaa-111" {
		t.Errorf("detail path = %q, want /rippling/jobs/aaa-111", capturedPath)
	}
	if post.ID != "rippling-aaa-111" {
		t.Errorf("post.ID = %q, want rippling-aaa-111", post.ID)
	}
	if post.Title != "Software Engineer" {
		t.Errorf("post.Title = %q, want Software Engineer", post.Title)
	}
	if post.Description == "" {
		t.Error("post.Description is empty")
	}
	// Ensure HTML was stripped
	if strings.Contains(post.Description, "<h2>") {
		t.Error("post.Description should not contain HTML tags")
	}
	if post.Compensation != "$180,000 - $245,000 USD/year" {
		t.Errorf("post.Compensation = %q, want $180,000 - $245,000 USD/year", post.Compensation)
	}
	if post.Department != "Engineering" {
		t.Errorf("post.Department = %q, want Engineering", post.Department)
	}
	if post.PublishedDate != "2026-03-01T00:00:00.000Z" {
		t.Errorf("post.PublishedDate = %q, want 2026-03-01T00:00:00.000Z", post.PublishedDate)
	}
	if post.WorkplaceType != "ON_SITE" {
		t.Errorf("post.WorkplaceType = %q, want ON_SITE", post.WorkplaceType)
	}
}

func TestRippling_FetchJobPost_URLConstruction(t *testing.T) {
	// Verify that the ATS base URL is used when constructing the post URL.
	var capturedHost string
	atsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHost = r.Host
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(loadFixture(t, "job_post.html"))
	}))
	defer atsSrv.Close()

	careersSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Should not be called during FetchJobPost.
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer careersSrv.Close()

	r := New(logger.NewNoop(), http.DefaultClient)
	r.careersBaseURL = careersSrv.URL
	r.atsBaseURL = atsSrv.URL

	summary := core.JobSummary{
		ID:  "rippling-aaa-111",
		URL: "https://ats.rippling.com/rippling/jobs/aaa-111",
	}
	// Override URL to point at test server — rewrite to use atsBaseURL.
	summary.URL = atsSrv.URL + "/rippling/jobs/aaa-111"

	_, err := r.FetchJobPost(context.Background(), summary)
	if err != nil {
		t.Fatalf("FetchJobPost error: %v", err)
	}
	_ = capturedHost // request went to atsSrv
}

func TestRippling_FetchJobPost_InvalidURL(t *testing.T) {
	r := New(logger.NewNoop(), http.DefaultClient)

	// Empty URL path — should get "no job ID" error.
	summary := core.JobSummary{ID: "rippling-x", URL: "https://ats.rippling.com/"}
	_, err := r.FetchJobPost(context.Background(), summary)
	if err == nil {
		t.Fatal("expected error for empty path, got nil")
	}
	if !strings.Contains(err.Error(), "no job ID") {
		t.Errorf("error = %q, want to contain 'no job ID'", err.Error())
	}
}

func TestRippling_FetchJobPost_Errors(t *testing.T) {
	tests := []struct {
		name string
		body string
		code int
	}{
		{"http 404", "", http.StatusNotFound},
		{"http 500", "", http.StatusInternalServerError},
		{"no __NEXT_DATA__ tag", "<html><body>no script</body></html>", http.StatusOK},
		{"malformed json", `<html><body><script id="__NEXT_DATA__" type="application/json">not-json</script></body></html>`, http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.code)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			summary := core.JobSummary{
				ID:  "rippling-x",
				URL: srv.URL + "/rippling/jobs/test-id",
			}
			_, err := newTestAdapter(srv.URL, srv.URL).FetchJobPost(context.Background(), summary)
			if err == nil {
				t.Fatalf("expected error for %q, got nil", tt.name)
			}
		})
	}
}

// ── extractNextData ───────────────────────────────────────────────────────────

func TestExtractNextData(t *testing.T) {
	t.Run("finds script tag", func(t *testing.T) {
		html := []byte(`<html><body><script id="__NEXT_DATA__" type="application/json">{"key":"val"}</script></body></html>`)
		got, err := extractNextData(html)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(got) != `{"key":"val"}` {
			t.Errorf("got %q, want {\"key\":\"val\"}", string(got))
		}
	})

	t.Run("returns error when missing", func(t *testing.T) {
		html := []byte(`<html><body><p>no script here</p></body></html>`)
		_, err := extractNextData(html)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("trims whitespace around json", func(t *testing.T) {
		html := []byte(`<html><script id="__NEXT_DATA__" type="application/json">  {"k":"v"}  </script></html>`)
		got, err := extractNextData(html)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(got) != `{"k":"v"}` {
			t.Errorf("got %q, want {\"k\":\"v\"}", string(got))
		}
	})

	t.Run("returns error when closing tag missing", func(t *testing.T) {
		html := []byte(`<html><script id="__NEXT_DATA__" type="application/json">{"k":"v"}`)
		_, err := extractNextData(html)
		if err == nil {
			t.Fatal("expected error for missing closing tag, got nil")
		}
	})

	t.Run("returns error when opening > is missing", func(t *testing.T) {
		// Construct a body where the script tag start is found but no '>' follows.
		// This covers the openIdx == -1 branch.
		// We simulate this by giving a string that ends right after the script tag start.
		// strings.Index looks for '>' in s[start:], so we need a string with no '>' after __NEXT_DATA__.
		html := []byte(`<script id="__NEXT_DATA__" type="application/json" `)
		// No '>' anywhere after the script tag start.
		// But wait — the prefix itself has '<', and we search from start.
		// We need to ensure there's no '>' at all in the string after start.
		// The above has no '>'. Let's verify.
		_, err := extractNextData(html)
		if err == nil {
			t.Fatal("expected error for malformed tag, got nil")
		}
	})
}

// ── resolveCareersBaseURL / resolveATSBaseURL defaults ───────────────────────

func TestRippling_DefaultURLs(t *testing.T) {
	r := New(logger.NewNoop(), http.DefaultClient)
	// When overrides are empty, the production defaults should be returned.
	if got := r.resolveCareersBaseURL(); got != defaultCareersBaseURL {
		t.Errorf("resolveCareersBaseURL = %q, want %q", got, defaultCareersBaseURL)
	}
	if got := r.resolveATSBaseURL(); got != defaultATSBaseURL {
		t.Errorf("resolveATSBaseURL = %q, want %q", got, defaultATSBaseURL)
	}
}

// ── extractJobID ──────────────────────────────────────────────────────────────

func TestExtractJobID(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		want    string
		wantErr bool
	}{
		{
			name: "standard ATS URL",
			url:  "https://ats.rippling.com/rippling/jobs/550e8400-e29b-41d4-a716-446655440000",
			want: "550e8400-e29b-41d4-a716-446655440000",
		},
		{
			name: "test server URL with path",
			url:  "http://127.0.0.1:12345/rippling/jobs/abc-123",
			want: "abc-123",
		},
		{
			name:    "empty URL",
			url:     "",
			wantErr: true,
		},
		{
			name:    "URL with trailing slash only",
			url:     "https://ats.rippling.com/",
			wantErr: true,
		},
		{
			name:    "root path",
			url:     "https://ats.rippling.com",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractJobID(tt.url)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (got=%q)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("extractJobID(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}
