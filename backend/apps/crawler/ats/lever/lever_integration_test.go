package lever

// Integration tests — hit the real Lever public API.
// Skipped under `go test -short` or in CI without network access.
//
// Run explicitly:
//   go test ./pipeline/ats/lever/ -run Integration -v
//   go test ./pipeline/ats/lever/ -run TestLever_Integration -v -count=1

import (
	"context"
	"strings"
	"testing"

	"github.com/trushilpatel/jobradar/apps/crawler/ats/types"
)

// Plaid uses Lever and consistently has open engineering roles.
const leverIntegrationCompanySlug = "plaid"

func TestLever_Integration_FetchJobs(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	l := &Lever{}
	result, err := l.FetchJobs(context.Background(), leverIntegrationCompanySlug)
	if err != nil {
		t.Fatalf("FetchJobs error: %v", err)
	}
	if len(result.Jobs) == 0 {
		t.Skip("no jobs returned — company may have no open roles right now")
	}

	t.Logf("FetchJobs returned %d jobs", len(result.Jobs))

	j := result.Jobs[0]
	if j.ID == "" {
		t.Error("job.ID is empty")
	}
	if !strings.HasPrefix(j.ID, "lever-") {
		t.Errorf("job.ID %q should start with lever-", j.ID)
	}
	if j.Title == "" {
		t.Error("job.Title is empty")
	}
	if j.URL == "" {
		t.Error("job.URL is empty")
	}
	if !strings.Contains(j.URL, "jobs.lever.co/"+leverIntegrationCompanySlug) {
		t.Errorf("job.URL %q should contain jobs.lever.co/%s", j.URL, leverIntegrationCompanySlug)
	}

	t.Logf("Sample job: id=%s title=%q location=%q", j.ID, j.Title, j.Location)
}

func TestLever_Integration_FetchJobContent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	l := &Lever{}
	result, err := l.FetchJobs(context.Background(), leverIntegrationCompanySlug)
	if err != nil {
		t.Fatalf("FetchJobs error: %v", err)
	}
	if len(result.Jobs) == 0 {
		t.Skip("no jobs returned, skipping content fetch")
	}

	item := result.Jobs[0]
	t.Logf("Fetching content for: %s — %q", item.ID, item.Title)

	raw := types.RawJob{ID: item.ID, Title: item.Title, URL: item.URL, Adapter: "lever"}
	content, err := l.FetchJobContent(context.Background(), raw)
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

	t.Logf("FetchJobContent: id=%s title=%q location=%q dept=%q employ_type=%q desc_len=%d",
		content.ID, content.Title, content.Location, content.Department, content.EmploymentType, len(content.Description))
}

func TestLever_Integration_FetchJobContent_AllFields(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	l := &Lever{}
	result, err := l.FetchJobs(context.Background(), leverIntegrationCompanySlug)
	if err != nil {
		t.Fatalf("FetchJobs error: %v", err)
	}
	if len(result.Jobs) == 0 {
		t.Skip("no jobs returned")
	}

	limit := min(3, len(result.Jobs))
	for i := range limit {
		item := result.Jobs[i]
		raw := types.RawJob{ID: item.ID, Title: item.Title, URL: item.URL, Adapter: "lever"}
		content, err := l.FetchJobContent(context.Background(), raw)
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
		t.Logf("job[%d]: %s — %q — desc=%d chars dept=%q employ_type=%q",
			i, content.ID, content.Title, len(content.Description), content.Department, content.EmploymentType)
	}
}
