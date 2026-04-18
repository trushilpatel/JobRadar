package lever

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/trushilpatel/jobradar/pipeline/ats/types"
)

func TestLever_CareersURL(t *testing.T) {
	l := &Lever{}
	got := l.CareersURL("netlify")
	want := "https://jobs.lever.co/netlify"
	if got != want {
		t.Errorf("CareersURL(%q) = %q, want %q", "netlify", got, want)
	}
}

// ── FetchJobs ─────────────────────────────────────────────────────────────────

func TestLever_FetchJobs(t *testing.T) {
	fakeResp := `[
		{
			"id": "abc-111-def",
			"text": "Backend Engineer",
			"hostedUrl": "https://jobs.lever.co/netlify/abc-111-def",
			"categories": {
				"location": "Remote",
				"team": "Engineering",
				"commitment": "Full-time"
			}
		},
		{
			"id": "abc-222-ghi",
			"text": "Senior Frontend Engineer",
			"hostedUrl": "https://jobs.lever.co/netlify/abc-222-ghi",
			"categories": {
				"location": "San Francisco",
				"team": "Product",
				"commitment": "Full-time"
			}
		}
	]`

	var capturedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path + "?" + r.URL.RawQuery
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(fakeResp))
	}))
	defer srv.Close()

	l := &Lever{baseURL: srv.URL}
	result, err := l.FetchJobs(context.Background(), "netlify")
	if err != nil {
		t.Fatalf("FetchJobs error: %v", err)
	}

	if len(result.Jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(result.Jobs))
	}

	if capturedPath != "/v0/postings/netlify?mode=json" {
		t.Errorf("API path = %q, want /v0/postings/netlify?mode=json", capturedPath)
	}

	j0 := result.Jobs[0]
	if j0.ID != "lever-abc-111-def" {
		t.Errorf("job[0].ID = %q, want lever-abc-111-def", j0.ID)
	}
	if j0.Title != "Backend Engineer" {
		t.Errorf("job[0].Title = %q, want Backend Engineer", j0.Title)
	}
	if j0.Location != "Remote" {
		t.Errorf("job[0].Location = %q, want Remote", j0.Location)
	}
	if j0.URL != "https://jobs.lever.co/netlify/abc-111-def" {
		t.Errorf("job[0].URL = %q, unexpected", j0.URL)
	}
	if j0.EmploymentType != "full_time" {
		t.Errorf("job[0].EmploymentType = %q, want full_time", j0.EmploymentType)
	}

	j1 := result.Jobs[1]
	if j1.ID != "lever-abc-222-ghi" {
		t.Errorf("job[1].ID = %q, want lever-abc-222-ghi", j1.ID)
	}
	if j1.Location != "San Francisco" {
		t.Errorf("job[1].Location = %q, want San Francisco", j1.Location)
	}
}

func TestLever_FetchJobs_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	l := &Lever{baseURL: srv.URL}
	result, err := l.FetchJobs(context.Background(), "emptycorp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Jobs) != 0 {
		t.Errorf("expected 0 jobs, got %d", len(result.Jobs))
	}
}

func TestLever_FetchJobs_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	l := &Lever{baseURL: srv.URL}
	_, err := l.FetchJobs(context.Background(), "netlify")
	if err == nil {
		t.Fatal("expected error on HTTP 403, got nil")
	}
}

func TestLever_FetchJobs_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`not json`))
	}))
	defer srv.Close()

	l := &Lever{baseURL: srv.URL}
	_, err := l.FetchJobs(context.Background(), "netlify")
	if err == nil {
		t.Fatal("expected error on malformed JSON, got nil")
	}
}

// ── normalizeLeverCommitment ──────────────────────────────────────────────────

func TestNormalizeLeverCommitment(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"Full-time", "full_time"},
		{"full-time", "full_time"},
		{"Fulltime", "full_time"},
		{"Full time", "full_time"},
		{"Part-time", "part_time"},
		{"Contract", "contract"},
		{"Contractor", "contract"},
		{"Internship", "internship"},
		{"Intern", "internship"},
		{"", ""},
		{"Unknown", ""},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got := normalizeLeverCommitment(tc.input)
			if got != tc.want {
				t.Errorf("normalizeLeverCommitment(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// ── FetchJobContent ───────────────────────────────────────────────────────────

func TestLever_FetchJobContent(t *testing.T) {
	fakePosting := map[string]any{
		"id":                     "abc-111-def",
		"text":                   "Backend Engineer",
		"hostedUrl":              "https://jobs.lever.co/netlify/abc-111-def",
		"descriptionPlain":       "We are looking for a backend engineer.",
		"salaryDescriptionPlain": "$120,000 - $160,000 / year",
		"workplaceType":          "remote",
		"lists": []any{
			map[string]any{"text": "Requirements", "content": "<li>Go experience</li>"},
		},
		"categories": map[string]any{
			"location":   "Remote",
			"team":       "Engineering",
			"department": "Product & Engineering",
			"commitment": "Full-time",
		},
	}
	fakeJSON, _ := json.Marshal(fakePosting)

	var capturedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path + "?" + r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(fakeJSON)
	}))
	defer srv.Close()

	l := &Lever{baseURL: srv.URL}
	job := types.RawJob{
		Adapter: "lever",
		URL:     "https://jobs.lever.co/netlify/abc-111-def",
	}

	content, err := l.FetchJobContent(context.Background(), job)
	if err != nil {
		t.Fatalf("FetchJobContent error: %v", err)
	}

	if capturedPath != "/v0/postings/netlify/abc-111-def?mode=json" {
		t.Errorf("API path = %q, want /v0/postings/netlify/abc-111-def?mode=json", capturedPath)
	}
	if content.ID != "lever-abc-111-def" {
		t.Errorf("content.ID = %q, want lever-abc-111-def", content.ID)
	}
	if content.Title != "Backend Engineer" {
		t.Errorf("content.Title = %q, want Backend Engineer", content.Title)
	}
	if content.Location != "Remote" {
		t.Errorf("content.Location = %q, want Remote", content.Location)
	}
	if content.EmploymentType != "full_time" {
		t.Errorf("content.EmploymentType = %q, want full_time", content.EmploymentType)
	}
	if content.WorkplaceType != "remote" {
		t.Errorf("content.WorkplaceType = %q, want remote", content.WorkplaceType)
	}
	if content.Compensation != "$120,000 - $160,000 / year" {
		t.Errorf("content.Compensation = %q, want salary string", content.Compensation)
	}
	if content.Department != "Product & Engineering" {
		t.Errorf("content.Department = %q, want Product & Engineering", content.Department)
	}
	if content.Description == "" {
		t.Error("content.Description is empty")
	}
	if content.Requirements == "" {
		t.Error("content.Requirements is empty")
	}
}

func TestLever_FetchJobContent_SalaryRangeFallback(t *testing.T) {
	fakePosting := map[string]any{
		"id":               "abc-999",
		"text":             "Data Engineer",
		"hostedUrl":        "https://jobs.lever.co/acme/abc-999",
		"descriptionPlain": "Great role.",
		"salaryRange": map[string]any{
			"currency": "USD",
			"interval": "per-year-salary",
			"min":      120000,
			"max":      160000,
		},
		"categories": map[string]any{
			"location":   "New York",
			"commitment": "Full-time",
		},
	}
	fakeJSON, _ := json.Marshal(fakePosting)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(fakeJSON)
	}))
	defer srv.Close()

	l := &Lever{baseURL: srv.URL}
	content, err := l.FetchJobContent(context.Background(), types.RawJob{
		Adapter: "lever",
		URL:     "https://jobs.lever.co/acme/abc-999",
	})
	if err != nil {
		t.Fatalf("FetchJobContent error: %v", err)
	}
	if content.Compensation == "" {
		t.Error("content.Compensation is empty — expected salaryRange fallback")
	}
}

func TestLever_FetchJobContent_AdditionalPlainAppended(t *testing.T) {
	fakePosting := map[string]any{
		"id":               "abc-888",
		"text":             "SRE",
		"hostedUrl":        "https://jobs.lever.co/acme/abc-888",
		"descriptionPlain": "Main description.",
		"additionalPlain":  "About our company.",
		"categories": map[string]any{
			"location":   "Remote",
			"commitment": "Full-time",
		},
	}
	fakeJSON, _ := json.Marshal(fakePosting)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(fakeJSON)
	}))
	defer srv.Close()

	l := &Lever{baseURL: srv.URL}
	content, err := l.FetchJobContent(context.Background(), types.RawJob{
		Adapter: "lever",
		URL:     "https://jobs.lever.co/acme/abc-888",
	})
	if err != nil {
		t.Fatalf("FetchJobContent error: %v", err)
	}
	if !strings.Contains(content.Description, "Main description.") {
		t.Error("Description missing descriptionPlain")
	}
	if !strings.Contains(content.Description, "About our company.") {
		t.Error("Description missing additionalPlain")
	}
}

func TestLever_FetchJobContent_BadURL_MissingID(t *testing.T) {
	l := &Lever{}
	job := types.RawJob{
		Adapter: "lever",
		URL:     "https://jobs.lever.co/netlify",
	}
	_, err := l.FetchJobContent(context.Background(), job)
	if err == nil {
		t.Fatal("expected error for URL missing job ID, got nil")
	}
}

func TestLever_FetchJobContent_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	l := &Lever{baseURL: srv.URL}
	job := types.RawJob{
		Adapter: "lever",
		URL:     "https://jobs.lever.co/netlify/abc-111-def",
	}
	_, err := l.FetchJobContent(context.Background(), job)
	if err == nil {
		t.Fatal("expected error on HTTP 404, got nil")
	}
}

func TestLever_FetchJobContent_InvalidURL(t *testing.T) {
	l := &Lever{}
	job := types.RawJob{
		Adapter: "lever",
		URL:     "://not-a-url",
	}
	_, err := l.FetchJobContent(context.Background(), job)
	if err == nil {
		t.Fatal("expected error for invalid URL, got nil")
	}
}
