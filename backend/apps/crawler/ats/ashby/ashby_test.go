package ashby

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/trushilpatel/jobradar/apps/crawler/ats/types"
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
		_ = json.NewDecoder(r.Body).Decode(&capturedBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(fakeResp))
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
		_, _ = w.Write([]byte(`{"data":{"jobBoardWithTeams":{"jobPostings":[]}}}`))
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
		_ = json.NewDecoder(r.Body).Decode(&capturedBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(fakeJSON)
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
		_, _ = w.Write([]byte(`{"data":{"jobPosting":null}}`))
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
		_ = json.NewDecoder(r.Body).Decode(&capturedBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(fakeJSON)
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
		_ = json.NewDecoder(r.Body).Decode(&capturedBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(fakeJSON)
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
		_, _ = w.Write([]byte(`{"success":false,"results":{"id":""}}`))
	}))
	defer srv.Close()

	a := &Ashby{apiBaseURL: srv.URL, apiKey: "test-key"}
	_, err := a.FetchJobContentViaAPI(context.Background(), "ashby-job-missing")
	if err == nil {
		t.Fatal("expected error for not found job")
	}
}

// ── toAPIFetchJobsResponse — ExternalName fallback ────────────────────────────

// TestToAPIFetchJobsResponse_ExternalNameFallback verifies that when
// Location.ExternalName is empty, the converter falls back to Location.Name.
func TestToAPIFetchJobsResponse_ExternalNameFallback(t *testing.T) {
	fakeResp := map[string]any{
		"success":           true,
		"moreDataAvailable": false,
		"results": []any{
			map[string]any{
				"id": "job-001", "title": "Engineer",
				"employmentType": "FullTime",
				"location": map[string]any{
					"id": "loc-1", "name": "Remote HQ",
					"externalName":  "", // empty — should fall back to Name
					"workplaceType": "Remote",
				},
			},
		},
	}
	fakeJSON, _ := json.Marshal(fakeResp)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(fakeJSON)
	}))
	defer srv.Close()

	a := &Ashby{apiBaseURL: srv.URL, apiKey: "test-key"}
	result, err := a.FetchJobsViaAPI(context.Background())
	if err != nil {
		t.Fatalf("FetchJobsViaAPI error: %v", err)
	}
	if len(result.Jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(result.Jobs))
	}
	if result.Jobs[0].Location != "Remote HQ" {
		t.Errorf("Location = %q, want fallback to Name %q", result.Jobs[0].Location, "Remote HQ")
	}
}

// ── toAPIFetchJobContentResponse — ExternalName fallback ─────────────────────

// TestToAPIFetchJobContentResponse_ExternalNameFallback verifies that when
// Location.ExternalName is empty, the converter falls back to Location.Name.
func TestToAPIFetchJobContentResponse_ExternalNameFallback(t *testing.T) {
	fakeResp := map[string]any{
		"success": true,
		"results": map[string]any{
			"id": "job-002", "title": "Designer",
			"employmentType": "FullTime",
			"location": map[string]any{
				"id": "loc-2", "name": "London Office",
				"externalName":  "", // empty — should fall back to Name
				"workplaceType": "OnSite",
			},
		},
	}
	fakeJSON, _ := json.Marshal(fakeResp)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(fakeJSON)
	}))
	defer srv.Close()

	a := &Ashby{apiBaseURL: srv.URL, apiKey: "test-key"}
	content, err := a.FetchJobContentViaAPI(context.Background(), "ashby-job-002")
	if err != nil {
		t.Fatalf("FetchJobContentViaAPI error: %v", err)
	}
	if content.Location != "London Office" {
		t.Errorf("Location = %q, want fallback to Name %q", content.Location, "London Office")
	}
	if content.WorkplaceType != "OnSite" {
		t.Errorf("WorkplaceType = %q, want OnSite", content.WorkplaceType)
	}
}

// ── FetchJobs — error paths ───────────────────────────────────────────────────

// TestAshby_FetchJobs_DoError verifies that FetchJobs returns an error when
// the HTTP request fails (server closes before a response is sent).
func TestAshby_FetchJobs_DoError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Close the connection immediately to cause a Do() error.
		hj, ok := w.(http.Hijacker)
		if !ok {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		conn, _, _ := hj.Hijack()
		_ = conn.Close()
	}))
	defer srv.Close()

	a := &Ashby{baseURL: srv.URL}
	_, err := a.FetchJobs(context.Background(), "openai")
	if err == nil {
		t.Fatal("expected error when HTTP Do fails")
	}
	if !strings.Contains(err.Error(), "ashby: fetch") {
		t.Errorf("error should mention 'ashby: fetch', got: %v", err)
	}
}

// TestAshby_FetchJobs_DecodeError verifies that FetchJobs returns an error
// when the server returns 200 but with non-JSON body.
func TestAshby_FetchJobs_DecodeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("this is not json"))
	}))
	defer srv.Close()

	a := &Ashby{baseURL: srv.URL}
	_, err := a.FetchJobs(context.Background(), "openai")
	if err == nil {
		t.Fatal("expected error for non-JSON response body")
	}
	if !strings.Contains(err.Error(), "ashby: decode") {
		t.Errorf("error should mention 'ashby: decode', got: %v", err)
	}
}

// TestAshby_FetchJobs_InvalidBaseURL verifies that FetchJobs returns an error
// when the base URL makes NewRequestWithContext fail.
func TestAshby_FetchJobs_InvalidBaseURL(t *testing.T) {
	// A URL with a control character in it is rejected by http.NewRequest.
	a := &Ashby{baseURL: "http://\x00invalid"}
	_, err := a.FetchJobs(context.Background(), "openai")
	if err == nil {
		t.Fatal("expected error for invalid base URL")
	}
	if !strings.Contains(err.Error(), "ashby: build request") {
		t.Errorf("error should mention 'ashby: build request', got: %v", err)
	}
}

// ── FetchJobContent — error paths ─────────────────────────────────────────────

// TestAshby_FetchJobContent_InvalidJobURL verifies that FetchJobContent
// returns an error when job.URL has an invalid scheme that url.Parse rejects.
func TestAshby_FetchJobContent_InvalidJobURL(t *testing.T) {
	a := &Ashby{}
	// url.Parse accepts almost everything, but a truly mangled URL with a
	// control character triggers an error.
	_, err := a.FetchJobContent(context.Background(), types.RawJob{
		URL: "://\x00bad",
	})
	if err == nil {
		t.Fatal("expected error for unparseable job URL")
	}
	if !strings.Contains(err.Error(), "ashby: parse job URL") {
		t.Errorf("error should mention 'ashby: parse job URL', got: %v", err)
	}
}

// TestAshby_FetchJobContent_InvalidContentBaseURL verifies the
// NewRequestWithContext error path in FetchJobContent.
func TestAshby_FetchJobContent_InvalidContentBaseURL(t *testing.T) {
	a := &Ashby{baseURL: "http://\x00invalid"}
	_, err := a.FetchJobContent(context.Background(), types.RawJob{
		URL: "https://jobs.ashbyhq.com/company/job-id-123",
	})
	if err == nil {
		t.Fatal("expected error for invalid base URL in FetchJobContent")
	}
	if !strings.Contains(err.Error(), "ashby: build content request") {
		t.Errorf("error should mention 'ashby: build content request', got: %v", err)
	}
}

// TestAshby_FetchJobContent_DoError verifies that FetchJobContent returns
// an error when the HTTP request fails.
func TestAshby_FetchJobContent_DoError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		conn, _, _ := hj.Hijack()
		_ = conn.Close()
	}))
	defer srv.Close()

	a := &Ashby{baseURL: srv.URL}
	_, err := a.FetchJobContent(context.Background(), types.RawJob{
		URL: "https://jobs.ashbyhq.com/company/job-id-123",
	})
	if err == nil {
		t.Fatal("expected error when HTTP Do fails in FetchJobContent")
	}
	if !strings.Contains(err.Error(), "ashby: fetch content") {
		t.Errorf("error should mention 'ashby: fetch content', got: %v", err)
	}
}

// TestAshby_FetchJobContent_DecodeError verifies that FetchJobContent returns
// an error when the server returns 200 with non-JSON body.
func TestAshby_FetchJobContent_DecodeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not json at all"))
	}))
	defer srv.Close()

	a := &Ashby{baseURL: srv.URL}
	_, err := a.FetchJobContent(context.Background(), types.RawJob{
		URL: "https://jobs.ashbyhq.com/company/job-id-123",
	})
	if err == nil {
		t.Fatal("expected error for non-JSON content response")
	}
	if !strings.Contains(err.Error(), "ashby: decode content") {
		t.Errorf("error should mention 'ashby: decode content', got: %v", err)
	}
}

// ── FetchJobsViaAPI — additional error paths ──────────────────────────────────

// TestAshby_FetchJobsViaAPI_InvalidBaseURL verifies the
// NewRequestWithContext error path in FetchJobsViaAPI.
func TestAshby_FetchJobsViaAPI_InvalidBaseURL(t *testing.T) {
	a := &Ashby{apiBaseURL: "http://\x00invalid", apiKey: "test-key"}
	_, err := a.FetchJobsViaAPI(context.Background())
	if err == nil {
		t.Fatal("expected error for invalid API base URL in FetchJobsViaAPI")
	}
	if !strings.Contains(err.Error(), "ashby: build job.list request") {
		t.Errorf("error should mention 'ashby: build job.list request', got: %v", err)
	}
}

// TestAshby_FetchJobsViaAPI_DoError verifies that FetchJobsViaAPI returns an
// error when the HTTP request fails.
func TestAshby_FetchJobsViaAPI_DoError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		conn, _, _ := hj.Hijack()
		_ = conn.Close()
	}))
	defer srv.Close()

	a := &Ashby{apiBaseURL: srv.URL, apiKey: "test-key"}
	_, err := a.FetchJobsViaAPI(context.Background())
	if err == nil {
		t.Fatal("expected error when HTTP Do fails in FetchJobsViaAPI")
	}
	if !strings.Contains(err.Error(), "ashby: fetch job.list") {
		t.Errorf("error should mention 'ashby: fetch job.list', got: %v", err)
	}
}

// TestAshby_FetchJobsViaAPI_DecodeError verifies that FetchJobsViaAPI returns
// an error when the server returns 200 with non-JSON body.
func TestAshby_FetchJobsViaAPI_DecodeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not json"))
	}))
	defer srv.Close()

	a := &Ashby{apiBaseURL: srv.URL, apiKey: "test-key"}
	_, err := a.FetchJobsViaAPI(context.Background())
	if err == nil {
		t.Fatal("expected error for non-JSON body in FetchJobsViaAPI")
	}
	if !strings.Contains(err.Error(), "ashby: decode job.list") {
		t.Errorf("error should mention 'ashby: decode job.list', got: %v", err)
	}
}

// TestAshby_FetchJobsViaAPI_SuccessFalse verifies that FetchJobsViaAPI returns
// an error when the response body decodes successfully but success=false.
func TestAshby_FetchJobsViaAPI_SuccessFalse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":false,"results":[]}`))
	}))
	defer srv.Close()

	a := &Ashby{apiBaseURL: srv.URL, apiKey: "test-key"}
	_, err := a.FetchJobsViaAPI(context.Background())
	if err == nil {
		t.Fatal("expected error when success=false in FetchJobsViaAPI")
	}
	if !strings.Contains(err.Error(), "success=false") {
		t.Errorf("error should mention 'success=false', got: %v", err)
	}
}

// ── FetchJobContentViaAPI — additional error paths ───────────────────────────

// TestAshby_FetchJobContentViaAPI_InvalidBaseURL verifies the
// NewRequestWithContext error path in FetchJobContentViaAPI.
func TestAshby_FetchJobContentViaAPI_InvalidBaseURL(t *testing.T) {
	a := &Ashby{apiBaseURL: "http://\x00invalid", apiKey: "test-key"}
	_, err := a.FetchJobContentViaAPI(context.Background(), "ashby-job-001")
	if err == nil {
		t.Fatal("expected error for invalid API base URL in FetchJobContentViaAPI")
	}
	if !strings.Contains(err.Error(), "ashby: build job.info request") {
		t.Errorf("error should mention 'ashby: build job.info request', got: %v", err)
	}
}

// TestAshby_FetchJobContentViaAPI_DoError verifies that FetchJobContentViaAPI
// returns an error when the HTTP request fails.
func TestAshby_FetchJobContentViaAPI_DoError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		conn, _, _ := hj.Hijack()
		_ = conn.Close()
	}))
	defer srv.Close()

	a := &Ashby{apiBaseURL: srv.URL, apiKey: "test-key"}
	_, err := a.FetchJobContentViaAPI(context.Background(), "ashby-job-001")
	if err == nil {
		t.Fatal("expected error when HTTP Do fails in FetchJobContentViaAPI")
	}
	if !strings.Contains(err.Error(), "ashby: fetch job.info") {
		t.Errorf("error should mention 'ashby: fetch job.info', got: %v", err)
	}
}

// TestAshby_FetchJobContentViaAPI_HTTPError verifies that FetchJobContentViaAPI
// returns an error on a non-200 status from the server.
func TestAshby_FetchJobContentViaAPI_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	a := &Ashby{apiBaseURL: srv.URL, apiKey: "test-key"}
	_, err := a.FetchJobContentViaAPI(context.Background(), "ashby-job-001")
	if err == nil {
		t.Fatal("expected error on HTTP 403 in FetchJobContentViaAPI")
	}
	if !strings.Contains(err.Error(), "job.info unexpected status 403") {
		t.Errorf("error should mention status 403, got: %v", err)
	}
}

// TestAshby_FetchJobContentViaAPI_DecodeError verifies that FetchJobContentViaAPI
// returns an error when the server returns 200 with non-JSON body.
func TestAshby_FetchJobContentViaAPI_DecodeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not json at all"))
	}))
	defer srv.Close()

	a := &Ashby{apiBaseURL: srv.URL, apiKey: "test-key"}
	_, err := a.FetchJobContentViaAPI(context.Background(), "ashby-job-001")
	if err == nil {
		t.Fatal("expected error for non-JSON body in FetchJobContentViaAPI")
	}
	if !strings.Contains(err.Error(), "ashby: decode job.info") {
		t.Errorf("error should mention 'ashby: decode job.info', got: %v", err)
	}
}
