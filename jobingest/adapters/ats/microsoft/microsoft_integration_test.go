package microsoft

// Integration tests — hit the real Microsoft careers API.
// Skipped under `go test -short`.
//
// Run explicitly:
//   go test ./adapters/ats/microsoft/ -run Integration -v
//   go test ./adapters/ats/microsoft/ -run TestMicrosoft_Integration -v -count=1

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/trushilpatel/jobradar/jobingest/common/logger"
	"github.com/trushilpatel/jobradar/jobingest/core"
)

func newIntegrationAdapter() *Microsoft {
	return New(logger.NewNoop(), http.DefaultClient)
}

func TestMicrosoft_Integration_FetchJobListing(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	m := newIntegrationAdapter()
	result, err := m.FetchJobListing(context.Background(), "")
	if err != nil {
		t.Fatalf("FetchJobListing error: %v", err)
	}
	if len(result.Jobs) == 0 {
		t.Fatal("expected at least one job, got 0")
	}

	t.Logf("FetchJobListing returned %d jobs", len(result.Jobs))

	j := result.Jobs[0]
	if j.ID == "" {
		t.Error("job.ID is empty")
	}
	if !strings.HasPrefix(j.ID, "microsoft-") {
		t.Errorf("job.ID %q should start with microsoft-", j.ID)
	}
	if j.Title == "" {
		t.Error("job.Title is empty")
	}
	if j.URL == "" {
		t.Error("job.URL is empty")
	}
	if !j.LocationKnown {
		t.Error("job.LocationKnown should be true")
	}

	t.Logf("Sample job: id=%s title=%q location=%q", j.ID, j.Title, j.Location)
}

func TestMicrosoft_Integration_FetchJobPost(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	m := newIntegrationAdapter()
	result, err := m.FetchJobListing(context.Background(), "")
	if err != nil {
		t.Fatalf("FetchJobListing error: %v", err)
	}
	if len(result.Jobs) == 0 {
		t.Skip("no jobs returned, skipping post fetch")
	}

	item := result.Jobs[0]
	t.Logf("Fetching post for: %s — %q", item.ID, item.Title)

	summary := core.JobSummary{ID: item.ID, Title: item.Title, URL: item.URL}
	post, err := m.FetchJobPost(context.Background(), summary)
	if err != nil {
		t.Fatalf("FetchJobPost error: %v", err)
	}
	if post.ID == "" {
		t.Fatal("post.ID is empty")
	}
	if post.Title == "" {
		t.Fatal("post.Title is empty")
	}
	if post.Description == "" {
		t.Error("post.Description is empty — HTML should have been stripped to non-empty text")
	}

	t.Logf("FetchJobPost: id=%s title=%q location=%q desc_len=%d dept=%q employ=%q",
		post.ID, post.Title, post.Location, len(post.Description), post.Department, post.EmploymentType)
}

func TestMicrosoft_Integration_FetchJobPost_AllFields(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	m := newIntegrationAdapter()
	result, err := m.FetchJobListing(context.Background(), "")
	if err != nil {
		t.Fatalf("FetchJobListing error: %v", err)
	}
	if len(result.Jobs) == 0 {
		t.Skip("no jobs returned")
	}

	limit := min(3, len(result.Jobs))
	for i := range limit {
		item := result.Jobs[i]
		summary := core.JobSummary{ID: item.ID, Title: item.Title, URL: item.URL}
		post, err := m.FetchJobPost(context.Background(), summary)
		if err != nil {
			t.Errorf("job[%d] %s: FetchJobPost error: %v", i, item.ID, err)
			continue
		}
		if post.ID == "" {
			t.Errorf("job[%d] %s: post.ID is empty", i, item.ID)
		}
		if post.Title == "" {
			t.Errorf("job[%d] %s: post.Title is empty", i, item.ID)
		}
		if post.Description == "" {
			t.Errorf("job[%d] %s: post.Description is empty", i, item.ID)
		}
		t.Logf("job[%d]: %s — %q — desc=%d chars dept=%q employ=%q",
			i, post.ID, post.Title, len(post.Description), post.Department, post.EmploymentType)
	}
}
