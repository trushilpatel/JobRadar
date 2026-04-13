package ashby

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/trushilpatel/jobradar/pipeline/ats/types"
)

func TestAshby_CareersURL(t *testing.T) {
	a := &Ashby{}
	got := a.CareersURL("openai")
	want := "https://jobs.ashbyhq.com/openai"
	if got != want {
		t.Errorf("CareersURL(%q) = %q, want %q", "openai", got, want)
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
	result, err := a.FetchJobs(context.Background(), "openai")
	if err != nil {
		t.Fatalf("FetchJobs error: %v", err)
	}

	if len(result.Jobs) != 3 {
		t.Fatalf("expected 3 jobs, got %d", len(result.Jobs))
	}

	vars, _ := capturedBody["variables"].(map[string]any)
	if vars["organizationHostedJobsPageName"] != "openai" {
		t.Errorf("expected organizationHostedJobsPageName=openai, got %v", vars["organizationHostedJobsPageName"])
	}

	j0 := result.Jobs[0]
	if j0.ID != "ashby-abc-111" {
		t.Errorf("job[0].ID = %q, want ashby-abc-111", j0.ID)
	}
	if j0.Title != "Backend Engineer" {
		t.Errorf("job[0].Title = %q, want Backend Engineer", j0.Title)
	}
	if j0.EmploymentType != "full_time" {
		t.Errorf("job[0].EmploymentType = %q, want full_time", j0.EmploymentType)
	}
	if j0.URL != "https://jobs.ashbyhq.com/openai/abc-111" {
		t.Errorf("job[0].URL = %q, want https://jobs.ashbyhq.com/openai/abc-111", j0.URL)
	}

	j2 := result.Jobs[2]
	if j2.EmploymentType != "internship" {
		t.Errorf("job[2].EmploymentType = %q, want internship", j2.EmploymentType)
	}
	if j2.Location == "New York ()" {
		t.Errorf("job[2].Location should not include empty workplaceType, got %q", j2.Location)
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
	result, err := a.FetchJobs(context.Background(), "emptycorp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Jobs) != 0 {
		t.Errorf("expected 0 jobs, got %d", len(result.Jobs))
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
	job := types.RawJob{Adapter: "ashby", URL: "https://jobs.ashbyhq.com/openai/abc-111"}
	content, err := a.FetchJobContent(context.Background(), job)
	if err != nil {
		t.Fatalf("FetchJobContent error: %v", err)
	}

	if content.ID != "ashby-abc-111" {
		t.Errorf("content.ID = %q, want ashby-abc-111", content.ID)
	}
	if content.Title != "Backend Engineer" {
		t.Errorf("content.Title = %q, want Backend Engineer", content.Title)
	}
	if content.Compensation != "$180K – $220K" {
		t.Errorf("content.Compensation = %q, want $180K – $220K", content.Compensation)
	}
	if content.WorkplaceType != "Hybrid" {
		t.Errorf("content.WorkplaceType = %q, want Hybrid", content.WorkplaceType)
	}
	if content.Description == "" {
		t.Error("content.Description is empty")
	}

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
	job := types.RawJob{Adapter: "ashby", URL: "https://jobs.ashbyhq.com/openai/missing-id"}
	_, err := a.FetchJobContent(context.Background(), job)
	if err == nil {
		t.Fatal("expected error for null jobPosting, got nil")
	}
}

func TestAshby_FetchJobContent_BadURL(t *testing.T) {
	a := &Ashby{}
	job := types.RawJob{Adapter: "ashby", URL: "https://jobs.ashbyhq.com/openai"}
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
	job := types.RawJob{Adapter: "ashby", URL: "https://jobs.ashbyhq.com/openai/abc-111"}
	_, err := a.FetchJobContent(context.Background(), job)
	if err == nil {
		t.Fatal("expected error on HTTP 401, got nil")
	}
}

// ── REST API (FetchJobsViaAPI / FetchJobContentViaAPI) ────────────────────────

func TestAshby_FetchJobsViaAPI(t *testing.T) {
	fakeResp := map[string]any{
		"success":           true,
		"moreDataAvailable": false,
		"results": []any{
			map[string]any{
				"id": "job-001", "title": "Backend Engineer",
				"status": "Open", "employmentType": "FullTime",
				"location": map[string]any{
					"id": "loc-1", "name": "San Francisco",
					"externalName": "San Francisco, CA", "workplaceType": "Hybrid",
				},
				"compensation": map[string]any{
					"compensationTiers": []any{
						map[string]any{"tierSummary": "$120,000 – $160,000 per year"},
					},
				},
			},
			map[string]any{
				"id": "job-002", "title": "ML Researcher",
				"status": "Open", "employmentType": "FullTime",
			},
		},
	}
	fakeJSON, _ := json.Marshal(fakeResp)

	var capturedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&capturedBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(fakeJSON)
	}))
	defer srv.Close()

	a := &Ashby{apiBaseURL: srv.URL, apiKey: "test-key"}
	result, err := a.FetchJobsViaAPI(context.Background())
	if err != nil {
		t.Fatalf("FetchJobsViaAPI error: %v", err)
	}

	if len(result.Jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(result.Jobs))
	}

	expand, _ := capturedBody["expand"].([]any)
	if len(expand) == 0 {
		t.Error("expected expand to be sent in payload")
	}

	j0 := result.Jobs[0]
	if j0.ID != "ashby-job-001" {
		t.Errorf("job[0].ID = %q, want ashby-job-001", j0.ID)
	}
	if j0.Location != "San Francisco, CA" {
		t.Errorf("job[0].Location = %q, want San Francisco, CA", j0.Location)
	}
	if j0.EmploymentType != "full_time" {
		t.Errorf("job[0].EmploymentType = %q, want full_time", j0.EmploymentType)
	}

	if result.Jobs[1].Location != "" {
		t.Errorf("job[1].Location = %q, want empty", result.Jobs[1].Location)
	}
}

func TestAshby_FetchJobsViaAPI_NoAPIKey(t *testing.T) {
	a := &Ashby{}
	_, err := a.FetchJobsViaAPI(context.Background())
	if err == nil {
		t.Fatal("expected error when apiKey is empty")
	}
}

func TestAshby_FetchJobsViaAPI_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	a := &Ashby{apiBaseURL: srv.URL, apiKey: "bad-key"}
	_, err := a.FetchJobsViaAPI(context.Background())
	if err == nil {
		t.Fatal("expected error on HTTP 401")
	}
}

func TestAshby_FetchJobContentViaAPI(t *testing.T) {
	fakeResp := map[string]any{
		"success": true,
		"results": map[string]any{
			"id": "job-001", "title": "Backend Engineer",
			"status": "Open", "employmentType": "FullTime",
			"openedAt": "2026-01-15T00:00:00Z",
			"location": map[string]any{
				"id": "loc-1", "name": "San Francisco",
				"externalName": "San Francisco, CA", "workplaceType": "Hybrid",
			},
			"compensation": map[string]any{
				"compensationTiers": []any{
					map[string]any{"tierSummary": "$120,000 – $160,000 per year"},
				},
			},
		},
	}
	fakeJSON, _ := json.Marshal(fakeResp)

	var capturedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&capturedBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(fakeJSON)
	}))
	defer srv.Close()

	a := &Ashby{apiBaseURL: srv.URL, apiKey: "test-key"}
	content, err := a.FetchJobContentViaAPI(context.Background(), "ashby-job-001")
	if err != nil {
		t.Fatalf("FetchJobContentViaAPI error: %v", err)
	}

	expand, _ := capturedBody["expand"].([]any)
	if len(expand) == 0 {
		t.Error("expected expand to be sent in payload")
	}
	if capturedBody["id"] != "job-001" {
		t.Errorf("expected id=job-001 in request, got %v", capturedBody["id"])
	}
	if content.ID != "ashby-job-001" {
		t.Errorf("content.ID = %q, want ashby-job-001", content.ID)
	}
	if content.Location != "San Francisco, CA" {
		t.Errorf("content.Location = %q, want San Francisco, CA", content.Location)
	}
	if content.WorkplaceType != "Hybrid" {
		t.Errorf("content.WorkplaceType = %q, want Hybrid", content.WorkplaceType)
	}
	if content.EmploymentType != "full_time" {
		t.Errorf("content.EmploymentType = %q, want full_time", content.EmploymentType)
	}
	if content.Compensation != "$120,000 – $160,000 per year" {
		t.Errorf("content.Compensation = %q, want $120,000 – $160,000 per year", content.Compensation)
	}
	if content.PublishedDate != "2026-01-15T00:00:00Z" {
		t.Errorf("content.PublishedDate = %q, want 2026-01-15T00:00:00Z", content.PublishedDate)
	}
}

func TestAshby_FetchJobContentViaAPI_NoAPIKey(t *testing.T) {
	a := &Ashby{}
	_, err := a.FetchJobContentViaAPI(context.Background(), "ashby-job-001")
	if err == nil {
		t.Fatal("expected error when apiKey is empty")
	}
}

func TestAshby_FetchJobContentViaAPI_EmptyID(t *testing.T) {
	a := &Ashby{apiKey: "test-key"}
	_, err := a.FetchJobContentViaAPI(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty job ID")
	}
}

func TestAshby_FetchJobContentViaAPI_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success":false,"results":{"id":""}}`))
	}))
	defer srv.Close()

	a := &Ashby{apiBaseURL: srv.URL, apiKey: "test-key"}
	_, err := a.FetchJobContentViaAPI(context.Background(), "ashby-job-missing")
	if err == nil {
		t.Fatal("expected error for not found job")
	}
}
