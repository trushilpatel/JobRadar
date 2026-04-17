package microsoft

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/trushilpatel/jobradar/model"
	"github.com/trushilpatel/jobradar/pipeline/ats/types"
)

// ── Interface compliance ────────────────────────────────────────────────────

func TestMicrosoft_ImplementsAdapter(t *testing.T) {
	var _ types.Adapter = (*Microsoft)(nil)
}

func TestMicrosoft_ImplementsContentFetcher(t *testing.T) {
	var _ types.ContentFetcher = (*Microsoft)(nil)
}

func TestMicrosoft_ImplementsURLProvider(t *testing.T) {
	var _ types.URLProvider = (*Microsoft)(nil)
}

// ── Constructor ─────────────────────────────────────────────────────────────

func TestNew_DefaultSettings(t *testing.T) {
	m := New()
	if m.baseURL != defaultBaseURL {
		t.Errorf("baseURL = %q, want %q", m.baseURL, defaultBaseURL)
	}
	if m.domain != defaultDomain {
		t.Errorf("domain = %q, want %q", m.domain, defaultDomain)
	}
	if m.pageSize != defaultPageSize {
		t.Errorf("pageSize = %d, want %d", m.pageSize, defaultPageSize)
	}
}

// ── CareersURL ──────────────────────────────────────────────────────────────

func TestCareersURL(t *testing.T) {
	m := New()
	got := m.CareersURL("anything")
	want := "https://careers.microsoft.com/us/en/careers"
	if got != want {
		t.Errorf("CareersURL() = %q, want %q", got, want)
	}
}

// ── FetchJobs — single page ────────────────────────────────────────────────

func TestFetchJobs_SinglePage(t *testing.T) {
	srv := newTestServer(t, []position{
		{
			ID:                 1812345678,
			DisplayJobID:       "1812345678",
			Name:               "Senior Software Engineer",
			Locations:          []string{"Redmond, Washington, United States"},
			PostedTs:           1712678400,
			Department:         "Engineering",
			WorkLocationOption: "Up to 50% work from home",
			AtsJobID:           "REQ-12345",
			PositionURL:        "https://careers.microsoft.com/v2/global/en/job/1812345678/senior-software-engineer",
		},
	})
	defer srv.Close()

	m := &Microsoft{baseURL: srv.URL}
	result, err := m.FetchJobs(context.Background(), "")
	if err != nil {
		t.Fatalf("FetchJobs error: %v", err)
	}

	if len(result.Jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(result.Jobs))
	}

	j := result.Jobs[0]
	assertEqual(t, "ID", j.ID, fmt.Sprintf("%s-1812345678", model.AdapterMicrosoft))
	assertEqual(t, "Title", j.Title, "Senior Software Engineer")
	assertEqual(t, "Location", j.Location, "Redmond, Washington, United States")
	if !strings.HasPrefix(j.URL, "https://apply.careers.microsoft.com/careers/job/") {
		t.Errorf("URL should have internal format, got %q", j.URL)
	}
	assertEqual(t, "EmploymentType", j.EmploymentType, model.EmployFullTime)

	if j.FullContent != nil {
		t.Fatal("FullContent must be nil — Microsoft is a thin adapter")
	}
}

// ── FetchJobs — multiple pages with pagination ─────────────────────────────

func TestFetchJobs_MultiPage_Pagination(t *testing.T) {
	pageCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		start := 0
		if s := r.URL.Query().Get("start"); s != "" {
			fmt.Sscanf(s, "%d", &start)
		}

		// Return 20 jobs for start=0, 20 for start=20, 10 for start=40 (end of pagination)
		var positions []position
		switch start {
		case 0:
			for i := 0; i < 20; i++ {
				positions = append(positions, position{
					ID:           int64(1000 + i),
					DisplayJobID: fmt.Sprintf("%d", 1000+i),
					Name:         fmt.Sprintf("Job %d", i),
					PostedTs:     1712678400,
				})
			}
		case 20:
			for i := 0; i < 20; i++ {
				positions = append(positions, position{
					ID:           int64(1020 + i),
					DisplayJobID: fmt.Sprintf("%d", 1020+i),
					Name:         fmt.Sprintf("Job %d", 20+i),
					PostedTs:     1712678400,
				})
			}
		case 40:
			for i := 0; i < 10; i++ {
				positions = append(positions, position{
					ID:           int64(1040 + i),
					DisplayJobID: fmt.Sprintf("%d", 1040+i),
					Name:         fmt.Sprintf("Job %d", 40+i),
					PostedTs:     1712678400,
				})
			}
		}

		pageCount++
		json.NewEncoder(w).Encode(searchResponse{
			Status: "success",
			Data: searchResponseData{
				Count:     50,
				Positions: positions,
			},
		})
	}))
	defer srv.Close()

	m := &Microsoft{baseURL: srv.URL}
	result, err := m.FetchJobs(context.Background(), "")
	if err != nil {
		t.Fatalf("FetchJobs error: %v", err)
	}

	if len(result.Jobs) != 50 {
		t.Fatalf("expected 50 jobs total, got %d", len(result.Jobs))
	}

	if pageCount != 3 {
		t.Fatalf("expected 3 API calls (pagination), got %d", pageCount)
	}
}

// ── FetchJobs — empty results ───────────────────────────────────────────────

func TestFetchJobs_EmptyResults(t *testing.T) {
	srv := newTestServer(t, []position{})
	defer srv.Close()

	m := &Microsoft{baseURL: srv.URL}
	result, err := m.FetchJobs(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Jobs) != 0 {
		t.Errorf("expected 0 jobs, got %d", len(result.Jobs))
	}
}

// ── FetchJobs — HTTP error ─────────────────────────────────────────────────

func TestFetchJobs_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	m := &Microsoft{baseURL: srv.URL}
	_, err := m.FetchJobs(context.Background(), "")
	if err == nil {
		t.Fatal("expected error on HTTP 500, got nil")
	}
	if !strings.Contains(err.Error(), "unexpected status 500") {
		t.Errorf("error should mention status code: %v", err)
	}
}

// ── FetchJobs — malformed JSON ──────────────────────────────────────────────

func TestFetchJobs_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`not valid json`))
	}))
	defer srv.Close()

	m := &Microsoft{baseURL: srv.URL}
	_, err := m.FetchJobs(context.Background(), "")
	if err == nil {
		t.Fatal("expected error on malformed JSON, got nil")
	}
	if !strings.Contains(err.Error(), "decode") {
		t.Errorf("error should mention 'decode': %v", err)
	}
}

// ── FetchJobs — context cancelled ───────────────────────────────────────────

func TestFetchJobs_ContextCancelled(t *testing.T) {
	srv := newTestServer(t, []position{})
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	m := &Microsoft{baseURL: srv.URL}
	_, err := m.FetchJobs(ctx, "")
	if err == nil {
		t.Fatal("expected error when context is cancelled")
	}
}

// ── FetchJobs — FullContent is nil ─────────────────────────────────────────

func TestFetchJobs_FullContentNil(t *testing.T) {
	srv := newTestServer(t, []position{
		{
			ID:           1812345678,
			DisplayJobID: "1812345678",
			Name:         "Senior Software Engineer",
			PostedTs:     1712678400,
		},
	})
	defer srv.Close()

	m := &Microsoft{baseURL: srv.URL}
	result, err := m.FetchJobs(context.Background(), "")
	if err != nil {
		t.Fatalf("FetchJobs error: %v", err)
	}

	if len(result.Jobs) > 0 && result.Jobs[0].FullContent != nil {
		t.Error("FullContent must be nil for thin adapter")
	}
}

// ── FetchJobs — job URL from PositionURL ───────────────────────────────────

func TestFetchJobs_JobURLFromPositionURL(t *testing.T) {
	srv := newTestServer(t, []position{
		{
			ID:           1812345678,
			DisplayJobID: "1812345678",
			Name:         "Senior Software Engineer",
			PostedTs:     1712678400,
			PositionURL:  "https://careers.microsoft.com/v2/global/en/job/1812345678/senior-software-engineer",
		},
	})
	defer srv.Close()

	m := &Microsoft{baseURL: srv.URL}
	result, err := m.FetchJobs(context.Background(), "")
	if err != nil {
		t.Fatalf("FetchJobs error: %v", err)
	}

	// The internal URL should be stored for ID extraction
	if len(result.Jobs) > 0 {
		if !strings.Contains(result.Jobs[0].URL, "1812345678") {
			t.Errorf("internal URL should contain position ID, got %q", result.Jobs[0].URL)
		}
	}
}

// ── FetchJobs — location filter verified ────────────────────────────────────

func TestFetchJobs_LocationFilter(t *testing.T) {
	var capturedURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		capturedURL = r.URL.String()
		json.NewEncoder(w).Encode(searchResponse{
			Status: "success",
			Data: searchResponseData{
				Count:     0,
				Positions: []position{},
			},
		})
	}))
	defer srv.Close()

	m := &Microsoft{baseURL: srv.URL}
	m.FetchJobs(context.Background(), "")

	// The searchURL encodes spaces as + (x-www-form-urlencoded) and commas as %2C.
	// Check that the location filter is present in the URL.
	if !strings.Contains(capturedURL, "location=United") || !strings.Contains(capturedURL, "Multiple") {
		t.Errorf("URL should contain US location filter, got %q", capturedURL)
	}
}

// ── FetchJobContent — success ───────────────────────────────────────────────

func TestFetchJobContent_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Verify position_id is in query
		posID := r.URL.Query().Get("position_id")
		if posID != "1812345678" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		json.NewEncoder(w).Encode(positionDetail{
			ID:                         1812345678,
			DisplayJobID:               "1812345678",
			Name:                       "Senior Software Engineer",
			Locations:                  []string{"Redmond, Washington, United States"},
			PostedTs:                   1712678400,
			Department:                 "Engineering",
			WorkLocationOption:         "Up to 50% work from home",
			JobDescription:             "<p>We are looking for a <b>talented engineer</b>.</p>",
			Location:                   "Redmond, Washington, United States",
			EfcustomTextEmploymentType: []string{"Full-Time"},
			PositionURL:                "https://careers.microsoft.com/v2/global/en/job/1812345678/senior-software-engineer",
			AtsJobID:                   "REQ-12345",
		})
	}))
	defer srv.Close()

	m := &Microsoft{baseURL: srv.URL}
	job := types.RawJob{
		ID:  "microsoft-1812345678",
		URL: fmt.Sprintf("%s/careers/job/1812345678", srv.URL),
	}

	result, err := m.FetchJobContent(context.Background(), job)
	if err != nil {
		t.Fatalf("FetchJobContent error: %v", err)
	}

	assertEqual(t, "ID", result.ID, fmt.Sprintf("%s-1812345678", model.AdapterMicrosoft))
	assertEqual(t, "Title", result.Title, "Senior Software Engineer")
	assertEqual(t, "Location", result.Location, "Redmond, Washington, United States")
	assertEqual(t, "Department", result.Department, "Engineering")
	assertEqual(t, "EmploymentType", result.EmploymentType, model.EmployFullTime)
	assertEqual(t, "WorkplaceType", result.WorkplaceType, "Up to 50% work from home")

	// Verify HTML was stripped
	if strings.Contains(result.Description, "<") {
		t.Errorf("Description should have HTML stripped, got %q", result.Description)
	}
	if !strings.Contains(result.Description, "talented engineer") {
		t.Errorf("Description should contain text content, got %q", result.Description)
	}

	// Verify published date format
	if result.PublishedDate == "" {
		t.Error("PublishedDate should not be empty")
	}
}

// ── FetchJobContent — HTTP error ────────────────────────────────────────────

func TestFetchJobContent_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	m := &Microsoft{baseURL: srv.URL}
	job := types.RawJob{
		URL: fmt.Sprintf("%s/careers/job/1812345678", srv.URL),
	}

	_, err := m.FetchJobContent(context.Background(), job)
	if err == nil {
		t.Fatal("expected error on HTTP 404, got nil")
	}
}

// ── FetchJobContent — malformed JSON ────────────────────────────────────────

func TestFetchJobContent_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`not json`))
	}))
	defer srv.Close()

	m := &Microsoft{baseURL: srv.URL}
	job := types.RawJob{
		URL: fmt.Sprintf("%s/careers/job/1812345678", srv.URL),
	}

	_, err := m.FetchJobContent(context.Background(), job)
	if err == nil {
		t.Fatal("expected error on malformed JSON, got nil")
	}
}

// ── mapEmploymentType ───────────────────────────────────────────────────────

func TestMapEmploymentType(t *testing.T) {
	tests := []struct {
		input []string
		want  string
	}{
		{[]string{"Full-Time"}, model.EmployFullTime},
		{[]string{"Part-Time"}, model.EmployPartTime},
		{[]string{"Contract"}, model.EmployContract},
		{[]string{"Intern"}, model.EmployInternship},
		{[]string{"Internship"}, model.EmployInternship},
		{[]string{""}, model.EmployFullTime},
		{[]string{"Unknown"}, model.EmployFullTime},
		{[]string{}, model.EmployFullTime},
	}

	for _, tc := range tests {
		got := mapEmploymentType(tc.input)
		if got != tc.want {
			t.Errorf("mapEmploymentType(%v) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// ── extractPositionID ───────────────────────────────────────────────────────

func TestExtractPositionID(t *testing.T) {
	tests := []struct {
		url  string
		want int64
	}{
		{"https://apply.careers.microsoft.com/careers/job/1812345678", 1812345678},
		{"https://apply.careers.microsoft.com/careers/job/1812345678/", 1812345678},
		{"https://example.com/careers/job/999999999", 999999999},
		{"https://example.com/careers/job/invalid", 0},
		{"https://example.com/careers/job/", 0},
		{"", 0},
	}

	for _, tc := range tests {
		got := extractPositionID(tc.url)
		if got != tc.want {
			t.Errorf("extractPositionID(%q) = %d, want %d", tc.url, got, tc.want)
		}
	}
}

// ── FetchJobs — Real integration test ───────────────────────────────────────
// Guard: set TEST_REAL=1 to run. This hits the live Microsoft careers API.

func TestFetchJobs_Real(t *testing.T) {
	if os.Getenv("TEST_REAL") == "" {
		t.Skip("skipping real API test; set TEST_REAL=1 to run")
	}

	m := New()
	result, err := m.FetchJobs(context.Background(), "")
	if err != nil {
		t.Fatalf("FetchJobs error: %v", err)
	}
	if len(result.Jobs) == 0 {
		t.Fatal("expected at least one job from live Microsoft API")
	}

	// Spot-check the first job
	j := result.Jobs[0]
	if !strings.HasPrefix(j.ID, "microsoft-") {
		t.Errorf("ID %q does not have 'microsoft-' prefix", j.ID)
	}
	if j.Title == "" {
		t.Error("Title should not be empty")
	}
	if j.URL == "" {
		t.Error("URL should not be empty")
	}
	if j.EmploymentType == "" {
		t.Error("EmploymentType should not be empty")
	}
	if j.FullContent != nil {
		t.Error("FullContent must be nil — Microsoft is a thin adapter")
	}

	t.Logf("fetched %d jobs; first: %q at %q", len(result.Jobs), j.Title, j.Location)
}

// ── FetchJobContent — Real integration test ──────────────────────────────────
// Guard: set TEST_REAL=1 to run. Calls FetchJobs then FetchJobContent on one job.

func TestFetchJobContent_Real(t *testing.T) {
	if os.Getenv("TEST_REAL") == "" {
		t.Skip("skipping real API test; set TEST_REAL=1 to run")
	}

	m := New()

	// Get one page of jobs to find a real position ID.
	result, err := m.FetchJobs(context.Background(), "")
	if err != nil {
		t.Fatalf("FetchJobs error: %v", err)
	}
	if len(result.Jobs) == 0 {
		t.Fatal("no jobs returned — cannot test FetchJobContent")
	}

	// Construct a RawJob from the first listing.
	firstJob := result.Jobs[0]
	rawJob := types.RawJob{
		ID:      firstJob.ID,
		URL:     firstJob.URL,
		Title:   firstJob.Title,
		Adapter: model.AdapterMicrosoft,
	}

	content, err := m.FetchJobContent(context.Background(), rawJob)
	if err != nil {
		t.Fatalf("FetchJobContent error: %v", err)
	}

	// Structural checks
	if content.Title == "" {
		t.Error("FetchJobContent: Title should not be empty")
	}
	if content.Description == "" {
		t.Error("FetchJobContent: Description should not be empty — jobDescription field missing from detail API?")
	}
	if content.Department == "" {
		t.Log("FetchJobContent: Department is empty (may be normal for some roles)")
	}

	t.Logf("FetchJobContent result: title=%q dept=%q workplace=%q employ=%q published=%q",
		content.Title, content.Department, content.WorkplaceType, content.EmploymentType, content.PublishedDate)

	// ── Salary in description audit ──────────────────────────────────────────
	// Microsoft does not expose a structured salary field — salary info (if any)
	// is embedded in jobDescription HTML (stripped to plain text).
	desc := strings.ToLower(content.Description)
	hasSalary := strings.Contains(desc, "$") || strings.Contains(desc, "salary") ||
		strings.Contains(desc, "compensation") || strings.Contains(desc, "usd")
	hasYOE := strings.Contains(desc, "years of experience") || strings.Contains(desc, "year(s) of experience") ||
		strings.Contains(desc, "years experience")

	t.Logf("[Salary] description contains salary keywords: %v", hasSalary)
	t.Logf("[YOE]    description contains YOE keywords: %v", hasYOE)

	if !hasSalary {
		t.Log("[Salary] Microsoft detail API description does not contain salary data for this job — Compensation field will be empty; LLM cannot extract what is not there")
	}
	if !hasYOE {
		t.Log("[YOE] Microsoft detail API description does not contain explicit YOE for this job")
	}

	// Compensation field is always empty for Microsoft — verify and document.
	if content.Compensation != "" {
		t.Logf("[Salary] UNEXPECTED: Compensation is non-empty: %q — update detailToContent() to map this field", content.Compensation)
	} else {
		t.Log("[Salary] Compensation field is empty (expected — not in Microsoft position_details API)")
	}

	t.Logf("Description preview (first 500 chars):\n%s", truncateStr(content.Description, 500))
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// ── Helpers ──────────────────────────────────────────────────────────────────

// newTestServer creates an httptest.Server that serves the Microsoft API.
func newTestServer(t *testing.T, positions []position) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		start := 0
		if s := r.URL.Query().Get("start"); s != "" {
			fmt.Sscanf(s, "%d", &start)
		}

		// Return all positions for simplicity (real API handles pagination)
		json.NewEncoder(w).Encode(searchResponse{
			Status: "success",
			Data: searchResponseData{
				Count:     len(positions),
				Positions: positions,
			},
		})
	}))
}

func assertEqual(t *testing.T, field, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("%s: got %q, want %q", field, got, want)
	}
}
