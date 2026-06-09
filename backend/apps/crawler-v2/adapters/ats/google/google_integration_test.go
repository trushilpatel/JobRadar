//go:build integration

package google

// Integration tests — hit the real Google Careers page.
// Skipped under `go test -short`.
//
// Run explicitly:
//   go test ./adapters/ats/google/ -run Integration -v
//   go test ./adapters/ats/google/ -run TestGoogle_Integration -v -count=1

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/trushilpatel/jobradar/apps/crawler-v2/common/logger"
)

func newIntegrationAdapter() *Google {
	g := New(logger.NewNoop(), &http.Client{Timeout: 15 * time.Second})
	// Use a very short pacing to avoid long pauses in tests.
	g.minDelayMs = 2000
	g.maxDelayMs = 4000
	return g
}

func TestGoogle_Integration_FetchJobListing(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Google has rate limiting — use a short context timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	g := newIntegrationAdapter()
	result, err := g.FetchJobListing(ctx, "ignored")
	if err != nil {
		t.Fatalf("FetchJobListing error: %v", err)
	}
	if len(result.Jobs) == 0 {
		t.Skip("no jobs returned from live Google Careers; possible rate limiting or structural change")
	}

	t.Logf("FetchJobListing returned %d jobs", len(result.Jobs))

	j := result.Jobs[0]
	if j.ID == "" {
		t.Error("job.ID is empty")
	}
	if !strings.HasPrefix(j.ID, "google-") {
		t.Errorf("job.ID %q should start with google-", j.ID)
	}
	if j.Title == "" {
		t.Error("job.Title is empty")
	}
	if j.URL == "" {
		t.Error("job.URL is empty")
	}
	if !strings.Contains(j.URL, "careers/applications/jobs") {
		t.Errorf("job.URL %q should contain careers/applications/jobs", j.URL)
	}
	if j.EmployType != "full_time" {
		t.Errorf("job.EmployType = %q, want full_time", j.EmployType)
	}
	if !j.LocationKnown {
		t.Error("job.LocationKnown should be true")
	}

	t.Logf("Sample job: id=%s title=%q location=%q", j.ID, j.Title, j.Location)
}

func TestGoogle_Integration_FetchJobPost(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	g := newIntegrationAdapter()
	result, err := g.FetchJobListing(ctx, "ignored")
	if err != nil {
		t.Fatalf("FetchJobListing error: %v", err)
	}
	if len(result.Jobs) == 0 {
		t.Skip("no jobs returned, skipping post fetch")
	}

	item := result.Jobs[0]
	t.Logf("Fetching post for: %s — %q", item.ID, item.Title)

	post, err := g.FetchJobPost(ctx, item)
	if err != nil {
		t.Fatalf("FetchJobPost error: %v", err)
	}
	if post.ID != item.ID {
		t.Errorf("post.ID = %q, want %q", post.ID, item.ID)
	}
	if post.Title == "" {
		t.Error("post.Title is empty")
	}
	// Description may be empty if Google's page structure changed.
	t.Logf("FetchJobPost: id=%s title=%q location=%q desc_len=%d dept=%q",
		post.ID, post.Title, post.Location, len(post.Description), post.Department)
}
