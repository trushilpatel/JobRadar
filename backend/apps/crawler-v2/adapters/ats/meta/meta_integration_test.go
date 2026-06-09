package meta

// Integration tests — hit the real Meta Careers endpoints.
// Skipped under `go test -short`.
//
// Run explicitly:
//   go test ./adapters/ats/meta/ -run Integration -v
//   go test ./adapters/ats/meta/ -run TestMeta_Integration -v -count=1

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/trushilpatel/jobradar/apps/crawler-v2/common/logger"
	"github.com/trushilpatel/jobradar/apps/crawler-v2/core"
)

func newIntegrationAdapter() *Meta {
	return New(logger.NewNoop(), &http.Client{})
}

func TestMeta_Integration_FetchJobListing(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	m := newIntegrationAdapter()
	listing, err := m.FetchJobListing(context.Background(), "")
	if err != nil {
		t.Fatalf("FetchJobListing error: %v", err)
	}
	if len(listing.Jobs) == 0 {
		t.Fatal("expected at least one job from Meta sitemap, got 0")
	}

	t.Logf("FetchJobListing returned %d jobs", len(listing.Jobs))

	j := listing.Jobs[0]
	if j.ID == "" {
		t.Error("job.ID is empty")
	}
	if !strings.HasPrefix(j.ID, "meta-") {
		t.Errorf("job.ID %q should start with meta-", j.ID)
	}
	if j.URL == "" {
		t.Error("job.URL is empty")
	}
	if !strings.Contains(j.URL, "metacareers.com") {
		t.Errorf("job.URL %q should be a metacareers.com URL", j.URL)
	}
	// Title and Location are empty at listing time — expected.
	t.Logf("Sample job: id=%s url=%s", j.ID, j.URL)
}

func TestMeta_Integration_FetchJobPost(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	m := newIntegrationAdapter()
	listing, err := m.FetchJobListing(context.Background(), "")
	if err != nil {
		t.Fatalf("FetchJobListing error: %v", err)
	}
	if len(listing.Jobs) == 0 {
		t.Skip("no jobs returned, skipping post fetch")
	}

	item := listing.Jobs[0]
	t.Logf("Fetching post for: %s", item.ID)

	summary := core.JobSummary{ID: item.ID, URL: item.URL}
	post, err := m.FetchJobPost(context.Background(), summary)
	if err != nil {
		t.Fatalf("FetchJobPost error: %v", err)
	}
	if post.ID == "" {
		t.Fatal("post.ID is empty")
	}
	if post.Title == "" {
		t.Error("post.Title is empty")
	}
	if post.Description == "" {
		t.Error("post.Description is empty")
	}
	t.Logf("FetchJobPost: id=%s title=%q location=%q country=%q desc_len=%d",
		post.ID, post.Title, post.Location, post.CountryCode, len(post.Description))
}
