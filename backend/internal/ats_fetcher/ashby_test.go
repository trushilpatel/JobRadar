package ats_fetcher

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ── slugFromURL ───────────────────────────────────────────────────────────────

func TestSlugFromURL(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		// Full Ashby API URLs
		{"https://api.ashbyhq.com/posting-api/job-board/openai", "openai"},
		{"https://api.ashbyhq.com/posting-api/job-board/stripe", "stripe"},
		{"https://api.ashbyhq.com/posting-api/job-board/anduril-industries", "anduril-industries"},
		// Bare slugs pass through unchanged
		{"openai", "openai"},
		{"my-company", "my-company"},
		// Trailing slash
		{"https://api.ashbyhq.com/posting-api/job-board/openai/", ""},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got := slugFromURL(tc.input)
			if got != tc.want {
				t.Errorf("slugFromURL(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// ── normalizeAshbyEmploymentType ──────────────────────────────────────────────

func TestNormalizeAshbyEmploymentType(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"FullTime", "full_time"},
		{"PartTime", "part_time"},
		{"Contract", "contract"},
		{"Contractor", "contract"},
		{"Intern", "internship"},
		{"Internship", "internship"},
		{"", ""},
		{"Unknown", ""},
		{"FULLTIME", ""}, // case-sensitive
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got := normalizeAshbyEmploymentType(tc.input)
			if got != tc.want {
				t.Errorf("normalizeAshbyEmploymentType(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// ── FetchJobs ─────────────────────────────────────────────────────────────────

func TestAshby_FetchJobs(t *testing.T) {
	fakeResp := `{
		"data": {
			"jobBoardWithTeams": {
				"jobPostings": [
					{
						"id": "abc-111",
						"title": "Backend Engineer",
						"locationName": "San Francisco",
						"workplaceType": "Hybrid",
						"employmentType": "FullTime",
						"compensationTierSummary": "$180K – $220K"
					},
					{
						"id": "abc-222",
						"title": "ML Researcher",
						"locationName": "Remote - US",
						"workplaceType": "Remote",
						"employmentType": "FullTime",
						"compensationTierSummary": ""
					},
					{
						"id": "abc-333",
						"title": "Intern - Software",
						"locationName": "New York",
						"workplaceType": null,
						"employmentType": "Intern",
						"compensationTierSummary": ""
					}
				]
			}
		}
	}`

	var capturedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		json.NewDecoder(r.Body).Decode(&capturedBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fakeResp))
	}))
	defer srv.Close()

	a := &Ashby{baseURL: srv.URL}
	jobs, err := a.FetchJobs(context.Background(), "openai")
	if err != nil {
		t.Fatalf("FetchJobs error: %v", err)
	}

	if len(jobs) != 3 {
		t.Fatalf("expected 3 jobs, got %d", len(jobs))
	}

	// Verify slug was extracted and sent in GraphQL variables
	vars, _ := capturedBody["variables"].(map[string]any)
	if vars["organizationHostedJobsPageName"] != "openai" {
		t.Errorf("expected organizationHostedJobsPageName=openai, got %v", vars["organizationHostedJobsPageName"])
	}

	// Job 0 — full time
	j0 := jobs[0]
	if j0.ID != "ashby-abc-111" {
		t.Errorf("job[0].ID = %q, want %q", j0.ID, "ashby-abc-111")
	}
	if j0.Title != "Backend Engineer" {
		t.Errorf("job[0].Title = %q, want %q", j0.Title, "Backend Engineer")
	}
	if j0.EmployType != "full_time" {
		t.Errorf("job[0].EmployType = %q, want %q", j0.EmployType, "full_time")
	}
	wantURL := srv.URL[:0] + "https://jobs.ashbyhq.com/openai/abc-111"
	// URL should point to the hosted jobs page, not the test server
	wantURL = "https://jobs.ashbyhq.com/openai/abc-111"
	if j0.URL != wantURL {
		t.Errorf("job[0].URL = %q, want %q", j0.URL, wantURL)
	}

	// Job 2 — intern normalisation
	j2 := jobs[2]
	if j2.EmployType != "internship" {
		t.Errorf("job[2].EmployType = %q, want %q", j2.EmployType, "internship")
	}

	// Location with null workplaceType should not append "()"
	if j2.Location == "New York ()" {
		t.Errorf("job[2].Location should not include empty workplaceType, got %q", j2.Location)
	}
}

func TestAshby_FetchJobs_SlugExtractedFromFullURL(t *testing.T) {
	var capturedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&capturedBody)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data":{"jobBoardWithTeams":{"jobPostings":[]}}}`))
	}))
	defer srv.Close()

	a := &Ashby{baseURL: srv.URL}
	_, err := a.FetchJobs(context.Background(), "https://api.ashbyhq.com/posting-api/job-board/stripe")
	if err != nil {
		t.Fatalf("FetchJobs error: %v", err)
	}

	vars, _ := capturedBody["variables"].(map[string]any)
	if vars["organizationHostedJobsPageName"] != "stripe" {
		t.Errorf("slug not extracted: got %v, want stripe", vars["organizationHostedJobsPageName"])
	}
}

func TestAshby_FetchJobs_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	a := &Ashby{baseURL: srv.URL}
	_, err := a.FetchJobs(context.Background(), "openai")
	if err == nil {
		t.Fatal("expected error on HTTP 500, got nil")
	}
}

func TestAshby_FetchJobs_EmptyBoard(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data":{"jobBoardWithTeams":{"jobPostings":[]}}}`))
	}))
	defer srv.Close()

	a := &Ashby{baseURL: srv.URL}
	jobs, err := a.FetchJobs(context.Background(), "emptycorp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(jobs) != 0 {
		t.Errorf("expected 0 jobs, got %d", len(jobs))
	}
}

// ── FetchJobContent ───────────────────────────────────────────────────────────

func TestAshby_FetchJobContent(t *testing.T) {
	fakePosting := map[string]any{
		"id":                      "abc-111",
		"title":                   "Backend Engineer",
		"locationName":            "San Francisco",
		"workplaceType":           "Hybrid",
		"descriptionHtml":         "<p>Join us.</p>",
		"compensationTierSummary": "$180K – $220K",
		"publishedDate":           "2026-01-15",
	}
	fakeResp := map[string]any{"data": map[string]any{"jobPosting": fakePosting}}
	fakeJSON, _ := json.Marshal(fakeResp)

	var capturedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&capturedBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(fakeJSON)
	}))
	defer srv.Close()

	a := &Ashby{baseURL: srv.URL}
	job := RawJob{
		Adapter: "ashby",
		URL:     "https://jobs.ashbyhq.com/openai/abc-111",
	}
	content, err := a.FetchJobContent(context.Background(), job)
	if err != nil {
		t.Fatalf("FetchJobContent error: %v", err)
	}

	// Should return the jobPosting JSON
	var parsed map[string]any
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		t.Fatalf("content is not valid JSON: %v", err)
	}
	if parsed["id"] != "abc-111" {
		t.Errorf("id = %v, want abc-111", parsed["id"])
	}
	if parsed["title"] != "Backend Engineer" {
		t.Errorf("title = %v, want Backend Engineer", parsed["title"])
	}

	// Verify variables sent in GraphQL request
	vars, _ := capturedBody["variables"].(map[string]any)
	if vars["organizationHostedJobsPageName"] != "openai" {
		t.Errorf("company = %v, want openai", vars["organizationHostedJobsPageName"])
	}
	if vars["jobPostingId"] != "abc-111" {
		t.Errorf("jobPostingId = %v, want abc-111", vars["jobPostingId"])
	}
}

func TestAshby_FetchJobContent_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data":{"jobPosting":null}}`))
	}))
	defer srv.Close()

	a := &Ashby{baseURL: srv.URL}
	job := RawJob{Adapter: "ashby", URL: "https://jobs.ashbyhq.com/openai/missing-id"}
	_, err := a.FetchJobContent(context.Background(), job)
	if err == nil {
		t.Fatal("expected error for null jobPosting, got nil")
	}
}

func TestAshby_FetchJobContent_BadURL(t *testing.T) {
	a := &Ashby{}
	job := RawJob{Adapter: "ashby", URL: "https://jobs.ashbyhq.com/openai"} // missing job ID
	_, err := a.FetchJobContent(context.Background(), job)
	if err == nil {
		t.Fatal("expected error for bad URL, got nil")
	}
}

func TestAshby_FetchJobContent_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	a := &Ashby{baseURL: srv.URL}
	job := RawJob{Adapter: "ashby", URL: "https://jobs.ashbyhq.com/openai/abc-111"}
	_, err := a.FetchJobContent(context.Background(), job)
	if err == nil {
		t.Fatal("expected error on HTTP 401, got nil")
	}
}
