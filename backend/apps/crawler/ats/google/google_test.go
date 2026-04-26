package google

import (
	"context"
	"strings"
	"testing"

	"github.com/trushilpatel/jobradar/apps/crawler/ats/types"
	"github.com/trushilpatel/jobradar/common/logger"
)

// ── CareersURL ───────────────────────────────────────────────────────────────

func TestGoogleAdapter_CareersURL(t *testing.T) {
	g := NewGoogleAdapterDefault()
	got := g.CareersURL("anything")
	if got != careersURL {
		t.Errorf("CareersURL() = %q, want %q", got, careersURL)
	}
}

// ── Constructor ──────────────────────────────────────────────────────────────

func TestNewGoogleAdapter_StoresConfig(t *testing.T) {
	cfg := Config{
		MinDelayMs: 5000,
		MaxDelayMs: 8000,
		TimeoutMs:  60000,
		MaxPages:   2,
	}
	g := NewGoogleAdapter(cfg, logger.NewNoop())
	if g.cfg.MinDelayMs != 5000 {
		t.Errorf("MinDelayMs: got %d, want 5000", g.cfg.MinDelayMs)
	}
	if g.cfg.MaxPages != 2 {
		t.Errorf("MaxPages: got %d, want 2", g.cfg.MaxPages)
	}
}

func TestNewGoogleAdapterDefault_UsesProductionDefaults(t *testing.T) {
	g := NewGoogleAdapterDefault()
	def := defaultConfig()
	if g.cfg.MinDelayMs != def.MinDelayMs {
		t.Errorf("MinDelayMs: got %d, want %d", g.cfg.MinDelayMs, def.MinDelayMs)
	}
	if g.cfg.TimeoutMs != def.TimeoutMs {
		t.Errorf("TimeoutMs: got %d, want %d", g.cfg.TimeoutMs, def.TimeoutMs)
	}
}

// ── Interface compliance ─────────────────────────────────────────────────────

func TestGoogleAdapter_ImplementsAdapter(t *testing.T) {
	var _ types.Adapter = (*GoogleAdapter)(nil)
}

func TestGoogleAdapter_ImplementsURLProvider(t *testing.T) {
	var _ types.URLProvider = (*GoogleAdapter)(nil)
}

func TestGoogleAdapter_DoesNotImplementKnownJobsProvider(t *testing.T) {
	var g any = NewGoogleAdapterDefault()
	if _, ok := g.(types.KnownJobsProvider); ok {
		t.Error("GoogleAdapter should not implement KnownJobsProvider — it uses SeenChecker instead")
	}
}

func TestGoogleAdapter_DoesNotImplementContentFetcher(t *testing.T) {
	var g any = NewGoogleAdapterDefault()
	if _, ok := g.(types.ContentFetcher); ok {
		t.Error("GoogleAdapter should not implement ContentFetcher — it is a listing-only adapter; detail fetching is handled by the analyzer")
	}
}

// ── toItem ───────────────────────────────────────────────────────────────────

func TestToItem_AlwaysThin(t *testing.T) {
	// The crawler is a listing-only scanner. FullContent is always nil so the
	// analyzer's HTTP fetch + LLM enrichment runs for every Google job.
	job := parsedJob{
		ID:       "google-123",
		Title:    "Software Engineer",
		URL:      "https://www.google.com/about/careers/applications/jobs/results/123-software-engineer",
		Location: "Mountain View, CA, USA",
		Org:      "YouTube",
	}
	item := toItem(job)
	if item.ID != "google-123" {
		t.Errorf("ID: got %q", item.ID)
	}
	if item.EmploymentType != "full_time" {
		t.Errorf("EmploymentType: got %q, want full_time", item.EmploymentType)
	}
	if item.FullContent != nil {
		t.Errorf("FullContent should always be nil (listing-only), got %+v", item.FullContent)
	}
}

// ── SeenChecker ──────────────────────────────────────────────────────────────

func TestGoogleAdapter_ImplementsSeenChecker(t *testing.T) {
	var _ types.SeenChecker = (*GoogleAdapter)(nil)
}

func TestSetSeenChecker_StoresAndCallsFunction(t *testing.T) {
	g := NewGoogleAdapterDefault()
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
	pageJobs := []parsedJob{
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
	pageJobs := []parsedJob{{ID: "google-1"}, {ID: "google-1"}}
	seenMap := map[string]bool{"google-1": false}
	localSeen := map[string]struct{}{}

	result := filterPageJobs(pageJobs, seenMap, localSeen)

	if len(result) != 1 {
		t.Errorf("expected 1 unique job, got %d", len(result))
	}
}

func TestFilterPageJobs_SkipsJobsAlreadySeenThisScan(t *testing.T) {
	pageJobs := []parsedJob{{ID: "google-1"}}
	seenMap := map[string]bool{"google-1": false}
	localSeen := map[string]struct{}{"google-1": {}} // already processed on a prior page

	result := filterPageJobs(pageJobs, seenMap, localSeen)

	if len(result) != 0 {
		t.Errorf("expected 0 jobs, got %d", len(result))
	}
}

func TestFilterPageJobs_UpdatesLocalSeen(t *testing.T) {
	pageJobs := []parsedJob{{ID: "google-1"}, {ID: "google-2"}}
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
	pageJobs := []parsedJob{{ID: "google-1"}, {ID: "google-2"}}
	seenMap := map[string]bool{"google-1": true, "google-2": true}
	localSeen := map[string]struct{}{}

	result := filterPageJobs(pageJobs, seenMap, localSeen)

	if len(result) != 0 {
		t.Errorf("expected 0 jobs when all seen, got %d", len(result))
	}
}

// ── parseListing ─────────────────────────────────────────────────────────────

// Synthetic listing HTML — matches the structural shape of the live page:
// per-card slice has corp icon → org text → place icon → location text →
// button → anchor (href THEN aria-label, in that attribute order).
//
// Google's live markup emits href before aria-label inside the anchor;
// parseListing's slicing relies on aria-label being lexically last so the
// href lands inside slice[i].
const fixtureListing = `<!doctype html><html><body>
<div class="header">Showing 1 to 20 of 764 rows</div>
<ul class="search-results">
  <li class="card">
    <h3>Software Engineer, Search</h3>
    <i class="google-material-icons">corporate_fare</i>
    <span><span>Google</span></span>
    <i class="google-material-icons">place</i>
    <span><span>Mountain View, CA, USA</span></span>
    <button>Save</button>
    <a class="link" href="jobs/results/12345-software-engineer-search?location=United%20States" aria-label="Learn more about Software Engineer, Search"></a>
  </li>
  <li class="card">
    <h3>Senior Software Engineer, Cloud</h3>
    <i class="google-material-icons">corporate_fare</i>
    <span><span>Google</span></span>
    <i class="google-material-icons">place</i>
    <span><span>Kirkland, WA, USA; Sunnyvale, CA, USA; +2 more</span></span>
    <button>Save</button>
    <a class="link" href="jobs/results/67890-senior-software-engineer-cloud?location=United%20States" aria-label="Learn more about Senior Software Engineer, Cloud"></a>
  </li>
</ul>
</body></html>`

func TestParseListing_ExtractsCardsAndTotal(t *testing.T) {
	cards, total := parseListing(fixtureListing)

	if total != 764 {
		t.Errorf("total: got %d, want 764", total)
	}
	if len(cards) != 2 {
		t.Fatalf("expected 2 cards, got %d", len(cards))
	}

	c0 := cards[0]
	if c0.ID != "google-12345" {
		t.Errorf("card[0].ID: got %q, want google-12345", c0.ID)
	}
	if c0.Title != "Software Engineer, Search" {
		t.Errorf("card[0].Title: got %q", c0.Title)
	}
	if !strings.HasSuffix(c0.URL, "12345-software-engineer-search") {
		t.Errorf("card[0].URL: got %q", c0.URL)
	}
	if c0.Location != "Mountain View, CA, USA" {
		t.Errorf("card[0].Location: got %q", c0.Location)
	}
	if c0.Org != "Google" {
		t.Errorf("card[0].Org: got %q, want Google (no leftover HTML)", c0.Org)
	}

	c1 := cards[1]
	if c1.ID != "google-67890" {
		t.Errorf("card[1].ID: got %q", c1.ID)
	}
	// "+2 more" should be stripped from multi-location strings.
	if strings.Contains(c1.Location, "+2 more") {
		t.Errorf("card[1].Location should not contain '+2 more': %q", c1.Location)
	}
}

// ── cleanText ────────────────────────────────────────────────────────────────

func TestCleanText_StripsTagsAndCollapsesWhitespace(t *testing.T) {
	got := cleanText(`  <span> Hello   <b>world</b> </span>  `, false)
	if got != "Hello world" {
		t.Errorf("got %q, want %q", got, "Hello world")
	}
}

func TestCleanText_StripsPlusNMoreWhenRequested(t *testing.T) {
	got := cleanText(`<span>Kirkland, WA, USA; Sunnyvale, CA, USA; +2 more</span>`, true)
	if strings.Contains(got, "+2 more") {
		t.Errorf("'+2 more' should be stripped: %q", got)
	}
	if !strings.Contains(got, "Kirkland") || !strings.Contains(got, "Sunnyvale") {
		t.Errorf("locations dropped: %q", got)
	}
}
