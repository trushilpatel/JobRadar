package workday

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
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/trushilpatel/jobradar/apps/crawler/ats/types"
	jobcommon "github.com/trushilpatel/jobradar/service/job/common"
)

// ── Interface compliance ────────────────────────────────────────────────────

func TestWorkday_ImplementsAdapter(t *testing.T) {
	var _ types.Adapter = (*Workday)(nil)
}

func TestWorkday_ImplementsConfigurableAdapter(t *testing.T) {
	var _ types.ConfigurableAdapter = (*Workday)(nil)
}

func TestWorkday_ImplementsContentFetcher(t *testing.T) {
	var _ types.ContentFetcher = (*Workday)(nil)
}

func TestWorkday_DoesNotImplementURLProvider(t *testing.T) {
	var v interface{} = New()
	if _, ok := v.(types.URLProvider); ok {
		t.Fatal("Workday must not implement URLProvider — tenant URLs cannot be derived from a slug")
	}
}

func TestWorkday_DoesNotImplementStreamPublisher(t *testing.T) {
	var v interface{} = New()
	if _, ok := v.(types.StreamPublisher); ok {
		t.Fatal("Workday must not implement StreamPublisher")
	}
}

// ── FetchJobs (no config) sanity ────────────────────────────────────────────

func TestFetchJobs_RequiresConfig(t *testing.T) {
	w := New()
	_, err := w.FetchJobs(context.Background(), "any")
	if err == nil || !strings.Contains(err.Error(), "adapter_config") {
		t.Fatalf("expected error mentioning adapter_config, got %v", err)
	}
}

// ── Config decoding / validation ────────────────────────────────────────────

func TestFetchJobsWithConfig_InvalidJSON(t *testing.T) {
	w := New()
	_, err := w.FetchJobsWithConfig(context.Background(), "co", json.RawMessage(`{`))
	if err == nil || !strings.Contains(err.Error(), "decode config") {
		t.Fatalf("expected decode config error, got %v", err)
	}
}

func TestFetchJobsWithConfig_MissingHost(t *testing.T) {
	w := New()
	cfg := json.RawMessage(`{"site":"NVIDIAExternalCareerSite"}`)
	_, err := w.FetchJobsWithConfig(context.Background(), "co", cfg)
	if err == nil || !strings.Contains(err.Error(), "host") {
		t.Fatalf("expected host error, got %v", err)
	}
}

func TestFetchJobsWithConfig_MissingSite(t *testing.T) {
	w := New()
	cfg := json.RawMessage(`{"host":"nvidia.wd5.myworkdayjobs.com"}`)
	_, err := w.FetchJobsWithConfig(context.Background(), "co", cfg)
	if err == nil || !strings.Contains(err.Error(), "site") {
		t.Fatalf("expected site error, got %v", err)
	}
}

func TestFetchJobsWithConfig_HostInvalidPattern(t *testing.T) {
	w := New()
	cfg := json.RawMessage(`{"host":"foo.bar.com","site":"S"}`)
	_, err := w.FetchJobsWithConfig(context.Background(), "co", cfg)
	if err == nil || !strings.Contains(err.Error(), "host") {
		t.Fatalf("expected host pattern error, got %v", err)
	}
}

func TestFetchJobsWithConfig_HostWithBadCharacters(t *testing.T) {
	w := New()
	cfg := json.RawMessage(`{"host":"Foo!.wd5.myworkdayjobs.com","site":"S"}`)
	_, err := w.FetchJobsWithConfig(context.Background(), "co", cfg)
	if err == nil || !strings.Contains(err.Error(), "host") {
		t.Fatalf("expected host pattern error, got %v", err)
	}
}

func TestFetchJobsWithConfig_SiteContainsSlash(t *testing.T) {
	w := New()
	cfg := json.RawMessage(`{"host":"nvidia.wd5.myworkdayjobs.com","site":"a/b"}`)
	_, err := w.FetchJobsWithConfig(context.Background(), "co", cfg)
	if err == nil || !strings.Contains(err.Error(), "site") {
		t.Fatalf("expected site slash error, got %v", err)
	}
}

func TestFetchJobsWithConfig_NilConfig(t *testing.T) {
	w := New()
	_, err := w.FetchJobsWithConfig(context.Background(), "co", nil)
	if err == nil || !strings.Contains(err.Error(), "adapter_config") {
		t.Fatalf("expected adapter_config error, got %v", err)
	}
}

func TestFetchJobsWithConfig_EmptyCompanyID(t *testing.T) {
	w := New()
	cfg := json.RawMessage(`{"host":"nvidia.wd5.myworkdayjobs.com","site":"S"}`)
	_, err := w.FetchJobsWithConfig(context.Background(), " ", cfg)
	if err == nil || !strings.Contains(err.Error(), "companyID") {
		t.Fatalf("expected companyID error, got %v", err)
	}
}

// ── Pagination ──────────────────────────────────────────────────────────────

func TestFetchJobsWithConfig_HappyPath(t *testing.T) {
	srv := newListingServer(t, []pageResp{
		{Total: 2, Postings: []rawJobPosting{
			{Title: "Senior SWE", ExternalPath: "/job/San-Jose/Senior-SWE_R-12345", LocationsText: "San Jose, CA", BulletFields: []string{"R-12345"}},
			{Title: "ML Engineer", ExternalPath: "/job/Remote/ML-Engineer_R-99999", LocationsText: "Remote, USA", BulletFields: []string{"R-99999"}},
		}},
	})
	defer srv.Close()

	w := newTestWorkday(srv)
	cfg := mustConfigJSON(t, srv, "NVIDIAExternalCareerSite", 0)
	resp, err := w.FetchJobsWithConfig(context.Background(), "co", cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Jobs) != 2 {
		t.Fatalf("got %d jobs, want 2", len(resp.Jobs))
	}
	j := resp.Jobs[0]
	if j.ID != "workday-R-12345" {
		t.Errorf("ID = %q, want workday-R-12345", j.ID)
	}
	if j.Title != "Senior SWE" {
		t.Errorf("Title = %q", j.Title)
	}
	// externalPath already starts with `/job/`; the listing URL concatenates it
	// directly. We assert the single-`/job/` shape because Workday's live API
	// rejects the doubled form with HTTP 406.
	wantURL := fmt.Sprintf("https://%s/%s/job/San-Jose/Senior-SWE_R-12345", hostFromServer(srv), "NVIDIAExternalCareerSite")
	if j.URL != wantURL {
		t.Errorf("URL = %q, want %q", j.URL, wantURL)
	}
	if j.Location != "San Jose, CA" {
		t.Errorf("Location = %q", j.Location)
	}
	if j.EmploymentType != jobcommon.EmployFullTime {
		t.Errorf("EmploymentType = %q, want full_time", j.EmploymentType)
	}
	if j.FullContent != nil {
		t.Errorf("FullContent must be nil on a two-phase adapter")
	}
}

func TestFetchJobsWithConfig_SinglePage(t *testing.T) {
	postings := makePostings(5, 0)
	srv := newListingServer(t, []pageResp{{Total: 5, Postings: postings}})
	defer srv.Close()

	w := newTestWorkday(srv)
	cfg := mustConfigJSON(t, srv, "S", 0)
	resp, err := w.FetchJobsWithConfig(context.Background(), "co", cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Jobs) != 5 {
		t.Fatalf("got %d jobs, want 5", len(resp.Jobs))
	}
}

func TestFetchJobsWithConfig_TwoPages(t *testing.T) {
	srv := newListingServer(t, []pageResp{
		{Total: 25, Postings: makePostings(20, 0)},
		{Total: 25, Postings: makePostings(5, 20)},
	})
	defer srv.Close()

	w := newTestWorkday(srv)
	cfg := mustConfigJSON(t, srv, "S", 0)
	resp, err := w.FetchJobsWithConfig(context.Background(), "co", cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Jobs) != 25 {
		t.Fatalf("got %d jobs, want 25", len(resp.Jobs))
	}
}

func TestFetchJobsWithConfig_ThreeFullPages(t *testing.T) {
	srv := newListingServer(t, []pageResp{
		{Total: 60, Postings: makePostings(20, 0)},
		{Total: 60, Postings: makePostings(20, 20)},
		{Total: 60, Postings: makePostings(20, 40)},
	})
	defer srv.Close()

	w := newTestWorkday(srv)
	cfg := mustConfigJSON(t, srv, "S", 0)
	resp, err := w.FetchJobsWithConfig(context.Background(), "co", cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Jobs) != 60 {
		t.Fatalf("got %d jobs, want 60", len(resp.Jobs))
	}
}

func TestFetchJobsWithConfig_PaginatesUntilEmpty(t *testing.T) {
	// Server claims total=100 but page 2 returns 0 jobs — adapter must stop.
	srv := newListingServer(t, []pageResp{
		{Total: 100, Postings: makePostings(20, 0)},
		{Total: 100, Postings: []rawJobPosting{}},
	})
	defer srv.Close()

	w := newTestWorkday(srv)
	cfg := mustConfigJSON(t, srv, "S", 0)
	resp, err := w.FetchJobsWithConfig(context.Background(), "co", cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Jobs) != 20 {
		t.Fatalf("got %d jobs, want 20", len(resp.Jobs))
	}
}

func TestFetchJobsWithConfig_RespectsMaxJobsPerScan(t *testing.T) {
	srv := newListingServer(t, []pageResp{
		{Total: 100, Postings: makePostings(20, 0)},
		{Total: 100, Postings: makePostings(20, 20)},
	})
	defer srv.Close()

	w := newTestWorkday(srv)
	cfg := mustConfigJSON(t, srv, "S", 10)
	resp, err := w.FetchJobsWithConfig(context.Background(), "co", cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Jobs) != 10 {
		t.Fatalf("got %d jobs, want 10 (capped by max_jobs_per_scan)", len(resp.Jobs))
	}
}

func TestFetchJobsWithConfig_HardCap200Pages(t *testing.T) {
	// Server lies about total and always returns 20 jobs; adapter must stop
	// at the hard 200-page cap.
	var pageCount atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pageCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(listingResponse{
			Total:       1_000_000,
			JobPostings: makePostings(20, int(pageCount.Load())*20),
		})
	}))
	defer srv.Close()

	w := newTestWorkdayNoDelay(srv)
	// Set max very high so the page cap, not the row cap, is the limiter.
	cfg := mustConfigJSON(t, srv, "S", 1_000_000)
	_, err := w.FetchJobsWithConfig(context.Background(), "co", cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := pageCount.Load(); got > int64(hardPageCap) {
		t.Fatalf("hard cap exceeded: %d > %d", got, hardPageCap)
	}
}

// ── Request shape ───────────────────────────────────────────────────────────

func TestFetchJobsWithConfig_RequestPathAndMethod(t *testing.T) {
	var capturedMethod, capturedPath string
	var capturedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		capturedPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &capturedBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"total":0,"jobPostings":[]}`))
	}))
	defer srv.Close()

	w := newTestWorkdayNoDelay(srv)
	cfg := mustConfigJSON(t, srv, "NVIDIAExternalCareerSite", 0)
	_, err := w.FetchJobsWithConfig(context.Background(), "co", cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", capturedMethod)
	}
	wantPath := fmt.Sprintf("/wday/cxs/%s/NVIDIAExternalCareerSite/jobs", tenantFromTestServer(srv))
	if capturedPath != wantPath {
		t.Errorf("path = %q, want %q", capturedPath, wantPath)
	}
	if got, _ := capturedBody["limit"].(float64); got != 20 {
		t.Errorf("body limit = %v, want 20", capturedBody["limit"])
	}
	if got, _ := capturedBody["offset"].(float64); got != 0 {
		t.Errorf("body offset = %v, want 0", capturedBody["offset"])
	}
	if got, _ := capturedBody["searchText"].(string); got != "" {
		t.Errorf("body searchText = %v, want empty", capturedBody["searchText"])
	}
	if _, ok := capturedBody["appliedFacets"].(map[string]any); !ok {
		t.Errorf("body appliedFacets should be an object, got %T", capturedBody["appliedFacets"])
	}
}

func TestFetchJobsWithConfig_TenantExtractedFromHost(t *testing.T) {
	tests := []struct {
		host       string
		wantTenant string
	}{
		{"nvidia.wd5.myworkdayjobs.com", "nvidia"},
		{"polar-semi.wd501.myworkdayjobs.com", "polar-semi"},
		{"abc-123.wd1.myworkdayjobs.com", "abc-123"},
	}
	for _, tc := range tests {
		t.Run(tc.host, func(t *testing.T) {
			cfg := adapterConfig{Host: tc.host}
			if got := cfg.tenant(); got != tc.wantTenant {
				t.Errorf("tenant for %q = %q, want %q", tc.host, got, tc.wantTenant)
			}
		})
	}
}

func TestFetchJobsWithConfig_UserAgentSet(t *testing.T) {
	var ua string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ua = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"total":0,"jobPostings":[]}`))
	}))
	defer srv.Close()

	w := newTestWorkdayNoDelay(srv)
	cfg := mustConfigJSON(t, srv, "S", 0)
	_, _ = w.FetchJobsWithConfig(context.Background(), "co", cfg)
	if !strings.Contains(ua, "Mozilla") {
		t.Errorf("User-Agent = %q, want browser-like", ua)
	}
}

// ── Error paths ─────────────────────────────────────────────────────────────

func TestFetchJobsWithConfig_404Listing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	w := newTestWorkdayNoDelay(srv)
	cfg := mustConfigJSON(t, srv, "S", 0)
	resp, err := w.FetchJobsWithConfig(context.Background(), "co", cfg)
	if err != nil {
		t.Fatalf("404 should not error, got %v", err)
	}
	if len(resp.Jobs) != 0 {
		t.Errorf("expected empty result on 404, got %d", len(resp.Jobs))
	}
}

func TestFetchJobsWithConfig_500Listing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	w := newTestWorkdayNoDelay(srv)
	cfg := mustConfigJSON(t, srv, "S", 0)
	_, err := w.FetchJobsWithConfig(context.Background(), "co", cfg)
	if err == nil || !strings.Contains(err.Error(), "500") {
		t.Fatalf("expected 500 error, got %v", err)
	}
}

func TestFetchJobsWithConfig_403CloudFront(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`<html><body>Forbidden</body></html>`))
	}))
	defer srv.Close()

	w := newTestWorkdayNoDelay(srv)
	cfg := mustConfigJSON(t, srv, "S", 0)
	_, err := w.FetchJobsWithConfig(context.Background(), "co", cfg)
	if err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("expected 403 error, got %v", err)
	}
}

func TestFetchJobsWithConfig_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`not json`))
	}))
	defer srv.Close()

	w := newTestWorkdayNoDelay(srv)
	cfg := mustConfigJSON(t, srv, "S", 0)
	_, err := w.FetchJobsWithConfig(context.Background(), "co", cfg)
	if err == nil || !strings.Contains(err.Error(), "decode") {
		t.Fatalf("expected decode error, got %v", err)
	}
}

func TestFetchJobsWithConfig_ContextCancelled(t *testing.T) {
	srv := newListingServer(t, []pageResp{{Total: 0, Postings: nil}})
	defer srv.Close()

	w := newTestWorkdayNoDelay(srv)
	cfg := mustConfigJSON(t, srv, "S", 0)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := w.FetchJobsWithConfig(ctx, "co", cfg); err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

func TestFetchJobsWithConfig_ContextCancelledMidPagination(t *testing.T) {
	// Adapter returns a full page then waits — cancel during the inter-page
	// delay should propagate immediately.
	srv := newListingServer(t, []pageResp{
		{Total: 100, Postings: makePostings(20, 0)},
		{Total: 100, Postings: makePostings(20, 20)},
	})
	defer srv.Close()

	w := New()
	w.httpClient = srv.Client()
	w.interPageDelay = 1 * time.Hour // long enough that the test will cancel first

	host := hostFromServer(srv)
	cfg, _ := json.Marshal(adapterConfig{Host: host, Site: "S"})

	ctx, cancel := context.WithCancel(context.Background())
	// We can't easily intercept the URL through DefaultClient using httpClient
	// override; instead, replace the transport with one that always hits srv.
	w.httpClient = &http.Client{Transport: redirectTransport{srv: srv}}

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	_, err := w.FetchJobsWithConfig(ctx, "co", cfg)
	if err == nil {
		t.Fatal("expected context-cancelled error")
	}
}

// ── Conversion / fields ─────────────────────────────────────────────────────

func TestJobToItem_FullMapping(t *testing.T) {
	cfg := adapterConfig{Host: "nvidia.wd5.myworkdayjobs.com", Site: "NVIDIAExternalCareerSite"}
	jp := rawJobPosting{
		Title:         "Senior SWE",
		ExternalPath:  "/job/San-Jose/Senior-SWE_R-12345",
		LocationsText: "San Jose, CA",
		BulletFields:  []string{"R-12345"},
	}
	got := jobToItem(jp, cfg)
	if got.ID != "workday-R-12345" {
		t.Errorf("ID = %q", got.ID)
	}
	if got.URL != "https://nvidia.wd5.myworkdayjobs.com/NVIDIAExternalCareerSite/job/San-Jose/Senior-SWE_R-12345" {
		t.Errorf("URL = %q", got.URL)
	}
	if got.Location != "San Jose, CA" {
		t.Errorf("Location = %q", got.Location)
	}
	if got.EmploymentType != jobcommon.EmployFullTime {
		t.Errorf("EmploymentType = %q", got.EmploymentType)
	}
}

func TestExtractRequisitionID_FromBulletFields(t *testing.T) {
	got := extractRequisitionID(rawJobPosting{BulletFields: []string{"R-123"}, ExternalPath: "/x_Y-1"})
	if got != "R-123" {
		t.Errorf("got %q, want R-123", got)
	}
}

func TestExtractRequisitionID_FromExternalPath(t *testing.T) {
	got := extractRequisitionID(rawJobPosting{ExternalPath: "/job/SF/Eng_R-456"})
	if got != "R-456" {
		t.Errorf("got %q, want R-456", got)
	}
}

func TestExtractRequisitionID_EmptyBulletField_FallsBackToPath(t *testing.T) {
	got := extractRequisitionID(rawJobPosting{BulletFields: []string{"   "}, ExternalPath: "/job/x_Y-99"})
	if got != "Y-99" {
		t.Errorf("got %q, want Y-99", got)
	}
}

func TestExtractRequisitionID_BothEmpty(t *testing.T) {
	got := extractRequisitionID(rawJobPosting{})
	if got != "unknown" {
		t.Errorf("got %q, want unknown", got)
	}
}

func TestExtractRequisitionID_PathWithoutUnderscore(t *testing.T) {
	got := extractRequisitionID(rawJobPosting{ExternalPath: "/job/SF/foo"})
	if got != "/job/SF/foo" {
		t.Errorf("got %q, want /job/SF/foo", got)
	}
}

func TestTenantFromHost(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"nvidia.wd5.myworkdayjobs.com", "nvidia"},
		{"NVIDIA.wd5.myworkdayjobs.com", "nvidia"},
		{"polar-semi.wd501.myworkdayjobs.com", "polar-semi"},
		{"", ""},
		{"no-dot", ""},
		{".wd5.myworkdayjobs.com", ""},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			if got := tenantFromHost(tc.in); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// ── Detail fetch (FetchJobContent) ──────────────────────────────────────────

func TestFetchJobContent_HappyPath(t *testing.T) {
	srv := newDetailServer(t, detailResponse{
		JobPostingInfo: rawJobPostingInfo{
			Title:          "Senior SWE",
			JobDescription: "<p>Build &amp; ship cool stuff.</p>",
			Location:       "San Jose, CA",
			StartDate:      "2026-05-01", // PublishedDate falls back to StartDate; PostedOn is a display string
			TimeType:       "Full time",
			JobReqID:       "R-12345",
		},
	})
	defer srv.Close()

	host := hostFromServer(srv)
	rawURL := fmt.Sprintf("https://%s/NVIDIAExternalCareerSite/job/SF/Senior-SWE_R-12345", host)
	w := newTestWorkdayNoDelay(srv)
	got, err := w.FetchJobContent(context.Background(), types.RawJob{URL: rawURL})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Title != "Senior SWE" {
		t.Errorf("Title = %q", got.Title)
	}
	if got.Description != "Build & ship cool stuff." {
		t.Errorf("Description = %q", got.Description)
	}
	if got.Location != "San Jose, CA" {
		t.Errorf("Location = %q", got.Location)
	}
	if got.PublishedDate != "2026-05-01" {
		t.Errorf("PublishedDate = %q", got.PublishedDate)
	}
	if got.EmploymentType != jobcommon.EmployFullTime {
		t.Errorf("EmploymentType = %q", got.EmploymentType)
	}
	if got.ID != "workday-R-12345" {
		t.Errorf("ID = %q", got.ID)
	}
	if got.URL != rawURL {
		t.Errorf("URL = %q", got.URL)
	}
}

func TestFetchJobContent_404Detail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	host := hostFromServer(srv)
	rawURL := fmt.Sprintf("https://%s/S/job/x_R-1", host)
	w := newTestWorkdayNoDelay(srv)
	_, err := w.FetchJobContent(context.Background(), types.RawJob{URL: rawURL})
	if err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("expected 404 error, got %v", err)
	}
}

func TestFetchJobContent_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`not json`))
	}))
	defer srv.Close()
	host := hostFromServer(srv)
	rawURL := fmt.Sprintf("https://%s/S/job/x_R-1", host)
	w := newTestWorkdayNoDelay(srv)
	_, err := w.FetchJobContent(context.Background(), types.RawJob{URL: rawURL})
	if err == nil || !strings.Contains(err.Error(), "decode") {
		t.Fatalf("expected decode error, got %v", err)
	}
}

func TestFetchJobContent_URLParseRejectsBadHost(t *testing.T) {
	w := New()
	_, err := w.FetchJobContent(context.Background(), types.RawJob{URL: "https://example.com/S/job/x_R-1"})
	if err == nil || !strings.Contains(err.Error(), "parse job URL") {
		t.Fatalf("expected URL parse error, got %v", err)
	}
}

func TestFetchJobContent_URLParseRejectsMissingJobSegment(t *testing.T) {
	w := New()
	_, err := w.FetchJobContent(context.Background(), types.RawJob{URL: "https://nvidia.wd5.myworkdayjobs.com/S/jobs/x"})
	if err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("expected missing-job-segment error, got %v", err)
	}
}

func TestFetchJobContent_URLParseRejectsEmptySite(t *testing.T) {
	w := New()
	_, err := w.FetchJobContent(context.Background(), types.RawJob{URL: "https://nvidia.wd5.myworkdayjobs.com//job/x_R-1"})
	if err == nil || !strings.Contains(err.Error(), "site") {
		t.Fatalf("expected site error, got %v", err)
	}
}

func TestFetchJobContent_URLParseRejectsUnparseable(t *testing.T) {
	w := New()
	_, err := w.FetchJobContent(context.Background(), types.RawJob{URL: "://"})
	if err == nil {
		t.Fatal("expected parse error")
	}
}

func TestFetchJobContent_HTMLStripped(t *testing.T) {
	srv := newDetailServer(t, detailResponse{
		JobPostingInfo: rawJobPostingInfo{
			JobDescription: "<p>Hello <b>world</b> &amp; friends.</p>",
		},
	})
	defer srv.Close()
	host := hostFromServer(srv)
	rawURL := fmt.Sprintf("https://%s/S/job/x_R-1", host)
	w := newTestWorkdayNoDelay(srv)
	got, err := w.FetchJobContent(context.Background(), types.RawJob{URL: rawURL})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Description != "Hello world & friends." {
		t.Errorf("Description = %q", got.Description)
	}
}

func TestFetchJobContent_MissingFields(t *testing.T) {
	srv := newDetailServer(t, detailResponse{
		JobPostingInfo: rawJobPostingInfo{},
	})
	defer srv.Close()
	host := hostFromServer(srv)
	rawURL := fmt.Sprintf("https://%s/S/job/x_R-1", host)
	w := newTestWorkdayNoDelay(srv)
	got, err := w.FetchJobContent(context.Background(), types.RawJob{URL: rawURL})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// ID falls back to the trailing segment of externalPath.
	if got.ID != "workday-R-1" {
		t.Errorf("ID = %q, want workday-R-1", got.ID)
	}
	if got.Title != "" || got.Location != "" || got.Description != "" {
		t.Errorf("expected all empty, got %+v", got)
	}
}

// ── mapEmploymentType ───────────────────────────────────────────────────────

func TestMapEmploymentType(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"Full time", jobcommon.EmployFullTime},
		{"FULL TIME", jobcommon.EmployFullTime},
		{"Full-time", jobcommon.EmployFullTime},
		{" full time ", jobcommon.EmployFullTime},
		{"Part time", jobcommon.EmployPartTime},
		{"part-time", jobcommon.EmployPartTime},
		{"Contract", jobcommon.EmployContract},
		{"contractor", jobcommon.EmployContract},
		{"Consultant", jobcommon.EmployContract},
		{"Fixed term", jobcommon.EmployContract},
		{"Intern", jobcommon.EmployInternship},
		{"Internship", jobcommon.EmployInternship},
		{"", jobcommon.EmployFullTime},
		{"unknown", jobcommon.EmployFullTime},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			if got := mapEmploymentType(tc.in); got != tc.want {
				t.Errorf("mapEmploymentType(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// ── buildID ─────────────────────────────────────────────────────────────────

func TestBuildID(t *testing.T) {
	tests := []struct {
		reqID, path, want string
	}{
		{"R-12345", "/job/x_R-12345", "workday-R-12345"},
		{"", "/job/x_R-555", "workday-R-555"},
		{"", "/job/no-underscore", "workday-unknown"},
		{"", "", "workday-unknown"},
		{"  ", "/job/x_Y-7", "workday-Y-7"},
	}
	for _, tc := range tests {
		t.Run(tc.reqID+"|"+tc.path, func(t *testing.T) {
			if got := buildID(tc.reqID, tc.path); got != tc.want {
				t.Errorf("buildID(%q, %q) = %q, want %q", tc.reqID, tc.path, got, tc.want)
			}
		})
	}
}

// ── parseJobURL ─────────────────────────────────────────────────────────────

func TestParseJobURL_Valid(t *testing.T) {
	host, site, ext, err := parseJobURL("https://nvidia.wd5.myworkdayjobs.com/NVIDIAExternalCareerSite/job/San-Jose/Senior-SWE_R-12345")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if host != "nvidia.wd5.myworkdayjobs.com" {
		t.Errorf("host = %q", host)
	}
	if site != "NVIDIAExternalCareerSite" {
		t.Errorf("site = %q", site)
	}
	if ext != "/job/San-Jose/Senior-SWE_R-12345" {
		t.Errorf("externalPath = %q", ext)
	}
}

// ── adapterConfig.validate ──────────────────────────────────────────────────

func TestAdapterConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		in      adapterConfig
		wantErr string
	}{
		{"valid", adapterConfig{Host: "x.wd5.myworkdayjobs.com", Site: "S"}, ""},
		{"empty host", adapterConfig{Site: "S"}, "host"},
		{"bad host", adapterConfig{Host: "foo.bar", Site: "S"}, "pattern"},
		{"empty site", adapterConfig{Host: "x.wd5.myworkdayjobs.com"}, "site"},
		{"slash site", adapterConfig{Host: "x.wd5.myworkdayjobs.com", Site: "a/b"}, "/"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.in.validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Errorf("expected no error, got %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("got %v, want substring %q", err, tc.wantErr)
			}
		})
	}
}

// ── Constructor ─────────────────────────────────────────────────────────────

func TestNew_Defaults(t *testing.T) {
	w := New()
	if w.httpClient != http.DefaultClient {
		t.Error("expected DefaultClient")
	}
	if w.interPageDelay != interPageDelay {
		t.Errorf("interPageDelay = %v, want %v", w.interPageDelay, interPageDelay)
	}
}

func TestClient_FallbackToDefault(t *testing.T) {
	w := &Workday{}
	if w.client() != http.DefaultClient {
		t.Error("nil httpClient should fall back to DefaultClient")
	}
}

// ── Real integration tests (gated on TEST_REAL=1) ───────────────────────────

func TestFetchJobsWithConfig_Real(t *testing.T) {
	if os.Getenv("TEST_REAL") == "" {
		t.Skip("skipping real API test; set TEST_REAL=1 to run")
	}
	w := New()
	cfg, _ := json.Marshal(adapterConfig{
		Host: "nvidia.wd5.myworkdayjobs.com",
		Site: "NVIDIAExternalCareerSite",
	})
	resp, err := w.FetchJobsWithConfig(context.Background(), "co", cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Jobs) == 0 {
		t.Fatal("expected at least one posting from live NVIDIA board")
	}
	first := resp.Jobs[0]
	if !strings.HasPrefix(first.ID, "workday-") {
		t.Errorf("ID prefix = %q", first.ID)
	}
	if !strings.HasPrefix(first.URL, "https://nvidia.wd5.myworkdayjobs.com/") {
		t.Errorf("URL = %q", first.URL)
	}
	t.Logf("fetched %d postings; first: %q at %q", len(resp.Jobs), first.Title, first.Location)
}

func TestFetchJobContent_Real(t *testing.T) {
	if os.Getenv("TEST_REAL") == "" {
		t.Skip("skipping real API test; set TEST_REAL=1 to run")
	}
	w := New()
	cfg, _ := json.Marshal(adapterConfig{
		Host: "nvidia.wd5.myworkdayjobs.com",
		Site: "NVIDIAExternalCareerSite",
	})
	resp, err := w.FetchJobsWithConfig(context.Background(), "co", cfg)
	if err != nil {
		t.Fatalf("FetchJobsWithConfig error: %v", err)
	}
	if len(resp.Jobs) == 0 {
		t.Fatal("no postings to enrich")
	}
	first := resp.Jobs[0]
	content, err := w.FetchJobContent(context.Background(), types.RawJob{URL: first.URL})
	if err != nil {
		t.Fatalf("FetchJobContent error: %v", err)
	}
	if content.Description == "" {
		t.Error("description should not be empty")
	}
	t.Logf("title=%q location=%q desc-len=%d", content.Title, content.Location, len(content.Description))
}

// ── adapterConfig.tenant() edge cases ──────────────────────────────────────

// TestAdapterConfigTenant_EmptyHost covers the idx <= 0 branch when host is
// the empty string after TrimSpace.
func TestAdapterConfigTenant_EmptyHost(t *testing.T) {
	cfg := adapterConfig{Host: ""}
	if got := cfg.tenant(); got != "" {
		t.Errorf("tenant() on empty host = %q, want empty string", got)
	}
}

// TestAdapterConfigTenant_HostStartingWithDot covers idx == 0 (dot is the
// first character so there is no leftmost subdomain).
func TestAdapterConfigTenant_HostStartingWithDot(t *testing.T) {
	cfg := adapterConfig{Host: ".wd5.myworkdayjobs.com"}
	if got := cfg.tenant(); got != "" {
		t.Errorf("tenant() on dot-prefixed host = %q, want empty string", got)
	}
}

// TestAdapterConfigTenant_WhitespaceOnlyHost covers the TrimSpace path: "   "
// becomes "" which has no dot → returns "".
func TestAdapterConfigTenant_WhitespaceOnlyHost(t *testing.T) {
	cfg := adapterConfig{Host: "   "}
	if got := cfg.tenant(); got != "" {
		t.Errorf("tenant() on whitespace-only host = %q, want empty string", got)
	}
}

// ── postJSON — marshal error ───────────────────────────────────────────────

// TestPostJSON_MarshalError covers the json.Marshal failure branch inside
// postJSON. Channels are not JSON-serialisable and trigger the error.
func TestPostJSON_MarshalError(t *testing.T) {
	w := &Workday{httpClient: http.DefaultClient}
	_, _, err := w.postJSON(context.Background(), "http://localhost", map[string]any{
		"bad": make(chan int),
	})
	if err == nil || !strings.Contains(err.Error(), "marshal") {
		t.Fatalf("expected marshal error, got %v", err)
	}
}

// ── postJSON / getJSON — body read error ──────────────────────────────────

// brokenBodyTransport returns an HTTP 200 whose body always errors on Read.
// This exercises the io.ReadAll failure branch in both postJSON and getJSON.
type brokenBodyTransport struct{}

func (brokenBodyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(&errorReader{}),
		Request:    req,
	}, nil
}

// errorReader always returns an error on Read.
type errorReader struct{}

func (*errorReader) Read(_ []byte) (int, error) {
	return 0, fmt.Errorf("simulated read error")
}

func TestPostJSON_ReadBodyError(t *testing.T) {
	w := &Workday{httpClient: &http.Client{Transport: brokenBodyTransport{}}}
	_, _, err := w.postJSON(context.Background(), "http://localhost/test", map[string]any{"k": "v"})
	if err == nil || !strings.Contains(err.Error(), "read") {
		t.Fatalf("expected read error, got %v", err)
	}
}

// TestPostJSON_BuildRequestError covers the http.NewRequestWithContext failure
// branch in postJSON. A null byte in the URL causes the standard library to
// reject request construction before any network call is made.
func TestPostJSON_BuildRequestError(t *testing.T) {
	w := &Workday{httpClient: http.DefaultClient}
	_, _, err := w.postJSON(context.Background(), "http://localhost/\x00bad", map[string]any{"k": "v"})
	if err == nil || !strings.Contains(err.Error(), "build request") {
		t.Fatalf("expected build request error, got %v", err)
	}
}

// ── getJSON — network errors ───────────────────────────────────────────────

// TestGetJSON_InvalidURL covers the http.NewRequestWithContext error branch.
// A null byte in the URL causes the standard library to reject request creation.
func TestGetJSON_InvalidURL(t *testing.T) {
	w := &Workday{httpClient: http.DefaultClient}
	_, _, err := w.getJSON(context.Background(), "http://localhost/\x00bad")
	if err == nil || !strings.Contains(err.Error(), "build request") {
		t.Fatalf("expected build request error, got %v", err)
	}
}

// failingTransport is an http.RoundTripper that always returns an error,
// exercising the client.Do error branch in getJSON.
type failingTransport struct{}

func (failingTransport) RoundTrip(_ *http.Request) (*http.Response, error) {
	return nil, fmt.Errorf("simulated connection refused")
}

func TestGetJSON_DoError(t *testing.T) {
	w := &Workday{httpClient: &http.Client{Transport: failingTransport{}}}
	_, _, err := w.getJSON(context.Background(), "http://localhost/test")
	if err == nil || !strings.Contains(err.Error(), "do") {
		t.Fatalf("expected do error, got %v", err)
	}
}

func TestGetJSON_ReadBodyError(t *testing.T) {
	w := &Workday{httpClient: &http.Client{Transport: brokenBodyTransport{}}}
	_, _, err := w.getJSON(context.Background(), "http://localhost/test")
	if err == nil || !strings.Contains(err.Error(), "read") {
		t.Fatalf("expected read error, got %v", err)
	}
}

// ── extractWorkdayCountryCode ──────────────────────────────────────────────

func TestExtractWorkdayCountryCode_NilRaw(t *testing.T) {
	if got := extractWorkdayCountryCode(nil); got != "" {
		t.Errorf("nil raw: got %q, want empty", got)
	}
}

func TestExtractWorkdayCountryCode_NullJSON(t *testing.T) {
	if got := extractWorkdayCountryCode(json.RawMessage("null")); got != "" {
		t.Errorf("null json: got %q, want empty", got)
	}
}

func TestExtractWorkdayCountryCode_ValidObject(t *testing.T) {
	raw := json.RawMessage(`{"descriptor":"United States of America","id":"US"}`)
	if got := extractWorkdayCountryCode(raw); got != "US" {
		t.Errorf("valid country object: got %q, want US", got)
	}
}

func TestExtractWorkdayCountryCode_EmptyID(t *testing.T) {
	raw := json.RawMessage(`{"descriptor":"Unknown","id":""}`)
	if got := extractWorkdayCountryCode(raw); got != "" {
		t.Errorf("empty id field: got %q, want empty", got)
	}
}

// TestExtractWorkdayCountryCode_ArrayInput passes a JSON array — structurally
// valid JSON but not an object — exercises the json.Unmarshal error branch.
func TestExtractWorkdayCountryCode_ArrayInput(t *testing.T) {
	raw := json.RawMessage(`[1,2,3]`)
	if got := extractWorkdayCountryCode(raw); got != "" {
		t.Errorf("array input: got %q, want empty", got)
	}
}

func TestExtractWorkdayCountryCode_MalformedJSON(t *testing.T) {
	raw := json.RawMessage(`{bad json`)
	if got := extractWorkdayCountryCode(raw); got != "" {
		t.Errorf("malformed JSON: got %q, want empty", got)
	}
}

// ── FetchJobContent with country code field ────────────────────────────────

// TestFetchJobContent_WithCountryCode verifies that the CountryCode field is
// correctly populated from a well-formed country object in the detail response.
func TestFetchJobContent_WithCountryCode(t *testing.T) {
	detail := detailResponse{
		JobPostingInfo: rawJobPostingInfo{
			Title:          "Staff Eng",
			JobDescription: "<p>Build things.</p>",
			Location:       "Santa Clara, CA",
			StartDate:      "2026-01-15",
			TimeType:       "Full time",
			JobReqID:       "R-77777",
			Country:        json.RawMessage(`{"descriptor":"United States of America","id":"US"}`),
		},
	}
	srv := newDetailServer(t, detail)
	defer srv.Close()

	host := hostFromServer(srv)
	rawURL := fmt.Sprintf("https://%s/NVIDIAExternalCareerSite/job/SC/Staff-Eng_R-77777", host)
	w := newTestWorkdayNoDelay(srv)
	got, err := w.FetchJobContent(context.Background(), types.RawJob{URL: rawURL})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.CountryCode != "US" {
		t.Errorf("CountryCode = %q, want US", got.CountryCode)
	}
}

// TestFetchJobContent_FallsBackToPostedOn verifies PublishedDate falls back to
// PostedOn when StartDate is absent.
func TestFetchJobContent_FallsBackToPostedOn(t *testing.T) {
	detail := detailResponse{
		JobPostingInfo: rawJobPostingInfo{
			PostedOn:  "Posted Today",
			StartDate: "",
			JobReqID:  "R-55555",
		},
	}
	srv := newDetailServer(t, detail)
	defer srv.Close()

	host := hostFromServer(srv)
	rawURL := fmt.Sprintf("https://%s/S/job/x_R-55555", host)
	w := newTestWorkdayNoDelay(srv)
	got, err := w.FetchJobContent(context.Background(), types.RawJob{URL: rawURL})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.PublishedDate != "Posted Today" {
		t.Errorf("PublishedDate = %q, want 'Posted Today'", got.PublishedDate)
	}
}

// TestFetchJobContent_GetJSONDoError verifies the "detail fetch" error path
// when the HTTP transport fails inside FetchJobContent.
func TestFetchJobContent_GetJSONDoError(t *testing.T) {
	w := &Workday{
		httpClient:     &http.Client{Transport: failingTransport{}},
		interPageDelay: 0,
	}
	rawURL := "https://test-tenant.wd5.myworkdayjobs.com/S/job/x_R-1"
	_, err := w.FetchJobContent(context.Background(), types.RawJob{URL: rawURL})
	if err == nil || !strings.Contains(err.Error(), "detail fetch") {
		t.Fatalf("expected detail fetch error, got %v", err)
	}
}

// ── Test helpers ────────────────────────────────────────────────────────────

type pageResp struct {
	Total    int
	Postings []rawJobPosting
}

// newListingServer returns a server that serves the supplied page responses in
// order. Calls beyond len(pages) get an empty response.
func newListingServer(t *testing.T, pages []pageResp) *httptest.Server {
	t.Helper()
	var idx atomic.Int64
	var mu sync.Mutex
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		i := int(idx.Load())
		idx.Add(1)
		w.Header().Set("Content-Type", "application/json")
		var resp listingResponse
		if i < len(pages) {
			resp = listingResponse{Total: pages[i].Total, JobPostings: pages[i].Postings}
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
}

func newDetailServer(t *testing.T, resp detailResponse) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
}

func makePostings(n, offset int) []rawJobPosting {
	out := make([]rawJobPosting, n)
	for i := 0; i < n; i++ {
		idx := offset + i
		out[i] = rawJobPosting{
			Title:         fmt.Sprintf("Role %d", idx),
			ExternalPath:  fmt.Sprintf("/job/Loc/Role_R-%d", idx),
			LocationsText: "Anywhere",
			BulletFields:  []string{fmt.Sprintf("R-%d", idx)},
		}
	}
	return out
}

// newTestWorkday returns a Workday adapter whose HTTP traffic is funnelled to
// the supplied test server regardless of the URL the adapter builds. The
// adapter uses a small interPageDelay so multi-page tests stay fast.
func newTestWorkday(srv *httptest.Server) *Workday {
	w := New()
	w.httpClient = &http.Client{Transport: redirectTransport{srv: srv}}
	w.interPageDelay = 1 * time.Millisecond
	return w
}

// newTestWorkdayNoDelay is like newTestWorkday but disables the inter-page
// cooldown entirely.
func newTestWorkdayNoDelay(srv *httptest.Server) *Workday {
	w := New()
	w.httpClient = &http.Client{Transport: redirectTransport{srv: srv}}
	w.interPageDelay = 0
	return w
}

// redirectTransport rewrites every request to target the test server while
// preserving the original method, headers, and body. This lets the adapter
// keep building real production-shaped URLs (so URL-construction tests still
// exercise the real code path) while the bytes flow to httptest.
type redirectTransport struct {
	srv *httptest.Server
}

func (rt redirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	u, err := url.Parse(rt.srv.URL)
	if err != nil {
		return nil, err
	}
	req.URL.Scheme = u.Scheme
	req.URL.Host = u.Host
	return rt.srv.Client().Transport.RoundTrip(req)
}

func hostFromServer(srv *httptest.Server) string {
	// We pretend the test server lives on a Workday tenant; the redirect
	// transport rewrites the URL host, so the adapter's URL construction is
	// still exercised end-to-end. Choose a fixed canonical host.
	return "test-tenant.wd5.myworkdayjobs.com"
}

func tenantFromTestServer(_ *httptest.Server) string {
	return "test-tenant"
}

func mustConfigJSON(t *testing.T, srv *httptest.Server, site string, maxJobs int) json.RawMessage {
	t.Helper()
	cfg := adapterConfig{
		Host:           hostFromServer(srv),
		Site:           site,
		MaxJobsPerScan: maxJobs,
	}
	b, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	return b
}
