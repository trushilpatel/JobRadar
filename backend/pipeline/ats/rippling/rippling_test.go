package rippling

import (
	"context"
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

func TestRippling_ImplementsAdapter(t *testing.T) {
	var _ types.Adapter = (*Rippling)(nil)
}

func TestRippling_ImplementsContentFetcher(t *testing.T) {
	var _ types.ContentFetcher = (*Rippling)(nil)
}

func TestRippling_ImplementsURLProvider(t *testing.T) {
	var _ types.URLProvider = (*Rippling)(nil)
}

// ── Constructor ─────────────────────────────────────────────────────────────

func TestNew_DefaultSettings(t *testing.T) {
	r := New()
	if r.careersBaseURL != defaultCareersBaseURL {
		t.Errorf("careersBaseURL = %q, want %q", r.careersBaseURL, defaultCareersBaseURL)
	}
	if r.atsBaseURL != defaultATSBaseURL {
		t.Errorf("atsBaseURL = %q, want %q", r.atsBaseURL, defaultATSBaseURL)
	}
}

// ── CareersURL ──────────────────────────────────────────────────────────────

func TestCareersURL(t *testing.T) {
	r := New()
	got := r.CareersURL("any")
	want := "https://www.rippling.com/careers/open-roles"
	assertEqual(t, "CareersURL", got, want)
}

// ── FetchJobs — single US job ───────────────────────────────────────────────

func TestFetchJobs_SingleUSJob(t *testing.T) {
	html := buildNextJSHTML(t, nextJobsJSON([]map[string]interface{}{
		{
			"id":   "job-uuid-1",
			"name": "Software Engineer",
			"url":  "https://ats.rippling.com/rippling/jobs/job-uuid-1",
			"department": map[string]string{
				"name": "Engineering",
			},
			"locations": []map[string]string{
				{
					"name":          "San Francisco, CA",
					"country":       "United States",
					"countryCode":   "US",
					"state":         "California",
					"stateCode":     "CA",
					"city":          "San Francisco",
					"workplaceType": "ON_SITE",
				},
			},
			"language": "en",
		},
	}))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(html))
	}))
	defer srv.Close()

	rip := &Rippling{careersBaseURL: srv.URL}
	result, err := rip.FetchJobs(context.Background(), "")
	if err != nil {
		t.Fatalf("FetchJobs error: %v", err)
	}

	if len(result.Jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(result.Jobs))
	}

	job := result.Jobs[0]
	assertEqual(t, "ID", job.ID, "rippling-job-uuid-1")
	assertEqual(t, "Title", job.Title, "Software Engineer")
	assertEqual(t, "Location", job.Location, "San Francisco, CA")
	assertEqual(t, "URL", job.URL, "https://ats.rippling.com/rippling/jobs/job-uuid-1")
	assertEqual(t, "EmploymentType", job.EmploymentType, model.EmployFullTime)
	if job.FullContent != nil {
		t.Error("FullContent should be nil for thin adapter")
	}
}

// ── FetchJobs — filter non-US jobs ──────────────────────────────────────────

func TestFetchJobs_FiltersNonUSJobs(t *testing.T) {
	html := buildNextJSHTML(t, nextJobsJSON([]map[string]interface{}{
		{
			"id":   "us-job",
			"name": "US Position",
			"url":  "https://ats.rippling.com/rippling/jobs/us-job",
			"locations": []map[string]string{
				{
					"countryCode": "US",
					"city":        "New York",
					"stateCode":   "NY",
				},
			},
		},
		{
			"id":   "uk-job",
			"name": "UK Position",
			"url":  "https://ats.rippling.com/rippling/jobs/uk-job",
			"locations": []map[string]string{
				{
					"countryCode": "GB",
					"city":        "London",
					"stateCode":   "EN",
				},
			},
		},
	}))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(html))
	}))
	defer srv.Close()

	rip := &Rippling{careersBaseURL: srv.URL}
	result, err := rip.FetchJobs(context.Background(), "")
	if err != nil {
		t.Fatalf("FetchJobs error: %v", err)
	}

	if len(result.Jobs) != 1 {
		t.Fatalf("expected 1 job (only US), got %d", len(result.Jobs))
	}
	if result.Jobs[0].ID != "rippling-us-job" {
		t.Errorf("expected US job, got %q", result.Jobs[0].ID)
	}
}

// ── FetchJobs — all non-US jobs ─────────────────────────────────────────────

func TestFetchJobs_AllNonUSJobs(t *testing.T) {
	html := buildNextJSHTML(t, nextJobsJSON([]map[string]interface{}{
		{
			"id":   "job-1",
			"name": "London Position",
			"locations": []map[string]string{
				{
					"countryCode": "GB",
					"city":        "London",
					"stateCode":   "EN",
				},
			},
		},
		{
			"id":   "job-2",
			"name": "Paris Position",
			"locations": []map[string]string{
				{
					"countryCode": "FR",
					"city":        "Paris",
					"stateCode":   "75",
				},
			},
		},
	}))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(html))
	}))
	defer srv.Close()

	rip := &Rippling{careersBaseURL: srv.URL}
	result, err := rip.FetchJobs(context.Background(), "")
	if err != nil {
		t.Fatalf("FetchJobs error: %v", err)
	}

	if len(result.Jobs) != 0 {
		t.Errorf("expected 0 jobs (all non-US), got %d", len(result.Jobs))
	}
}

// ── FetchJobs — HTTP error ──────────────────────────────────────────────────

func TestFetchJobs_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	rip := &Rippling{careersBaseURL: srv.URL}
	_, err := rip.FetchJobs(context.Background(), "")
	if err == nil {
		t.Fatal("expected error on HTTP 500, got nil")
	}
	if !strings.Contains(err.Error(), "unexpected status 500") {
		t.Errorf("error should mention status 500, got: %v", err)
	}
}

// ── FetchJobs — malformed HTML (no __NEXT_DATA__) ───────────────────────────

func TestFetchJobs_MalformedHTML(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><body>No next data here</body></html>`))
	}))
	defer srv.Close()

	rip := &Rippling{careersBaseURL: srv.URL}
	_, err := rip.FetchJobs(context.Background(), "")
	if err == nil {
		t.Fatal("expected error on missing __NEXT_DATA__, got nil")
	}
	if !strings.Contains(err.Error(), "__NEXT_DATA__") {
		t.Errorf("error should mention __NEXT_DATA__: %v", err)
	}
}

// ── FetchJobs — multiple US locations ───────────────────────────────────────

func TestFetchJobs_MultipleLocations(t *testing.T) {
	html := buildNextJSHTML(t, nextJobsJSON([]map[string]interface{}{
		{
			"id":   "multi-loc-job",
			"name": "Multi-location Position",
			"url":  "https://ats.rippling.com/rippling/jobs/multi-loc-job",
			"locations": []map[string]string{
				{
					"countryCode": "US",
					"city":        "San Francisco",
					"stateCode":   "CA",
				},
				{
					"countryCode": "US",
					"city":        "New York",
					"stateCode":   "NY",
				},
				{
					"countryCode": "US",
					"city":        "Austin",
					"stateCode":   "TX",
				},
			},
		},
	}))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(html))
	}))
	defer srv.Close()

	rip := &Rippling{careersBaseURL: srv.URL}
	result, err := rip.FetchJobs(context.Background(), "")
	if err != nil {
		t.Fatalf("FetchJobs error: %v", err)
	}

	want := "San Francisco, CA; New York, NY; Austin, TX"
	assertEqual(t, "Location", result.Jobs[0].Location, want)
}

// ── FetchJobs — context cancelled ────────────────────────────────────────────

func TestFetchJobs_ContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Never responds, forcing context cancellation
		select {}
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	rip := &Rippling{careersBaseURL: srv.URL}
	_, err := rip.FetchJobs(ctx, "")
	if err == nil {
		t.Fatal("expected error when context is cancelled")
	}
}

// ── FetchJobs — ID format ───────────────────────────────────────────────────

func TestFetchJobs_IDFormat(t *testing.T) {
	html := buildNextJSHTML(t, nextJobsJSON([]map[string]interface{}{
		{
			"id":   "550e8400-e29b-41d4-a716-446655440000",
			"name": "Test Job",
			"url":  "https://ats.rippling.com/rippling/jobs/550e8400-e29b-41d4-a716-446655440000",
			"locations": []map[string]string{
				{
					"countryCode": "US",
					"city":        "SF",
					"stateCode":   "CA",
				},
			},
		},
	}))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(html))
	}))
	defer srv.Close()

	rip := &Rippling{careersBaseURL: srv.URL}
	result, err := rip.FetchJobs(context.Background(), "")
	if err != nil {
		t.Fatalf("FetchJobs error: %v", err)
	}

	if len(result.Jobs) > 0 {
		jobID := result.Jobs[0].ID
		if !strings.HasPrefix(jobID, "rippling-") {
			t.Errorf("ID %q should have 'rippling-' prefix", jobID)
		}
		expectedID := fmt.Sprintf("rippling-%s", "550e8400-e29b-41d4-a716-446655440000")
		assertEqual(t, "ID", jobID, expectedID)
	}
}

// ── FetchJobs — FullContent is nil ──────────────────────────────────────────

func TestFetchJobs_FullContentNil(t *testing.T) {
	html := buildNextJSHTML(t, nextJobsJSON([]map[string]interface{}{
		{
			"id":   "job-1",
			"name": "Test Job",
			"url":  "https://ats.rippling.com/rippling/jobs/job-1",
			"locations": []map[string]string{
				{
					"countryCode": "US",
					"city":        "SF",
					"stateCode":   "CA",
				},
			},
		},
	}))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(html))
	}))
	defer srv.Close()

	rip := &Rippling{careersBaseURL: srv.URL}
	result, err := rip.FetchJobs(context.Background(), "")
	if err != nil {
		t.Fatalf("FetchJobs error: %v", err)
	}

	if result.Jobs[0].FullContent != nil {
		t.Error("FullContent must be nil — Rippling is a thin adapter")
	}
}

// ── FetchJobContent — success ───────────────────────────────────────────────

func TestFetchJobContent_Success(t *testing.T) {
	detailHTML := buildNextJSHTMLDetail(t, nextDetailJSON(map[string]interface{}{
		"id":              "job-uuid-1",
		"title":           "Senior Software Engineer",
		"descriptionHtml": "<h2>About the role</h2><p>Build amazing systems.</p>",
		"compensation":    "$180,000 - $245,000 USD/year",
		"workplaceType":   "ON_SITE",
		"createdAt":       "2026-03-01T00:00:00.000Z",
		"department": map[string]string{
			"name": "Engineering",
		},
		"locations": []map[string]string{
			{
				"countryCode": "US",
				"city":        "San Francisco",
				"stateCode":   "CA",
			},
		},
	}))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(detailHTML))
	}))
	defer srv.Close()

	rip := &Rippling{atsBaseURL: srv.URL}
	rawJob := types.RawJob{
		ID: "rippling-job-uuid-1",
	}
	result, err := rip.FetchJobContent(context.Background(), rawJob)
	if err != nil {
		t.Fatalf("FetchJobContent error: %v", err)
	}

	assertEqual(t, "Title", result.Title, "Senior Software Engineer")
	if !strings.Contains(result.Description, "Build amazing systems") {
		t.Errorf("Description should contain job text, got: %q", result.Description)
	}
	assertEqual(t, "Department", result.Department, "Engineering")
	assertEqual(t, "Compensation", result.Compensation, "$180,000 - $245,000 USD/year")
	assertEqual(t, "WorkplaceType", result.WorkplaceType, "ON_SITE")
	assertEqual(t, "PublishedDate", result.PublishedDate, "2026-03-01T00:00:00.000Z")
}

// ── FetchJobContent — HTTP error ────────────────────────────────────────────

func TestFetchJobContent_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	rip := &Rippling{atsBaseURL: srv.URL}
	rawJob := types.RawJob{ID: "rippling-job-1"}
	_, err := rip.FetchJobContent(context.Background(), rawJob)
	if err == nil {
		t.Fatal("expected error on HTTP 404, got nil")
	}
	if !strings.Contains(err.Error(), "detail status 404") {
		t.Errorf("error should mention status 404: %v", err)
	}
}

// ── FetchJobContent — malformed HTML ────────────────────────────────────────

func TestFetchJobContent_MalformedHTML(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Returns HTML with no __NEXT_DATA__; should fallback to StripHTML
		_, _ = w.Write([]byte(`<html><body><h1>Job Page</h1><p>No structured data here</p></body></html>`))
	}))
	defer srv.Close()

	rip := &Rippling{atsBaseURL: srv.URL}
	rawJob := types.RawJob{ID: "rippling-job-1"}
	result, err := rip.FetchJobContent(context.Background(), rawJob)
	if err != nil {
		t.Fatalf("FetchJobContent should not error on fallback: %v", err)
	}
	// Should have stripped some content
	if result.Description == "" {
		t.Error("Description should be populated via fallback StripHTML")
	}
}

// ── Real integration tests ──────────────────────────────────────────────────
// Guard: set TEST_REAL=1 to run against real Rippling API.

func TestFetchJobs_Real(t *testing.T) {
	if os.Getenv("TEST_REAL") == "" {
		t.Skip("skipping real API test; set TEST_REAL=1 to run")
	}

	rip := New()
	result, err := rip.FetchJobs(context.Background(), "")
	if err != nil {
		t.Fatalf("FetchJobs error: %v", err)
	}
	if len(result.Jobs) == 0 {
		t.Fatal("expected at least one job from live Rippling API")
	}

	// Spot-check the first job
	j := result.Jobs[0]
	if !strings.HasPrefix(j.ID, "rippling-") {
		t.Errorf("ID %q does not have 'rippling-' prefix", j.ID)
	}
	if j.Title == "" {
		t.Error("Title should not be empty")
	}
	if j.URL == "" {
		t.Error("URL should not be empty")
	}
	if j.Location == "" {
		t.Error("Location should not be empty (all jobs are US)")
	}

	t.Logf("fetched %d jobs; first: %q at %q", len(result.Jobs), j.Title, j.Location)
}

// ── Helpers ─────────────────────────────────────────────────────────────────

// buildNextJSHTML constructs an HTML page with __NEXT_DATA__ embedded.
func buildNextJSHTML(t *testing.T, nextDataJSON string) string {
	t.Helper()
	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head><title>Rippling Careers</title></head>
<body>
<script id="__NEXT_DATA__" type="application/json">%s</script>
</body>
</html>`, nextDataJSON)
}

// buildNextJSHTMLDetail constructs a detail page HTML with __NEXT_DATA__ embedded.
func buildNextJSHTMLDetail(t *testing.T, nextDataJSON string) string {
	t.Helper()
	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head><title>Job Detail</title></head>
<body>
<script id="__NEXT_DATA__" type="application/json">%s</script>
</body>
</html>`, nextDataJSON)
}

// nextJobsJSON builds the props.pageProps.jobs structure from a list of jobs.
func nextJobsJSON(jobs []map[string]interface{}) string {
	jsonStr := `{"props":{"pageProps":{"jobs":{"items":[`
	for i, job := range jobs {
		if i > 0 {
			jsonStr += ","
		}
		// Convert job map to minimal JSON
		jsonStr += jobToJSON(job)
	}
	jsonStr += `]}}}}`
	return jsonStr
}

// nextDetailJSON builds the props.pageProps.apiData structure for detail pages.
func nextDetailJSON(data map[string]interface{}) string {
	jsonStr := `{"props":{"pageProps":{"apiData":{`
	first := true
	for key, val := range data {
		if !first {
			jsonStr += ","
		}
		first = false
		jsonStr += fmt.Sprintf(`"%s":%s`, key, toJSON(val))
	}
	jsonStr += `}}}}`
	return jsonStr
}

// jobToJSON converts a job map to JSON string. Minimal implementation for testing.
func jobToJSON(job map[string]interface{}) string {
	jsonStr := "{"
	first := true
	for key, val := range job {
		if !first {
			jsonStr += ","
		}
		first = false
		jsonStr += fmt.Sprintf(`"%s":%s`, key, toJSON(val))
	}
	jsonStr += "}"
	return jsonStr
}

// toJSON converts a value to JSON. Minimal implementation.
func toJSON(val interface{}) string {
	switch v := val.(type) {
	case string:
		return fmt.Sprintf(`"%s"`, v)
	case int:
		return fmt.Sprintf(`%d`, v)
	case []map[string]string:
		if len(v) == 0 {
			return "[]"
		}
		jsonStr := "["
		for i, item := range v {
			if i > 0 {
				jsonStr += ","
			}
			itemStr := "{"
			first := true
			for k, val := range item {
				if !first {
					itemStr += ","
				}
				first = false
				itemStr += fmt.Sprintf(`"%s":"%s"`, k, val)
			}
			itemStr += "}"
			jsonStr += itemStr
		}
		jsonStr += "]"
		return jsonStr
	case map[string]string:
		if len(v) == 0 {
			return "{}"
		}
		jsonStr := "{"
		first := true
		for k, val := range v {
			if !first {
				jsonStr += ","
			}
			first = false
			jsonStr += fmt.Sprintf(`"%s":"%s"`, k, val)
		}
		jsonStr += "}"
		return jsonStr
	default:
		return "{}"
	}
}

func assertEqual(t *testing.T, field, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("%s: got %q, want %q", field, got, want)
	}
}
