package apple

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/trushilpatel/jobradar/pipeline/ats/types"
	companycommon "github.com/trushilpatel/jobradar/service/company/common"
	jobcommon "github.com/trushilpatel/jobradar/service/job/common"
)

// ── Interface compliance ────────────────────────────────────────────────────

func TestApple_ImplementsAdapter(t *testing.T) {
	var _ types.Adapter = (*Apple)(nil)
}

func TestApple_ImplementsURLProvider(t *testing.T) {
	var _ types.URLProvider = (*Apple)(nil)
}

// ── Constructor ─────────────────────────────────────────────────────────────

func TestNew_DefaultSettings(t *testing.T) {
	a := New()
	if a.baseURL != defaultBaseURL {
		t.Errorf("baseURL = %q, want %q", a.baseURL, defaultBaseURL)
	}
}

// ── CareersURL ──────────────────────────────────────────────────────────────

func TestCareersURL(t *testing.T) {
	a := New()
	got := a.CareersURL("anything")
	want := "https://www.apple.com/careers/us/"
	if got != want {
		t.Errorf("CareersURL() = %q, want %q", got, want)
	}
}

// ── FetchJobs — single page ────────────────────────────────────────────────

func TestFetchJobs_SinglePage(t *testing.T) {
	srv := newTestServer(t, []pageConfig{
		{
			page: 1,
			jobs: []appleJob{
				{
					ID:                      "200647207-3543",
					PositionID:              "200647207",
					PostingTitle:            "Software Engineer",
					TransformedPostingTitle: "software-engineer",
					JobSummary:              "<p>About the position: Build great software.</p>",
					Locations: []appleLocation{
						{City: "Cupertino", StateCode: "CA", CountryCode: "USA"},
					},
					PostDateInGMT:       "2026-03-15 00:00:00",
					Team:                appleTeam{TeamName: "Software and Services", TeamCode: "SWDVR"},
					Type:                "Regular",
					StandardWeeklyHours: 40,
				},
			},
		},
		{page: 2, jobs: []appleJob{}}, // empty second page stops iteration
	})
	defer srv.Close()

	a := &Apple{baseURL: srv.URL}
	result, err := a.FetchJobs(context.Background(), "apple")
	if err != nil {
		t.Fatalf("FetchJobs error: %v", err)
	}

	if len(result.Jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(result.Jobs))
	}

	j := result.Jobs[0]
	assertEqual(t, "ID", j.ID, "apple-200647207-3543")
	assertEqual(t, "Title", j.Title, "Software Engineer")
	assertEqual(t, "Location", j.Location, "Cupertino, CA")
	assertEqual(t, "URL", j.URL, "https://jobs.apple.com/en-us/details/200647207-3543/software-engineer")
	assertEqual(t, "EmploymentType", j.EmploymentType, jobcommon.EmployFullTime)

	if j.FullContent == nil {
		t.Fatal("FullContent must not be nil — Apple is a fat adapter")
	}
	if !strings.Contains(j.FullContent.Description, "Build great software") {
		t.Error("FullContent.Description should contain job text without HTML")
	}
	assertEqual(t, "Department", j.FullContent.Department, "Software and Services")
}

// ── FetchJobs — multiple pages ───────────────────────────────────────────────

func TestFetchJobs_MultiPage_Pagination(t *testing.T) {
	srv := newTestServer(t, []pageConfig{
		{
			page: 1,
			jobs: []appleJob{
				{ID: "100001-1", PostingTitle: "Job 1", TransformedPostingTitle: "job-1", Type: "Regular"},
				{ID: "100002-2", PostingTitle: "Job 2", TransformedPostingTitle: "job-2", Type: "Regular"},
			},
		},
		{
			page: 2,
			jobs: []appleJob{
				{ID: "100003-3", PostingTitle: "Job 3", TransformedPostingTitle: "job-3", Type: "Regular"},
				{ID: "100004-4", PostingTitle: "Job 4", TransformedPostingTitle: "job-4", Type: "Regular"},
			},
		},
		{
			page: 3,
			jobs: []appleJob{
				{ID: "100005-5", PostingTitle: "Job 5", TransformedPostingTitle: "job-5", Type: "Regular"},
			},
		},
		{page: 4, jobs: []appleJob{}}, // empty page stops iteration
	})
	defer srv.Close()

	a := &Apple{baseURL: srv.URL}
	result, err := a.FetchJobs(context.Background(), "apple")
	if err != nil {
		t.Fatalf("FetchJobs error: %v", err)
	}

	if len(result.Jobs) != 5 {
		t.Fatalf("expected 5 jobs across 3 pages, got %d", len(result.Jobs))
	}

	expectedIDs := []string{"apple-100001-1", "apple-100002-2", "apple-100003-3", "apple-100004-4", "apple-100005-5"}
	for i, job := range result.Jobs {
		if job.ID != expectedIDs[i] {
			t.Errorf("job %d: ID = %q, want %q", i, job.ID, expectedIDs[i])
		}
	}
}

// ── FetchJobs — empty first page ────────────────────────────────────────────

func TestFetchJobs_EmptyFirstPage(t *testing.T) {
	srv := newTestServer(t, []pageConfig{
		{page: 1, jobs: []appleJob{}},
	})
	defer srv.Close()

	a := &Apple{baseURL: srv.URL}
	result, err := a.FetchJobs(context.Background(), "apple")
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

	a := &Apple{baseURL: srv.URL}
	_, err := a.FetchJobs(context.Background(), "apple")
	if err == nil {
		t.Fatal("expected error on HTTP 500, got nil")
	}
	if !strings.Contains(err.Error(), "unexpected status 500") {
		t.Errorf("error should mention status 500: %v", err)
	}
}

// ── FetchJobs — malformed HTML (no hydration data) ─────────────────────────

func TestFetchJobs_MalformedHTML(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<html><body>No hydration data here</body></html>`))
	}))
	defer srv.Close()

	a := &Apple{baseURL: srv.URL}
	_, err := a.FetchJobs(context.Background(), "apple")
	if err == nil {
		t.Fatal("expected error when hydration data is missing")
	}
	if !strings.Contains(err.Error(), "window.__staticRouterHydrationData not found") {
		t.Errorf("error should mention missing hydration data: %v", err)
	}
}

// ── FetchJobs — ID format ───────────────────────────────────────────────────

func TestFetchJobs_IDFormat(t *testing.T) {
	srv := newTestServer(t, []pageConfig{
		{
			page: 1,
			jobs: []appleJob{
				{ID: "200647207-3543", PostingTitle: "Test Job", TransformedPostingTitle: "test-job", Type: "Regular"},
			},
		},
		{page: 2, jobs: []appleJob{}},
	})
	defer srv.Close()

	a := &Apple{baseURL: srv.URL}
	result, err := a.FetchJobs(context.Background(), "apple")
	if err != nil {
		t.Fatalf("FetchJobs error: %v", err)
	}

	expectedID := fmt.Sprintf("%s-200647207-3543", companycommon.AdapterApple)
	if result.Jobs[0].ID != expectedID {
		t.Errorf("ID = %q, want %q", result.Jobs[0].ID, expectedID)
	}
}

// ── FetchJobs — job URL ─────────────────────────────────────────────────────

func TestFetchJobs_JobURL(t *testing.T) {
	srv := newTestServer(t, []pageConfig{
		{
			page: 1,
			jobs: []appleJob{
				{ID: "999-123", PostingTitle: "Senior Engineer", TransformedPostingTitle: "senior-engineer", Type: "Regular"},
			},
		},
		{page: 2, jobs: []appleJob{}},
	})
	defer srv.Close()

	a := &Apple{baseURL: srv.URL}
	result, err := a.FetchJobs(context.Background(), "apple")
	if err != nil {
		t.Fatalf("FetchJobs error: %v", err)
	}

	wantURL := "https://jobs.apple.com/en-us/details/999-123/senior-engineer"
	if result.Jobs[0].URL != wantURL {
		t.Errorf("URL = %q, want %q", result.Jobs[0].URL, wantURL)
	}
}

// ── FetchJobs — FullContent always set ──────────────────────────────────────

func TestFetchJobs_FullContentAlwaysSet(t *testing.T) {
	srv := newTestServer(t, []pageConfig{
		{
			page: 1,
			jobs: []appleJob{
				{ID: "100-1", PostingTitle: "Job", TransformedPostingTitle: "job", JobSummary: "<p>Full description</p>", Type: "Regular"},
			},
		},
		{page: 2, jobs: []appleJob{}},
	})
	defer srv.Close()

	a := &Apple{baseURL: srv.URL}
	result, err := a.FetchJobs(context.Background(), "apple")
	if err != nil {
		t.Fatalf("FetchJobs error: %v", err)
	}

	if result.Jobs[0].FullContent == nil {
		t.Fatal("FullContent must not be nil — Apple is a fat adapter")
	}
}

// ── FetchJobs — description strips HTML ─────────────────────────────────────

func TestFetchJobs_DescriptionStripsHTML(t *testing.T) {
	htmlSummary := `<p>About the role:</p><ul><li>Skill 1</li><li>Skill 2</li></ul><p>Great opportunity!</p>`
	srv := newTestServer(t, []pageConfig{
		{
			page: 1,
			jobs: []appleJob{
				{ID: "100-1", PostingTitle: "Job", TransformedPostingTitle: "job", JobSummary: htmlSummary, Type: "Regular"},
			},
		},
		{page: 2, jobs: []appleJob{}},
	})
	defer srv.Close()

	a := &Apple{baseURL: srv.URL}
	result, err := a.FetchJobs(context.Background(), "apple")
	if err != nil {
		t.Fatalf("FetchJobs error: %v", err)
	}

	description := result.Jobs[0].FullContent.Description
	// Should not contain HTML tags
	if strings.Contains(description, "<p>") || strings.Contains(description, "<li>") {
		t.Errorf("Description should not contain HTML tags: %q", description)
	}
	// Should contain the text content
	if !strings.Contains(description, "Skill 1") || !strings.Contains(description, "Great opportunity") {
		t.Errorf("Description should contain text content: %q", description)
	}
}

// ── FetchJobs — location format (single location) ──────────────────────────

func TestFetchJobs_LocationFormat(t *testing.T) {
	srv := newTestServer(t, []pageConfig{
		{
			page: 1,
			jobs: []appleJob{
				{
					ID:                      "100-1",
					PostingTitle:            "Job",
					TransformedPostingTitle: "job",
					Locations: []appleLocation{
						{City: "San Francisco", StateCode: "CA", CountryCode: "USA"},
					},
					Type: "Regular",
				},
			},
		},
		{page: 2, jobs: []appleJob{}},
	})
	defer srv.Close()

	a := &Apple{baseURL: srv.URL}
	result, err := a.FetchJobs(context.Background(), "apple")
	if err != nil {
		t.Fatalf("FetchJobs error: %v", err)
	}

	wantLocation := "San Francisco, CA"
	if result.Jobs[0].Location != wantLocation {
		t.Errorf("Location = %q, want %q", result.Jobs[0].Location, wantLocation)
	}
}

// ── FetchJobs — multiple locations ──────────────────────────────────────────

func TestFetchJobs_MultiLocation(t *testing.T) {
	srv := newTestServer(t, []pageConfig{
		{
			page: 1,
			jobs: []appleJob{
				{
					ID:                      "100-1",
					PostingTitle:            "Job",
					TransformedPostingTitle: "job",
					Locations: []appleLocation{
						{City: "Cupertino", StateCode: "CA", CountryCode: "USA"},
						{City: "Austin", StateCode: "TX", CountryCode: "USA"},
						{City: "New York", StateCode: "NY", CountryCode: "USA"},
					},
					Type: "Regular",
				},
			},
		},
		{page: 2, jobs: []appleJob{}},
	})
	defer srv.Close()

	a := &Apple{baseURL: srv.URL}
	result, err := a.FetchJobs(context.Background(), "apple")
	if err != nil {
		t.Fatalf("FetchJobs error: %v", err)
	}

	wantLocation := "Cupertino, CA; Austin, TX; New York, NY"
	if result.Jobs[0].Location != wantLocation {
		t.Errorf("Location = %q, want %q", result.Jobs[0].Location, wantLocation)
	}
}

// ── FetchJobs — context cancelled ───────────────────────────────────────────

func TestFetchJobs_ContextCancelled(t *testing.T) {
	srv := newTestServer(t, []pageConfig{
		{page: 1, jobs: []appleJob{}},
	})
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	a := &Apple{baseURL: srv.URL}
	_, err := a.FetchJobs(ctx, "apple")
	if err == nil {
		t.Fatal("expected error when context is cancelled")
	}
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

// ── mapJobType ──────────────────────────────────────────────────────────────

func TestMapJobType(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Regular", jobcommon.EmployFullTime},
		{"regular", jobcommon.EmployFullTime},
		{"", jobcommon.EmployFullTime},
		{"Part-Time", jobcommon.EmployPartTime},
		{"part-time", jobcommon.EmployPartTime},
		{"Contract", jobcommon.EmployContract},
		{"contract", jobcommon.EmployContract},
		{"Intern", jobcommon.EmployInternship},
		{"intern", jobcommon.EmployInternship},
		{"Unknown", jobcommon.EmployFullTime}, // unrecognised defaults to full_time
	}
	for _, tc := range tests {
		got := mapJobType(tc.input)
		if got != tc.want {
			t.Errorf("mapJobType(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// ── formatLocation ──────────────────────────────────────────────────────────

func TestFormatLocation(t *testing.T) {
	tests := []struct {
		name      string
		locations []appleLocation
		want      string
	}{
		{
			name:      "single location",
			locations: []appleLocation{{City: "Cupertino", StateCode: "CA"}},
			want:      "Cupertino, CA",
		},
		{
			name: "multiple locations",
			locations: []appleLocation{
				{City: "Cupertino", StateCode: "CA"},
				{City: "Austin", StateCode: "TX"},
			},
			want: "Cupertino, CA; Austin, TX",
		},
		{
			name:      "empty city is skipped",
			locations: []appleLocation{{City: "", StateCode: "CA"}},
			want:      "CA",
		},
		{
			name:      "empty locations",
			locations: []appleLocation{},
			want:      "",
		},
		{
			name:      "no state code",
			locations: []appleLocation{{City: "London"}},
			want:      "London",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := formatLocation(tc.locations)
			if got != tc.want {
				t.Errorf("formatLocation() = %q, want %q", got, tc.want)
			}
		})
	}
}

// ── formatSingleLocation ────────────────────────────────────────────────────

func TestFormatSingleLocation(t *testing.T) {
	tests := []struct {
		loc  appleLocation
		want string
	}{
		{appleLocation{City: "Cupertino", StateCode: "CA"}, "Cupertino, CA"},
		{appleLocation{City: "London", StateCode: ""}, "London"},
		{appleLocation{City: "", StateCode: "CA"}, "CA"},
		{appleLocation{City: "", StateCode: ""}, ""},
	}
	for _, tc := range tests {
		got := formatSingleLocation(tc.loc)
		if got != tc.want {
			t.Errorf("formatSingleLocation(%+v) = %q, want %q", tc.loc, got, tc.want)
		}
	}
}

// ── Real integration tests ──────────────────────────────────────────────────
// Guard: set TEST_REAL=1 to run. These hit the live jobs.apple.com.

func TestFetchJobs_Real(t *testing.T) {
	if os.Getenv("TEST_REAL") == "" {
		t.Skip("skipping real API test; set TEST_REAL=1 to run")
	}

	a := New()
	result, err := a.FetchJobs(context.Background(), "")
	if err != nil {
		t.Fatalf("FetchJobs error: %v", err)
	}
	if len(result.Jobs) == 0 {
		t.Fatal("expected at least one job from live Apple careers")
	}

	// Spot-check the first job
	j := result.Jobs[0]
	if !strings.HasPrefix(j.ID, "apple-") {
		t.Errorf("ID %q does not have 'apple-' prefix", j.ID)
	}
	if j.Title == "" {
		t.Error("Title should not be empty")
	}
	if j.URL == "" {
		t.Error("URL should not be empty")
	}
	if !strings.HasPrefix(j.URL, "https://jobs.apple.com/en-us/details/") {
		t.Errorf("URL %q does not match expected pattern", j.URL)
	}
	if j.FullContent == nil {
		t.Fatal("FullContent must not be nil — Apple is a fat adapter")
	}
	if j.FullContent.Description == "" {
		t.Error("FullContent.Description should not be empty")
	}
	if j.FullContent.Department == "" {
		t.Error("FullContent.Department should not be empty")
	}

	t.Logf("fetched %d jobs; first: %q at %q (dept: %q)", len(result.Jobs), j.Title, j.Location, j.FullContent.Department)
}

// ── Real: salary and experience diagnostic ───────────────────────────────────
// Verifies what salary/YOE data the live Apple listing API actually returns.
// Apple exposes no structured salary field — this checks if jobSummary descriptions
// contain salary or experience text the LLM can extract.

func TestFetchJobs_Real_SalaryAndExperience(t *testing.T) {
	if os.Getenv("TEST_REAL") == "" {
		t.Skip("skipping real API test; set TEST_REAL=1 to run")
	}

	a := New()
	// Fetch only the first page to keep the test fast.
	jobs, err := a.fetchJobsPage(context.Background(), 1)
	if err != nil {
		t.Fatalf("fetchJobsPage error: %v", err)
	}
	if len(jobs) == 0 {
		t.Fatal("no jobs returned from live Apple API page 1")
	}

	jobsWithSalary := 0
	jobsWithYOE := 0
	var salarySample, yoeSample string

	for _, j := range jobs {
		if j.FullContent == nil {
			continue
		}
		desc := strings.ToLower(j.FullContent.Description)

		if strings.Contains(desc, "$") || strings.Contains(desc, "salary") ||
			strings.Contains(desc, "compensation") || strings.Contains(desc, "usd") {
			jobsWithSalary++
			if salarySample == "" {
				idx := strings.Index(desc, "$")
				if idx < 0 {
					idx = strings.Index(desc, "salary")
				}
				if idx >= 0 {
					start := idx - 80
					if start < 0 {
						start = 0
					}
					end := idx + 200
					if end > len(j.FullContent.Description) {
						end = len(j.FullContent.Description)
					}
					salarySample = fmt.Sprintf("job=%q: ...%s...", j.Title, j.FullContent.Description[start:end])
				}
			}
		}

		for _, pat := range []string{"years of experience", "year(s) of experience", "years experience"} {
			if strings.Contains(desc, pat) {
				jobsWithYOE++
				if yoeSample == "" {
					if idx := strings.Index(desc, pat); idx >= 0 {
						start := idx - 80
						if start < 0 {
							start = 0
						}
						end := idx + 200
						if end > len(j.FullContent.Description) {
							end = len(j.FullContent.Description)
						}
						yoeSample = fmt.Sprintf("job=%q: ...%s...", j.Title, j.FullContent.Description[start:end])
					}
				}
				break
			}
		}
	}

	t.Logf("[Salary] page-1 jobs: %d total, %d with salary keywords in description", len(jobs), jobsWithSalary)
	if salarySample != "" {
		t.Logf("[Salary] sample: %s", salarySample)
	} else {
		t.Log("[Salary] Apple jobSummary descriptions do not contain salary data — Compensation field will always be empty")
	}

	t.Logf("[YOE] page-1 jobs: %d total, %d with YOE keywords in description", len(jobs), jobsWithYOE)
	if yoeSample != "" {
		t.Logf("[YOE] sample: %s", yoeSample)
	} else {
		t.Log("[YOE] Apple jobSummary descriptions do not contain explicit YOE keywords")
	}
}

// ── Helpers ─────────────────────────────────────────────────────────────────

// pageConfig describes what to return for a given page number.
type pageConfig struct {
	page int
	jobs []appleJob
}

// newTestServer creates an httptest.Server that serves job listing pages
// with embedded window.__staticRouterHydrationData based on the provided config.
func newTestServer(t *testing.T, pages []pageConfig) *httptest.Server {
	t.Helper()

	// Build a map for quick lookup
	pageMap := make(map[int][]appleJob)
	for _, pc := range pages {
		pageMap[pc.page] = pc.jobs
	}

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract page number from query param
		pageStr := r.URL.Query().Get("page")
		if pageStr == "" {
			pageStr = "1"
		}

		var page int
		_, _ = fmt.Sscanf(pageStr, "%d", &page)

		jobs, ok := pageMap[page]
		if !ok || jobs == nil {
			jobs = []appleJob{}
		}

		// Build the hydration data structure
		hydrationData := hydrationData{
			LoaderData: loaderData{
				Search: searchData{
					SearchResults: jobs,
					TotalRecords:  len(jobs),
				},
			},
		}

		hydrationJSON, _ := json.Marshal(hydrationData)

		// Render a minimal HTML page with embedded hydration data
		html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head><title>Apple Jobs</title></head>
<body>
<script>
window.__staticRouterHydrationData = %s;
</script>
</body>
</html>`, string(hydrationJSON))

		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(html))
	}))
}

func assertEqual(t *testing.T, field, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("%s: got %q, want %q", field, got, want)
	}
}
