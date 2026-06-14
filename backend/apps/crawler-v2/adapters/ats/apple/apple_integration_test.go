//go:build integration

package apple

// Integration tests — hit the real jobs.apple.com API.
// Skipped under `go test -short`.
//
// Run explicitly:
//   go test ./adapters/ats/apple/ -run Integration -v
//   go test ./adapters/ats/apple/ -run TestApple_Integration -v -count=1

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/trushilpatel/jobradar/apps/crawler-v2/common/logger"
	"github.com/trushilpatel/jobradar/common/ats/core"
)

func newIntegrationAdapter() *Apple {
	return New(logger.NewNoop(), http.DefaultClient)
}

func TestApple_Integration_FetchJobListing(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	a := newIntegrationAdapter()
	result, err := a.FetchJobListing(context.Background(), "ignored")
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
	if !strings.HasPrefix(j.ID, "apple-") {
		t.Errorf("job.ID %q should start with apple-", j.ID)
	}
	if j.Title == "" {
		t.Error("job.Title is empty")
	}
	if j.URL == "" {
		t.Error("job.URL is empty")
	}
	if !strings.Contains(j.URL, "jobs.apple.com") {
		t.Errorf("job.URL %q should contain jobs.apple.com", j.URL)
	}

	t.Logf("Sample job: id=%s title=%q location=%q employ=%q locationKnown=%v",
		j.ID, j.Title, j.Location, j.EmployType, j.LocationKnown)
}

func TestApple_Integration_FetchJobPost(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	a := newIntegrationAdapter()
	result, err := a.FetchJobListing(context.Background(), "ignored")
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

	t.Logf("FetchJobPost: id=%s title=%q location=%q country=%q desc_len=%d comp=%q",
		post.ID, post.Title, post.Location, post.CountryCode, len(post.Description), post.Compensation)
}
