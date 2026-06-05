package workday

// Integration tests — hit the real Workday CXS public API.
// Skipped under `go test -short`.
//
// Run explicitly:
//   go test ./adapters/ats/workday/ -run Integration -v
//   go test ./adapters/ats/workday/ -run TestWorkday_Integration -v -count=1

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/trushilpatel/jobradar/jobingest/common/logger"
	"github.com/trushilpatel/jobradar/jobingest/core"
)

// wdIntegrationConfig uses NVIDIA's public Workday board which is consistently
// available and returns real job postings.
var wdIntegrationConfig = json.RawMessage(`{
	"host": "nvidia.wd5.myworkdayjobs.com",
	"site": "NVIDIAExternalCareerSite",
	"locale": "en-US",
	"max_jobs_per_scan": 20
}`)

func newIntegrationAdapter() *Workday {
	return New(logger.NewNoop(), http.DefaultClient)
}

func TestWorkday_Integration_FetchJobListingWithConfig(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	w := newIntegrationAdapter()
	result, err := w.FetchJobListingWithConfig(context.Background(), "nvidia", wdIntegrationConfig)
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
	if !strings.HasPrefix(j.ID, "workday-") {
		t.Errorf("job.ID %q should start with workday-", j.ID)
	}
	if j.Title == "" {
		t.Error("job.Title is empty")
	}
	if j.URL == "" {
		t.Error("job.URL is empty")
	}
	if !strings.Contains(j.URL, "nvidia.wd5.myworkdayjobs.com") {
		t.Errorf("job.URL %q should contain nvidia.wd5.myworkdayjobs.com", j.URL)
	}
	if j.EmployType != core.EmployFullTime {
		t.Errorf("job.EmployType = %q, want full_time (Workday default)", j.EmployType)
	}

	t.Logf("Sample job: id=%s title=%q location=%q url=%s", j.ID, j.Title, j.Location, j.URL)
}

func TestWorkday_Integration_FetchJobPost(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	w := newIntegrationAdapter()
	result, err := w.FetchJobListingWithConfig(context.Background(), "nvidia", wdIntegrationConfig)
	if err != nil {
		t.Fatalf("FetchJobListingWithConfig error: %v", err)
	}
	if len(result.Jobs) == 0 {
		t.Skip("no jobs returned, skipping post fetch")
	}

	item := result.Jobs[0]
	t.Logf("Fetching post for: %s — %q", item.ID, item.Title)

	summary := core.JobSummary{ID: item.ID, Title: item.Title, URL: item.URL}
	post, err := w.FetchJobPost(context.Background(), summary)
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
	if strings.Contains(post.Description, "<") {
		t.Error("post.Description should have HTML stripped")
	}

	t.Logf("FetchJobPost: id=%s title=%q location=%q country=%q desc_len=%d",
		post.ID, post.Title, post.Location, post.CountryCode, len(post.Description))
}
