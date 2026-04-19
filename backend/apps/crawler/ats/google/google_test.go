package google

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/trushilpatel/jobradar/apps/crawler/ats/types"
	"github.com/trushilpatel/jobradar/common/logger"
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
		MinDelayMs:  5000,
		MaxDelayMs:  8000,
		TimeoutMs:   60000,
		MaxPages:    2,
		EnrichMinMs: 3000,
		EnrichMaxMs: 5000,
	}
	g := New(cfg, logger.NewNoop())
	if g.cfg.MinDelayMs != 5000 {
		t.Errorf("MinDelayMs: got %d, want 5000", g.cfg.MinDelayMs)
	}
	if g.cfg.MaxDelayMs != 8000 {
		t.Errorf("MaxDelayMs: got %d, want 8000", g.cfg.MaxDelayMs)
	}
	if g.cfg.MaxPages != 2 {
		t.Errorf("MaxPages: got %d, want 2", g.cfg.MaxPages)
	}
	if g.cfg.EnrichMinMs != 3000 {
		t.Errorf("EnrichMinMs: got %d, want 3000", g.cfg.EnrichMinMs)
	}
	if g.cfg.EnrichMaxMs != 5000 {
		t.Errorf("EnrichMaxMs: got %d, want 5000", g.cfg.EnrichMaxMs)
	}
}

func TestNewDefault_UsesProductionDefaults(t *testing.T) {
	g := NewDefault()
	if g.cfg.MinDelayMs != 120000 {
		t.Errorf("MinDelayMs: got %d, want 120000", g.cfg.MinDelayMs)
	}
	if g.cfg.MaxDelayMs != 240000 {
		t.Errorf("MaxDelayMs: got %d, want 240000", g.cfg.MaxDelayMs)
	}
	if g.cfg.TimeoutMs != 14400000 {
		t.Errorf("TimeoutMs: got %d, want 14400000", g.cfg.TimeoutMs)
	}
	if g.cfg.MaxPages != 0 {
		t.Errorf("MaxPages: got %d, want 0 (no limit)", g.cfg.MaxPages)
	}
	if g.cfg.EnrichMinMs != 60000 {
		t.Errorf("EnrichMinMs: got %d, want 60000", g.cfg.EnrichMinMs)
	}
	if g.cfg.EnrichMaxMs != 120000 {
		t.Errorf("EnrichMaxMs: got %d, want 120000", g.cfg.EnrichMaxMs)
	}
}

// ── Interface compliance ────────────────────────────────────────────────────

func TestGoogleImplementsAdapter(t *testing.T) {
	var _ types.Adapter = (*Google)(nil)
}

func TestGoogleImplementsURLProvider(t *testing.T) {
	var _ types.URLProvider = (*Google)(nil)
}

// Google uses the FullContent fast path — it does NOT implement ContentFetcher.
// The Playwright scraper populates FullContent at scan time.
func TestGoogleDoesNotImplementContentFetcher(t *testing.T) {
	var adp types.Adapter = NewDefault()
	if _, ok := adp.(types.ContentFetcher); ok {
		t.Error("Google should NOT implement ContentFetcher — it is a full-detail adapter; FullContent is set by the scraper")
	}
}

// ── JSON parsing — full scraper output ─────────────────────────────────────

func TestParseScraperOutput_ValidJobs(t *testing.T) {
	sampleOutput := `[
		{
			"id": "google-123456",
			"title": "Software Engineer, Backend",
			"url": "https://www.google.com/about/careers/applications/jobs/results/123456",
			"location": "Mountain View, CA",
			"employ_type": "full_time",
			"full_content": {
				"description": "Software Engineer, Backend\n\nMountain View, CA\n\nMinimum qualifications:\n- BS in CS",
				"requirements": "",
				"compensation": "",
				"workplace_type": "",
				"department": "Google",
				"published_date": ""
			}
		},
		{
			"id": "google-789012",
			"title": "Product Manager, Cloud",
			"url": "https://www.google.com/about/careers/applications/jobs/results/789012",
			"location": "New York, NY",
			"employ_type": "full_time",
			"full_content": {
				"description": "Product Manager, Cloud\n\nNew York, NY\n\nResponsibilities\n- Lead the product vision",
				"requirements": "",
				"compensation": "",
				"workplace_type": "",
				"department": "Google Cloud",
				"published_date": ""
			}
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
	if j.FullContent == nil {
		t.Fatal("FullContent should not be nil")
	}
	if j.FullContent.Department != "Google" {
		t.Errorf("Department: got %q, want %q", j.FullContent.Department, "Google")
	}
	if !contains(j.FullContent.Description, "Minimum qualifications") {
		t.Error("Description should contain 'Minimum qualifications'")
	}

	j2 := scraperJobs[1]
	assertEqual(t, "id", j2.ID, "google-789012")
	if j2.FullContent == nil {
		t.Fatal("FullContent should not be nil for second job")
	}
	assertEqual(t, "department", j2.FullContent.Department, "Google Cloud")
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

// Full-detail adapter: FullContent must be set when the scraper outputs full_content.
func TestConvertToPipelineTypes_FullContentSet(t *testing.T) {
	sj := scraperJob{
		ID:         "google-42",
		Title:      "SRE",
		URL:        "https://www.google.com/about/careers/applications/jobs/results/42",
		Location:   "London",
		EmployType: "full_time",
		FullContent: &scraperFullContent{
			Description: "SRE\n\nLondon\n\nMinimum qualifications:\n- 2 years experience",
			Department:  "Google",
		},
	}

	var fc *types.FullJobContent
	if sj.FullContent != nil {
		fc = &types.FullJobContent{
			Description:   sj.FullContent.Description,
			Requirements:  sj.FullContent.Requirements,
			Compensation:  sj.FullContent.Compensation,
			WorkplaceType: sj.FullContent.WorkplaceType,
			Department:    sj.FullContent.Department,
			PublishedDate: sj.FullContent.PublishedDate,
		}
	}

	item := types.FetchJobsItem{
		ID:             sj.ID,
		Title:          sj.Title,
		URL:            sj.URL,
		Location:       sj.Location,
		EmploymentType: sj.EmployType,
		FullContent:    fc,
	}

	assertEqual(t, "ID", item.ID, "google-42")
	assertEqual(t, "Title", item.Title, "SRE")
	assertEqual(t, "EmploymentType", item.EmploymentType, "full_time")

	if item.FullContent == nil {
		t.Fatal("FullContent must not be nil — Google is a full-detail adapter")
	}
	if item.FullContent.Department != "Google" {
		t.Errorf("Department: got %q, want %q", item.FullContent.Department, "Google")
	}
	if !contains(item.FullContent.Description, "Minimum qualifications") {
		t.Error("Description should contain section content")
	}
}

// Jobs where the scraper timed out before visiting the detail page have nil
// FullContent — the analyzer falls back to HTTP fetch for those.
func TestConvertToPipelineTypes_NilFullContentWhenScraperTimedOut(t *testing.T) {
	sj := scraperJob{
		ID:          "google-99",
		Title:       "Engineer",
		URL:         "https://www.google.com/about/careers/applications/jobs/results/99",
		Location:    "Remote",
		EmployType:  "full_time",
		FullContent: nil, // scraper timed out before visiting this job
	}

	var fc *types.FullJobContent
	if sj.FullContent != nil {
		fc = &types.FullJobContent{}
	}

	item := types.FetchJobsItem{FullContent: fc}
	if item.FullContent != nil {
		t.Error("FullContent must be nil when scraper timed out — analyzer will fall back to HTTP fetch")
	}
}

// ── FetchJobs — scraper not found ───────────────────────────────────────────

func TestFetchJobs_ScraperNotFound(t *testing.T) {
	g := &Google{cfg: defaultConfig(), log: logger.NewNoop(), scraperDir: "/nonexistent/path"}
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
	fakeScript := `
		const jobs = [
			{
				"id": "google-111",
				"title": "Test Engineer",
				"url": "https://www.google.com/about/careers/applications/jobs/results/111",
				"location": "Remote",
				"employ_type": "full_time",
				"full_content": {
					"description": "Test Engineer\n\nRemote\n\nMinimum qualifications:\n- 1 year experience",
					"requirements": "",
					"compensation": "",
					"workplace_type": "",
					"department": "Google",
					"published_date": ""
				}
			}
		];
		process.stdout.write(JSON.stringify(jobs));
	`
	writeScript(t, dir, fakeScript)

	g := &Google{cfg: defaultConfig(), log: logger.NewNoop(), scraperDir: dir}
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

	if job.FullContent == nil {
		t.Fatal("FullContent must not be nil — Google is a full-detail adapter")
	}
	assertEqual(t, "Department", job.FullContent.Department, "Google")
	if !contains(job.FullContent.Description, "Minimum qualifications") {
		t.Error("Description should contain section content")
	}
}

func TestFetchJobs_MockScraper_EmptyResult(t *testing.T) {
	dir := t.TempDir()
	writeScript(t, dir, `process.stdout.write("[]");`)

	g := &Google{cfg: defaultConfig(), log: logger.NewNoop(), scraperDir: dir}
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

	g := &Google{cfg: defaultConfig(), log: logger.NewNoop(), scraperDir: dir}
	_, err := g.FetchJobs(context.Background(), "google")
	if err == nil {
		t.Fatal("expected error when scraper returns error")
	}
}

func TestFetchJobs_MockScraper_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	writeScript(t, dir, `process.stdout.write("this is not json");`)

	g := &Google{cfg: defaultConfig(), log: logger.NewNoop(), scraperDir: dir}
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
				employ_type: "full_time",
				full_content: {
					description: "Job " + i + "\n\nCity " + i + "\n\nMinimum qualifications:\n- Req " + i,
					requirements: "",
					compensation: "",
					workplace_type: "",
					department: "Dept " + i,
					published_date: ""
				}
			});
		}
		process.stdout.write(JSON.stringify(jobs));
	`
	writeScript(t, dir, fakeScript)

	g := &Google{cfg: defaultConfig(), log: logger.NewNoop(), scraperDir: dir}
	result, err := g.FetchJobs(context.Background(), "google")
	if err != nil {
		t.Fatalf("FetchJobs failed: %v", err)
	}
	if len(result.Jobs) != 5 {
		t.Fatalf("expected 5 jobs, got %d", len(result.Jobs))
	}
	for _, job := range result.Jobs {
		if job.FullContent == nil {
			t.Errorf("job %s: FullContent must not be nil — full-detail adapter", job.ID)
		}
	}
}

// Jobs with missing full_content (e.g. scraper timed out mid-run) gracefully
// produce FullContent == nil — the analyzer falls back to HTTP fetch for those.
func TestFetchJobs_MockScraper_MissingFullContent(t *testing.T) {
	dir := t.TempDir()
	fakeScript := `
		process.stdout.write(JSON.stringify([{
			"id": "google-999",
			"title": "Partial Job",
			"url": "https://www.google.com/about/careers/applications/jobs/results/999",
			"location": "Unknown",
			"employ_type": "full_time"
		}]));
	`
	writeScript(t, dir, fakeScript)

	g := &Google{cfg: defaultConfig(), log: logger.NewNoop(), scraperDir: dir}
	result, err := g.FetchJobs(context.Background(), "google")
	if err != nil {
		t.Fatalf("FetchJobs failed: %v", err)
	}
	if len(result.Jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(result.Jobs))
	}
	if result.Jobs[0].FullContent != nil {
		t.Error("FullContent must be nil when scraper did not output full_content (timeout case)")
	}
}

// ── Config is passed to Node as env vars ────────────────────────────────────

func TestFetchJobs_ConfigPassedToNode(t *testing.T) {
	dir := t.TempDir()
	// Script echoes env vars back as the job title to verify they were set
	fakeScript := `
		const minDelay = process.env.GOOGLE_SCRAPER_MIN_DELAY || "missing";
		const maxDelay = process.env.GOOGLE_SCRAPER_MAX_DELAY || "missing";
		const maxPages = process.env.GOOGLE_SCRAPER_MAX_PAGES || "missing";
		const enrichMin = process.env.GOOGLE_SCRAPER_ENRICH_MIN || "missing";
		const enrichMax = process.env.GOOGLE_SCRAPER_ENRICH_MAX || "missing";
		const jobs = [{
			id: "google-1",
			title: minDelay + "|" + maxDelay + "|" + maxPages + "|" + enrichMin + "|" + enrichMax,
			url: "https://www.google.com/about/careers/applications/jobs/results/1",
			location: "Test",
			employ_type: "full_time",
			full_content: { description: "test", requirements: "", compensation: "", workplace_type: "", department: "G", published_date: "" }
		}];
		process.stdout.write(JSON.stringify(jobs));
	`
	writeScript(t, dir, fakeScript)

	g := &Google{
		cfg: Config{
			MinDelayMs:  3000,
			MaxDelayMs:  5000,
			TimeoutMs:   60000,
			MaxPages:    1,
			EnrichMinMs: 1000,
			EnrichMaxMs: 2000,
		},
		log:        logger.NewNoop(),
		scraperDir: dir,
	}
	result, err := g.FetchJobs(context.Background(), "google")
	if err != nil {
		t.Fatalf("FetchJobs failed: %v", err)
	}
	if len(result.Jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(result.Jobs))
	}
	// Title encodes all env var values — verify all were passed correctly
	assertEqual(t, "env vars in title", result.Jobs[0].Title, "3000|5000|1|1000|2000")
}

// ── Context cancellation ────────────────────────────────────────────────────

func TestFetchJobs_ContextCancelled(t *testing.T) {
	dir := t.TempDir()
	writeScript(t, dir, `setTimeout(() => { process.stdout.write("[]"); }, 30000);`)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	g := &Google{cfg: defaultConfig(), log: logger.NewNoop(), scraperDir: dir}
	_, err := g.FetchJobs(ctx, "google")
	if err == nil {
		t.Fatal("expected error when context is cancelled")
	}
}

// ── resolveScraperDir ────────────────────────────────────────────────────────

func TestResolveScraperDir_ExplicitOverride(t *testing.T) {
	g := &Google{log: logger.NewNoop(), scraperDir: "/explicit/path"}
	if got := g.resolveScraperDir(); got != "/explicit/path" {
		t.Errorf("resolveScraperDir() = %q, want /explicit/path", got)
	}
}

func TestResolveScraperDir_EnvVar(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GOOGLE_SCRAPER_DIR", dir)

	g := &Google{log: logger.NewNoop()}
	if got := g.resolveScraperDir(); got != dir {
		t.Errorf("resolveScraperDir() = %q, want %q (from GOOGLE_SCRAPER_DIR)", got, dir)
	}
}

func TestResolveScraperDir_ExplicitTakesPrecedenceOverEnv(t *testing.T) {
	t.Setenv("GOOGLE_SCRAPER_DIR", "/from-env")
	g := &Google{log: logger.NewNoop(), scraperDir: "/explicit"}
	if got := g.resolveScraperDir(); got != "/explicit" {
		t.Errorf("expected explicit override to win, got %q", got)
	}
}

// ── truncate / itoa helpers ──────────────────────────────────────────────────

func TestTruncate_ShortString(t *testing.T) {
	if got := truncate("hello", 10); got != "hello" {
		t.Errorf("truncate(short) = %q, want %q", got, "hello")
	}
}

func TestTruncate_ExactLength(t *testing.T) {
	if got := truncate("hello", 5); got != "hello" {
		t.Errorf("truncate(exact) = %q, want %q", got, "hello")
	}
}

func TestTruncate_Overflow(t *testing.T) {
	got := truncate("hello world", 5)
	want := "hello..."
	if got != want {
		t.Errorf("truncate(overflow) = %q, want %q", got, want)
	}
}

func TestItoa(t *testing.T) {
	cases := []struct {
		n    int
		want string
	}{{0, "0"}, {42, "42"}, {-1, "-1"}, {30000, "30000"}}
	for _, tc := range cases {
		if got := itoa(tc.n); got != tc.want {
			t.Errorf("itoa(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}

// ── FetchJobs edge cases ─────────────────────────────────────────────────────

// Scraper exits 0 but writes {"error": "..."} — should be caught as an error.
func TestFetchJobs_MockScraper_ErrorInBodyExitZero(t *testing.T) {
	dir := t.TempDir()
	writeScript(t, dir, `process.stdout.write(JSON.stringify({error: "blocked by captcha"}));`)

	g := &Google{cfg: defaultConfig(), log: logger.NewNoop(), scraperDir: dir}
	_, err := g.FetchJobs(context.Background(), "google")
	if err == nil {
		t.Fatal("expected error for error-body response with exit 0")
	}
	if !contains(err.Error(), "blocked by captcha") {
		t.Errorf("expected error message in output, got: %s", err.Error())
	}
}

// Scraper exits 0 but writes nothing to stdout — should fail to parse.
func TestFetchJobs_MockScraper_EmptyStdout(t *testing.T) {
	dir := t.TempDir()
	writeScript(t, dir, `// writes nothing`)

	g := &Google{cfg: defaultConfig(), log: logger.NewNoop(), scraperDir: dir}
	_, err := g.FetchJobs(context.Background(), "google")
	if err == nil {
		t.Fatal("expected error for empty scraper stdout")
	}
}

// Scraper output has extra unknown fields — forward-compat, should not break.
func TestFetchJobs_MockScraper_ExtraFields(t *testing.T) {
	dir := t.TempDir()
	fakeScript := `
		process.stdout.write(JSON.stringify([{
			"id": "google-99",
			"title": "SWE",
			"url": "https://www.google.com/about/careers/applications/jobs/results/99",
			"location": "Remote",
			"employ_type": "full_time",
			"full_content": {
				"description": "SWE\n\nRemote\n\nMinimum qualifications:\n- Req",
				"requirements": "",
				"compensation": "",
				"workplace_type": "",
				"department": "Google",
				"published_date": ""
			},
			"unknown_future_field": "ignored",
			"another_field": 42
		}]));
	`
	writeScript(t, dir, fakeScript)

	g := &Google{cfg: defaultConfig(), log: logger.NewNoop(), scraperDir: dir}
	result, err := g.FetchJobs(context.Background(), "google")
	if err != nil {
		t.Fatalf("FetchJobs failed with extra fields: %v", err)
	}
	if len(result.Jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(result.Jobs))
	}
	assertEqual(t, "ID", result.Jobs[0].ID, "google-99")
}

// Employment type strings from the scraper are passed through as-is.
func TestFetchJobs_MockScraper_EmploymentTypes(t *testing.T) {
	cases := []struct{ raw, want string }{
		{"full_time", "full_time"},
		{"part_time", "part_time"},
		{"contract", "contract"},
		{"internship", "internship"},
		{"", ""},
	}

	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			dir := t.TempDir()
			script := `process.stdout.write(JSON.stringify([{
				"id": "google-1", "title": "T", "url": "https://example.com", "location": "L",
				"employ_type": "` + tc.raw + `",
				"full_content": {"description":"d","requirements":"","compensation":"","workplace_type":"","department":"G","published_date":""}
			}]));`
			writeScript(t, dir, script)

			g := &Google{cfg: defaultConfig(), log: logger.NewNoop(), scraperDir: dir}
			result, err := g.FetchJobs(context.Background(), "google")
			if err != nil {
				t.Fatalf("FetchJobs failed: %v", err)
			}
			if len(result.Jobs) != 1 {
				t.Fatalf("expected 1 job, got %d", len(result.Jobs))
			}
			if result.Jobs[0].EmploymentType != tc.want {
				t.Errorf("EmploymentType = %q, want %q", result.Jobs[0].EmploymentType, tc.want)
			}
		})
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
