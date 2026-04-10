package greenhouse

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/trushilpatel/jobradar/internal/ats_fetcher/types"
)

// ── FetchJobs ─────────────────────────────────────────────────────────────────

func TestGreenhouse_FetchJobs(t *testing.T) {
	fakeResp := `{
		"jobs": [
			{
				"id": 12345,
				"title": "Backend Engineer",
				"location": {"name": "San Francisco, CA"},
				"absolute_url": "https://job-boards.greenhouse.io/stripe/jobs/12345"
			},
			{
				"id": 67890,
				"title": "ML Researcher",
				"location": {"name": "Remote"},
				"absolute_url": "https://job-boards.greenhouse.io/stripe/jobs/67890"
			}
		]
	}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fakeResp))
	}))
	defer srv.Close()

	g := &Greenhouse{listBaseURL: srv.URL}
	result, err := g.FetchJobs(context.Background(), "stripe")
	if err != nil {
		t.Fatalf("FetchJobs error: %v", err)
	}

	if len(result.Jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(result.Jobs))
	}

	j0 := result.Jobs[0]
	if j0.ID != "greenhouse-12345" {
		t.Errorf("job[0].ID = %q, want greenhouse-12345", j0.ID)
	}
	if j0.Title != "Backend Engineer" {
		t.Errorf("job[0].Title = %q, want Backend Engineer", j0.Title)
	}
	if j0.Location != "San Francisco, CA" {
		t.Errorf("job[0].Location = %q, want San Francisco, CA", j0.Location)
	}
	if j0.URL != "https://job-boards.greenhouse.io/stripe/jobs/12345" {
		t.Errorf("job[0].URL = %q, unexpected", j0.URL)
	}

	j1 := result.Jobs[1]
	if j1.ID != "greenhouse-67890" {
		t.Errorf("job[1].ID = %q, want greenhouse-67890", j1.ID)
	}
	if j1.Location != "Remote" {
		t.Errorf("job[1].Location = %q, want Remote", j1.Location)
	}
}

func TestGreenhouse_FetchJobs_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"jobs":[]}`))
	}))
	defer srv.Close()

	g := &Greenhouse{listBaseURL: srv.URL}
	result, err := g.FetchJobs(context.Background(), "stripe")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Jobs) != 0 {
		t.Errorf("expected 0 jobs, got %d", len(result.Jobs))
	}
}

func TestGreenhouse_FetchJobs_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	g := &Greenhouse{listBaseURL: srv.URL}
	_, err := g.FetchJobs(context.Background(), "stripe")
	if err == nil {
		t.Fatal("expected error on HTTP 500, got nil")
	}
}

func TestGreenhouse_FetchJobs_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`not json`))
	}))
	defer srv.Close()

	g := &Greenhouse{listBaseURL: srv.URL}
	_, err := g.FetchJobs(context.Background(), "stripe")
	if err == nil {
		t.Fatal("expected error on malformed JSON, got nil")
	}
}

func TestGreenhouse_CareersURL(t *testing.T) {
	g := &Greenhouse{}
	got := g.CareersURL("stripe")
	want := "https://boards-api.greenhouse.io/v1/boards/stripe/jobs"
	if got != want {
		t.Errorf("CareersURL(%q) = %q, want %q", "stripe", got, want)
	}
}

func TestGreenhouse_FetchJobs_UsesSlug(t *testing.T) {
	var capturedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"jobs":[]}`))
	}))
	defer srv.Close()

	g := &Greenhouse{listBaseURL: srv.URL}
	_, err := g.FetchJobs(context.Background(), "stripe")
	if err != nil {
		t.Fatalf("FetchJobs error: %v", err)
	}
	if capturedPath != "/v1/boards/stripe/jobs" {
		t.Errorf("API path = %q, want /v1/boards/stripe/jobs", capturedPath)
	}
}

// ── FetchJobContent ───────────────────────────────────────────────────────────

func TestGreenhouse_FetchJobContent(t *testing.T) {
	fakePosting := map[string]any{
		"id":       12345,
		"title":    "Backend Engineer",
		"location": map[string]any{"name": "San Francisco, CA"},
		"content":  "<p>Join our team.</p>",
		"departments": []any{
			map[string]any{"name": "Engineering"},
		},
		"pay_input_ranges": []any{
			map[string]any{
				"min_cents":     5000000,
				"max_cents":     7500000,
				"currency_type": "USD",
				"title":         "NYC Salary Range",
				"blurb":         "In order to provide transparency...",
			},
		},
	}
	fakeJSON, _ := json.Marshal(fakePosting)

	var capturedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(fakeJSON)
	}))
	defer srv.Close()

	g := &Greenhouse{contentBaseURL: srv.URL}
	job := types.RawJob{
		Adapter: "greenhouse",
		URL:     "https://job-boards.greenhouse.io/stripe/jobs/12345",
	}

	content, err := g.FetchJobContent(context.Background(), job)
	if err != nil {
		t.Fatalf("FetchJobContent error: %v", err)
	}

	if capturedPath != "/v1/boards/stripe/jobs/12345" {
		t.Errorf("API path = %q, want /v1/boards/stripe/jobs/12345", capturedPath)
	}
	if content.ID != "greenhouse-12345" {
		t.Errorf("content.ID = %q, want greenhouse-12345", content.ID)
	}
	if content.Title != "Backend Engineer" {
		t.Errorf("content.Title = %q, want Backend Engineer", content.Title)
	}
	if content.Location != "San Francisco, CA" {
		t.Errorf("content.Location = %q, want San Francisco, CA", content.Location)
	}
	if content.Department != "Engineering" {
		t.Errorf("content.Department = %q, want Engineering", content.Department)
	}
	if content.Compensation != "$50000 – $75000 USD" {
		t.Errorf("content.Compensation = %q, want $50000 – $75000 USD", content.Compensation)
	}
	if content.Description == "" {
		t.Error("content.Description is empty")
	}
}

func TestGreenhouse_FetchJobContent_BadURL_MissingID(t *testing.T) {
	g := &Greenhouse{}
	job := types.RawJob{
		Adapter: "greenhouse",
		URL:     "https://job-boards.greenhouse.io/stripe",
	}
	_, err := g.FetchJobContent(context.Background(), job)
	if err == nil {
		t.Fatal("expected error for URL missing job ID, got nil")
	}
}

func TestGreenhouse_FetchJobContent_BadURL_WrongSegment(t *testing.T) {
	g := &Greenhouse{}
	job := types.RawJob{
		Adapter: "greenhouse",
		URL:     "https://job-boards.greenhouse.io/stripe/roles/12345",
	}
	_, err := g.FetchJobContent(context.Background(), job)
	if err == nil {
		t.Fatal("expected error for wrong URL structure, got nil")
	}
}

func TestGreenhouse_FetchJobContent_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	g := &Greenhouse{contentBaseURL: srv.URL}
	job := types.RawJob{
		Adapter: "greenhouse",
		URL:     "https://job-boards.greenhouse.io/stripe/jobs/12345",
	}
	_, err := g.FetchJobContent(context.Background(), job)
	if err == nil {
		t.Fatal("expected error on HTTP 404, got nil")
	}
}

func TestGreenhouse_FetchJobContent_InvalidURL(t *testing.T) {
	g := &Greenhouse{}
	job := types.RawJob{
		Adapter: "greenhouse",
		URL:     "://not-a-url",
	}
	_, err := g.FetchJobContent(context.Background(), job)
	if err == nil {
		t.Fatal("expected error for invalid URL, got nil")
	}
}
