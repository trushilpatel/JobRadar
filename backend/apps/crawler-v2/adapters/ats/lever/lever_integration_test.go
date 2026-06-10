//go:build integration

package lever

// Integration tests — hit the real Lever public API.
// Skipped under `go test -short`.
//
// Run explicitly:
//   go test ./adapters/ats/lever/ -run Integration -v
//   go test ./adapters/ats/lever/ -run TestLever_Integration -v -count=1

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/trushilpatel/jobradar/apps/crawler-v2/common/logger"
	"github.com/trushilpatel/jobradar/apps/crawler-v2/core"
)

// netlify uses Lever for their careers page and is a well-known Lever customer.
const leverIntegrationSlug = "netlify"

func newIntegrationAdapter() *Lever {
	return New(logger.NewNoop(), http.DefaultClient)
}

func TestLever_Integration_FetchJobListing(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	l := newIntegrationAdapter()
	result, err := l.FetchJobListing(context.Background(), leverIntegrationSlug)
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
	if !strings.HasPrefix(j.ID, "lever-") {
		t.Errorf("job.ID %q should start with lever-", j.ID)
	}
	if j.Title == "" {
		t.Error("job.Title is empty")
	}
	if j.URL == "" {
		t.Error("job.URL is empty")
	}
	if !strings.Contains(j.URL, leverIntegrationSlug) {
		t.Errorf("job.URL %q should contain slug %q", j.URL, leverIntegrationSlug)
	}
	if !j.LocationKnown {
		t.Error("job.LocationKnown should be true")
	}

	t.Logf("Sample job: id=%s title=%q location=%q employ_type=%q", j.ID, j.Title, j.Location, j.EmployType)
}

func TestLever_Integration_FetchJobPost(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	l := newIntegrationAdapter()
	result, err := l.FetchJobListing(context.Background(), leverIntegrationSlug)
	if err != nil {
		t.Fatalf("FetchJobListing error: %v", err)
	}
	if len(result.Jobs) == 0 {
		t.Skip("no jobs returned, skipping post fetch")
	}

	item := result.Jobs[0]
	t.Logf("Fetching post for: %s — %q", item.ID, item.Title)

	summary := core.JobSummary{ID: item.ID, Title: item.Title, URL: item.URL}
	post, err := l.FetchJobPost(context.Background(), summary)
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

	t.Logf("FetchJobPost: id=%s title=%q location=%q desc_len=%d dept=%q workplace=%q",
		post.ID, post.Title, post.Location, len(post.Description), post.Department, post.WorkplaceType)
}
