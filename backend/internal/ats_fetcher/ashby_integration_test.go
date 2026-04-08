package ats_fetcher

// Integration tests — hit the real Ashby public GraphQL API.
// Skipped under `go test -short` or in CI without network access.
//
// Run explicitly:
//   go test ./internal/adapter/ -run Integration -v
//   go test ./internal/adapter/ -run Integration -v -count=1

import (
	"context"
	"strings"
	"testing"
)

const integrationCompanySlug = "openai"
const integrationCompanyURL = "https://api.ashbyhq.com/posting-api/job-board/openai"

func TestAshby_Integration_FetchJobs(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	a := &Ashby{}
	jobs, err := a.FetchJobs(context.Background(), integrationCompanyURL)
	if err != nil {
		t.Fatalf("FetchJobs error: %v", err)
	}

	if len(jobs) == 0 {
		t.Fatal("expected at least one job, got 0")
	}

	t.Logf("FetchJobs returned %d jobs", len(jobs))

	// Spot-check first job has all required fields populated
	j := jobs[0]
	if j.ID == "" {
		t.Error("job.ID is empty")
	}
	if !strings.HasPrefix(j.ID, "ashby-") {
		t.Errorf("job.ID %q should start with ashby-", j.ID)
	}
	if j.Title == "" {
		t.Error("job.Title is empty")
	}
	if j.URL == "" {
		t.Error("job.URL is empty")
	}
	if !strings.HasPrefix(j.URL, "https://jobs.ashbyhq.com/openai/") {
		t.Errorf("job.URL %q should start with https://jobs.ashbyhq.com/openai/", j.URL)
	}
	if j.Adapter != "ashby" {
		t.Errorf("job.Adapter = %q, want ashby", j.Adapter)
	}

	t.Logf("Sample job: id=%s title=%q location=%q employ_type=%q", j.ID, j.Title, j.Location, j.EmployType)
}

func TestAshby_Integration_FetchJobs_SlugVariants(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	a := &Ashby{}

	// Both forms should return the same jobs
	bySlug, err := a.FetchJobs(context.Background(), integrationCompanySlug)
	if err != nil {
		t.Fatalf("FetchJobs (slug) error: %v", err)
	}
	byURL, err := a.FetchJobs(context.Background(), integrationCompanyURL)
	if err != nil {
		t.Fatalf("FetchJobs (URL) error: %v", err)
	}

	if len(bySlug) != len(byURL) {
		t.Errorf("slug returned %d jobs, full URL returned %d — should match", len(bySlug), len(byURL))
	}
	t.Logf("Both forms returned %d jobs", len(bySlug))
}

func TestAshby_Integration_FetchJobContent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	a := &Ashby{}

	// First fetch the list to get a real job ID
	jobs, err := a.FetchJobs(context.Background(), integrationCompanySlug)
	if err != nil {
		t.Fatalf("FetchJobs error: %v", err)
	}
	if len(jobs) == 0 {
		t.Skip("no jobs returned, skipping content fetch")
	}

	job := jobs[0]
	t.Logf("Fetching content for: %s — %q", job.ID, job.Title)

	content, err := a.FetchJobContent(context.Background(), job)
	if err != nil {
		t.Fatalf("FetchJobContent error: %v", err)
	}

	if content == "" {
		t.Fatal("FetchJobContent returned empty content")
	}
	if !strings.Contains(content, job.ID[len("ashby-"):]) {
		t.Errorf("content does not contain job UUID %q", job.ID)
	}
	// Should be valid JSON with a title field
	if !strings.Contains(content, `"title"`) {
		t.Error("content does not contain title field")
	}
	if !strings.Contains(content, `"descriptionHtml"`) {
		t.Error("content does not contain descriptionHtml field")
	}

	t.Logf("FetchJobContent returned %d chars of JSON", len(content))
	if len(content) > 200 {
		t.Logf("Preview: %s...", content[:200])
	}
}

func TestAshby_Integration_FetchJobContent_AllFields(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	a := &Ashby{}
	jobs, err := a.FetchJobs(context.Background(), integrationCompanySlug)
	if err != nil {
		t.Fatalf("FetchJobs error: %v", err)
	}
	if len(jobs) == 0 {
		t.Skip("no jobs returned")
	}

	// Check a few jobs to verify field coverage
	limit := 3
	if len(jobs) < limit {
		limit = len(jobs)
	}

	for i := 0; i < limit; i++ {
		job := jobs[i]
		content, err := a.FetchJobContent(context.Background(), job)
		if err != nil {
			t.Errorf("job[%d] %s: FetchJobContent error: %v", i, job.ID, err)
			continue
		}
		expectedFields := []string{"id", "title", "locationName", "descriptionHtml", "compensationTierSummary", "publishedDate"}
		for _, field := range expectedFields {
			if !strings.Contains(content, `"`+field+`"`) {
				t.Errorf("job[%d] %s: content missing field %q", i, job.ID, field)
			}
		}
		t.Logf("job[%d]: %s — %q — %d chars", i, job.ID, job.Title, len(content))
	}
}
