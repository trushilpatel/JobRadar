package netflix

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/trushilpatel/jobradar/apps/crawler/ats/types"
	companycommon "github.com/trushilpatel/jobradar/service/company/common"
	jobcommon "github.com/trushilpatel/jobradar/service/job/common"
)

// ── Interface compliance ────────────────────────────────────────────────────

func TestNetflix_ImplementsAdapter(t *testing.T) {
	var _ types.Adapter = (*Netflix)(nil)
}

func TestNetflix_ImplementsContentFetcher(t *testing.T) {
	var _ types.ContentFetcher = (*Netflix)(nil)
}

func TestNetflix_ImplementsURLProvider(t *testing.T) {
	var _ types.URLProvider = (*Netflix)(nil)
}

// ── Constructor ─────────────────────────────────────────────────────────────

func TestNew_DefaultSettings(t *testing.T) {
	n := New()
	if n.baseURL != defaultBaseURL {
		t.Errorf("baseURL = %q, want %q", n.baseURL, defaultBaseURL)
	}
	if n.domain != defaultDomain {
		t.Errorf("domain = %q, want %q", n.domain, defaultDomain)
	}
	if n.pageSize != defaultPageSize {
		t.Errorf("pageSize = %d, want %d", n.pageSize, defaultPageSize)
	}
}

// ── CareersURL ──────────────────────────────────────────────────────────────

func TestCareersURL(t *testing.T) {
	n := New()
	got := n.CareersURL("anything")
	want := "https://explore.jobs.netflix.net/careers"
	if got != want {
		t.Errorf("CareersURL() = %q, want %q", got, want)
	}
}

// ── FetchJobs — single page ─────────────────────────────────────────────────

func TestFetchJobs_SinglePage(t *testing.T) {
	// Real API uses int64 for id, int64 unix timestamps for t_create/t_update.
	positions := []position{
		{
			ID:                 790298021069,
			Name:               "Senior Software Engineer",
			Location:           "Los Gatos, CA",
			Department:         "Engineering",
			WorkLocationOption: "remote",
			CanonicalURL:       "https://explore.jobs.netflix.net/careers/job/790298021069",
			TCreate:            1743465600, // 2025-04-01T00:00:00Z
		},
		{
			ID:                 790313385518,
			Name:               "Web Software Engineer 4",
			Location:           "Remote, USA",
			Department:         "Platform",
			WorkLocationOption: "remote",
			CanonicalURL:       "https://explore.jobs.netflix.net/careers/job/790313385518",
			TCreate:            1743811200, // 2025-04-05T00:00:00Z
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertMethod(t, r, http.MethodGet)
		assertQueryParam(t, r, "domain", "netflix.com")
		assertQueryParam(t, r, "hl", "en")

		resp := listResponse{Positions: positions, Count: len(positions)}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	n := &Netflix{baseURL: srv.URL, domain: "netflix.com", pageSize: 10}
	result, err := n.FetchJobs(context.Background(), "netflix")
	if err != nil {
		t.Fatalf("FetchJobs error: %v", err)
	}

	if len(result.Jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(result.Jobs))
	}

	j0 := result.Jobs[0]
	assertEqual(t, "ID", j0.ID, "netflix-790298021069")
	assertEqual(t, "Title", j0.Title, "Senior Software Engineer")
	assertEqual(t, "Location", j0.Location, "Los Gatos, CA")
	assertEqual(t, "URL", j0.URL, "https://explore.jobs.netflix.net/careers/job/790298021069")

	if j0.FullContent != nil {
		t.Error("FullContent must be nil — Netflix is a thin adapter")
	}

	j1 := result.Jobs[1]
	assertEqual(t, "ID", j1.ID, "netflix-790313385518")
	assertEqual(t, "Title", j1.Title, "Web Software Engineer 4")
}

// ── FetchJobs — pagination ──────────────────────────────────────────────────

func TestFetchJobs_Pagination(t *testing.T) {
	pagesCalled := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pagesCalled++
		start := r.URL.Query().Get("start")

		var positions []position
		switch start {
		case "0":
			// Full page of 3 items (= pageSize) → triggers next page
			for i := 0; i < 3; i++ {
				positions = append(positions, position{
					ID:   int64(1000 + i),
					Name: fmt.Sprintf("Job Page1-%d", i),
				})
			}
		case "3":
			// Partial page → last page
			positions = append(positions, position{
				ID:   2000,
				Name: "Job Page2-0",
			})
		default:
			t.Errorf("unexpected start param: %s", start)
		}

		resp := listResponse{Positions: positions, Count: 4}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	n := &Netflix{baseURL: srv.URL, domain: "netflix.com", pageSize: 3}
	result, err := n.FetchJobs(context.Background(), "netflix")
	if err != nil {
		t.Fatalf("FetchJobs error: %v", err)
	}

	if pagesCalled != 2 {
		t.Errorf("expected 2 API calls (paginated), got %d", pagesCalled)
	}
	if len(result.Jobs) != 4 {
		t.Fatalf("expected 4 total jobs, got %d", len(result.Jobs))
	}
	assertEqual(t, "last job ID", result.Jobs[3].ID, "netflix-2000")
}

// ── FetchJobs — empty ───────────────────────────────────────────────────────

func TestFetchJobs_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := listResponse{Positions: []position{}, Count: 0}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	n := &Netflix{baseURL: srv.URL, domain: "netflix.com", pageSize: 10}
	result, err := n.FetchJobs(context.Background(), "netflix")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Jobs) != 0 {
		t.Errorf("expected 0 jobs, got %d", len(result.Jobs))
	}
}

// ── FetchJobs — HTTP error ──────────────────────────────────────────────────

func TestFetchJobs_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	n := &Netflix{baseURL: srv.URL, domain: "netflix.com", pageSize: 10}
	_, err := n.FetchJobs(context.Background(), "netflix")
	if err == nil {
		t.Fatal("expected error on HTTP 500, got nil")
	}
	if !strings.Contains(err.Error(), "unexpected status 500") {
		t.Errorf("error should mention status 500, got: %s", err.Error())
	}
}

// ── FetchJobs — malformed JSON ──────────────────────────────────────────────

func TestFetchJobs_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`this is not json`))
	}))
	defer srv.Close()

	n := &Netflix{baseURL: srv.URL, domain: "netflix.com", pageSize: 10}
	_, err := n.FetchJobs(context.Background(), "netflix")
	if err == nil {
		t.Fatal("expected error on malformed JSON, got nil")
	}
}

// ── FetchJobs — context cancellation ────────────────────────────────────────

func TestFetchJobs_ContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"positions":[],"count":0}`))
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	n := &Netflix{baseURL: srv.URL, domain: "netflix.com", pageSize: 10}
	_, err := n.FetchJobs(ctx, "netflix")
	if err == nil {
		t.Fatal("expected error when context is cancelled")
	}
}

// ── FetchJobs — location fallback ───────────────────────────────────────────

func TestFetchJobs_LocationFallbackToLocations(t *testing.T) {
	positions := []position{
		{
			ID:        99001,
			Name:      "Engineer",
			Location:  "", // empty primary location
			Locations: []string{"New York, NY", "Los Angeles, CA"},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := listResponse{Positions: positions, Count: 1}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	n := &Netflix{baseURL: srv.URL, domain: "netflix.com", pageSize: 10}
	result, err := n.FetchJobs(context.Background(), "netflix")
	if err != nil {
		t.Fatalf("FetchJobs error: %v", err)
	}

	assertEqual(t, "Location", result.Jobs[0].Location, "New York, NY; Los Angeles, CA")
}

// ── FetchJobs — canonical URL fallback ──────────────────────────────────────

func TestFetchJobs_FallbackURL(t *testing.T) {
	const jobID int64 = 88001
	positions := []position{
		{
			ID:           jobID,
			Name:         "Engineer",
			CanonicalURL: "", // empty — should construct fallback URL
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := listResponse{Positions: positions, Count: 1}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	n := &Netflix{baseURL: srv.URL, domain: "netflix.com", pageSize: 10}
	result, err := n.FetchJobs(context.Background(), "netflix")
	if err != nil {
		t.Fatalf("FetchJobs error: %v", err)
	}

	expected := fmt.Sprintf("https://explore.jobs.netflix.net/careers/job/%d", jobID)
	assertEqual(t, "URL", result.Jobs[0].URL, expected)
}

// ── FetchJobContent ─────────────────────────────────────────────────────────

func TestFetchJobContent_Success(t *testing.T) {
	const tCreate int64 = 1743465600 // 2025-04-01T00:00:00Z
	detail := positionDetail{
		ID:                 790298021069,
		Name:               "Senior Software Engineer",
		Location:           "Los Gatos, CA",
		Department:         "Engineering",
		WorkLocationOption: "remote",
		CanonicalURL:       "https://explore.jobs.netflix.net/careers/job/790298021069",
		JobDescription:     "<p>Netflix is one of the world's leading entertainment services. We are looking for a <strong>Senior Software Engineer</strong> to join our team.</p>",
		TCreate:            tCreate,
	}

	var capturedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		assertMethod(t, r, http.MethodGet)

		// Real detail endpoint returns positionDetail flat — no {"position": ...} wrapper.
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(detail)
	}))
	defer srv.Close()

	n := &Netflix{baseURL: srv.URL, domain: "netflix.com", pageSize: 10}
	job := types.RawJob{
		ID:      "netflix-790298021069",
		Adapter: "netflix",
		URL:     "https://explore.jobs.netflix.net/careers/job/790298021069",
	}

	content, err := n.FetchJobContent(context.Background(), job)
	if err != nil {
		t.Fatalf("FetchJobContent error: %v", err)
	}

	if capturedPath != "/api/apply/v2/jobs/790298021069" {
		t.Errorf("API path = %q, want /api/apply/v2/jobs/790298021069", capturedPath)
	}
	assertEqual(t, "ID", content.ID, "netflix-790298021069")
	assertEqual(t, "Title", content.Title, "Senior Software Engineer")
	assertEqual(t, "Location", content.Location, "Los Gatos, CA")
	assertEqual(t, "Department", content.Department, "Engineering")
	assertEqual(t, "WorkplaceType", content.WorkplaceType, "remote")

	wantDate := time.Unix(tCreate, 0).UTC().Format(time.RFC3339)
	assertEqual(t, "PublishedDate", content.PublishedDate, wantDate)

	if content.Description == "" {
		t.Error("Description should not be empty")
	}
	// HTML should be stripped
	if strings.Contains(content.Description, "<p>") {
		t.Error("Description should not contain HTML tags")
	}
	if !strings.Contains(content.Description, "Senior Software Engineer") {
		t.Error("Description should contain job text")
	}
}

// ── FetchJobContent — HTTP error ────────────────────────────────────────────

func TestFetchJobContent_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	n := &Netflix{baseURL: srv.URL, domain: "netflix.com", pageSize: 10}
	job := types.RawJob{ID: "netflix-12345", Adapter: "netflix"}
	_, err := n.FetchJobContent(context.Background(), job)
	if err == nil {
		t.Fatal("expected error on HTTP 404, got nil")
	}
}

// ── FetchJobContent — malformed JSON ────────────────────────────────────────

func TestFetchJobContent_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{bad json`))
	}))
	defer srv.Close()

	n := &Netflix{baseURL: srv.URL, domain: "netflix.com", pageSize: 10}
	job := types.RawJob{ID: "netflix-12345", Adapter: "netflix"}
	_, err := n.FetchJobContent(context.Background(), job)
	if err == nil {
		t.Fatal("expected error on malformed JSON, got nil")
	}
}

// ── FetchJobContent — empty job ID ──────────────────────────────────────────

func TestFetchJobContent_EmptyID(t *testing.T) {
	n := New()
	job := types.RawJob{ID: "", Adapter: "netflix"}
	_, err := n.FetchJobContent(context.Background(), job)
	if err == nil {
		t.Fatal("expected error for empty job ID, got nil")
	}
}

// ── FetchJobContent — ID without prefix ─────────────────────────────────────

func TestFetchJobContent_IDWithoutPrefix(t *testing.T) {
	// IDs without the "netflix-" prefix are passed through as-is to the URL.
	// The detail response uses its own numeric ID for the output ID.
	detail := positionDetail{
		ID:   77001,
		Name: "Test Job",
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Real detail endpoint: flat positionDetail, no wrapper.
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(detail)
	}))
	defer srv.Close()

	n := &Netflix{baseURL: srv.URL, domain: "netflix.com", pageSize: 10}
	// ID without "netflix-" prefix — stripPrefix should pass it through as path
	job := types.RawJob{ID: "77001", Adapter: "netflix"}
	content, err := n.FetchJobContent(context.Background(), job)
	if err != nil {
		t.Fatalf("FetchJobContent error: %v", err)
	}
	assertEqual(t, "Title", content.Title, "Test Job")
	assertEqual(t, "ID", content.ID, "netflix-77001")
}

// ── unixToRFC3339 ───────────────────────────────────────────────────────────

func TestUnixToRFC3339(t *testing.T) {
	tests := []struct {
		ts   int64
		want string
	}{
		{0, ""},
		{1743465600, time.Unix(1743465600, 0).UTC().Format(time.RFC3339)},
		{1775779200, time.Unix(1775779200, 0).UTC().Format(time.RFC3339)},
	}
	for _, tc := range tests {
		got := unixToRFC3339(tc.ts)
		if got != tc.want {
			t.Errorf("unixToRFC3339(%d) = %q, want %q", tc.ts, got, tc.want)
		}
	}
}

// ── stripPrefix ─────────────────────────────────────────────────────────────

func TestStripPrefix(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"netflix-790298021069", "790298021069"},
		{"netflix-abc-def", "abc-def"},
		{"790298021069", "790298021069"},     // no prefix — pass through
		{"greenhouse-999", "greenhouse-999"}, // wrong prefix — pass through
	}
	for _, tc := range tests {
		got := stripPrefix(tc.input)
		if got != tc.want {
			t.Errorf("stripPrefix(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// ── URL building ────────────────────────────────────────────────────────────

func TestListURL(t *testing.T) {
	n := &Netflix{baseURL: "https://test.example.com", domain: "test.com", pageSize: 10}
	got := n.listURL(20)
	want := "https://test.example.com/api/apply/v2/jobs?domain=test.com&hl=en&start=20&num=10"
	if got != want {
		t.Errorf("listURL(20) = %q, want %q", got, want)
	}
}

func TestDetailURL(t *testing.T) {
	n := &Netflix{baseURL: "https://test.example.com", domain: "test.com", pageSize: 10}
	got := n.detailURL("790298021069")
	want := "https://test.example.com/api/apply/v2/jobs/790298021069?domain=test.com"
	if got != want {
		t.Errorf("detailURL() = %q, want %q", got, want)
	}
}

// ── FetchJobs — exact page boundary ─────────────────────────────────────────
// If the last real page returns exactly pageSize items, the adapter must fetch
// one more page (which returns 0 items < pageSize) before stopping.
// Total API calls = 3 for a 2-page result set where page 2 is full.

func TestFetchJobs_ExactPageBoundary(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		start := r.URL.Query().Get("start")
		var positions []position
		switch start {
		case "0", "3":
			// Both pages return exactly pageSize (3) items
			for i := 0; i < 3; i++ {
				positions = append(positions, position{
					ID:   int64(callCount*10 + i),
					Name: fmt.Sprintf("Job-%s-%d", start, i),
				})
			}
		default:
			// Third call returns 0 — signals end of results
		}
		resp := listResponse{Positions: positions, Count: 6}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	n := &Netflix{baseURL: srv.URL, domain: "netflix.com", pageSize: 3}
	result, err := n.FetchJobs(context.Background(), "netflix")
	if err != nil {
		t.Fatalf("FetchJobs error: %v", err)
	}
	if callCount != 3 {
		t.Errorf("expected 3 API calls (page0=full, page1=full, page2=empty), got %d", callCount)
	}
	if len(result.Jobs) != 6 {
		t.Errorf("expected 6 total jobs, got %d", len(result.Jobs))
	}
}

// ── FetchJobs — hl=en query param ───────────────────────────────────────────

func TestFetchJobs_HLParamSent(t *testing.T) {
	var capturedHL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHL = r.URL.Query().Get("hl")
		resp := listResponse{Positions: []position{}, Count: 0}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	n := &Netflix{baseURL: srv.URL, domain: "netflix.com", pageSize: 10}
	_, _ = n.FetchJobs(context.Background(), "netflix")

	if capturedHL != "en" {
		t.Errorf("hl query param = %q, want %q", capturedHL, "en")
	}
}

// ── FetchJobContent — location fallback ─────────────────────────────────────

func TestFetchJobContent_LocationFallbackToLocations(t *testing.T) {
	detail := positionDetail{
		ID:        55001,
		Name:      "Engineer",
		Location:  "", // empty — must fall back to Locations slice
		Locations: []string{"San Francisco, CA", "New York, NY"},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(detail)
	}))
	defer srv.Close()

	n := &Netflix{baseURL: srv.URL, domain: "netflix.com", pageSize: 10}
	job := types.RawJob{ID: "netflix-55001", Adapter: "netflix"}
	content, err := n.FetchJobContent(context.Background(), job)
	if err != nil {
		t.Fatalf("FetchJobContent error: %v", err)
	}
	assertEqual(t, "Location", content.Location, "San Francisco, CA; New York, NY")
}

// ── FetchJobContent — canonical URL fallback ─────────────────────────────────

func TestFetchJobContent_FallbackURL(t *testing.T) {
	const jobID int64 = 66001
	detail := positionDetail{
		ID:           jobID,
		Name:         "Engineer",
		CanonicalURL: "", // empty — must construct fallback URL
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(detail)
	}))
	defer srv.Close()

	n := &Netflix{baseURL: srv.URL, domain: "netflix.com", pageSize: 10}
	job := types.RawJob{ID: fmt.Sprintf("netflix-%d", jobID), Adapter: "netflix"}
	content, err := n.FetchJobContent(context.Background(), job)
	if err != nil {
		t.Fatalf("FetchJobContent error: %v", err)
	}
	expected := fmt.Sprintf("https://explore.jobs.netflix.net/careers/job/%d", jobID)
	assertEqual(t, "URL", content.URL, expected)
}

// ── FetchJobContent — context cancellation ───────────────────────────────────

func TestFetchJobContent_ContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	n := &Netflix{baseURL: srv.URL, domain: "netflix.com", pageSize: 10}
	job := types.RawJob{ID: "netflix-12345", Adapter: "netflix"}
	_, err := n.FetchJobContent(ctx, job)
	if err == nil {
		t.Fatal("expected error when context is cancelled")
	}
}

// ── normalizeWorkLocation ────────────────────────────────────────────────────

func TestNormalizeWorkLocation(t *testing.T) {
	// Netflix's Eightfold API doesn't expose employment type (full-time vs contract),
	// so the adapter always maps any work_location_option to full_time.
	cases := []struct {
		input string
		want  string
	}{
		{"remote", jobcommon.EmployFullTime},
		{"REMOTE", jobcommon.EmployFullTime},
		{"onsite", jobcommon.EmployFullTime},
		{"on-site", jobcommon.EmployFullTime},
		{"hybrid", jobcommon.EmployFullTime},
		{"", jobcommon.EmployFullTime},
		{"unknown_value", jobcommon.EmployFullTime},
	}
	for _, tc := range cases {
		got := normalizeWorkLocation(tc.input)
		if got != tc.want {
			t.Errorf("normalizeWorkLocation(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// ── FetchJobContent — domain query param ─────────────────────────────────────

func TestFetchJobContent_DomainParamSent(t *testing.T) {
	var capturedDomain string
	detail := positionDetail{ID: 77777, Name: "Engineer"}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedDomain = r.URL.Query().Get("domain")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(detail)
	}))
	defer srv.Close()

	n := &Netflix{baseURL: srv.URL, domain: "testdomain.com", pageSize: 10}
	job := types.RawJob{ID: "netflix-77777", Adapter: "netflix"}
	_, _ = n.FetchJobContent(context.Background(), job)

	if capturedDomain != "testdomain.com" {
		t.Errorf("domain query param = %q, want %q", capturedDomain, "testdomain.com")
	}
}

// ── Real integration tests ───────────────────────────────────────────────────
// Guard: set TEST_REAL=1 to run. These hit the live Eightfold API.

func TestFetchJobs_Real(t *testing.T) {
	if os.Getenv("TEST_REAL") == "" {
		t.Skip("skipping real API test; set TEST_REAL=1 to run")
	}

	n := New()
	result, err := n.FetchJobs(context.Background(), "")
	if err != nil {
		t.Fatalf("FetchJobs error: %v", err)
	}
	if len(result.Jobs) == 0 {
		t.Fatal("expected at least one job from live Netflix API")
	}

	// Spot-check the first job
	j := result.Jobs[0]
	if !strings.HasPrefix(j.ID, "netflix-") {
		t.Errorf("ID %q does not have 'netflix-' prefix", j.ID)
	}
	if j.Title == "" {
		t.Error("Title should not be empty")
	}
	if j.URL == "" {
		t.Error("URL should not be empty")
	}
	if j.FullContent != nil {
		t.Error("FullContent must be nil — Netflix is a thin adapter")
	}

	t.Logf("fetched %d jobs; first: %q at %q", len(result.Jobs), j.Title, j.Location)
}

func TestFetchJobContent_Real(t *testing.T) {
	if os.Getenv("TEST_REAL") == "" {
		t.Skip("skipping real API test; set TEST_REAL=1 to run")
	}

	n := New()

	// First fetch the listing to get a real job ID
	listResult, err := n.FetchJobs(context.Background(), "")
	if err != nil {
		t.Fatalf("FetchJobs error: %v", err)
	}
	if len(listResult.Jobs) == 0 {
		t.Skip("no jobs returned from listing — cannot test FetchJobContent")
	}

	first := listResult.Jobs[0]
	job := types.RawJob{
		ID:      first.ID,
		Adapter: companycommon.AdapterNetflix,
		URL:     first.URL,
	}

	content, err := n.FetchJobContent(context.Background(), job)
	if err != nil {
		t.Fatalf("FetchJobContent error: %v", err)
	}

	if content.ID == "" {
		t.Error("ID should not be empty")
	}
	if content.Title == "" {
		t.Error("Title should not be empty")
	}
	if content.Description == "" {
		t.Error("Description should not be empty")
	}
	if strings.Contains(content.Description, "<") {
		t.Error("Description should not contain HTML tags — StripHTML should have cleaned it")
	}

	t.Logf("job: %q | location: %q | dept: %q | published: %q | desc length: %d",
		content.Title, content.Location, content.Department, content.PublishedDate, len(content.Description))
}

// ── Helpers ─────────────────────────────────────────────────────────────────

func assertMethod(t *testing.T, r *http.Request, want string) {
	t.Helper()
	if r.Method != want {
		t.Errorf("method = %s, want %s", r.Method, want)
	}
}

func assertQueryParam(t *testing.T, r *http.Request, key, want string) {
	t.Helper()
	got := r.URL.Query().Get(key)
	if got != want {
		t.Errorf("query param %q = %q, want %q", key, got, want)
	}
}

func assertEqual(t *testing.T, field, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("%s: got %q, want %q", field, got, want)
	}
}

// ── Resolver fallback ───────────────────────────────────────────────────────

// TestResolveBaseURL_ZeroValueFallsBackToDefault — empty branch coverage.
func TestResolveBaseURL_ZeroValueFallsBackToDefault(t *testing.T) {
	n := &Netflix{}
	if got := n.resolveBaseURL(); got != defaultBaseURL {
		t.Errorf("zero-value resolveBaseURL: got %q, want %q", got, defaultBaseURL)
	}
}

// TestResolveDomain_ZeroValueFallsBackToDefault — empty branch coverage.
func TestResolveDomain_ZeroValueFallsBackToDefault(t *testing.T) {
	n := &Netflix{}
	if got := n.resolveDomain(); got != defaultDomain {
		t.Errorf("zero-value resolveDomain: got %q, want %q", got, defaultDomain)
	}
}
