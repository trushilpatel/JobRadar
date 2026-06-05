package netflix

// Integration tests — hit the real Netflix Eightfold API.
// Skipped under `go test -short`.
//
// Run explicitly:
//   go test ./adapters/ats/netflix/ -run Integration -v
//   go test ./adapters/ats/netflix/ -run TestNetflix_Integration -v -count=1

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/trushilpatel/jobradar/jobingest/common/logger"
	"github.com/trushilpatel/jobradar/jobingest/core"
)

func newIntegrationAdapter() *Netflix {
	return New(logger.NewNoop(), http.DefaultClient)
}

func TestNetflix_Integration_FetchJobListing(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	n := newIntegrationAdapter()
	result, err := n.FetchJobListing(context.Background(), "")
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
	if !strings.HasPrefix(j.ID, "netflix-") {
		t.Errorf("job.ID %q should start with netflix-", j.ID)
	}
	if j.Title == "" {
		t.Error("job.Title is empty")
	}
	if j.URL == "" {
		t.Error("job.URL is empty")
	}
	if j.EmployType != core.EmployFullTime {
		t.Errorf("job.EmployType = %q, want %q", j.EmployType, core.EmployFullTime)
	}
	if !j.LocationKnown {
		t.Error("job.LocationKnown should be true")
	}

	t.Logf("Sample job: id=%s title=%q location=%q", j.ID, j.Title, j.Location)
}

func TestNetflix_Integration_FetchJobPost(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	n := newIntegrationAdapter()
	result, err := n.FetchJobListing(context.Background(), "")
	if err != nil {
		t.Fatalf("FetchJobListing error: %v", err)
	}
	if len(result.Jobs) == 0 {
		t.Skip("no jobs returned, skipping post fetch")
	}

	item := result.Jobs[0]
	t.Logf("Fetching post for: %s — %q", item.ID, item.Title)

	summary := core.JobSummary{ID: item.ID, Title: item.Title, URL: item.URL}
	post, err := n.FetchJobPost(context.Background(), summary)
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
	// Verify HTML was stripped
	if strings.Contains(post.Description, "<") {
		t.Error("post.Description should have HTML stripped")
	}

	t.Logf("FetchJobPost: id=%s title=%q location=%q desc_len=%d dept=%q workplace=%q",
		post.ID, post.Title, post.Location, len(post.Description), post.Department, post.WorkplaceType)
}
