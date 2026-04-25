package google

import (
	"context"
	"testing"

	"github.com/trushilpatel/jobradar/apps/crawler/ats/types"
	"github.com/trushilpatel/jobradar/common/logger"
)

// ── CareersURL ───────────────────────────────────────────────────────────────

func TestRodGoogle_CareersURL(t *testing.T) {
	g := NewRodGoogleDefault()
	got := g.CareersURL("anything")
	if got != careersURL {
		t.Errorf("CareersURL() = %q, want %q", got, careersURL)
	}
}

// ── Constructor ──────────────────────────────────────────────────────────────

func TestNewRodGoogle_StoresConfig(t *testing.T) {
	cfg := Config{
		MinDelayMs:    5000,
		MaxDelayMs:    8000,
		TimeoutMs:     60000,
		MaxPages:      2,
		EnrichMinMs:   3000,
		EnrichMaxMs:   5000,
		MaxDetailJobs: 10,
	}
	g := NewRodGoogle(cfg, logger.NewNoop())
	if g.cfg.MinDelayMs != 5000 {
		t.Errorf("MinDelayMs: got %d, want 5000", g.cfg.MinDelayMs)
	}
	if g.cfg.MaxDetailJobs != 10 {
		t.Errorf("MaxDetailJobs: got %d, want 10", g.cfg.MaxDetailJobs)
	}
}

func TestNewRodGoogleDefault_UsesProductionDefaults(t *testing.T) {
	g := NewRodGoogleDefault()
	def := defaultConfig()
	if g.cfg.MinDelayMs != def.MinDelayMs {
		t.Errorf("MinDelayMs: got %d, want %d", g.cfg.MinDelayMs, def.MinDelayMs)
	}
	if g.cfg.TimeoutMs != def.TimeoutMs {
		t.Errorf("TimeoutMs: got %d, want %d", g.cfg.TimeoutMs, def.TimeoutMs)
	}
}

// ── Interface compliance ─────────────────────────────────────────────────────

func TestRodGoogle_ImplementsAdapter(t *testing.T) {
	var _ types.Adapter = (*RodGoogle)(nil)
}

func TestRodGoogle_ImplementsURLProvider(t *testing.T) {
	var _ types.URLProvider = (*RodGoogle)(nil)
}

func TestRodGoogle_DoesNotImplementKnownJobsProvider(t *testing.T) {
	var g any = NewRodGoogleDefault()
	if _, ok := g.(types.KnownJobsProvider); ok {
		t.Error("RodGoogle should not implement KnownJobsProvider — it uses SeenChecker instead")
	}
}

func TestRodGoogle_DoesNotImplementContentFetcher(t *testing.T) {
	var g any = NewRodGoogleDefault()
	if _, ok := g.(types.ContentFetcher); ok {
		t.Error("RodGoogle should not implement ContentFetcher — it is a full-detail adapter")
	}
}

// ── SetKnownJobIDs ───────────────────────────────────────────────────────────

// ── WithBrowser ──────────────────────────────────────────────────────────────

func TestWithBrowser_StoresBrowser(t *testing.T) {
	g := NewRodGoogleDefault()
	if g.browser != nil {
		t.Fatal("browser should be nil before WithBrowser")
	}
	result := g.WithBrowser(nil)
	if result != g {
		t.Error("WithBrowser should return the same RodGoogle for chaining")
	}
}

// ── toRodItems ───────────────────────────────────────────────────────────────

func TestToRodItems_MapsFields(t *testing.T) {
	jobs := []jsJob{{
		ID:          "google-123",
		Title:       "Software Engineer",
		URL:         "https://careers.google.com/jobs/results/123",
		Location:    "Mountain View, CA, USA",
		Org:         "YouTube",
		Description: "Minimum qualifications:\n- 3 years experience",
	}}
	items := toRodItems(jobs)
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	item := items[0]
	if item.ID != "google-123" {
		t.Errorf("ID: got %q", item.ID)
	}
	if item.EmploymentType != "full_time" {
		t.Errorf("EmploymentType: got %q, want full_time", item.EmploymentType)
	}
	if item.FullContent == nil {
		t.Fatal("FullContent should not be nil")
	}
	if item.FullContent.Department != "YouTube" {
		t.Errorf("Department: got %q, want YouTube", item.FullContent.Department)
	}
	if item.FullContent.Description == "" {
		t.Error("Description should not be empty")
	}
}

func TestToRodItems_EmptySlice(t *testing.T) {
	if len(toRodItems(nil)) != 0 {
		t.Error("expected empty slice for nil input")
	}
}

// ── SeenChecker ──────────────────────────────────────────────────────────────

func TestRodGoogle_ImplementsSeenChecker(t *testing.T) {
	var _ types.SeenChecker = (*RodGoogle)(nil)
}

func TestSetSeenChecker_StoresAndCallsFunction(t *testing.T) {
	g := NewRodGoogleDefault()
	called := false
	g.SetSeenChecker(func(ctx context.Context, ids []string) (map[string]bool, error) {
		called = true
		return map[string]bool{}, nil
	})
	_, _ = g.seenChecker(context.Background(), []string{"google-123"})
	if !called {
		t.Error("seenChecker was not stored or not called")
	}
}

// ── filterPageJobs ───────────────────────────────────────────────────────────

func TestFilterPageJobs_ReturnsOnlyUnseenJobs(t *testing.T) {
	pageJobs := []jsJob{
		{ID: "google-1"},
		{ID: "google-2"},
		{ID: "google-3"},
	}
	seenMap := map[string]bool{
		"google-1": true,
		"google-2": true,
		"google-3": false,
	}
	localSeen := map[string]struct{}{}

	result := filterPageJobs(pageJobs, seenMap, localSeen)

	if len(result) != 1 {
		t.Fatalf("expected 1 unseen job, got %d", len(result))
	}
	if result[0].ID != "google-3" {
		t.Errorf("expected google-3, got %s", result[0].ID)
	}
}

func TestFilterPageJobs_DeduplicatesWithinScan(t *testing.T) {
	pageJobs := []jsJob{{ID: "google-1"}, {ID: "google-1"}}
	seenMap := map[string]bool{"google-1": false}
	localSeen := map[string]struct{}{}

	result := filterPageJobs(pageJobs, seenMap, localSeen)

	if len(result) != 1 {
		t.Errorf("expected 1 unique job, got %d", len(result))
	}
}

func TestFilterPageJobs_SkipsJobsAlreadySeenThisScan(t *testing.T) {
	pageJobs := []jsJob{{ID: "google-1"}}
	seenMap := map[string]bool{"google-1": false}
	localSeen := map[string]struct{}{"google-1": {}} // already processed on a prior page

	result := filterPageJobs(pageJobs, seenMap, localSeen)

	if len(result) != 0 {
		t.Errorf("expected 0 jobs, got %d", len(result))
	}
}

func TestFilterPageJobs_UpdatesLocalSeen(t *testing.T) {
	pageJobs := []jsJob{{ID: "google-1"}, {ID: "google-2"}}
	seenMap := map[string]bool{"google-1": false, "google-2": false}
	localSeen := map[string]struct{}{}

	filterPageJobs(pageJobs, seenMap, localSeen)

	if _, ok := localSeen["google-1"]; !ok {
		t.Error("localSeen should contain google-1 after filtering")
	}
	if _, ok := localSeen["google-2"]; !ok {
		t.Error("localSeen should contain google-2 after filtering")
	}
}

func TestFilterPageJobs_AllSeen_ReturnsEmpty(t *testing.T) {
	pageJobs := []jsJob{{ID: "google-1"}, {ID: "google-2"}}
	seenMap := map[string]bool{"google-1": true, "google-2": true}
	localSeen := map[string]struct{}{}

	result := filterPageJobs(pageJobs, seenMap, localSeen)

	if len(result) != 0 {
		t.Errorf("expected 0 jobs when all seen, got %d", len(result))
	}
}
