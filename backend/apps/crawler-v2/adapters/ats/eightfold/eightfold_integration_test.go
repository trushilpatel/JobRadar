package eightfold

// Integration tests — hit the real Eightfold API for Netflix (a well-known
// public Eightfold board). Skipped under `go test -short`.
//
// Run explicitly:
//
//	go test ./adapters/ats/eightfold/ -run Integration -v
//	go test ./adapters/ats/eightfold/ -run TestEightfold_Integration -v -count=1

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/trushilpatel/jobradar/apps/crawler-v2/common/logger"
)

func newIntegrationAdapter() *Eightfold {
	return New(logger.NewNoop(), http.DefaultClient)
}

func TestEightfold_Integration_FetchJobListingWithConfig(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	cfg, _ := json.Marshal(adapterConfig{
		Host:           "explore.jobs.netflix.net",
		Domain:         "netflix.com",
		MaxJobsPerScan: 20, // limit to first 20 for the integration test
	})

	e := newIntegrationAdapter()
	result, err := e.FetchJobListingWithConfig(context.Background(), "netflix", cfg)
	if err != nil {
		t.Fatalf("FetchJobListingWithConfig error: %v", err)
	}
	if len(result.Jobs) == 0 {
		t.Fatal("expected at least one job, got 0")
	}

	t.Logf("FetchJobListingWithConfig returned %d jobs", len(result.Jobs))

	j := result.Jobs[0]
	if j.ID == "" {
		t.Error("job.ID is empty")
	}
	if !strings.HasPrefix(j.ID, "eightfold-") {
		t.Errorf("job.ID %q should start with eightfold-", j.ID)
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

	t.Logf("Sample job: id=%s title=%q location=%q url=%q", j.ID, j.Title, j.Location, j.URL)
}

func TestEightfold_Integration_FetchJobPost(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	cfg, _ := json.Marshal(adapterConfig{
		Host:           "explore.jobs.netflix.net",
		Domain:         "netflix.com",
		MaxJobsPerScan: 5,
	})

	e := newIntegrationAdapter()
	listing, err := e.FetchJobListingWithConfig(context.Background(), "netflix", cfg)
	if err != nil {
		t.Fatalf("FetchJobListingWithConfig error: %v", err)
	}
	if len(listing.Jobs) == 0 {
		t.Skip("no jobs returned, skipping post fetch")
	}

	item := listing.Jobs[0]
	t.Logf("Fetching post for: %s — %q url=%q", item.ID, item.Title, item.URL)

	post, err := e.FetchJobPost(context.Background(), item)
	if err != nil {
		t.Fatalf("FetchJobPost error: %v", err)
	}
	if post.ID == "" {
		t.Fatal("post.ID is empty")
	}
	// Description may or may not be populated by the API for this posting.
	if strings.Contains(post.Description, "<") {
		t.Error("post.Description should have HTML stripped")
	}

	t.Logf("FetchJobPost: id=%s dept=%q workplace=%q published=%q desc_len=%d",
		post.ID, post.Department, post.WorkplaceType, post.PublishedDate, len(post.Description))
}
