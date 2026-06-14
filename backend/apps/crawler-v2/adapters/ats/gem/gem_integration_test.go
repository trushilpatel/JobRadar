//go:build integration

package gem

// Integration tests — hit the real Gem public Job Board API.
// Skipped under `go test -short`.
//
// Run explicitly:
//   go test ./adapters/ats/gem/ -run Integration -v
//   go test ./adapters/ats/gem/ -run TestGem_Integration -v -count=1

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/trushilpatel/jobradar/common/logger"
)

// "inception" is a known Gem customer used for integration testing.
const gemIntegrationSlug = "inception"

func newIntegrationAdapter() *Gem {
	return New(logger.NewNoop(), http.DefaultClient)
}

func TestGem_Integration_FetchJobListing(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	g := newIntegrationAdapter()
	result, err := g.FetchJobListing(context.Background(), gemIntegrationSlug)
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
	if !strings.HasPrefix(j.ID, "gem-") {
		t.Errorf("job.ID %q should start with gem-", j.ID)
	}
	if j.Title == "" {
		t.Error("job.Title is empty")
	}
	if j.URL == "" {
		t.Error("job.URL is empty")
	}
	if j.JobContent == nil {
		t.Error("job.JobContent is nil — fat adapter must populate at listing time")
	}
	if !j.LocationKnown {
		t.Error("LocationKnown should be true")
	}

	t.Logf("Sample job: id=%s title=%q location=%q workplace=%q dept=%q",
		j.ID, j.Title, j.Location,
		func() string {
			if j.JobContent != nil {
				return j.JobContent.WorkplaceType
			}
			return ""
		}(),
		func() string {
			if j.JobContent != nil {
				return j.JobContent.Department
			}
			return ""
		}(),
	)
}
