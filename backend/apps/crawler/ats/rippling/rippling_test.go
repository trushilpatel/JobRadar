package rippling

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/trushilpatel/jobradar/apps/crawler/ats/types"
	jobcommon "github.com/trushilpatel/jobradar/service/job/common"
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
	assertEqual(t, "EmploymentType", job.EmploymentType, jobcommon.EmployFullTime)
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

// ── Resolver fallback ───────────────────────────────────────────────────────

// TestResolveCareersBaseURL_ZeroValueFallsBackToDefault — empty branch.
func TestResolveCareersBaseURL_ZeroValueFallsBackToDefault(t *testing.T) {
	r := &Rippling{}
	if got := r.resolveCareersBaseURL(); got != defaultCareersBaseURL {
		t.Errorf("zero-value resolveCareersBaseURL: got %q, want %q", got, defaultCareersBaseURL)
	}
}

// TestResolveATSBaseURL_ZeroValueFallsBackToDefault — empty branch.
func TestResolveATSBaseURL_ZeroValueFallsBackToDefault(t *testing.T) {
	r := &Rippling{}
	if got := r.resolveATSBaseURL(); got != defaultATSBaseURL {
		t.Errorf("zero-value resolveATSBaseURL: got %q, want %q", got, defaultATSBaseURL)
	}
}

// ── FetchJobs — malformed JSON in __NEXT_DATA__ ─────────────────────────────

// TestFetchJobs_MalformedNextDataJSON verifies that when __NEXT_DATA__ is
// present but contains invalid JSON, FetchJobs returns a parse error.
func TestFetchJobs_MalformedNextDataJSON(t *testing.T) {
	// Wrap invalid JSON in the expected script tag.
	html := `<html><body><script id="__NEXT_DATA__" type="application/json">NOT_VALID_JSON</script></body></html>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(html))
	}))
	defer srv.Close()

	rip := &Rippling{careersBaseURL: srv.URL}
	_, err := rip.FetchJobs(context.Background(), "")
	if err == nil {
		t.Fatal("expected error on malformed __NEXT_DATA__ JSON, got nil")
	}
	if !strings.Contains(err.Error(), "parse next data") {
		t.Errorf("error should mention 'parse next data': %v", err)
	}
}

// ── FetchJobs — duplicate job IDs are deduplicated ──────────────────────────

// TestFetchJobs_DeduplicatesJobs verifies that jobs with the same ID appearing
// more than once in the response are deduplicated — only one item per ID is
// returned.
func TestFetchJobs_DeduplicatesJobs(t *testing.T) {
	// Build raw JSON directly to force two identical IDs.
	rawJSON := `{"props":{"pageProps":{"jobs":{"items":[` +
		`{"id":"dup-id","name":"Dup Job","url":"https://ats.rippling.com/rippling/jobs/dup-id","locations":[{"countryCode":"US","city":"SF","stateCode":"CA"}]},` +
		`{"id":"dup-id","name":"Dup Job Again","url":"https://ats.rippling.com/rippling/jobs/dup-id","locations":[{"countryCode":"US","city":"SF","stateCode":"CA"}]}` +
		`]}}}}`

	html := fmt.Sprintf(`<html><body><script id="__NEXT_DATA__" type="application/json">%s</script></body></html>`, rawJSON)

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
		t.Errorf("expected 1 deduplicated job, got %d", len(result.Jobs))
	}
}

// ── FetchJobContent — malformed JSON in __NEXT_DATA__ ──────────────────────

// TestFetchJobContent_MalformedNextDataJSON verifies that when __NEXT_DATA__
// is present but contains invalid JSON, FetchJobContent falls back gracefully
// (returns stripped body, no error).
func TestFetchJobContent_MalformedNextDataJSON(t *testing.T) {
	html := `<html><body><h1>Job</h1><script id="__NEXT_DATA__" type="application/json">NOT_VALID_JSON</script></body></html>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(html))
	}))
	defer srv.Close()

	rip := &Rippling{atsBaseURL: srv.URL}
	rawJob := types.RawJob{ID: "rippling-job-1"}
	result, err := rip.FetchJobContent(context.Background(), rawJob)
	if err != nil {
		t.Fatalf("FetchJobContent should not error on malformed JSON (fallback): %v", err)
	}
	// Fallback should strip HTML and return the original job ID.
	if result.ID != "rippling-job-1" {
		t.Errorf("fallback ID: got %q, want %q", result.ID, "rippling-job-1")
	}
}

// ── extractNextData ─────────────────────────────────────────────────────────

// TestExtractNextData_MalformedScriptTag verifies that when the __NEXT_DATA__
// script tag is found but has no closing '>' for the opening tag,
// extractNextData returns the "malformed" error.
func TestExtractNextData_MalformedScriptTag(t *testing.T) {
	// Build HTML where the script tag has no '>' after the id attribute.
	html := `<html><body><script id="__NEXT_DATA__"` // no '>' here
	_, err := extractNextData(html)
	if err == nil {
		t.Fatal("expected error on malformed script tag, got nil")
	}
	if !strings.Contains(err.Error(), "malformed") {
		t.Errorf("error should mention 'malformed': %v", err)
	}
}

// TestExtractNextData_MissingClosingTag verifies the "closing </script> not
// found" error path.
func TestExtractNextData_MissingClosingTag(t *testing.T) {
	// __NEXT_DATA__ open tag is present, '>' is present, but </script> is absent.
	html := `<html><body><script id="__NEXT_DATA__" type="application/json">{"foo":1}`
	_, err := extractNextData(html)
	if err == nil {
		t.Fatal("expected error on missing </script>, got nil")
	}
	if !strings.Contains(err.Error(), "closing </script>") {
		t.Errorf("error should mention closing tag: %v", err)
	}
}

// TestExtractNextData_NotFound verifies the "__NEXT_DATA__ script not found"
// error when the script tag is absent entirely.
func TestExtractNextData_NotFound(t *testing.T) {
	_, err := extractNextData(`<html><body></body></html>`)
	if err == nil {
		t.Fatal("expected error when __NEXT_DATA__ is absent")
	}
	if !strings.Contains(err.Error(), "__NEXT_DATA__ script not found") {
		t.Errorf("unexpected error: %v", err)
	}
}

// ── detailToContent — fallback field chains ─────────────────────────────────

// TestDetailToContent_FallbackDescription verifies that when DescriptionHTML is
// empty, the Description field is used.
func TestDetailToContent_FallbackDescription(t *testing.T) {
	detail := jobDetailData{
		ID:          "job-1",
		Title:       "Engineer",
		Description: "Plain text description",
	}
	got := detailToContent("rippling-job-1", detail)
	if !strings.Contains(got.Description, "Plain text description") {
		t.Errorf("expected Description fallback, got %q", got.Description)
	}
}

// TestDetailToContent_FallbackJobDescription verifies that when
// DescriptionHTML and Description are both empty, JobDescription is used.
func TestDetailToContent_FallbackJobDescription(t *testing.T) {
	detail := jobDetailData{
		ID:             "job-1",
		Title:          "Engineer",
		JobDescription: "Job description text",
	}
	got := detailToContent("rippling-job-1", detail)
	if !strings.Contains(got.Description, "Job description text") {
		t.Errorf("expected JobDescription fallback, got %q", got.Description)
	}
}

// TestDetailToContent_FallbackBody verifies that the Body field is the last
// resort for description content.
func TestDetailToContent_FallbackBody(t *testing.T) {
	detail := jobDetailData{
		ID:    "job-1",
		Title: "Engineer",
		Body:  "Body text",
	}
	got := detailToContent("rippling-job-1", detail)
	if !strings.Contains(got.Description, "Body text") {
		t.Errorf("expected Body fallback, got %q", got.Description)
	}
}

// TestDetailToContent_FallbackCompensationRange verifies that when
// Compensation is empty, CompensationRng is used.
func TestDetailToContent_FallbackCompensationRange(t *testing.T) {
	detail := jobDetailData{
		ID:              "job-1",
		Title:           "Engineer",
		CompensationRng: "$120k-$180k",
	}
	got := detailToContent("rippling-job-1", detail)
	if got.Compensation != "$120k-$180k" {
		t.Errorf("Compensation: got %q, want %q", got.Compensation, "$120k-$180k")
	}
}

// TestDetailToContent_FallbackSalary verifies that when Compensation and
// CompensationRng are both empty, Salary is used.
func TestDetailToContent_FallbackSalary(t *testing.T) {
	detail := jobDetailData{
		ID:     "job-1",
		Title:  "Engineer",
		Salary: "$100k",
	}
	got := detailToContent("rippling-job-1", detail)
	if got.Compensation != "$100k" {
		t.Errorf("Compensation (Salary fallback): got %q, want %q", got.Compensation, "$100k")
	}
}

// TestDetailToContent_FallbackWorkplaceType verifies that when WorkplaceType
// is empty, the snake_case workplace_type field is used.
func TestDetailToContent_FallbackWorkplaceType(t *testing.T) {
	detail := jobDetailData{
		ID:           "job-1",
		Title:        "Engineer",
		WorkplaceTyp: "REMOTE",
	}
	got := detailToContent("rippling-job-1", detail)
	if got.WorkplaceType != "REMOTE" {
		t.Errorf("WorkplaceType: got %q, want %q", got.WorkplaceType, "REMOTE")
	}
}

// TestDetailToContent_FallbackDepartmentName verifies that when the nested
// Department.Name is empty, the flat DepartmentName field is used.
func TestDetailToContent_FallbackDepartmentName(t *testing.T) {
	detail := jobDetailData{
		ID:             "job-1",
		Title:          "Engineer",
		DepartmentName: "Product",
	}
	got := detailToContent("rippling-job-1", detail)
	if got.Department != "Product" {
		t.Errorf("Department: got %q, want %q", got.Department, "Product")
	}
}

// ── formatAllLocations ──────────────────────────────────────────────────────

// TestFormatAllLocations_CityOnly verifies that a location with a city but no
// stateCode is emitted as just the city name.
func TestFormatAllLocations_CityOnly(t *testing.T) {
	locs := []jobLocation{{City: "London"}}
	got := formatAllLocations(locs)
	if got != "London" {
		t.Errorf("city-only: got %q, want %q", got, "London")
	}
}

// TestFormatAllLocations_NameFallback verifies that a location with no city
// falls back to the Name field.
func TestFormatAllLocations_NameFallback(t *testing.T) {
	locs := []jobLocation{{Name: "Remote"}}
	got := formatAllLocations(locs)
	if got != "Remote" {
		t.Errorf("name fallback: got %q, want %q", got, "Remote")
	}
}

// TestFormatAllLocations_Empty verifies that a location with no city and no
// name produces an empty string.
func TestFormatAllLocations_Empty(t *testing.T) {
	locs := []jobLocation{{CountryCode: "US"}} // no city, no name
	got := formatAllLocations(locs)
	if got != "" {
		t.Errorf("empty location: got %q, want empty", got)
	}
}

// TestFormatAllLocations_MixedVariants verifies correct joining when multiple
// location shapes appear together.
func TestFormatAllLocations_MixedVariants(t *testing.T) {
	locs := []jobLocation{
		{City: "San Francisco", StateCode: "CA"}, // city + state
		{City: "Austin"},                         // city only
		{Name: "Remote"},                         // name fallback
	}
	got := formatAllLocations(locs)
	want := "San Francisco, CA; Austin; Remote"
	if got != want {
		t.Errorf("mixed: got %q, want %q", got, want)
	}
}

// TestFormatAllLocations_NilSlice verifies that a nil slice returns empty.
func TestFormatAllLocations_NilSlice(t *testing.T) {
	got := formatAllLocations(nil)
	if got != "" {
		t.Errorf("nil slice: got %q, want empty", got)
	}
}

// ── stripPrefix ─────────────────────────────────────────────────────────────

// TestStripPrefix_NoPrefix verifies that an ID without the "rippling-" prefix
// is returned unchanged (passthrough for already-bare IDs).
func TestStripPrefix_NoPrefix(t *testing.T) {
	got := stripPrefix("some-bare-uuid")
	if got != "some-bare-uuid" {
		t.Errorf("stripPrefix no-prefix: got %q, want %q", got, "some-bare-uuid")
	}
}

// TestStripPrefix_WithPrefix verifies that the "rippling-" prefix is removed.
func TestStripPrefix_WithPrefix(t *testing.T) {
	got := stripPrefix("rippling-some-bare-uuid")
	if got != "some-bare-uuid" {
		t.Errorf("stripPrefix with-prefix: got %q, want %q", got, "some-bare-uuid")
	}
}

// TestStripPrefix_EmptyID verifies that an empty string returns empty.
func TestStripPrefix_EmptyID(t *testing.T) {
	got := stripPrefix("")
	if got != "" {
		t.Errorf("stripPrefix empty: got %q, want empty", got)
	}
}

// ── FetchJobContent — empty job ID after stripping prefix ──────────────────

// TestFetchJobContent_EmptyJobID verifies that when the job ID is empty after
// stripping the "rippling-" prefix, FetchJobContent returns an error.
func TestFetchJobContent_EmptyJobID(t *testing.T) {
	rip := &Rippling{atsBaseURL: "https://ats.rippling.com"}
	// "rippling-" prefix only → jobID becomes "" after stripPrefix.
	rawJob := types.RawJob{ID: "rippling-"}
	_, err := rip.FetchJobContent(context.Background(), rawJob)
	if err == nil {
		t.Fatal("expected error on empty job ID, got nil")
	}
	if !strings.Contains(err.Error(), "empty job ID") {
		t.Errorf("error should mention empty job ID: %v", err)
	}
}

// ── FetchJobs — build request error (bad base URL) ─────────────────────────

// TestFetchJobs_InvalidBaseURL exercises the http.NewRequestWithContext error
// branch in FetchJobs via a base URL with an embedded NUL byte.
func TestFetchJobs_InvalidBaseURL(t *testing.T) {
	rip := &Rippling{careersBaseURL: "http://\x00invalid"}
	_, err := rip.FetchJobs(context.Background(), "")
	if err == nil {
		t.Fatal("expected error on invalid base URL, got nil")
	}
	if !strings.Contains(err.Error(), "build request") {
		t.Errorf("error should mention 'build request': %v", err)
	}
}

// ── FetchJobs — read body error ─────────────────────────────────────────────

// TestFetchJobs_ReadBodyError exercises the io.ReadAll error branch by closing
// the connection immediately after writing the status line and headers (before
// the body is sent).
func TestFetchJobs_ReadBodyError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Hijack the connection and close it after sending a partial 200
		// response — this causes io.ReadAll to fail on the client side.
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Error("httptest server does not support Hijacker")
			return
		}
		conn, buf, err := hj.Hijack()
		if err != nil {
			t.Errorf("Hijack: %v", err)
			return
		}
		defer func() { _ = conn.Close() }()
		// Send a valid HTTP status line then immediately close without a body.
		_, _ = buf.WriteString("HTTP/1.1 200 OK\r\nContent-Type: text/html\r\nContent-Length: 1000\r\n\r\n")
		_ = buf.Flush()
		// Closing conn here makes io.ReadAll return an error on the client.
	}))
	defer srv.Close()

	rip := &Rippling{careersBaseURL: srv.URL}
	_, err := rip.FetchJobs(context.Background(), "")
	if err == nil {
		t.Fatal("expected error on read body failure, got nil")
	}
	if !strings.Contains(err.Error(), "read response body") {
		t.Errorf("error should mention 'read response body': %v", err)
	}
}

// ── FetchJobContent — build request error ──────────────────────────────────

// TestFetchJobContent_InvalidBaseURL exercises the http.NewRequestWithContext
// error branch in FetchJobContent via a base URL that cannot be parsed.
func TestFetchJobContent_InvalidBaseURL(t *testing.T) {
	rip := &Rippling{atsBaseURL: "http://\x00invalid"}
	rawJob := types.RawJob{ID: "rippling-job-uuid-1"}
	_, err := rip.FetchJobContent(context.Background(), rawJob)
	if err == nil {
		t.Fatal("expected error on invalid ATS base URL, got nil")
	}
	if !strings.Contains(err.Error(), "build detail request") {
		t.Errorf("error should mention 'build detail request': %v", err)
	}
}

// ── FetchJobContent — context cancelled ─────────────────────────────────────

// TestFetchJobContent_ContextCancelled exercises the http.Do error branch in
// FetchJobContent by pre-cancelling the context.
func TestFetchJobContent_ContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	rip := &Rippling{atsBaseURL: srv.URL}
	rawJob := types.RawJob{ID: "rippling-job-uuid-1"}
	_, err := rip.FetchJobContent(ctx, rawJob)
	if err == nil {
		t.Fatal("expected error when context is cancelled, got nil")
	}
}

// ── FetchJobContent — read detail response error ────────────────────────────

// TestFetchJobContent_ReadBodyError exercises the io.ReadAll error branch in
// FetchJobContent by hijacking the connection mid-response.
func TestFetchJobContent_ReadBodyError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Error("httptest server does not support Hijacker")
			return
		}
		conn, buf, err := hj.Hijack()
		if err != nil {
			t.Errorf("Hijack: %v", err)
			return
		}
		defer func() { _ = conn.Close() }()
		_, _ = buf.WriteString("HTTP/1.1 200 OK\r\nContent-Type: text/html\r\nContent-Length: 1000\r\n\r\n")
		_ = buf.Flush()
	}))
	defer srv.Close()

	rip := &Rippling{atsBaseURL: srv.URL}
	rawJob := types.RawJob{ID: "rippling-job-uuid-1"}
	_, err := rip.FetchJobContent(context.Background(), rawJob)
	if err == nil {
		t.Fatal("expected error on read detail response failure, got nil")
	}
	if !strings.Contains(err.Error(), "read detail response") {
		t.Errorf("error should mention 'read detail response': %v", err)
	}
}
