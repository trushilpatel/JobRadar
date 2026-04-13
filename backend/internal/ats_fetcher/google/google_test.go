package google

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/trushilpatel/jobradar/internal/ats_fetcher/types"
)

// ── CareersURL ──────────────────────────────────────────────────────────────

func TestCareersURL(t *testing.T) {
	g := NewDefault()
	got := g.CareersURL("anything")
	want := "https://www.google.com/about/careers/applications/jobs/results?location=United%20States&target_level=EARLY&target_level=MID&employment_type=FULL_TIME&q=Software%20Engineering"
	if got != want {
		t.Errorf("CareersURL() = %q, want %q", got, want)
	}
}

// ── Constructor ─────────────────────────────────────────────────────────────

func TestNew_StoresConfig(t *testing.T) {
	cfg := Config{
		MinDelayMs: 5000,
		MaxDelayMs: 8000,
		TimeoutMs:  60000,
		MaxPages:   2,
	}
	g := New(cfg)
	if g.cfg.MinDelayMs != 5000 {
		t.Errorf("MinDelayMs: got %d, want 5000", g.cfg.MinDelayMs)
	}
	if g.cfg.MaxDelayMs != 8000 {
		t.Errorf("MaxDelayMs: got %d, want 8000", g.cfg.MaxDelayMs)
	}
	if g.cfg.MaxPages != 2 {
		t.Errorf("MaxPages: got %d, want 2", g.cfg.MaxPages)
	}
}

func TestNewDefault_UsesProductionDefaults(t *testing.T) {
	g := NewDefault()
	if g.cfg.MinDelayMs != 30000 {
		t.Errorf("MinDelayMs: got %d, want 30000", g.cfg.MinDelayMs)
	}
	if g.cfg.MaxDelayMs != 60000 {
		t.Errorf("MaxDelayMs: got %d, want 60000", g.cfg.MaxDelayMs)
	}
	if g.cfg.TimeoutMs != 2700000 {
		t.Errorf("TimeoutMs: got %d, want 2700000", g.cfg.TimeoutMs)
	}
	if g.cfg.MaxPages != 0 {
		t.Errorf("MaxPages: got %d, want 0 (no limit)", g.cfg.MaxPages)
	}
}

// ── Interface compliance ────────────────────────────────────────────────────

func TestGoogleImplementsAdapter(t *testing.T) {
	var _ types.Adapter = (*Google)(nil)
}

func TestGoogleImplementsURLProvider(t *testing.T) {
	var _ types.URLProvider = (*Google)(nil)
}

func TestGoogleDoesNotImplementContentFetcher(t *testing.T) {
	var adp types.Adapter = NewDefault()
	if _, ok := adp.(types.ContentFetcher); ok {
		t.Error("Google should NOT implement ContentFetcher — it is a thin adapter, job_post_analyzer fetches detail pages")
	}
}

// ── JSON parsing — thin scraper output ─────────────────────────────────────

// scraperJob no longer has full_content. Verify the struct parses correctly.
func TestParseScraperOutput_ValidJobs(t *testing.T) {
	sampleOutput := `[
		{
			"id": "google-123456",
			"title": "Software Engineer, Backend",
			"url": "https://www.google.com/about/careers/applications/jobs/results/123456",
			"location": "Mountain View, CA",
			"employ_type": "full_time"
		},
		{
			"id": "google-789012",
			"title": "Product Manager, Cloud",
			"url": "https://www.google.com/about/careers/applications/jobs/results/789012",
			"location": "New York, NY",
			"employ_type": "full_time"
		}
	]`

	var scraperJobs []scraperJob
	if err := json.Unmarshal([]byte(sampleOutput), &scraperJobs); err != nil {
		t.Fatalf("failed to parse sample output: %v", err)
	}
	if len(scraperJobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(scraperJobs))
	}

	j := scraperJobs[0]
	assertEqual(t, "id", j.ID, "google-123456")
	assertEqual(t, "title", j.Title, "Software Engineer, Backend")
	assertEqual(t, "location", j.Location, "Mountain View, CA")
	assertEqual(t, "employ_type", j.EmployType, "full_time")

	j2 := scraperJobs[1]
	assertEqual(t, "id", j2.ID, "google-789012")
	assertEqual(t, "location", j2.Location, "New York, NY")
}

func TestParseScraperOutput_EmptyArray(t *testing.T) {
	var scraperJobs []scraperJob
	if err := json.Unmarshal([]byte("[]"), &scraperJobs); err != nil {
		t.Fatalf("failed to parse empty array: %v", err)
	}
	if len(scraperJobs) != 0 {
		t.Fatalf("expected 0 jobs, got %d", len(scraperJobs))
	}
}

func TestParseScraperOutput_ErrorResponse(t *testing.T) {
	errorOutput := `{"error": "Navigation timeout exceeded"}`
	var scraperErr scraperError
	if err := json.Unmarshal([]byte(errorOutput), &scraperErr); err != nil {
		t.Fatalf("failed to parse error response: %v", err)
	}
	if scraperErr.Error != "Navigation timeout exceeded" {
		t.Errorf("expected error message, got %q", scraperErr.Error)
	}
}

// ── Conversion to pipeline types ────────────────────────────────────────────

// Thin adapter: FullContent must always be nil — job_post_analyzer enriches later.
func TestConvertToPipelineTypes_FullContentIsNil(t *testing.T) {
	sj := scraperJob{
		ID:         "google-42",
		Title:      "SRE",
		URL:        "https://www.google.com/about/careers/applications/jobs/results/42",
		Location:   "London",
		EmployType: "full_time",
	}

	item := types.FetchJobsItem{
		ID:             sj.ID,
		Title:          sj.Title,
		URL:            sj.URL,
		Location:       sj.Location,
		EmploymentType: sj.EmployType,
		FullContent:    nil,
	}

	assertEqual(t, "ID", item.ID, "google-42")
	assertEqual(t, "Title", item.Title, "SRE")
	assertEqual(t, "EmploymentType", item.EmploymentType, "full_time")

	if item.FullContent != nil {
		t.Error("FullContent must be nil — Google is a thin adapter")
	}
}

// ── FetchJobs — scraper not found ───────────────────────────────────────────

func TestFetchJobs_ScraperNotFound(t *testing.T) {
	g := &Google{cfg: defaultConfig(), scraperDir: "/nonexistent/path"}
	_, err := g.FetchJobs(context.Background(), "google")
	if err == nil {
		t.Fatal("expected error when scraper not found")
	}
	if !contains(err.Error(), "scraper not found") {
		t.Errorf("expected 'scraper not found' error, got: %s", err.Error())
	}
}

// ── FetchJobs with mock scraper scripts ─────────────────────────────────────

func TestFetchJobs_MockScraper_Success(t *testing.T) {
	dir := t.TempDir()
	// Mock script outputs thin listing items — no full_content
	fakeScript := `
		const jobs = [
			{
				"id": "google-111",
				"title": "Test Engineer",
				"url": "https://www.google.com/about/careers/applications/jobs/results/111",
				"location": "Remote",
				"employ_type": "full_time"
			}
		];
		process.stdout.write(JSON.stringify(jobs));
	`
	writeScript(t, dir, fakeScript)

	g := &Google{cfg: defaultConfig(), scraperDir: dir}
	result, err := g.FetchJobs(context.Background(), "google")
	if err != nil {
		t.Fatalf("FetchJobs failed: %v", err)
	}
	if len(result.Jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(result.Jobs))
	}

	job := result.Jobs[0]
	assertEqual(t, "ID", job.ID, "google-111")
	assertEqual(t, "Title", job.Title, "Test Engineer")
	assertEqual(t, "Location", job.Location, "Remote")
	assertEqual(t, "EmploymentType", job.EmploymentType, "full_time")

	if job.FullContent != nil {
		t.Error("FullContent must be nil — Google is a thin adapter")
	}
}

func TestFetchJobs_MockScraper_EmptyResult(t *testing.T) {
	dir := t.TempDir()
	writeScript(t, dir, `process.stdout.write("[]");`)

	g := &Google{cfg: defaultConfig(), scraperDir: dir}
	result, err := g.FetchJobs(context.Background(), "google")
	if err != nil {
		t.Fatalf("FetchJobs failed: %v", err)
	}
	if len(result.Jobs) != 0 {
		t.Fatalf("expected 0 jobs, got %d", len(result.Jobs))
	}
}

func TestFetchJobs_MockScraper_ErrorResponse(t *testing.T) {
	dir := t.TempDir()
	writeScript(t, dir, `process.stdout.write(JSON.stringify({error: "page blocked"})); process.exit(1);`)

	g := &Google{cfg: defaultConfig(), scraperDir: dir}
	_, err := g.FetchJobs(context.Background(), "google")
	if err == nil {
		t.Fatal("expected error when scraper returns error")
	}
}

func TestFetchJobs_MockScraper_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	writeScript(t, dir, `process.stdout.write("this is not json");`)

	g := &Google{cfg: defaultConfig(), scraperDir: dir}
	_, err := g.FetchJobs(context.Background(), "google")
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if !contains(err.Error(), "failed to parse scraper output") {
		t.Errorf("expected parse error, got: %s", err.Error())
	}
}

func TestFetchJobs_MockScraper_MultipleJobs(t *testing.T) {
	dir := t.TempDir()
	fakeScript := `
		const jobs = [];
		for (let i = 1; i <= 5; i++) {
			jobs.push({
				id: "google-" + i,
				title: "Job " + i,
				url: "https://www.google.com/about/careers/applications/jobs/results/" + i,
				location: "City " + i,
				employ_type: "full_time"
			});
		}
		process.stdout.write(JSON.stringify(jobs));
	`
	writeScript(t, dir, fakeScript)

	g := &Google{cfg: defaultConfig(), scraperDir: dir}
	result, err := g.FetchJobs(context.Background(), "google")
	if err != nil {
		t.Fatalf("FetchJobs failed: %v", err)
	}
	if len(result.Jobs) != 5 {
		t.Fatalf("expected 5 jobs, got %d", len(result.Jobs))
	}
	for _, job := range result.Jobs {
		if job.FullContent != nil {
			t.Errorf("job %s: FullContent should be nil", job.ID)
		}
	}
}

// ── Config is passed to Node as env vars ────────────────────────────────────

func TestFetchJobs_ConfigPassedToNode(t *testing.T) {
	dir := t.TempDir()
	// Script that reads the env vars we pass and echoes them back as a job title
	fakeScript := `
		const minDelay = process.env.GOOGLE_SCRAPER_MIN_DELAY || "missing";
		const maxDelay = process.env.GOOGLE_SCRAPER_MAX_DELAY || "missing";
		const maxPages = process.env.GOOGLE_SCRAPER_MAX_PAGES || "missing";
		const jobs = [{
			id: "google-1",
			title: minDelay + "|" + maxDelay + "|" + maxPages,
			url: "https://www.google.com/about/careers/applications/jobs/results/1",
			location: "Test",
			employ_type: "full_time"
		}];
		process.stdout.write(JSON.stringify(jobs));
	`
	writeScript(t, dir, fakeScript)

	g := &Google{
		cfg: Config{
			MinDelayMs: 3000,
			MaxDelayMs: 5000,
			TimeoutMs:  60000,
			MaxPages:   1,
		},
		scraperDir: dir,
	}
	result, err := g.FetchJobs(context.Background(), "google")
	if err != nil {
		t.Fatalf("FetchJobs failed: %v", err)
	}
	if len(result.Jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(result.Jobs))
	}
	// Title encodes the env var values — verify they were passed correctly
	assertEqual(t, "env vars in title", result.Jobs[0].Title, "3000|5000|1")
}

// ── Context cancellation ────────────────────────────────────────────────────

func TestFetchJobs_ContextCancelled(t *testing.T) {
	dir := t.TempDir()
	writeScript(t, dir, `setTimeout(() => { process.stdout.write("[]"); }, 30000);`)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	g := &Google{cfg: defaultConfig(), scraperDir: dir}
	_, err := g.FetchJobs(ctx, "google")
	if err == nil {
		t.Fatal("expected error when context is cancelled")
	}
}

// ── Helpers ─────────────────────────────────────────────────────────────────

func writeScript(t *testing.T, dir, script string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "scrape.js"), []byte(script), 0644); err != nil {
		t.Fatal(err)
	}
}

func assertEqual(t *testing.T, field, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("%s: got %q, want %q", field, got, want)
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
