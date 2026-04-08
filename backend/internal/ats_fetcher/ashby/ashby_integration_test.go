package ashby

// Integration tests — hit the real Ashby public GraphQL API.
// Skipped under `go test -short` or in CI without network access.
//
// Run explicitly:
//   go test ./internal/ats_fetcher/ashby/ -run Integration -v
//   go test ./internal/ats_fetcher/ashby/ -run TestAshby_Integration -v -count=1

import (
	"context"
	"strings"
	"testing"

	"github.com/trushilpatel/jobradar/internal/ats_fetcher/types"
)

const integrationCompanySlug = "openai"
const integrationCompanyURL = "https://api.ashbyhq.com/posting-api/job-board/openai"

func TestAshby_Integration_FetchJobs(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	a := &Ashby{}
	result, err := a.FetchJobs(context.Background(), integrationCompanyURL)
	if err != nil {
		t.Fatalf("FetchJobs error: %v", err)
	}
	if len(result.Jobs) == 0 {
		t.Fatal("expected at least one job, got 0")
	}

	t.Logf("FetchJobs returned %d jobs", len(result.Jobs))

	j := result.Jobs[0]
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

	t.Logf("Sample job: id=%s title=%q location=%q employ_type=%q", j.ID, j.Title, j.Location, j.EmploymentType)
}

func TestAshby_Integration_FetchJobs_SlugVariants(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	a := &Ashby{}

	bySlug, err := a.FetchJobs(context.Background(), integrationCompanySlug)
	if err != nil {
		t.Fatalf("FetchJobs (slug) error: %v", err)
	}
	byURL, err := a.FetchJobs(context.Background(), integrationCompanyURL)
	if err != nil {
		t.Fatalf("FetchJobs (URL) error: %v", err)
	}

	if len(bySlug.Jobs) != len(byURL.Jobs) {
		t.Errorf("slug returned %d jobs, full URL returned %d — should match", len(bySlug.Jobs), len(byURL.Jobs))
	}
	t.Logf("Both forms returned %d jobs", len(bySlug.Jobs))
}

func TestAshby_Integration_FetchJobContent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	a := &Ashby{}
	result, err := a.FetchJobs(context.Background(), integrationCompanySlug)
	if err != nil {
		t.Fatalf("FetchJobs error: %v", err)
	}
	if len(result.Jobs) == 0 {
		t.Skip("no jobs returned, skipping content fetch")
	}

	item := result.Jobs[0]
	t.Logf("Fetching content for: %s — %q", item.ID, item.Title)

	raw := types.RawJob{ID: item.ID, Title: item.Title, URL: item.URL, Adapter: "ashby"}
	content, err := a.FetchJobContent(context.Background(), raw)
	if err != nil {
		t.Fatalf("FetchJobContent error: %v", err)
	}

	if content.ID == "" {
		t.Fatal("FetchJobContent returned empty ID")
	}
	if content.Title == "" {
		t.Fatal("FetchJobContent returned empty Title")
	}
	if content.Description == "" {
		t.Error("FetchJobContent returned empty Description")
	}

	t.Logf("FetchJobContent: id=%s title=%q location=%q compensation=%q desc_len=%d",
		content.ID, content.Title, content.Location, content.Compensation, len(content.Description))
}

func TestAshby_Integration_FetchJobContent_AllFields(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	a := &Ashby{}
	result, err := a.FetchJobs(context.Background(), integrationCompanySlug)
	if err != nil {
		t.Fatalf("FetchJobs error: %v", err)
	}
	if len(result.Jobs) == 0 {
		t.Skip("no jobs returned")
	}

	limit := min(3, len(result.Jobs))
	for i := range limit {
		item := result.Jobs[i]
		raw := types.RawJob{ID: item.ID, Title: item.Title, URL: item.URL, Adapter: "ashby"}
		content, err := a.FetchJobContent(context.Background(), raw)
		if err != nil {
			t.Errorf("job[%d] %s: FetchJobContent error: %v", i, item.ID, err)
			continue
		}
		if content.ID == "" {
			t.Errorf("job[%d] %s: content.ID is empty", i, item.ID)
		}
		if content.Title == "" {
			t.Errorf("job[%d] %s: content.Title is empty", i, item.ID)
		}
		if content.Description == "" {
			t.Errorf("job[%d] %s: content.Description is empty", i, item.ID)
		}
		t.Logf("job[%d]: %s — %q — desc=%d chars compensation=%q workplace=%q",
			i, content.ID, content.Title, len(content.Description), content.Compensation, content.WorkplaceType)
	}
}
