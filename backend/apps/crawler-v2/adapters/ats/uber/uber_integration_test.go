package uber

// Integration tests — hit the real Uber careers API.
// Skipped under `go test -short`.
//
// Run explicitly:
//   go test ./adapters/ats/uber/ -run Integration -v
//   go test ./adapters/ats/uber/ -run TestUber_Integration -v -count=1

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/trushilpatel/jobradar/apps/crawler-v2/common/logger"
)

func newIntegrationAdapter() *Uber {
	return New(logger.NewNoop(), http.DefaultClient)
}

func TestUber_Integration_FetchJobListing(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	u := newIntegrationAdapter()
	// companyID is ignored by the Uber adapter.
	result, err := u.FetchJobListing(context.Background(), "uber")
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
	if !strings.HasPrefix(j.ID, "uber-") {
		t.Errorf("job.ID %q should start with uber-", j.ID)
	}
	if j.Title == "" {
		t.Error("job.Title is empty")
	}
	if j.URL == "" {
		t.Error("job.URL is empty")
	}
	if !strings.Contains(j.URL, "/careers/list/") {
		t.Errorf("job.URL %q should contain /careers/list/", j.URL)
	}
	if !j.LocationKnown {
		t.Error("job.LocationKnown should be true")
	}
	if j.JobContent == nil {
		t.Fatal("job.JobContent is nil — fat adapter must always set it")
	}
	if j.JobContent.Description == "" {
		t.Error("job.JobContent.Description is empty")
	}

	t.Logf("Sample job: id=%s title=%q location=%q dept=%q desc_len=%d",
		j.ID, j.Title, j.Location, j.JobContent.Department, len(j.JobContent.Description))
}
