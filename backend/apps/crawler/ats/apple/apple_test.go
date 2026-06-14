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

	"github.com/trushilpatel/jobradar/apps/crawler/ats/types"
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

func TestApple_ImplementsContentFetcher(t *testing.T) {
	var _ types.ContentFetcher = (*Apple)(nil)
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

// ── FetchJobs — single page ─────────────────────────────────────────────────

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
						{
							Name:          "Cupertino",
							City:          "Cupertino",
							StateProvince: "California",
							CountryName:   "United States",
							CountryID:     usaCountryID,
						},
					},
					PostDateInGMT:       "2026-03-15 00:00:00",
					Team:                appleTeam{TeamName: "Software and Services", TeamCode: "SWDVR"},
					Type:                "Regular",
					StandardWeeklyHours: 40,
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

	if len(result.Jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(result.Jobs))
	}

	j := result.Jobs[0]
	assertEqual(t, "ID", j.ID, "apple-200647207-3543")
	assertEqual(t, "Title", j.Title, "Software Engineer")
	assertEqual(t, "Location", j.Location, "Cupertino, California")
	assertEqual(t, "URL", j.URL, srv.URL+"/en-us/details/200647207-3543/software-engineer")
	assertEqual(t, "EmploymentType", j.EmploymentType, jobcommon.EmployFullTime)

	if j.FullContent != nil {
		t.Error("FullContent must be nil — Apple is now a thin adapter")
	}
	if !j.LocationKnown {
		t.Error("LocationKnown should be true when a USA countryID is present")
	}
}

// ── FetchJobs — multiple pages ──────────────────────────────────────────────

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
		{page: 4, jobs: []appleJob{}},
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

// ── FetchJobs — HTTP error ──────────────────────────────────────────────────

func TestFetchJobs_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
	}))
	srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
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

// ── FetchJobs — JSON.parse() hydration format ───────────────────────────────

func TestFetchJobs_JSONParseHydrationFormat(t *testing.T) {
	job := appleJob{
		ID:                      "200647207-3543",
		PostingTitle:            "Software Engineer",
		TransformedPostingTitle: "software-engineer",
		JobSummary:              "<p>Build great software.</p>",
		Locations: []appleLocation{
			{Name: "Cupertino", StateProvince: "California", CountryID: usaCountryID},
		},
		Type: "Regular",
		Team: appleTeam{TeamName: "Software and Services"},
	}

	hydrationObj := hydrationData{
		LoaderData: loaderData{
			Search: searchData{
				SearchResults: []appleJob{job},
				TotalRecords:  1,
			},
		},
	}
	rawJSON, _ := json.Marshal(hydrationObj)
	escapedJSON := strings.ReplaceAll(string(rawJSON), `\`, `\\`)
	escapedJSON = strings.ReplaceAll(escapedJSON, `"`, `\"`)

	emptyHydration := hydrationData{LoaderData: loaderData{Search: searchData{SearchResults: []appleJob{}}}}
	emptyJSON, _ := json.Marshal(emptyHydration)
	escapedEmpty := strings.ReplaceAll(string(emptyJSON), `\`, `\\`)
	escapedEmpty = strings.ReplaceAll(escapedEmpty, `"`, `\"`)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pageStr := r.URL.Query().Get("page")
		escaped := escapedJSON
		if pageStr == "2" {
			escaped = escapedEmpty
		}
		html := fmt.Sprintf(`<!DOCTYPE html><html><head></head><body><script>window.__staticRouterHydrationData = JSON.parse("%s");</script></body></html>`, escaped)
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(html))
	}))
	defer srv.Close()

	a := &Apple{baseURL: srv.URL}
	result, err := a.FetchJobs(context.Background(), "apple")
	if err != nil {
		t.Fatalf("FetchJobs error: %v", err)
	}
	if len(result.Jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(result.Jobs))
	}
	assertEqual(t, "Title", result.Jobs[0].Title, "Software Engineer")
}

// ── FetchJobs — JSON.parse() single-quote hydration format ──────────────────

func TestFetchJobs_JSONParseHydrationFormat_SingleQuote(t *testing.T) {
	job := appleJob{
		ID:                      "200647207-3543",
		PostingTitle:            "Software Engineer",
		TransformedPostingTitle: "software-engineer",
		JobSummary:              "<p>Build great software.</p>",
		Locations: []appleLocation{
			{Name: "Cupertino", StateProvince: "California", CountryID: usaCountryID},
		},
		Type: "Regular",
		Team: appleTeam{TeamName: "Software and Services"},
	}

	hydrationObj := hydrationData{
		LoaderData: loaderData{
			Search: searchData{
				SearchResults: []appleJob{job},
				TotalRecords:  1,
			},
		},
	}
	rawJSON, _ := json.Marshal(hydrationObj)
	escapedJSON := strings.ReplaceAll(string(rawJSON), `\`, `\\`)
	escapedJSON = strings.ReplaceAll(escapedJSON, `'`, `\'`)

	emptyHydration := hydrationData{LoaderData: loaderData{Search: searchData{SearchResults: []appleJob{}}}}
	emptyJSON, _ := json.Marshal(emptyHydration)
	escapedEmpty := strings.ReplaceAll(string(emptyJSON), `\`, `\\`)
	escapedEmpty = strings.ReplaceAll(escapedEmpty, `'`, `\'`)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pageStr := r.URL.Query().Get("page")
		escaped := escapedJSON
		if pageStr == "2" {
			escaped = escapedEmpty
		}
		html := fmt.Sprintf(`<!DOCTYPE html><html><head></head><body><script>window.__staticRouterHydrationData = JSON.parse('%s');</script></body></html>`, escaped)
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(html))
	}))
	defer srv.Close()

	a := &Apple{baseURL: srv.URL}
	result, err := a.FetchJobs(context.Background(), "apple")
	if err != nil {
		t.Fatalf("FetchJobs error: %v", err)
	}
	if len(result.Jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(result.Jobs))
	}
	assertEqual(t, "Title", result.Jobs[0].Title, "Software Engineer")
}

// ── FetchJobs — trailing content after JSON.parse() ─────────────────────────

func TestFetchJobs_TrailingContentAfterJSONParse(t *testing.T) {
	job := appleJob{
		ID:                      "100-1",
		PostingTitle:            "Staff Engineer",
		TransformedPostingTitle: "staff-engineer",
		Type:                    "Regular",
	}

	hydrationObj := hydrationData{
		LoaderData: loaderData{Search: searchData{SearchResults: []appleJob{job}, TotalRecords: 1}},
	}
	rawJSON, _ := json.Marshal(hydrationObj)
	escapedJSON := strings.ReplaceAll(string(rawJSON), `\`, `\\`)
	escapedJSON = strings.ReplaceAll(escapedJSON, `"`, `\"`)

	emptyHydration := hydrationData{LoaderData: loaderData{Search: searchData{SearchResults: []appleJob{}}}}
	emptyJSON, _ := json.Marshal(emptyHydration)
	escapedEmpty := strings.ReplaceAll(string(emptyJSON), `\`, `\\`)
	escapedEmpty = strings.ReplaceAll(escapedEmpty, `"`, `\"`)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pageStr := r.URL.Query().Get("page")
		escaped := escapedJSON
		if pageStr == "2" {
			escaped = escapedEmpty
		}
		html := fmt.Sprintf(`<!DOCTYPE html><html><body><script>window.__staticRouterHydrationData = JSON.parse("%s");
// apple internal
window.__otherVar = {};
</script></body></html>`, escaped)
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(html))
	}))
	defer srv.Close()

	a := &Apple{baseURL: srv.URL}
	result, err := a.FetchJobs(context.Background(), "apple")
	if err != nil {
		t.Fatalf("FetchJobs error: %v", err)
	}
	if len(result.Jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(result.Jobs))
	}
	assertEqual(t, "Title", result.Jobs[0].Title, "Staff Engineer")
}

// ── FetchJobs — malformed HTML (no hydration data) ──────────────────────────

func TestFetchJobs_MalformedHTML(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
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

// ── FetchJobs — job URL uses adapter base ───────────────────────────────────

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

	wantURL := srv.URL + "/en-us/details/999-123/senior-engineer"
	if result.Jobs[0].URL != wantURL {
		t.Errorf("URL = %q, want %q", result.Jobs[0].URL, wantURL)
	}
}

// ── FetchJobs — production URL pattern ──────────────────────────────────────

func TestFetchJobs_ProductionURL(t *testing.T) {
	// Default baseURL must yield the canonical jobs.apple.com host.
	a := New()
	item := a.jobToItem(appleJob{
		ID:                      "200647207-3543",
		PostingTitle:            "Engineer",
		TransformedPostingTitle: "engineer",
		Type:                    "Regular",
	})
	want := "https://jobs.apple.com/en-us/details/200647207-3543/engineer"
	if item.URL != want {
		t.Errorf("production URL = %q, want %q", item.URL, want)
	}
}

// ── FetchJobs — FullContent intentionally nil ───────────────────────────────

func TestFetchJobs_NoFullContent(t *testing.T) {
	srv := newTestServer(t, []pageConfig{
		{
			page: 1,
			jobs: []appleJob{
				{ID: "100-1", PostingTitle: "Job", TransformedPostingTitle: "job", JobSummary: "<p>Intro only</p>", Type: "Regular"},
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

	if result.Jobs[0].FullContent != nil {
		t.Error("FullContent must be nil — Apple is a thin adapter; rich content is fetched via ContentFetcher")
	}
}

// ── FetchJobs — location format (single location, new schema) ───────────────

func TestFetchJobs_LocationFormat_NewSchema(t *testing.T) {
	srv := newTestServer(t, []pageConfig{
		{
			page: 1,
			jobs: []appleJob{
				{
					ID:                      "100-1",
					PostingTitle:            "Job",
					TransformedPostingTitle: "job",
					Locations: []appleLocation{
						{
							Name:          "San Francisco",
							City:          "San Francisco",
							StateProvince: "California",
							CountryID:     usaCountryID,
						},
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

	wantLocation := "San Francisco, California"
	if result.Jobs[0].Location != wantLocation {
		t.Errorf("Location = %q, want %q", result.Jobs[0].Location, wantLocation)
	}
}

// ── FetchJobs — location falls back to Name when City empty ─────────────────

func TestFetchJobs_LocationFallbackToName(t *testing.T) {
	srv := newTestServer(t, []pageConfig{
		{
			page: 1,
			jobs: []appleJob{
				{
					ID:                      "100-1",
					PostingTitle:            "Job",
					TransformedPostingTitle: "job",
					Locations: []appleLocation{
						{Name: "Austin", CountryID: usaCountryID, Level: 5},
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

	if result.Jobs[0].Location != "Austin" {
		t.Errorf("Location = %q, want %q", result.Jobs[0].Location, "Austin")
	}
}

// ── FetchJobs — multiple locations joined ───────────────────────────────────

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
						{City: "Cupertino", StateProvince: "California", CountryID: usaCountryID},
						{City: "Austin", StateProvince: "Texas", CountryID: usaCountryID},
						{City: "New York", StateProvince: "New York", CountryID: usaCountryID},
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

	wantLocation := "Cupertino, California; Austin, Texas; New York, New York"
	if result.Jobs[0].Location != wantLocation {
		t.Errorf("Location = %q, want %q", result.Jobs[0].Location, wantLocation)
	}
	if len(result.Jobs[0].AllLocations) != 3 {
		t.Errorf("AllLocations len = %d, want 3", len(result.Jobs[0].AllLocations))
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

// ── FetchJobContent — rich detail extraction ────────────────────────────────

func TestFetchJobContent_FullDetail(t *testing.T) {
	detail := appleJobDetail{
		ID:                      "PIPE-200664943",
		JobNumber:               "200664943-3337",
		PostingTitle:            "Senior Software Engineer, Apple Services Engineering",
		TransformedPostingTitle: "senior-software-engineer-apple-services-engineering",
		JobSummary:              "We are looking for an experienced Software Engineer...",
		Description:             "CloudKit manages data at a massive scale!",
		Responsibilities:        "Design, develop, and support a multi-tenant platform",
		MinimumQualifications:   "7+ years of industry experience developing software in Java, C, or C++",
		PreferredQualifications: "Strong background with KV data stores, e.g., FoundationDB, DynamoDB, or Cassandra",
		TeamNames:               []string{"Software and Services"},
		Locations: []appleLocation{
			{City: "San Francisco", StateProvince: "California", CountryID: usaCountryID},
			{City: "Seattle", StateProvince: "Washington", CountryID: usaCountryID},
		},
		EmploymentType: "Standard",
		JobType:        "CORPORATE",
		HomeOffice:     false,
		PostDateInGMT:  "2026-05-22T00:00:00Z",
		PostingFooters: []appleFooter{
			{
				Type:    "POSTING_FOOTER_RULES_NEW",
				Content: "At Apple, base pay is one part of our total compensation package and is determined within a range. The base pay range for this role is between $171,600 and $302,200, and your base pay will depend on your skills.",
			},
		},
	}

	srv := newDetailServer(t, detail)
	defer srv.Close()

	a := &Apple{baseURL: srv.URL}
	job := types.RawJob{
		ID:    "apple-200664943-3337",
		Title: "Senior Software Engineer, Apple Services Engineering",
		URL:   srv.URL + "/en-us/details/200664943-3337/senior-software-engineer-apple-services-engineering",
	}
	got, err := a.FetchJobContent(context.Background(), job)
	if err != nil {
		t.Fatalf("FetchJobContent error: %v", err)
	}

	assertEqual(t, "ID", got.ID, "apple-200664943-3337")
	assertEqual(t, "Title", got.Title, "Senior Software Engineer, Apple Services Engineering")
	assertEqual(t, "URL", got.URL, job.URL)
	assertEqual(t, "Department", got.Department, "Software and Services")
	assertEqual(t, "Compensation", got.Compensation, "$171,600 and $302,200")
	assertEqual(t, "Location", got.Location, "San Francisco, California; Seattle, Washington")
	assertEqual(t, "CountryCode", got.CountryCode, "US")
	assertEqual(t, "WorkplaceType", got.WorkplaceType, "")
	assertEqual(t, "PublishedDate", got.PublishedDate, "2026-05-22T00:00:00Z")
	assertEqual(t, "EmploymentType", got.EmploymentType, jobcommon.EmployFullTime)

	// Description must contain every section, with labels for the structured parts.
	for _, want := range []string{
		"We are looking for an experienced",
		"CloudKit manages data",
		"Responsibilities:",
		"Design, develop",
		"Minimum Qualifications:",
		"7+ years of industry experience",
		"Preferred Qualifications:",
		"FoundationDB",
	} {
		if !strings.Contains(got.Description, want) {
			t.Errorf("Description missing %q\n--- description:\n%s", want, got.Description)
		}
	}
}

// ── FetchJobContent — homeOffice maps to remote ─────────────────────────────

func TestFetchJobContent_HomeOfficeRemote(t *testing.T) {
	detail := appleJobDetail{
		PostingTitle: "Remote Role",
		HomeOffice:   true,
		Locations: []appleLocation{
			{City: "Cupertino", StateProvince: "California", CountryID: usaCountryID},
		},
	}
	srv := newDetailServer(t, detail)
	defer srv.Close()

	a := &Apple{baseURL: srv.URL}
	got, err := a.FetchJobContent(context.Background(), types.RawJob{URL: srv.URL + "/x"})
	if err != nil {
		t.Fatalf("FetchJobContent error: %v", err)
	}
	if got.WorkplaceType != "remote" {
		t.Errorf("WorkplaceType = %q, want %q (homeOffice=true must map to remote)", got.WorkplaceType, "remote")
	}
}

// ── FetchJobContent — no salary range present ───────────────────────────────

func TestFetchJobContent_NoSalary(t *testing.T) {
	detail := appleJobDetail{
		PostingTitle: "Apple Retail Specialist",
		JobSummary:   "Apple Retail is where the best of Apple comes together.",
		Locations: []appleLocation{
			{Name: "United States", CountryID: usaCountryID, Level: 1},
		},
		PostingFooters: []appleFooter{
			{Type: "POSTING_FOOTER_GENERIC", Content: "Apple is an equal opportunity employer."},
		},
	}
	srv := newDetailServer(t, detail)
	defer srv.Close()

	a := &Apple{baseURL: srv.URL}
	got, err := a.FetchJobContent(context.Background(), types.RawJob{URL: srv.URL + "/x"})
	if err != nil {
		t.Fatalf("FetchJobContent error: %v", err)
	}
	if got.Compensation != "" {
		t.Errorf("Compensation = %q, want empty (no salary range present)", got.Compensation)
	}
}

// ── FetchJobContent — salary uses dash separator ────────────────────────────

func TestFetchJobContent_SalaryDashSeparator(t *testing.T) {
	detail := appleJobDetail{
		PostingTitle: "Engineer",
		PostingFooters: []appleFooter{
			{Type: "POSTING_FOOTER_RULES_NEW", Content: "The base pay range for this role is $150,000 - $250,000 depending on location."},
		},
	}
	srv := newDetailServer(t, detail)
	defer srv.Close()

	a := &Apple{baseURL: srv.URL}
	got, err := a.FetchJobContent(context.Background(), types.RawJob{URL: srv.URL + "/x"})
	if err != nil {
		t.Fatalf("FetchJobContent error: %v", err)
	}
	if got.Compensation != "$150,000 - $250,000" {
		t.Errorf("Compensation = %q, want %q", got.Compensation, "$150,000 - $250,000")
	}
}

// ── FetchJobContent — non-USA country code ──────────────────────────────────

func TestFetchJobContent_NonUSACountryCode(t *testing.T) {
	detail := appleJobDetail{
		PostingTitle: "Engineer",
		Locations: []appleLocation{
			{City: "London", CountryName: "United Kingdom", CountryID: "iso-country-GBR"},
		},
	}
	srv := newDetailServer(t, detail)
	defer srv.Close()

	a := &Apple{baseURL: srv.URL}
	got, err := a.FetchJobContent(context.Background(), types.RawJob{URL: srv.URL + "/x"})
	if err != nil {
		t.Fatalf("FetchJobContent error: %v", err)
	}
	if got.CountryCode == "US" || got.CountryCode == "" {
		t.Errorf("CountryCode = %q, want a non-empty non-US code for UK job", got.CountryCode)
	}
}

// ── FetchJobContent — malformed detail page ────────────────────────────────

func TestFetchJobContent_MalformedDetail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<html><body>no hydration here</body></html>`))
	}))
	defer srv.Close()

	a := &Apple{baseURL: srv.URL}
	_, err := a.FetchJobContent(context.Background(), types.RawJob{URL: srv.URL + "/x"})
	if err == nil {
		t.Fatal("expected error on malformed detail page")
	}
	if !strings.Contains(err.Error(), "extract detail hydration") {
		t.Errorf("error should mention extract detail hydration: %v", err)
	}
}

// ── FetchJobContent — HTTP error ────────────────────────────────────────────

func TestFetchJobContent_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	a := &Apple{baseURL: srv.URL}
	_, err := a.FetchJobContent(context.Background(), types.RawJob{URL: srv.URL + "/x"})
	if err == nil {
		t.Fatal("expected error on HTTP 500")
	}
	if !strings.Contains(err.Error(), "unexpected status 500") {
		t.Errorf("error should mention status 500: %v", err)
	}
}

// ── FetchJobContent — empty URL ────────────────────────────────────────────

func TestFetchJobContent_EmptyURL(t *testing.T) {
	a := New()
	_, err := a.FetchJobContent(context.Background(), types.RawJob{URL: ""})
	if err == nil {
		t.Fatal("expected error on empty URL")
	}
	if !strings.Contains(err.Error(), "empty job URL") {
		t.Errorf("error should mention empty job URL: %v", err)
	}
}

// ── FetchJobContent — context cancelled ─────────────────────────────────────

func TestFetchJobContent_ContextCancelled(t *testing.T) {
	srv := newDetailServer(t, appleJobDetail{PostingTitle: "x"})
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	a := &Apple{baseURL: srv.URL}
	_, err := a.FetchJobContent(ctx, types.RawJob{URL: srv.URL + "/x"})
	if err == nil {
		t.Fatal("expected error when context is cancelled")
	}
}

// ── FetchJobContent — falls back to listing-supplied values ────────────────

func TestFetchJobContent_FallsBackToListingValues(t *testing.T) {
	detail := appleJobDetail{
		// PostingTitle intentionally empty; Locations empty
	}
	srv := newDetailServer(t, detail)
	defer srv.Close()

	a := &Apple{baseURL: srv.URL}
	got, err := a.FetchJobContent(context.Background(), types.RawJob{
		URL:      srv.URL + "/x",
		Title:    "Listing Title",
		Location: "Listing Location",
	})
	if err != nil {
		t.Fatalf("FetchJobContent error: %v", err)
	}
	if got.Title != "Listing Title" {
		t.Errorf("Title = %q, want fallback to listing title", got.Title)
	}
	if got.Location != "Listing Location" {
		t.Errorf("Location = %q, want fallback to listing location", got.Location)
	}
}

// ── FetchJobContent — internship mapping ────────────────────────────────────

func TestFetchJobContent_InternshipMapping(t *testing.T) {
	detail := appleJobDetail{
		PostingTitle: "Software Engineering Intern",
		Type:         "Intern",
	}
	srv := newDetailServer(t, detail)
	defer srv.Close()

	a := &Apple{baseURL: srv.URL}
	got, err := a.FetchJobContent(context.Background(), types.RawJob{URL: srv.URL + "/x"})
	if err != nil {
		t.Fatalf("FetchJobContent error: %v", err)
	}
	if got.EmploymentType != jobcommon.EmployInternship {
		t.Errorf("EmploymentType = %q, want %q", got.EmploymentType, jobcommon.EmployInternship)
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
		{"PIPE", jobcommon.EmployFullTime},
		{"REQ", jobcommon.EmployFullTime},
		{"CORPORATE", jobcommon.EmployFullTime},
		{"Standard", jobcommon.EmployFullTime},
		{"Unknown", jobcommon.EmployFullTime},
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
			name:      "city + stateProvince",
			locations: []appleLocation{{City: "Cupertino", StateProvince: "California"}},
			want:      "Cupertino, California",
		},
		{
			name:      "name fallback when city empty",
			locations: []appleLocation{{Name: "Austin", StateProvince: "Texas"}},
			want:      "Austin, Texas",
		},
		{
			name:      "stateCode fallback when stateProvince empty",
			locations: []appleLocation{{City: "Cupertino", StateCode: "CA"}},
			want:      "Cupertino, CA",
		},
		{
			name: "multiple locations joined",
			locations: []appleLocation{
				{City: "Cupertino", StateProvince: "California"},
				{City: "Austin", StateProvince: "Texas"},
			},
			want: "Cupertino, California; Austin, Texas",
		},
		{
			name:      "empty city falls back to empty when name also empty",
			locations: []appleLocation{{StateProvince: "CA"}},
			want:      "CA",
		},
		{
			name:      "empty locations",
			locations: []appleLocation{},
			want:      "",
		},
		{
			name:      "no state info",
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
		{appleLocation{City: "Cupertino", StateProvince: "California"}, "Cupertino, California"},
		{appleLocation{Name: "Cupertino", StateCode: "CA"}, "Cupertino, CA"},
		{appleLocation{City: "London"}, "London"},
		{appleLocation{StateProvince: "CA"}, "CA"},
		{appleLocation{}, ""},
	}
	for _, tc := range tests {
		got := formatSingleLocation(tc.loc)
		if got != tc.want {
			t.Errorf("formatSingleLocation(%+v) = %q, want %q", tc.loc, got, tc.want)
		}
	}
}

// ── isUSALocation / primaryCountryCode ─────────────────────────────────────

func TestIsUSALocation(t *testing.T) {
	tests := []struct {
		name string
		loc  appleLocation
		want bool
	}{
		{"countryID iso-country-USA", appleLocation{CountryID: usaCountryID}, true},
		{"countryCode USA", appleLocation{CountryCode: "USA"}, true},
		{"countryCode US", appleLocation{CountryCode: "US"}, true},
		{"countryName United States", appleLocation{CountryName: "United States"}, true},
		{"countryName United States of America", appleLocation{CountryName: "United States of America"}, true},
		{"non-USA countryID", appleLocation{CountryID: "iso-country-GBR"}, false},
		{"empty", appleLocation{}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := isUSALocation(tc.loc)
			if got != tc.want {
				t.Errorf("isUSALocation(%+v) = %v, want %v", tc.loc, got, tc.want)
			}
		})
	}
}

func TestPrimaryCountryCode(t *testing.T) {
	tests := []struct {
		name string
		locs []appleLocation
		want string
	}{
		{"USA via countryID", []appleLocation{{CountryID: usaCountryID}}, "US"},
		{"UK via iso-country prefix", []appleLocation{{CountryID: "iso-country-GBR"}}, "GBR"},
		{"explicit countryCode", []appleLocation{{CountryCode: "fr"}}, "FR"},
		{"empty locations", nil, ""},
		{"empty fields", []appleLocation{{}}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := primaryCountryCode(tc.locs)
			if got != tc.want {
				t.Errorf("primaryCountryCode(%+v) = %q, want %q", tc.locs, got, tc.want)
			}
		})
	}
}

// ── Resolver fallback ──────────────────────────────────────────────────────

func TestResolveBaseURL_ZeroValueFallsBackToDefault(t *testing.T) {
	a := &Apple{}
	if got := a.resolveBaseURL(); got != defaultBaseURL {
		t.Errorf("zero-value resolveBaseURL: got %q, want %q", got, defaultBaseURL)
	}
}

func TestResolveBaseURL_OverrideHonoured(t *testing.T) {
	a := &Apple{baseURL: "https://override.example"}
	if got := a.resolveBaseURL(); got != "https://override.example" {
		t.Errorf("resolveBaseURL: got %q, want override", got)
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

	j := result.Jobs[0]
	if !strings.HasPrefix(j.ID, "apple-") {
		t.Errorf("ID %q does not have 'apple-' prefix", j.ID)
	}
	if j.Title == "" {
		t.Error("Title should not be empty")
	}
	if !strings.HasPrefix(j.URL, "https://jobs.apple.com/en-us/details/") {
		t.Errorf("URL %q does not match expected pattern", j.URL)
	}
	if j.FullContent != nil {
		t.Error("FullContent must be nil — Apple is a thin adapter")
	}

	t.Logf("fetched %d jobs; first: %q at %q (employType=%s)", len(result.Jobs), j.Title, j.Location, j.EmploymentType)
}

func TestFetchJobContent_Real(t *testing.T) {
	if os.Getenv("TEST_REAL") == "" {
		t.Skip("skipping real API test; set TEST_REAL=1 to run")
	}

	a := New()
	// Use only page 1 so the test doesn't paginate Apple's full ~5000-job catalog.
	pageJobs, err := a.fetchJobsPage(context.Background(), 1)
	if err != nil {
		t.Fatalf("fetchJobsPage error: %v", err)
	}
	if len(pageJobs) == 0 {
		t.Fatal("no jobs on page 1 to enrich")
	}

	// Pick the first non-PIPE job (PIPE roles often have minimal detail content).
	var picked types.FetchJobsItem
	for _, j := range pageJobs {
		if !strings.Contains(j.URL, "PIPE-") {
			picked = j
			break
		}
	}
	if picked.URL == "" {
		picked = pageJobs[0]
	}

	got, err := a.FetchJobContent(context.Background(), types.RawJob{
		ID:       picked.ID,
		Title:    picked.Title,
		Location: picked.Location,
		URL:      picked.URL,
		Adapter:  companycommon.AdapterApple,
	})
	if err != nil {
		t.Fatalf("FetchJobContent error: %v", err)
	}

	if got.Description == "" {
		t.Error("Description should not be empty for a real Apple job")
	}
	if got.Department == "" {
		t.Error("Department should not be empty for a real Apple job")
	}
	if got.CountryCode == "" {
		t.Error("CountryCode should not be empty for a real Apple job")
	}

	t.Logf("Enriched %q (%s):\n  Department=%q Compensation=%q CountryCode=%q\n  Description (first 300): %s",
		got.Title, picked.URL, got.Department, got.Compensation, got.CountryCode,
		truncate(got.Description, 300))
}

// ── Test server helpers ─────────────────────────────────────────────────────

// pageConfig describes what to return for a given listing page number.
type pageConfig struct {
	page int
	jobs []appleJob
}

// newTestServer creates an httptest.Server that serves job listing pages with
// embedded window.__staticRouterHydrationData based on the provided config.
func newTestServer(t *testing.T, pages []pageConfig) *httptest.Server {
	t.Helper()

	pageMap := make(map[int][]appleJob)
	for _, pc := range pages {
		pageMap[pc.page] = pc.jobs
	}

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

		hyd := hydrationData{
			LoaderData: loaderData{
				Search: searchData{
					SearchResults: jobs,
					TotalRecords:  len(jobs),
				},
			},
		}
		hydJSON, _ := json.Marshal(hyd)

		html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head><title>Apple Jobs</title></head>
<body>
<script>
window.__staticRouterHydrationData = %s;
</script>
</body>
</html>`, string(hydJSON))

		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(html))
	}))
}

// newDetailServer serves a job-detail page whose hydration data wraps the
// supplied appleJobDetail.
func newDetailServer(t *testing.T, detail appleJobDetail) *httptest.Server {
	t.Helper()
	hyd := detailHydrationData{
		LoaderData: detailLoaderData{
			JobDetails: detailJobDetails{JobsData: detail},
		},
	}
	hydJSON, _ := json.Marshal(hyd)
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		html := fmt.Sprintf(`<!DOCTYPE html><html><head></head><body><script>window.__staticRouterHydrationData = %s;</script></body></html>`, string(hydJSON))
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

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// ── fetchJobsPage — invalid JSON in hydration body ──────────────────────────

// TestFetchJobsPage_InvalidHydrationJSON verifies that fetchJobsPage returns
// an error wrapping "decode hydration JSON" when the page contains the
// hydration marker but the trailing JSON is syntactically invalid.
func TestFetchJobsPage_InvalidHydrationJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Marker present, but the value is broken JSON (missing closing brace).
		html := `<!DOCTYPE html><html><body><script>` +
			`window.__staticRouterHydrationData = {broken json here` +
			"\n</script></body></html>"
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(html))
	}))
	defer srv.Close()

	a := &Apple{baseURL: srv.URL}
	_, err := a.FetchJobs(context.Background(), "apple")
	if err == nil {
		t.Fatal("expected error for invalid JSON in hydration data")
	}
	if !strings.Contains(err.Error(), "decode hydration JSON") {
		t.Errorf("error should mention 'decode hydration JSON', got: %v", err)
	}
}

// ── FetchJobContent — invalid JSON in detail hydration body ─────────────────

// TestFetchJobContent_InvalidDetailHydrationJSON verifies that FetchJobContent
// returns an error wrapping "decode detail JSON" when the detail page has the
// marker but invalid JSON.
func TestFetchJobContent_InvalidDetailHydrationJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		html := `<!DOCTYPE html><html><body><script>` +
			`window.__staticRouterHydrationData = {not valid json` +
			"\n</script></body></html>"
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(html))
	}))
	defer srv.Close()

	a := &Apple{baseURL: srv.URL}
	_, err := a.FetchJobContent(context.Background(), types.RawJob{
		URL: srv.URL + "/en-us/details/1/job-title",
	})
	if err == nil {
		t.Fatal("expected error for invalid JSON in detail hydration data")
	}
	if !strings.Contains(err.Error(), "decode detail JSON") {
		t.Errorf("error should mention 'decode detail JSON', got: %v", err)
	}
}

// ── fetchHTML — http.NewRequestWithContext build failure ─────────────────────

// TestFetchHTML_BuildRequestError verifies that fetchHTML propagates the error
// from http.NewRequestWithContext when given an unparseable URL scheme.
// FetchJobContent is used as the entry point because fetchHTML is unexported.
func TestFetchHTML_BuildRequestError(t *testing.T) {
	a := &Apple{baseURL: "http://127.0.0.1"}
	// "://bad" is an invalid URL — NewRequestWithContext will reject it.
	_, err := a.FetchJobContent(context.Background(), types.RawJob{URL: "://bad"})
	if err == nil {
		t.Fatal("expected error for unparseable URL")
	}
	// The error bubbles up through FetchJobContent wrapping.
	if !strings.Contains(err.Error(), "fetch detail") {
		t.Errorf("error should mention 'fetch detail', got: %v", err)
	}
}

// ── extractHydrationData — edge cases ───────────────────────────────────────

// TestExtractHydrationData_EmptyJSONParseArg covers the branch where
// JSON.parse( is followed by no argument at all (len(arg) == 0).
func TestExtractHydrationData_EmptyJSONParseArg(t *testing.T) {
	// After the marker, the rest is exactly "JSON.parse(" with nothing after.
	body := "window.__staticRouterHydrationData = JSON.parse("
	_, err := extractHydrationData(body)
	if err == nil {
		t.Fatal("expected error for empty JSON.parse() call")
	}
	if !strings.Contains(err.Error(), "empty JSON.parse() call") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestExtractHydrationData_UnexpectedOpenChar covers the branch where
// JSON.parse() opens with a character other than " or '.
func TestExtractHydrationData_UnexpectedOpenChar(t *testing.T) {
	// backtick as the opening character — not supported.
	body := "window.__staticRouterHydrationData = JSON.parse(`{}`);\n</script>"
	_, err := extractHydrationData(body)
	if err == nil {
		t.Fatal("expected error for unexpected opening character in JSON.parse()")
	}
	if !strings.Contains(err.Error(), "unexpected opening character") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestExtractHydrationData_UnclosedDoubleQuoteArg covers the branch where
// scanToClosingQuote returns an error because the double-quoted string has
// no closing quote.
func TestExtractHydrationData_UnclosedDoubleQuoteArg(t *testing.T) {
	// JSON.parse(" with the string never terminated.
	body := `window.__staticRouterHydrationData = JSON.parse("unclosed`
	_, err := extractHydrationData(body)
	if err == nil {
		t.Fatal("expected error for unclosed double-quoted JSON.parse() argument")
	}
	if !strings.Contains(err.Error(), "JSON.parse() argument") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestExtractHydrationData_InvalidJSONInDoubleQuoteArg covers the branch
// where the content inside JSON.parse("...") is itself not valid JSON once
// decoded (json.Unmarshal returns an error).
func TestExtractHydrationData_InvalidJSONInDoubleQuoteArg(t *testing.T) {
	// "\\x" inside the double-quoted arg decodes to \x, which is not a valid
	// JSON escape sequence — json.Unmarshal returns an error.
	body := "window.__staticRouterHydrationData = JSON.parse(\"\\x\")\n</script>"
	_, err := extractHydrationData(body)
	if err == nil {
		t.Fatal("expected error for invalid JSON in double-quoted JSON.parse() arg")
	}
	if !strings.Contains(err.Error(), "decode JSON.parse string") {
		t.Errorf("error should mention 'decode JSON.parse string', got: %v", err)
	}
}

// TestExtractHydrationData_MissingClosingScriptTag covers the branch where
// the direct-object form is used but </script> is absent.
func TestExtractHydrationData_MissingClosingScriptTag(t *testing.T) {
	// Marker present, direct object form, but no </script> tag follows.
	body := `window.__staticRouterHydrationData = {"key":"value"}`
	_, err := extractHydrationData(body)
	if err == nil {
		t.Fatal("expected error when </script> tag is absent")
	}
	if !strings.Contains(err.Error(), "closing </script> not found") {
		t.Errorf("unexpected error: %v", err)
	}
}

// ── scanToClosingQuote — unclosed string ────────────────────────────────────

// TestScanToClosingQuote_Unclosed covers the return path where the loop
// exhausts the string without finding the closing quote.
func TestScanToClosingQuote_Unclosed(t *testing.T) {
	_, err := scanToClosingQuote("abcdef", '"')
	if err == nil {
		t.Fatal("expected error for string with no closing quote")
	}
	if !strings.Contains(err.Error(), "unclosed string") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestScanToClosingQuote_TrailingBackslash covers the case where the input
// ends with a backslash — the loop increments i past the end, exits without
// finding the closing quote, and the error path is reached.
func TestScanToClosingQuote_TrailingBackslash(t *testing.T) {
	_, err := scanToClosingQuote(`abc\`, '"')
	if err == nil {
		t.Fatal("expected error for string ending with an unmatched backslash")
	}
	if !strings.Contains(err.Error(), "unclosed string") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestScanToClosingQuote_HappyPath and TestScanToClosingQuote_WithEscape are
// white-box unit tests confirming the happy path and escape-skip logic.
func TestScanToClosingQuote_HappyPath(t *testing.T) {
	got, err := scanToClosingQuote(`hello"rest`, '"')
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
}

func TestScanToClosingQuote_SkipsEscapedQuote(t *testing.T) {
	// \" inside the string should be skipped; the closing " comes after.
	got, err := scanToClosingQuote(`he\"llo"rest`, '"')
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != `he\"llo` {
		t.Errorf("got %q, want %q", got, `he\"llo`)
	}
}

// ── fetchHTML — io.ReadAll error ─────────────────────────────────────────────

// TestFetchHTML_ReadBodyError verifies that fetchHTML returns an error
// wrapping "read body" when the server sends a 200 OK but then abruptly
// closes the connection before the body is fully delivered.
func TestFetchHTML_ReadBodyError(t *testing.T) {
	// hijackServer returns 200 OK with Content-Length set, then closes the
	// underlying TCP connection without sending the body. This forces
	// io.ReadAll to return an unexpected-EOF error.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Claim we will send 1000 bytes but immediately close without them.
		w.Header().Set("Content-Length", "1000")
		w.WriteHeader(http.StatusOK)
		// Hijack the connection and close it to simulate a torn-down body.
		hj, ok := w.(http.Hijacker)
		if !ok {
			// If Hijack is unavailable, just write partial data and close.
			_, _ = w.Write([]byte("partial"))
			return
		}
		conn, _, _ := hj.Hijack()
		// Send the response headers already buffered, then close.
		_ = conn.Close()
	}))
	defer srv.Close()

	a := &Apple{baseURL: srv.URL}
	_, err := a.FetchJobs(context.Background(), "apple")
	// The error may be wrapped in "fetch page" or "do request"; either way
	// it must be non-nil since the body is never fully delivered.
	if err == nil {
		t.Fatal("expected error when body read fails due to closed connection")
	}
}
