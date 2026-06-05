package ashby

// Integration tests — hit the real Ashby public GraphQL API.
// Skipped under `go test -short`.
//
// Run explicitly:
//   go test ./adapters/ats/ashby/ -run Integration -v
//   go test ./adapters/ats/ashby/ -run TestAshby_Integration -v -count=1

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/trushilpatel/jobradar/jobingest/common/logger"
	"github.com/trushilpatel/jobradar/jobingest/core"
)

// ashby uses its own product's hosted jobs page.
const ashbyIntegrationSlug = "ashby"

func newIntegrationAdapter() *Ashby {
	return New(logger.NewNoop(), http.DefaultClient)
}

func TestAshby_Integration_FetchJobListing(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	a := newIntegrationAdapter()
	result, err := a.FetchJobListing(context.Background(), ashbyIntegrationSlug)
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
	if !strings.HasPrefix(j.ID, "ashby-") {
		t.Errorf("job.ID %q should start with ashby-", j.ID)
	}
	if j.Title == "" {
		t.Error("job.Title is empty")
	}
	if j.URL == "" {
		t.Error("job.URL is empty")
	}
	if !strings.Contains(j.URL, ashbyIntegrationSlug) {
		t.Errorf("job.URL %q should contain slug %q", j.URL, ashbyIntegrationSlug)
	}
	if !j.LocationKnown {
		t.Error("LocationKnown should be true")
	}

	t.Logf("Sample job: id=%s title=%q location=%q", j.ID, j.Title, j.Location)
}

func TestAshby_Integration_FetchJobPost(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	a := newIntegrationAdapter()
	result, err := a.FetchJobListing(context.Background(), ashbyIntegrationSlug)
	if err != nil {
		t.Fatalf("FetchJobListing error: %v", err)
	}
	if len(result.Jobs) == 0 {
		t.Skip("no jobs returned, skipping post fetch")
	}

	item := result.Jobs[0]
	t.Logf("Fetching post for: %s — %q", item.ID, item.Title)

	summary := core.JobSummary{ID: item.ID, Title: item.Title, URL: item.URL}
	post, err := a.FetchJobPost(context.Background(), summary)
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
		t.Error("post.Description is empty")
	}

	t.Logf("FetchJobPost: id=%s title=%q location=%q desc_len=%d comp=%q",
		post.ID, post.Title, post.Location, len(post.Description), post.Compensation)
}

func TestAshby_Integration_FetchJobPost_AllFields(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	a := newIntegrationAdapter()
	result, err := a.FetchJobListing(context.Background(), ashbyIntegrationSlug)
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
		post, err := a.FetchJobPost(context.Background(), summary)
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
		t.Logf("job[%d]: %s — %q — desc=%d chars workplace=%q",
			i, post.ID, post.Title, len(post.Description), post.WorkplaceType)
	}
}
