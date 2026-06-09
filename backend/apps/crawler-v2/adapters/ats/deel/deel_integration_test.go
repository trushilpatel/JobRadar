//go:build integration

package deel

// Integration tests — hit the real jobs.deel.com API.
// Skipped under `go test -short`.
//
// Run explicitly:
//   go test ./adapters/ats/deel/ -run Integration -v -count=1

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/trushilpatel/jobradar/apps/crawler-v2/common/logger"
)

// klarna maintains a large job board on jobs.deel.com and is a stable
// integration target for the adapter.
const deelIntegrationSlug = "klarna"

func TestDeel_Integration_FetchJobListing(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	d := New(logger.NewNoop(), http.DefaultClient)
	listing, err := d.FetchJobListing(context.Background(), deelIntegrationSlug)
	if err != nil {
		t.Fatalf("FetchJobListing error: %v", err)
	}
	if len(listing.Jobs) == 0 {
		t.Fatal("expected at least one job, got 0")
	}

	t.Logf("FetchJobListing returned %d jobs", len(listing.Jobs))

	j := listing.Jobs[0]
	if !strings.HasPrefix(j.ID, "deel-") {
		t.Errorf("job ID %q should start with deel-", j.ID)
	}
	if j.Title == "" {
		t.Error("job Title is empty")
	}
	wantURLPrefix := "https://jobs.deel.com/" + deelIntegrationSlug + "/job-details/"
	if !strings.HasPrefix(j.URL, wantURLPrefix) {
		t.Errorf("job URL %q should start with %q", j.URL, wantURLPrefix)
	}
	if j.JobContent == nil {
		t.Error("JobContent must be set — deel is a fat adapter")
	}

	t.Logf("Sample job: id=%s title=%q location=%q desc_len=%d",
		j.ID, j.Title, j.Location, len(j.JobContent.Description))
}
