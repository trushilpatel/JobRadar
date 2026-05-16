package eightfold

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/trushilpatel/jobradar/apps/crawler/ats/types"
	companycommon "github.com/trushilpatel/jobradar/service/company/common"
	jobcommon "github.com/trushilpatel/jobradar/service/job/common"
)

// ── Interface compliance ────────────────────────────────────────────────────

func TestEightfold_ImplementsAdapter(t *testing.T) {
	var _ types.Adapter = (*Eightfold)(nil)
}

func TestEightfold_ImplementsConfigurableAdapter(t *testing.T) {
	var _ types.ConfigurableAdapter = (*Eightfold)(nil)
}

func TestEightfold_ImplementsContentFetcher(t *testing.T) {
	// Two-phase adapter: FetchJobs returns thin items, ContentFetcher fills
	// the rest. Removing this interface would silently break the analyzer.
	var _ types.ContentFetcher = (*Eightfold)(nil)
}

func TestEightfold_ImplementsURLProvider(t *testing.T) {
	var _ types.URLProvider = (*Eightfold)(nil)
}

// ── Constructor + client fallback ──────────────────────────────────────────

func TestNew_DefaultsAreSet(t *testing.T) {
	e := New()
	if e.httpClient == nil {
		t.Fatal("httpClient should default to http.DefaultClient, got nil")
	}
	if e.interPageDelay != interPageDelay {
		t.Errorf("interPageDelay = %v, want %v", e.interPageDelay, interPageDelay)
	}
}

func TestClient_NilFallbackIsHTTPDefault(t *testing.T) {
	e := &Eightfold{} // explicit zero — httpClient nil
	if e.client() != http.DefaultClient {
		t.Fatal("nil httpClient should fall back to http.DefaultClient")
	}
}

// ── CareersURL best-effort fallback ────────────────────────────────────────

func TestCareersURL(t *testing.T) {
	e := New()
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"whitespace", "   ", ""},
		{"bare-slug-becomes-subdomain", "netflix", "https://netflix.eightfold.ai/careers"},
		{"fqdn-passes-through", "explore.jobs.netflix.net", "https://explore.jobs.netflix.net/careers"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := e.CareersURL(tc.in); got != tc.want {
				t.Errorf("CareersURL(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// ── Config validation ──────────────────────────────────────────────────────

func TestAdapterConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     adapterConfig
		wantErr string // empty means must succeed
	}{
		{"happy", adapterConfig{Host: "explore.jobs.netflix.net", Domain: "netflix.com"}, ""},
		{"missing-host", adapterConfig{Host: "", Domain: "netflix.com"}, "host is required"},
		{"whitespace-host", adapterConfig{Host: "   ", Domain: "netflix.com"}, "host is required"},
		{"missing-domain", adapterConfig{Host: "x.example.com", Domain: ""}, "domain is required"},
		{"host-with-scheme", adapterConfig{Host: "https://x.example.com", Domain: "x.com"}, "bare host"},
		{"host-with-path", adapterConfig{Host: "x.example.com/foo", Domain: "x.com"}, "bare host"},
		{"host-with-space", adapterConfig{Host: "bad host.example.com", Domain: "x.com"}, "bare host"},
		{"host-without-dot", adapterConfig{Host: "localhost", Domain: "x.com"}, "valid FQDN"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("err = %v, want substring %q", err, tc.wantErr)
			}
		})
	}
}

func TestAdapterConfig_ResolveLocale(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty-defaults-en", "", "en"},
		{"whitespace-defaults-en", "   ", "en"},
		{"explicit-en-us", "en-US", "en-US"},
		{"explicit-other", "ja-JP", "ja-JP"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := adapterConfig{Locale: tc.in}
			if got := c.resolveLocale(); got != tc.want {
				t.Errorf("resolveLocale(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// ── URL builders ───────────────────────────────────────────────────────────

func TestBuildListURL_Defaults(t *testing.T) {
	cfg := adapterConfig{Host: "explore.jobs.netflix.net", Domain: "netflix.com"}
	got := buildListURL(cfg, 0)

	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("buildListURL produced invalid URL: %v", err)
	}
	if u.Scheme != "https" || u.Host != "explore.jobs.netflix.net" || u.Path != "/api/apply/v2/jobs" {
		t.Errorf("scheme/host/path wrong: %q", got)
	}
	q := u.Query()
	if q.Get("domain") != "netflix.com" {
		t.Errorf("domain = %q", q.Get("domain"))
	}
	if q.Get("hl") != "en" {
		t.Errorf("hl = %q, want en", q.Get("hl"))
	}
	if q.Get("start") != "0" {
		t.Errorf("start = %q", q.Get("start"))
	}
	if q.Get("num") != fmt.Sprintf("%d", defaultPageSize) {
		t.Errorf("num = %q, want %d", q.Get("num"), defaultPageSize)
	}
}

func TestBuildListURL_NonZeroOffset(t *testing.T) {
	cfg := adapterConfig{Host: "x.example.com", Domain: "x.com"}
	got := buildListURL(cfg, 30)
	if !strings.Contains(got, "start=30") {
		t.Errorf("expected start=30 in %q", got)
	}
}

func TestBuildListURL_RespectsExplicitLocale(t *testing.T) {
	cfg := adapterConfig{Host: "x.example.com", Domain: "x.com", Locale: "ja-JP"}
	got := buildListURL(cfg, 0)
	if !strings.Contains(got, "hl=ja-JP") {
		t.Errorf("expected hl=ja-JP in %q", got)
	}
}

func TestApexDomain(t *testing.T) {
	tests := []struct{ in, want string }{
		{"careers.activision.com", "activision.com"},
		{"explore.jobs.netflix.net", "netflix.net"},
		{"deep.sub.example.co.uk", "co.uk"}, // best-effort; we don't carry a PSL
		{"netflix.com", "netflix.com"},
		{"localhost", "localhost"},
		{"", ""},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			if got := apexDomain(tc.in); got != tc.want {
				t.Errorf("apexDomain(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// ── parseJobURL ────────────────────────────────────────────────────────────

func TestParseJobURL_HappyPath(t *testing.T) {
	scheme, host, id, domain, err := parseJobURL("https://explore.jobs.netflix.net/careers/job/790315927614")
	if scheme != "https" {
		t.Errorf("scheme = %q", scheme)
	}
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if host != "explore.jobs.netflix.net" {
		t.Errorf("host = %q", host)
	}
	if id != "790315927614" {
		t.Errorf("id = %q", id)
	}
	if domain != "netflix.net" {
		// apex of explore.jobs.netflix.net; this is the documented fallback.
		t.Errorf("domain = %q, want apex fallback netflix.net", domain)
	}
}

func TestParseJobURL_ExplicitDomainQueryWins(t *testing.T) {
	_, _, _, domain, err := parseJobURL("https://explore.jobs.netflix.net/careers/job/123?domain=netflix.com")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if domain != "netflix.com" {
		t.Errorf("domain = %q, want netflix.com (explicit query overrides apex)", domain)
	}
}

func TestParseJobURL_TolerantesTrailingSegments(t *testing.T) {
	// Eightfold sometimes emits `/careers/job/<id>/apply` or extra trailing
	// segments — id must be parsed without them.
	_, _, id, _, err := parseJobURL("https://careers.activision.com/careers/job/123456789/apply")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if id != "123456789" {
		t.Errorf("id = %q, want 123456789", id)
	}
}

func TestParseJobURL_Errors(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"missing-scheme", "/careers/job/123", "must be absolute"},
		{"missing-host", "https:///careers/job/123", "must be absolute"},
		{"wrong-path", "https://x.example.com/jobs/123", "missing `/careers/job/`"},
		{"empty-id-trailing-slash", "https://x.example.com/careers/job/", "empty position id"},
		{"empty-id-immediate-slash", "https://x.example.com/careers/job//apply", "empty position id after truncation"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, _, _, _, err := parseJobURL(tc.in)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("parseJobURL(%q) err = %v, want substring %q", tc.in, err, tc.want)
			}
		})
	}
}

// ── FetchJobs (plain) — must error without config ──────────────────────────

func TestFetchJobs_RequiresConfigSoErrorsByDesign(t *testing.T) {
	_, err := New().FetchJobs(context.Background(), "anything")
	if err == nil || !strings.Contains(err.Error(), "requires adapter_config") {
		t.Fatalf("FetchJobs without config should error; got %v", err)
	}
}

// ── FetchJobsWithConfig — input validation ─────────────────────────────────

func TestFetchJobsWithConfig_EmptyCompanyID(t *testing.T) {
	_, err := New().FetchJobsWithConfig(context.Background(), "", json.RawMessage(`{"host":"x.com","domain":"x.com"}`))
	if err == nil || !strings.Contains(err.Error(), "empty companyID") {
		t.Fatalf("expected empty companyID error, got %v", err)
	}
}

func TestFetchJobsWithConfig_NilConfig(t *testing.T) {
	_, err := New().FetchJobsWithConfig(context.Background(), "netflix", nil)
	if err == nil || !strings.Contains(err.Error(), "adapter_config is required") {
		t.Fatalf("expected config-required error, got %v", err)
	}
}

func TestFetchJobsWithConfig_MalformedConfigJSON(t *testing.T) {
	_, err := New().FetchJobsWithConfig(context.Background(), "netflix", json.RawMessage(`{"host":`))
	if err == nil || !strings.Contains(err.Error(), "decode config") {
		t.Fatalf("expected decode-config error, got %v", err)
	}
}

func TestFetchJobsWithConfig_InvalidConfigBubbledUp(t *testing.T) {
	_, err := New().FetchJobsWithConfig(context.Background(), "netflix", json.RawMessage(`{"host":"","domain":"x"}`))
	if err == nil || !strings.Contains(err.Error(), "invalid config") {
		t.Fatalf("expected invalid-config error, got %v", err)
	}
}

// ── FetchJobsWithConfig — happy path + pagination ──────────────────────────

func TestFetchJobsWithConfig_HappyPath_SinglePage(t *testing.T) {
	srv := newListServer(t, []listResponse{{
		Count: 2,
		Positions: []position{
			{
				ID: 100, Name: "Internal Title", PostingName: "Senior Backend Engineer",
				Location: "Los Gatos, CA", Department: "Streaming",
				Type: "Full Time", WorkLocationOption: "onsite",
				CanonicalURL: "https://srv/careers/job/100", TCreate: 1700000000,
				JobDescription: "<p>Build great things.</p>",
			},
			{
				ID: 101, Name: "Internal", PostingName: "Backend Engineer",
				Locations: []string{"London", "Berlin"},
				Type:      "Contract",
			},
		},
	}})
	defer srv.Close()

	e := newTestAdapter(srv)
	cfg := json.RawMessage(fmt.Sprintf(`{"host":%q,"domain":"netflix.com"}`, hostOf(srv.URL)))

	resp, err := e.FetchJobsWithConfig(context.Background(), "netflix", cfg)
	if err != nil {
		t.Fatalf("FetchJobsWithConfig: %v", err)
	}
	if len(resp.Jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(resp.Jobs))
	}

	j := resp.Jobs[0]
	if want := companycommon.AdapterEightfold + "-100"; j.ID != want {
		t.Errorf("ID = %q, want %q", j.ID, want)
	}
	if j.Title != "Senior Backend Engineer" {
		t.Errorf("Title prefers PostingName, got %q", j.Title)
	}
	if j.URL != "https://srv/careers/job/100" {
		t.Errorf("URL should pass through canonical: %q", j.URL)
	}
	if j.Location != "Los Gatos, CA" {
		t.Errorf("Location = %q", j.Location)
	}
	if j.EmploymentType != jobcommon.EmployFullTime {
		t.Errorf("EmploymentType = %q", j.EmploymentType)
	}
	if j.FullContent != nil {
		t.Error("FetchJobsWithConfig must NOT populate FullContent (two-phase adapter)")
	}

	// Second job: no canonical URL, no Location → joined from Locations[].
	j2 := resp.Jobs[1]
	if !strings.HasSuffix(j2.URL, "/careers/job/101") {
		t.Errorf("URL fallback should target /careers/job/101: %q", j2.URL)
	}
	if j2.Location != "London; Berlin" {
		t.Errorf("Location join = %q", j2.Location)
	}
	if j2.EmploymentType != jobcommon.EmployContract {
		t.Errorf("Contract type = %q", j2.EmploymentType)
	}
}

func TestFetchJobsWithConfig_PaginatesAndStopsOnShortPage(t *testing.T) {
	// Two pages: 10 jobs, then 3 (short → terminates).
	pageA := listResponse{Count: 13, Positions: make([]position, defaultPageSize)}
	for i := range pageA.Positions {
		pageA.Positions[i] = position{ID: int64(i + 1), Name: fmt.Sprintf("Job %d", i+1)}
	}
	pageB := listResponse{Count: 13, Positions: make([]position, 3)}
	for i := range pageB.Positions {
		pageB.Positions[i] = position{ID: int64(100 + i), Name: fmt.Sprintf("LastPage %d", i)}
	}

	var callCount int32
	srv := newListServerCounting(t, []listResponse{pageA, pageB}, &callCount)
	defer srv.Close()

	e := newTestAdapter(srv)
	resp, err := e.FetchJobsWithConfig(
		context.Background(), "x",
		json.RawMessage(fmt.Sprintf(`{"host":%q,"domain":"x.com"}`, hostOf(srv.URL))),
	)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(resp.Jobs) != 13 {
		t.Errorf("expected 13 jobs, got %d", len(resp.Jobs))
	}
	if atomic.LoadInt32(&callCount) != 2 {
		t.Errorf("expected 2 HTTP calls, got %d", callCount)
	}
}

func TestFetchJobsWithConfig_RespectsMaxJobsPerScan(t *testing.T) {
	page := listResponse{Count: 30, Positions: make([]position, defaultPageSize)}
	for i := range page.Positions {
		page.Positions[i] = position{ID: int64(i + 1), Name: fmt.Sprintf("Job %d", i+1)}
	}
	srv := newListServer(t, []listResponse{page, page, page})
	defer srv.Close()

	e := newTestAdapter(srv)
	cfg := json.RawMessage(fmt.Sprintf(`{"host":%q,"domain":"x.com","max_jobs_per_scan":7}`, hostOf(srv.URL)))
	resp, err := e.FetchJobsWithConfig(context.Background(), "x", cfg)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(resp.Jobs) != 7 {
		t.Errorf("expected cap to 7, got %d", len(resp.Jobs))
	}
}

func TestFetchJobsWithConfig_EmptyBoard(t *testing.T) {
	srv := newListServer(t, []listResponse{{Count: 0, Positions: nil}})
	defer srv.Close()

	e := newTestAdapter(srv)
	resp, err := e.FetchJobsWithConfig(
		context.Background(), "x",
		json.RawMessage(fmt.Sprintf(`{"host":%q,"domain":"x.com"}`, hostOf(srv.URL))),
	)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(resp.Jobs) != 0 {
		t.Errorf("expected 0 jobs, got %d", len(resp.Jobs))
	}
}

func TestFetchJobsWithConfig_404TreatedAsEmptyResult(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	e := newTestAdapter(srv)
	resp, err := e.FetchJobsWithConfig(
		context.Background(), "x",
		json.RawMessage(fmt.Sprintf(`{"host":%q,"domain":"x.com"}`, hostOf(srv.URL))),
	)
	if err != nil {
		t.Fatalf("404 must be soft-failure (empty result), got err: %v", err)
	}
	if len(resp.Jobs) != 0 {
		t.Errorf("expected 0 jobs on 404, got %d", len(resp.Jobs))
	}
}

func TestFetchJobsWithConfig_5xxBubblesAsError(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	e := newTestAdapter(srv)
	_, err := e.FetchJobsWithConfig(
		context.Background(), "x",
		json.RawMessage(fmt.Sprintf(`{"host":%q,"domain":"x.com"}`, hostOf(srv.URL))),
	)
	if err == nil || !strings.Contains(err.Error(), "502") {
		t.Fatalf("expected 502 error, got %v", err)
	}
}

func TestFetchJobsWithConfig_MalformedListBody(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "not json")
	}))
	defer srv.Close()

	e := newTestAdapter(srv)
	_, err := e.FetchJobsWithConfig(
		context.Background(), "x",
		json.RawMessage(fmt.Sprintf(`{"host":%q,"domain":"x.com"}`, hostOf(srv.URL))),
	)
	if err == nil || !strings.Contains(err.Error(), "decode") {
		t.Fatalf("expected list decode error, got %v", err)
	}
}

func TestFetchJobsWithConfig_ContextCancelledMidPagination(t *testing.T) {
	// First page returns 10; before second page would fire, context is
	// cancelled. Adapter must surface ctx.Err() promptly.
	page := listResponse{Count: 30, Positions: make([]position, defaultPageSize)}
	for i := range page.Positions {
		page.Positions[i] = position{ID: int64(i + 1)}
	}

	first := true
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if first {
			first = false
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(page)
			return
		}
		// Should never be reached if cancellation works.
		t.Errorf("second page should not be requested after ctx cancel")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	e := &Eightfold{httpClient: srv.Client(), interPageDelay: 200 * time.Millisecond}
	// Cancel shortly after the first page is consumed.
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	_, err := e.FetchJobsWithConfig(
		ctx, "x",
		json.RawMessage(fmt.Sprintf(`{"host":%q,"domain":"x.com"}`, hostOf(srv.URL))),
	)
	if err == nil {
		t.Fatal("expected error after context cancel")
	}
}

func TestFetchJobsWithConfig_SendsExpectedHeadersAndQueryParams(t *testing.T) {
	var capturedUA, capturedAccept, capturedPath, capturedDomain, capturedHL string
	var capturedStart int
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUA = r.Header.Get("User-Agent")
		capturedAccept = r.Header.Get("Accept")
		capturedPath = r.URL.Path
		capturedDomain = r.URL.Query().Get("domain")
		capturedHL = r.URL.Query().Get("hl")
		_, _ = fmt.Sscanf(r.URL.Query().Get("start"), "%d", &capturedStart)
		_ = json.NewEncoder(w).Encode(listResponse{Count: 0, Positions: nil})
	}))
	defer srv.Close()

	e := newTestAdapter(srv)
	_, err := e.FetchJobsWithConfig(
		context.Background(), "netflix",
		json.RawMessage(fmt.Sprintf(`{"host":%q,"domain":"netflix.com","locale":"en-US"}`, hostOf(srv.URL))),
	)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(capturedUA, "Mozilla") {
		t.Errorf("UA should be browser-like, got %q", capturedUA)
	}
	if capturedAccept != "application/json" {
		t.Errorf("Accept = %q", capturedAccept)
	}
	if capturedPath != "/api/apply/v2/jobs" {
		t.Errorf("path = %q", capturedPath)
	}
	if capturedDomain != "netflix.com" {
		t.Errorf("domain query = %q", capturedDomain)
	}
	if capturedHL != "en-US" {
		t.Errorf("hl query = %q", capturedHL)
	}
	if capturedStart != 0 {
		t.Errorf("start = %d", capturedStart)
	}
}

// ── FetchJobContent ────────────────────────────────────────────────────────

func TestFetchJobContent_HappyPath(t *testing.T) {
	detail := position{
		ID: 555, Name: "Internal", PostingName: "Staff Engineer",
		Location: "Remote, USA", Department: "Platform",
		Type: "Full Time", WorkLocationOption: "remote",
		JobDescription: "<p>Hello <b>world</b></p>",
		TCreate:        1700000000,
		CanonicalURL:   "", // detail sometimes drops it; URL must fall back
	}

	var capturedPath, capturedDomain string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedDomain = r.URL.Query().Get("domain")
		_ = json.NewEncoder(w).Encode(detail)
	}))
	defer srv.Close()

	e := newTestAdapter(srv)
	url := fmt.Sprintf("%s/careers/job/555?domain=netflix.com", srv.URL)
	got, err := e.FetchJobContent(context.Background(), types.RawJob{URL: url})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if capturedPath != "/api/apply/v2/jobs/555" {
		t.Errorf("detail path = %q", capturedPath)
	}
	if capturedDomain != "netflix.com" {
		t.Errorf("detail domain = %q", capturedDomain)
	}
	if got.ID != companycommon.AdapterEightfold+"-555" {
		t.Errorf("ID = %q", got.ID)
	}
	if got.Title != "Staff Engineer" {
		t.Errorf("Title prefers PostingName: got %q", got.Title)
	}
	if got.Description != "Hello world" {
		t.Errorf("Description should be stripped of HTML, got %q", got.Description)
	}
	if got.EmploymentType != jobcommon.EmployFullTime {
		t.Errorf("EmploymentType = %q", got.EmploymentType)
	}
	if got.WorkplaceType != "remote" {
		t.Errorf("WorkplaceType = %q", got.WorkplaceType)
	}
	if got.URL != url {
		t.Errorf("URL fallback to job.URL when canonical empty; got %q", got.URL)
	}
	if got.PublishedDate == "" || !strings.HasPrefix(got.PublishedDate, "2023-") {
		t.Errorf("PublishedDate should be RFC3339 derived from TCreate, got %q", got.PublishedDate)
	}
}

func TestFetchJobContent_404Errors(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	e := newTestAdapter(srv)
	url := fmt.Sprintf("%s/careers/job/999?domain=x.com", srv.URL)
	_, err := e.FetchJobContent(context.Background(), types.RawJob{URL: url})
	if err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("expected 404 error, got %v", err)
	}
}

func TestFetchJobContent_BadURLBubbledUp(t *testing.T) {
	_, err := New().FetchJobContent(context.Background(), types.RawJob{URL: "/no-host/careers/job/1"})
	if err == nil || !strings.Contains(err.Error(), "parse job URL") {
		t.Fatalf("expected parse-error wrap, got %v", err)
	}
}

func TestFetchJobContent_MalformedDetailJSON(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "not json")
	}))
	defer srv.Close()

	e := newTestAdapter(srv)
	url := fmt.Sprintf("%s/careers/job/12345", srv.URL)
	_, err := e.FetchJobContent(context.Background(), types.RawJob{URL: url})
	if err == nil || !strings.Contains(err.Error(), "decode") {
		t.Fatalf("expected detail decode error, got %v", err)
	}
}

// ── Converters: positionToItem / detailToContent ───────────────────────────

func TestPositionToItem_FallbackTitleAndURL(t *testing.T) {
	cfg := adapterConfig{Host: "careers.activision.com", Domain: "activision.com"}
	item := positionToItem(position{ID: 42, Name: "Fallback Title", PostingName: ""}, cfg)
	if item.Title != "Fallback Title" {
		t.Errorf("Title fallback to Name failed, got %q", item.Title)
	}
	if !strings.HasSuffix(item.URL, "/careers/job/42") {
		t.Errorf("URL fallback wrong: %q", item.URL)
	}
}

func TestDetailToContent_FallbackURL(t *testing.T) {
	got := detailToContent(position{ID: 7}, "https://example.com/careers/job/7")
	if got.URL != "https://example.com/careers/job/7" {
		t.Errorf("URL should fall back to job.URL, got %q", got.URL)
	}
}

// ── firstNonEmpty / unixToRFC3339 / mapEmploymentType ──────────────────────

func TestFirstNonEmpty(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want string
	}{
		{"empty-args", nil, ""},
		{"all-blank", []string{"", "   ", "\t"}, ""},
		{"first-wins", []string{"a", "b"}, "a"},
		{"second-after-blank", []string{"  ", "real"}, "real"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := firstNonEmpty(tc.in...); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestUnixToRFC3339(t *testing.T) {
	tests := []struct {
		name string
		ts   int64
		want string
	}{
		{"zero-returns-empty", 0, ""},
		{"negative-returns-empty", -1, ""},
		{"epoch", 0, ""},
		{"known", 1700000000, "2023-11-14T22:13:20Z"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := unixToRFC3339(tc.ts); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestMapEmploymentType(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Full Time", jobcommon.EmployFullTime},
		{"full-time", jobcommon.EmployFullTime},
		{"FULL_TIME", jobcommon.EmployFullTime},
		{"Regular", jobcommon.EmployFullTime},
		{"Regular Full Time", jobcommon.EmployFullTime},
		{"Permanent", jobcommon.EmployFullTime},
		{"Part Time", jobcommon.EmployPartTime},
		{"part-time", jobcommon.EmployPartTime},
		{"Regular Part Time", jobcommon.EmployPartTime},
		{"Contract", jobcommon.EmployContract},
		{"Contractor", jobcommon.EmployContract},
		{"Consultant", jobcommon.EmployContract},
		{"Contingent Worker", jobcommon.EmployContract},
		{"Temporary", jobcommon.EmployContract},
		{"Fixed Term", jobcommon.EmployContract},
		{"Intern", jobcommon.EmployInternship},
		{"Internship", jobcommon.EmployInternship},
		{"Trainee", jobcommon.EmployInternship},
		{"", jobcommon.EmployFullTime},             // empty defaults
		{"   ", jobcommon.EmployFullTime},          // whitespace defaults
		{"Mystery Type", jobcommon.EmployFullTime}, // unknown defaults
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			if got := mapEmploymentType(tc.in); got != tc.want {
				t.Errorf("mapEmploymentType(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// ── Real integration test (gated on TEST_REAL=1) ───────────────────────────

func TestFetchJobsWithConfig_Real(t *testing.T) {
	if os.Getenv("TEST_REAL") == "" {
		t.Skip("skipping real API test; set TEST_REAL=1 to run")
	}
	e := New()
	cfg := json.RawMessage(`{"host":"explore.jobs.netflix.net","domain":"netflix.com","max_jobs_per_scan":5}`)
	resp, err := e.FetchJobsWithConfig(context.Background(), "netflix", cfg)
	if err != nil {
		t.Fatalf("FetchJobsWithConfig: %v", err)
	}
	if len(resp.Jobs) == 0 {
		t.Fatal("expected at least one job from live explore.jobs.netflix.net")
	}
	j := resp.Jobs[0]
	if !strings.HasPrefix(j.ID, companycommon.AdapterEightfold+"-") {
		t.Errorf("ID %q should have %q prefix", j.ID, companycommon.AdapterEightfold)
	}
	if j.Title == "" {
		t.Error("title should not be empty")
	}
	if !strings.Contains(j.URL, "/careers/job/") {
		t.Errorf("URL %q should contain /careers/job/", j.URL)
	}
	t.Logf("fetched %d jobs; first: %q (%s) at %q", len(resp.Jobs), j.Title, j.EmploymentType, j.Location)
}

// ── Test helpers ───────────────────────────────────────────────────────────

// newTestAdapter wires the adapter to use the test server's httptest.Client
// (so the bogus TLS cert doesn't break the request) and removes the production
// inter-page delay so pagination tests run fast.
func newTestAdapter(srv *httptest.Server) *Eightfold {
	return &Eightfold{
		httpClient:     srv.Client(),
		interPageDelay: 0,
	}
}

// hostOf returns the bare `host:port` portion of an httptest URL (the adapter
// always prefixes "https://", so we strip schemes here for the config).
func hostOf(rawURL string) string {
	u, _ := url.Parse(rawURL)
	return u.Host
}

// newListServer returns a test server that serves the provided list responses
// in order, one per request. Extra requests get an empty 200.
func newListServer(t *testing.T, pages []listResponse) *httptest.Server {
	t.Helper()
	var idx int32
	return httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		i := int(atomic.AddInt32(&idx, 1) - 1)
		w.Header().Set("Content-Type", "application/json")
		if i >= len(pages) {
			_ = json.NewEncoder(w).Encode(listResponse{Count: 0, Positions: nil})
			return
		}
		_ = json.NewEncoder(w).Encode(pages[i])
	}))
}

// newListServerCounting is newListServer + an atomic counter the test asserts
// on. Used to prove pagination stops after the short page.
func newListServerCounting(t *testing.T, pages []listResponse, counter *int32) *httptest.Server {
	t.Helper()
	var idx int32
	return httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(counter, 1)
		i := int(atomic.AddInt32(&idx, 1) - 1)
		w.Header().Set("Content-Type", "application/json")
		if i >= len(pages) {
			_ = json.NewEncoder(w).Encode(listResponse{Count: 0, Positions: nil})
			return
		}
		_ = json.NewEncoder(w).Encode(pages[i])
	}))
}
