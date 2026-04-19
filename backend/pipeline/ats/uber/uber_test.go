package uber

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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

func TestUber_ImplementsAdapter(t *testing.T) {
	var _ types.Adapter = (*Uber)(nil)
}

func TestUber_ImplementsURLProvider(t *testing.T) {
	var _ types.URLProvider = (*Uber)(nil)
}

// ── Constructor ─────────────────────────────────────────────────────────────

func TestNew_DefaultSettings(t *testing.T) {
	u := New()
	if u.baseURL != defaultBaseURL {
		t.Errorf("baseURL = %q, want %q", u.baseURL, defaultBaseURL)
	}
}

// ── CareersURL ──────────────────────────────────────────────────────────────

func TestCareersURL(t *testing.T) {
	u := New()
	got := u.CareersURL("anything")
	want := "https://www.uber.com/us/en/careers/list/"
	if got != want {
		t.Errorf("CareersURL() = %q, want %q", got, want)
	}
}

// ── FetchJobs — single department ──────────────────────────────────────────

func TestFetchJobs_SingleDepartment(t *testing.T) {
	srv := newTestServer(t, map[string][]uberJob{
		"Engineering": {
			{
				ID:           10001,
				Title:        "Senior Software Engineer",
				Description:  "**About the Role**\n\nBuild distributed systems.",
				Department:   "Engineering",
				Location:     uberLocation{Country: "USA", Region: "California", City: "San Francisco", CountryName: "United States"},
				AllLocations: []uberLocation{{Country: "USA", Region: "California", City: "San Francisco", CountryName: "United States"}},
				TimeType:     "Full-Time",
				CreationDate: "2026-03-01T00:00:00.000Z",
			},
			{
				ID:           10002,
				Title:        "Staff Engineer",
				Description:  "Lead architecture for payments.",
				Department:   "Engineering",
				Location:     uberLocation{Country: "USA", Region: "New York", City: "New York", CountryName: "United States"},
				AllLocations: []uberLocation{{Country: "USA", Region: "New York", City: "New York", CountryName: "United States"}},
				TimeType:     "Full-Time",
				CreationDate: "2026-03-15T00:00:00.000Z",
			},
		},
	})
	defer srv.Close()

	u := &Uber{baseURL: srv.URL}
	result, err := u.FetchJobs(context.Background(), "uber")
	if err != nil {
		t.Fatalf("FetchJobs error: %v", err)
	}

	if len(result.Jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(result.Jobs))
	}

	j0 := result.Jobs[0]
	assertEqual(t, "ID", j0.ID, fmt.Sprintf("%s-10001", companycommon.AdapterUber))
	assertEqual(t, "Title", j0.Title, "Senior Software Engineer")
	assertEqual(t, "Location", j0.Location, "San Francisco, California, United States")
	assertEqual(t, "URL", j0.URL, "https://www.uber.com/careers/list/10001")
	assertEqual(t, "EmploymentType", j0.EmploymentType, jobcommon.EmployFullTime)

	if j0.FullContent == nil {
		t.Fatal("FullContent must not be nil — Uber is a fat adapter")
	}
	if !strings.Contains(j0.FullContent.Description, "Build distributed systems") {
		t.Error("FullContent.Description should contain job text")
	}
	assertEqual(t, "Department", j0.FullContent.Department, "Engineering")
	assertEqual(t, "PublishedDate", j0.FullContent.PublishedDate, "2026-03-01T00:00:00.000Z")
}

// ── FetchJobs — multiple departments ────────────────────────────────────────

func TestFetchJobs_MultipleDepartments(t *testing.T) {
	srv := newTestServer(t, map[string][]uberJob{
		"Engineering": {
			{ID: 1001, Title: "Backend Engineer", Department: "Engineering", TimeType: "Full-Time"},
			{ID: 1002, Title: "Frontend Engineer", Department: "Engineering", TimeType: "Full-Time"},
		},
		"Design": {
			{ID: 2001, Title: "Product Designer", Department: "Design", TimeType: "Full-Time"},
		},
		"Product": {
			{ID: 3001, Title: "Product Manager", Department: "Product", TimeType: "Full-Time"},
		},
	})
	defer srv.Close()

	u := &Uber{baseURL: srv.URL}
	result, err := u.FetchJobs(context.Background(), "uber")
	if err != nil {
		t.Fatalf("FetchJobs error: %v", err)
	}

	if len(result.Jobs) != 4 {
		t.Fatalf("expected 4 jobs (2+1+1), got %d", len(result.Jobs))
	}

	// Verify all unique IDs are present
	ids := make(map[string]bool)
	for _, j := range result.Jobs {
		ids[j.ID] = true
	}
	for _, want := range []string{"uber-1001", "uber-1002", "uber-2001", "uber-3001"} {
		if !ids[want] {
			t.Errorf("expected job ID %q in results", want)
		}
	}
}

// ── FetchJobs — deduplication ────────────────────────────────────────────────

func TestFetchJobs_Deduplication(t *testing.T) {
	// If the same job ID somehow appears in two departments, it should be deduped.
	duplicateJob := uberJob{ID: 9999, Title: "Dup Job", Department: "Engineering", TimeType: "Full-Time"}
	srv := newTestServer(t, map[string][]uberJob{
		"Engineering": {duplicateJob},
		"Product":     {duplicateJob}, // same ID
	})
	defer srv.Close()

	u := &Uber{baseURL: srv.URL}
	result, err := u.FetchJobs(context.Background(), "uber")
	if err != nil {
		t.Fatalf("FetchJobs error: %v", err)
	}
	if len(result.Jobs) != 1 {
		t.Errorf("expected 1 job after dedup, got %d", len(result.Jobs))
	}
}

// ── FetchJobs — empty ───────────────────────────────────────────────────────

func TestFetchJobs_Empty(t *testing.T) {
	srv := newTestServer(t, map[string][]uberJob{
		"Engineering": {},
	})
	defer srv.Close()

	u := &Uber{baseURL: srv.URL}
	result, err := u.FetchJobs(context.Background(), "uber")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Jobs) != 0 {
		t.Errorf("expected 0 jobs, got %d", len(result.Jobs))
	}
}

// ── FetchJobs — HTTP error on filter options ─────────────────────────────────

func TestFetchJobs_FilterOptionsHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "loadFilterOptions") {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	u := &Uber{baseURL: srv.URL}
	_, err := u.FetchJobs(context.Background(), "uber")
	if err == nil {
		t.Fatal("expected error on filter options 500, got nil")
	}
	if !strings.Contains(err.Error(), "fetch departments") {
		t.Errorf("error should mention 'fetch departments': %v", err)
	}
}

// ── FetchJobs — HTTP error on job search ────────────────────────────────────

func TestFetchJobs_SearchJobsHTTPError(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if strings.Contains(r.URL.Path, "loadFilterOptions") {
			_ = json.NewEncoder(w).Encode(filterOptionsResponse{
				Data: filterOptionsData{Department: map[string][]string{"Engineering": {}}},
			})
			return
		}
		// Job search returns error
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	u := &Uber{baseURL: srv.URL}
	_, err := u.FetchJobs(context.Background(), "uber")
	if err == nil {
		t.Fatal("expected error on search jobs 503, got nil")
	}
}

// ── FetchJobs — malformed JSON ───────────────────────────────────────────────

func TestFetchJobs_MalformedFilterJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`not json`))
	}))
	defer srv.Close()

	u := &Uber{baseURL: srv.URL}
	_, err := u.FetchJobs(context.Background(), "uber")
	if err == nil {
		t.Fatal("expected error on malformed JSON, got nil")
	}
}

// ── FetchJobs — context cancellation ────────────────────────────────────────

func TestFetchJobs_ContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(filterOptionsResponse{
			Data: filterOptionsData{Department: map[string][]string{"Engineering": {}}},
		})
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	u := &Uber{baseURL: srv.URL}
	_, err := u.FetchJobs(ctx, "uber")
	if err == nil {
		t.Fatal("expected error when context is cancelled")
	}
}

// ── FetchJobs — CSRF header sent ────────────────────────────────────────────

func TestFetchJobs_CSRFHeaderSent(t *testing.T) {
	var capturedCSRF string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedCSRF = r.Header.Get("x-csrf-token")
		if strings.Contains(r.URL.Path, "loadFilterOptions") {
			_ = json.NewEncoder(w).Encode(filterOptionsResponse{
				Data: filterOptionsData{Department: map[string][]string{}},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(searchJobsResponse{})
	}))
	defer srv.Close()

	u := &Uber{baseURL: srv.URL}
	_, _ = u.FetchJobs(context.Background(), "uber")

	if capturedCSRF == "" {
		t.Error("x-csrf-token header must be present in all requests")
	}
}

// ── FetchJobs — multi-location ──────────────────────────────────────────────

func TestFetchJobs_MultiLocation(t *testing.T) {
	srv := newTestServer(t, map[string][]uberJob{
		"Product": {
			{
				ID:    5001,
				Title: "Senior PM",
				AllLocations: []uberLocation{
					{City: "New York", Region: "New York", CountryName: "United States"},
					{City: "San Francisco", Region: "California", CountryName: "United States"},
					{City: "Chicago", Region: "Illinois", CountryName: "United States"},
				},
				TimeType: "Full-Time",
			},
		},
	})
	defer srv.Close()

	u := &Uber{baseURL: srv.URL}
	result, err := u.FetchJobs(context.Background(), "uber")
	if err != nil {
		t.Fatalf("FetchJobs error: %v", err)
	}

	want := "New York, New York, United States; San Francisco, California, United States; Chicago, Illinois, United States"
	assertEqual(t, "Location", result.Jobs[0].Location, want)
}

// ── FetchJobs — description pass-through (Markdown, not HTML) ───────────────

func TestFetchJobs_DescriptionIsMarkdown(t *testing.T) {
	markdown := "**About the Role**\n\nWe are looking for a *great* engineer.\n\n- Skill A\n- Skill B"
	srv := newTestServer(t, map[string][]uberJob{
		"Engineering": {
			{ID: 6001, Title: "Engineer", Description: markdown, TimeType: "Full-Time"},
		},
	})
	defer srv.Close()

	u := &Uber{baseURL: srv.URL}
	result, err := u.FetchJobs(context.Background(), "uber")
	if err != nil {
		t.Fatalf("FetchJobs error: %v", err)
	}

	// Description should be preserved as-is (Markdown, no HTML stripping)
	assertEqual(t, "Description", result.Jobs[0].FullContent.Description, markdown)
}

// ── mapTimeType ──────────────────────────────────────────────────────────────

func TestMapTimeType(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Full-Time", jobcommon.EmployFullTime},
		{"full-time", jobcommon.EmployFullTime},
		{"FULL-TIME", jobcommon.EmployFullTime},
		{"Part-Time", jobcommon.EmployPartTime},
		{"part-time", jobcommon.EmployPartTime},
		{"Contract", jobcommon.EmployContract},
		{"contract", jobcommon.EmployContract},
		{"Internship", jobcommon.EmployInternship},
		{"internship", jobcommon.EmployInternship},
		{"", jobcommon.EmployFullTime},        // empty defaults to full_time
		{"Unknown", jobcommon.EmployFullTime}, // unrecognised defaults to full_time
	}
	for _, tc := range tests {
		got := mapTimeType(tc.input)
		if got != tc.want {
			t.Errorf("mapTimeType(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// ── formatLocation ───────────────────────────────────────────────────────────

func TestFormatLocation(t *testing.T) {
	tests := []struct {
		name    string
		all     []uberLocation
		primary uberLocation
		want    string
	}{
		{
			name:    "single allLocations",
			all:     []uberLocation{{City: "SF", Region: "CA", CountryName: "United States"}},
			primary: uberLocation{},
			want:    "SF, CA, United States",
		},
		{
			name: "multiple allLocations",
			all: []uberLocation{
				{City: "SF", Region: "CA", CountryName: "United States"},
				{City: "NYC", Region: "NY", CountryName: "United States"},
			},
			primary: uberLocation{},
			want:    "SF, CA, United States; NYC, NY, United States",
		},
		{
			name:    "empty allLocations falls back to primary",
			all:     nil,
			primary: uberLocation{City: "Amsterdam", Region: "", CountryName: "Netherlands"},
			want:    "Amsterdam, Netherlands",
		},
		{
			name:    "empty region is skipped",
			all:     []uberLocation{{City: "London", Region: "", CountryName: "United Kingdom"}},
			primary: uberLocation{},
			want:    "London, United Kingdom",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := formatLocation(tc.all, tc.primary)
			if got != tc.want {
				t.Errorf("formatLocation() = %q, want %q", got, tc.want)
			}
		})
	}
}

// ── formatSingleLocation ─────────────────────────────────────────────────────

func TestFormatSingleLocation(t *testing.T) {
	tests := []struct {
		loc  uberLocation
		want string
	}{
		{uberLocation{City: "SF", Region: "CA", CountryName: "United States"}, "SF, CA, United States"},
		{uberLocation{City: "Amsterdam", Region: "", CountryName: "Netherlands"}, "Amsterdam, Netherlands"},
		{uberLocation{City: "", Region: "", CountryName: "Global"}, "Global"},
		{uberLocation{}, ""},
	}
	for _, tc := range tests {
		got := formatSingleLocation(tc.loc)
		if got != tc.want {
			t.Errorf("formatSingleLocation(%+v) = %q, want %q", tc.loc, got, tc.want)
		}
	}
}

// ── Job URL format ────────────────────────────────────────────────────────────

func TestFetchJobs_JobURLFormat(t *testing.T) {
	srv := newTestServer(t, map[string][]uberJob{
		"Engineering": {{ID: 99999, Title: "Test Job", TimeType: "Full-Time"}},
	})
	defer srv.Close()

	u := &Uber{baseURL: srv.URL}
	result, err := u.FetchJobs(context.Background(), "uber")
	if err != nil {
		t.Fatalf("FetchJobs error: %v", err)
	}

	want := "https://www.uber.com/careers/list/99999"
	assertEqual(t, "URL", result.Jobs[0].URL, want)
}

// ── FullContent always set ────────────────────────────────────────────────────

func TestFetchJobs_FullContentAlwaysSet(t *testing.T) {
	srv := newTestServer(t, map[string][]uberJob{
		"Engineering": {
			{ID: 11001, Title: "Engineer", Description: "Great job.", TimeType: "Full-Time"},
		},
	})
	defer srv.Close()

	u := &Uber{baseURL: srv.URL}
	result, err := u.FetchJobs(context.Background(), "uber")
	if err != nil {
		t.Fatalf("FetchJobs error: %v", err)
	}

	job := result.Jobs[0]
	if job.FullContent == nil {
		t.Fatal("FullContent must not be nil — Uber is a fat adapter (description in list API)")
	}
}

// ── FetchJobs — zero departments (empty map) ─────────────────────────────────

func TestFetchJobs_NoDepartments(t *testing.T) {
	srv := newTestServer(t, map[string][]uberJob{})
	defer srv.Close()

	u := &Uber{baseURL: srv.URL}
	result, err := u.FetchJobs(context.Background(), "uber")
	if err != nil {
		t.Fatalf("unexpected error with zero departments: %v", err)
	}
	if len(result.Jobs) != 0 {
		t.Errorf("expected 0 jobs, got %d", len(result.Jobs))
	}
}

// ── FetchJobs — request body structure ───────────────────────────────────────
// Verifies that the search request body has the correct structure:
// params.limit=1000, params.page=0, params.department=["<dept>"].

func TestFetchJobs_SearchRequestBodyStructure(t *testing.T) {
	var capturedBody searchJobsRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.String(), "loadFilterOptions") {
			_ = json.NewEncoder(w).Encode(filterOptionsResponse{
				Data: filterOptionsData{Department: map[string][]string{"Engineering": {}}},
			})
			return
		}
		// Capture and decode the request body
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &capturedBody)
		_ = json.NewEncoder(w).Encode(searchJobsResponse{})
	}))
	defer srv.Close()

	u := &Uber{baseURL: srv.URL}
	_, _ = u.FetchJobs(context.Background(), "uber")

	if capturedBody.Params.Limit != 1000 {
		t.Errorf("params.limit = %d, want 1000", capturedBody.Params.Limit)
	}
	if capturedBody.Params.Page != 0 {
		t.Errorf("params.page = %d, want 0", capturedBody.Params.Page)
	}
	if len(capturedBody.Params.Department) != 1 || capturedBody.Params.Department[0] != "Engineering" {
		t.Errorf("params.department = %v, want [Engineering]", capturedBody.Params.Department)
	}
	if capturedBody.Params.Query != "" {
		t.Errorf("params.query = %q, want empty string", capturedBody.Params.Query)
	}
}

// ── FetchJobs — Content-Type header on search request ────────────────────────

func TestFetchJobs_ContentTypeHeaderSent(t *testing.T) {
	var capturedContentType string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.String(), "loadFilterOptions") {
			_ = json.NewEncoder(w).Encode(filterOptionsResponse{
				Data: filterOptionsData{Department: map[string][]string{"Engineering": {}}},
			})
			return
		}
		capturedContentType = r.Header.Get("Content-Type")
		_ = json.NewEncoder(w).Encode(searchJobsResponse{})
	}))
	defer srv.Close()

	u := &Uber{baseURL: srv.URL}
	_, _ = u.FetchJobs(context.Background(), "uber")

	if capturedContentType != "application/json" {
		t.Errorf("Content-Type = %q, want %q", capturedContentType, "application/json")
	}
}

// ── fetchDepartments — malformed JSON ────────────────────────────────────────

func TestFetchDepartments_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{not valid json`))
	}))
	defer srv.Close()

	u := &Uber{baseURL: srv.URL}
	_, err := u.fetchDepartments(context.Background())
	if err == nil {
		t.Fatal("expected error on malformed JSON from filter options")
	}
	if !strings.Contains(err.Error(), "decode") {
		t.Errorf("error should mention 'decode', got: %s", err.Error())
	}
}

// ── fetchJobsForDepartment — malformed JSON ───────────────────────────────────

func TestFetchJobsForDepartment_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{not valid json`))
	}))
	defer srv.Close()

	u := &Uber{baseURL: srv.URL}
	_, err := u.fetchJobsForDepartment(context.Background(), "Engineering")
	if err == nil {
		t.Fatal("expected error on malformed JSON from search jobs")
	}
	if !strings.Contains(err.Error(), "decode") {
		t.Errorf("error should mention 'decode', got: %s", err.Error())
	}
}

// ── fetchJobsForDepartment — HTTP error ──────────────────────────────────────

func TestFetchJobsForDepartment_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusGatewayTimeout)
	}))
	defer srv.Close()

	u := &Uber{baseURL: srv.URL}
	_, err := u.fetchJobsForDepartment(context.Background(), "Engineering")
	if err == nil {
		t.Fatal("expected error on HTTP 504")
	}
	if !strings.Contains(err.Error(), "unexpected status 504") {
		t.Errorf("error should mention status 504, got: %s", err.Error())
	}
}

// ── FetchJobs — context cancelled between departments ────────────────────────

func TestFetchJobs_ContextCancelledBetweenDepartments(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.String(), "loadFilterOptions") {
			_ = json.NewEncoder(w).Encode(filterOptionsResponse{
				Data: filterOptionsData{Department: map[string][]string{
					"Dept1": {},
					"Dept2": {},
					"Dept3": {},
				}},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(searchJobsResponse{})
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before the first department fetch

	u := &Uber{baseURL: srv.URL}
	_, err := u.FetchJobs(ctx, "uber")
	if err == nil {
		t.Fatal("expected error when context is cancelled during department iteration")
	}
}

// ── FetchJobs — FullContent fields populated correctly ───────────────────────

func TestFetchJobs_FullContentFields(t *testing.T) {
	srv := newTestServer(t, map[string][]uberJob{
		"Engineering": {
			{
				ID:           20001,
				Title:        "Senior SWE",
				Description:  "## About\n\nBuild things at scale.",
				Department:   "Engineering",
				CreationDate: "2026-03-20T10:00:00.000Z",
				TimeType:     "Full-Time",
			},
		},
	})
	defer srv.Close()

	u := &Uber{baseURL: srv.URL}
	result, err := u.FetchJobs(context.Background(), "uber")
	if err != nil {
		t.Fatalf("FetchJobs error: %v", err)
	}
	if len(result.Jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(result.Jobs))
	}

	fc := result.Jobs[0].FullContent
	if fc == nil {
		t.Fatal("FullContent must not be nil")
	}
	// Level is empty in this fixture, so description should be unchanged.
	assertEqual(t, "Description", fc.Description, "## About\n\nBuild things at scale.")
	assertEqual(t, "Department", fc.Department, "Engineering")
	assertEqual(t, "PublishedDate", fc.PublishedDate, "2026-03-20T10:00:00.000Z")
	// WorkplaceType and Compensation are intentionally empty — not in Uber API
	if fc.WorkplaceType != "" {
		t.Errorf("WorkplaceType should be empty (not in Uber API), got %q", fc.WorkplaceType)
	}
	if fc.Compensation != "" {
		t.Errorf("Compensation should be empty (not in Uber API), got %q", fc.Compensation)
	}
}

// ── FetchJobs — Level appended to Description ────────────────────────────────

func TestFetchJobs_LevelAppendedToDescription(t *testing.T) {
	srv := newTestServer(t, map[string][]uberJob{
		"Engineering": {
			{
				ID:          30001,
				Title:       "Senior Software Engineer",
				Description: "**About the Role**\n\nBuild things at scale.",
				Department:  "Engineering",
				Level:       "5",
				TimeType:    "Full-Time",
			},
			{
				ID:          30002,
				Title:       "Staff Engineer",
				Description: "Lead architecture.",
				Department:  "Engineering",
				Level:       "", // no level
				TimeType:    "Full-Time",
			},
		},
	})
	defer srv.Close()

	u := &Uber{baseURL: srv.URL}
	result, err := u.FetchJobs(context.Background(), "uber")
	if err != nil {
		t.Fatalf("FetchJobs error: %v", err)
	}
	if len(result.Jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(result.Jobs))
	}

	// Find jobs by title (map order is non-deterministic)
	var withLevel, withoutLevel *types.FetchJobsItem
	for i := range result.Jobs {
		j := &result.Jobs[i]
		if j.Title == "Senior Software Engineer" {
			withLevel = j
		} else {
			withoutLevel = j
		}
	}

	if withLevel == nil || withLevel.FullContent == nil {
		t.Fatal("job with Level not found or FullContent nil")
	}
	if !strings.Contains(withLevel.FullContent.Description, "Uber Level: 5") {
		t.Errorf("Description should contain 'Uber Level: 5', got: %q", withLevel.FullContent.Description)
	}
	if !strings.HasSuffix(withLevel.FullContent.Description, "Uber Level: 5") {
		t.Error("'Uber Level' should be appended at the end of the description")
	}

	if withoutLevel == nil || withoutLevel.FullContent == nil {
		t.Fatal("job without Level not found or FullContent nil")
	}
	if strings.Contains(withoutLevel.FullContent.Description, "Uber Level") {
		t.Errorf("description with empty Level should not contain 'Uber Level', got: %q", withoutLevel.FullContent.Description)
	}
	assertEqual(t, "Description without level", withoutLevel.FullContent.Description, "Lead architecture.")
}

// ── FetchJobs — employment type mapping for all TimeType values ───────────────

func TestFetchJobs_EmploymentTypeMapping(t *testing.T) {
	cases := []struct {
		timeType string
		want     string
	}{
		{"Full-Time", jobcommon.EmployFullTime},
		{"Part-Time", jobcommon.EmployPartTime},
		{"Contract", jobcommon.EmployContract},
		{"Internship", jobcommon.EmployInternship},
		{"", jobcommon.EmployFullTime},
	}

	for _, tc := range cases {
		t.Run(tc.timeType, func(t *testing.T) {
			srv := newTestServer(t, map[string][]uberJob{
				"Dept": {{ID: 1, Title: "Job", TimeType: tc.timeType}},
			})
			defer srv.Close()

			u := &Uber{baseURL: srv.URL}
			result, err := u.FetchJobs(context.Background(), "uber")
			if err != nil {
				t.Fatalf("FetchJobs error: %v", err)
			}
			if len(result.Jobs) != 1 {
				t.Fatalf("expected 1 job")
			}
			if result.Jobs[0].EmploymentType != tc.want {
				t.Errorf("EmploymentType = %q, want %q", result.Jobs[0].EmploymentType, tc.want)
			}
		})
	}
}

// ── Real integration tests ───────────────────────────────────────────────────
// Guard: set TEST_REAL=1 to run. These hit the live Uber internal API.

func TestFetchJobs_Real(t *testing.T) {
	if os.Getenv("TEST_REAL") == "" {
		t.Skip("skipping real API test; set TEST_REAL=1 to run")
	}

	u := New()
	result, err := u.FetchJobs(context.Background(), "")
	if err != nil {
		t.Fatalf("FetchJobs error: %v", err)
	}
	if len(result.Jobs) == 0 {
		t.Fatal("expected at least one job from live Uber API")
	}

	// Spot-check the first job
	j := result.Jobs[0]
	if !strings.HasPrefix(j.ID, "uber-") {
		t.Errorf("ID %q does not have 'uber-' prefix", j.ID)
	}
	if j.Title == "" {
		t.Error("Title should not be empty")
	}
	if j.URL == "" {
		t.Error("URL should not be empty")
	}
	if !strings.HasPrefix(j.URL, "https://www.uber.com/careers/list/") {
		t.Errorf("URL %q does not match expected pattern", j.URL)
	}
	if j.FullContent == nil {
		t.Fatal("FullContent must not be nil — Uber is a fat adapter")
	}
	if j.FullContent.Description == "" {
		t.Error("FullContent.Description should not be empty")
	}
	if j.FullContent.Department == "" {
		t.Error("FullContent.Department should not be empty")
	}

	// Verify ID format: "uber-{int}"
	idNum := strings.TrimPrefix(j.ID, "uber-")
	if idNum == "" {
		t.Errorf("ID %q has empty numeric part after prefix", j.ID)
	}

	t.Logf("fetched %d jobs; first: %q at %q (dept: %q)", len(result.Jobs), j.Title, j.Location, j.FullContent.Department)
}

// ── Real: salary and experience diagnostic ───────────────────────────────────
// Verifies what salary/YOE data the live Uber API actually returns.
// Also inspects the raw Level field (captured but not forwarded to FullContent).

func TestFetchJobs_Real_SalaryAndExperience(t *testing.T) {
	if os.Getenv("TEST_REAL") == "" {
		t.Skip("skipping real API test; set TEST_REAL=1 to run")
	}

	u := New()

	// Fetch one department's raw jobs to inspect all struct fields including Level.
	departments, err := u.fetchDepartments(context.Background())
	if err != nil {
		t.Fatalf("fetchDepartments error: %v", err)
	}
	if len(departments) == 0 {
		t.Fatal("no departments returned from live Uber API")
	}

	// Use "Engineering" if available, otherwise the first department.
	dept := departments[0]
	for _, d := range departments {
		if d == "Engineering" {
			dept = d
			break
		}
	}

	rawJobs, err := u.fetchJobsForDepartment(context.Background(), dept)
	if err != nil {
		t.Fatalf("fetchJobsForDepartment(%q) error: %v", dept, err)
	}
	if len(rawJobs) == 0 {
		t.Fatalf("no jobs returned for department %q", dept)
	}

	// ── Level field audit ────────────────────────────────────────────────────
	// Level is captured in uberJob but discarded by jobToItem() — never reaches FullContent.
	levelsFound := 0
	for _, j := range rawJobs {
		if j.Level != "" {
			levelsFound++
		}
	}
	t.Logf("[Level] dept=%q: %d/%d jobs have a non-empty Level field (e.g. %q)",
		dept, levelsFound, len(rawJobs), rawJobs[0].Level)
	if levelsFound == 0 {
		t.Log("[Level] WARNING: Level field is empty for all sampled jobs — may not be populated by Uber API")
	} else {
		t.Logf("[Level] Level field is present and now appended to FullContent.Description as 'Uber Level: <value>'")
	}

	// ── Salary in description audit ──────────────────────────────────────────
	// Uber does not expose a structured salary field. Salary info (if any)
	// must be extracted by the LLM from the description Markdown.
	jobsWithSalary := 0
	var salarySample string
	for _, j := range rawJobs {
		desc := strings.ToLower(j.Description)
		if strings.Contains(desc, "$") || strings.Contains(desc, "salary") || strings.Contains(desc, "compensation") || strings.Contains(desc, "usd") {
			jobsWithSalary++
			if salarySample == "" && len(j.Description) > 0 {
				// Capture a 400-char preview around the first salary mention
				if idx := strings.Index(desc, "$"); idx >= 0 {
					start := idx - 100
					if start < 0 {
						start = 0
					}
					end := idx + 300
					if end > len(j.Description) {
						end = len(j.Description)
					}
					salarySample = fmt.Sprintf("job=%q: ...%s...", j.Title, j.Description[start:end])
				}
			}
		}
	}
	t.Logf("[Salary] dept=%q: %d/%d jobs contain salary/compensation keywords in description", dept, jobsWithSalary, len(rawJobs))
	if salarySample != "" {
		t.Logf("[Salary] sample: %s", salarySample)
	}
	if jobsWithSalary == 0 {
		t.Log("[Salary] Uber API descriptions do not contain salary data — compensation field will always be empty (LLM cannot extract what is not there)")
	}

	// ── Years of experience in description audit ─────────────────────────────
	jobsWithYOE := 0
	var yoeSample string
	yoePatterns := []string{"years of experience", "year(s) of experience", "years experience", "yoe"}
	for _, j := range rawJobs {
		desc := strings.ToLower(j.Description)
		for _, pat := range yoePatterns {
			if strings.Contains(desc, pat) {
				jobsWithYOE++
				if yoeSample == "" {
					if idx := strings.Index(desc, pat); idx >= 0 {
						start := idx - 80
						if start < 0 {
							start = 0
						}
						end := idx + 200
						if end > len(j.Description) {
							end = len(j.Description)
						}
						yoeSample = fmt.Sprintf("job=%q: ...%s...", j.Title, j.Description[start:end])
					}
				}
				break
			}
		}
	}
	t.Logf("[YOE] dept=%q: %d/%d jobs contain years-of-experience keywords in description", dept, jobsWithYOE, len(rawJobs))
	if yoeSample != "" {
		t.Logf("[YOE] sample: %s", yoeSample)
	}
	if jobsWithYOE == 0 {
		t.Log("[YOE] Uber API descriptions do not contain YOE keywords — experience field will always require LLM inference from description context")
	}
}

// ── Helpers ──────────────────────────────────────────────────────────────────

// newTestServer creates an httptest.Server that serves:
//   - POST /api/loadFilterOptions → returns departments map
//   - POST /api/loadSearchJobsResults → returns jobs for the requested department
func newTestServer(t *testing.T, deptJobs map[string][]uberJob) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if strings.Contains(r.URL.String(), "loadFilterOptions") {
			depts := make(map[string][]string, len(deptJobs))
			for d := range deptJobs {
				depts[d] = []string{} // sub-teams not needed
			}
			_ = json.NewEncoder(w).Encode(filterOptionsResponse{
				Data: filterOptionsData{Department: depts},
			})
			return
		}

		if strings.Contains(r.URL.String(), "loadSearchJobsResults") {
			var reqBody searchJobsRequest
			_ = json.NewDecoder(r.Body).Decode(&reqBody)

			var jobs []uberJob
			if len(reqBody.Params.Department) > 0 {
				jobs = deptJobs[reqBody.Params.Department[0]]
			}
			if jobs == nil {
				jobs = []uberJob{}
			}

			_ = json.NewEncoder(w).Encode(searchJobsResponse{
				Data: searchJobsData{
					Results:      jobs,
					TotalResults: uberTotal{Low: len(jobs)},
				},
			})
			return
		}

		http.NotFound(w, r)
	}))
}

func assertEqual(t *testing.T, field, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("%s: got %q, want %q", field, got, want)
	}
}
