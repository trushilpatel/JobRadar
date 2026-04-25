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
		MinDelayMs:    5000,
		MaxDelayMs:    8000,
		TimeoutMs:     60000,
		MaxPages:      2,
		EnrichMinMs:   3000,
		EnrichMaxMs:   5000,
		MaxDetailJobs: 10,
	}
	g := NewGoogleAdapter(cfg, logger.NewNoop())
	if g.cfg.MinDelayMs != 5000 {
		t.Errorf("MinDelayMs: got %d, want 5000", g.cfg.MinDelayMs)
	}
	if g.cfg.MaxDetailJobs != 10 {
		t.Errorf("MaxDetailJobs: got %d, want 10", g.cfg.MaxDetailJobs)
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
		t.Error("GoogleAdapter should not implement ContentFetcher — it is a full-detail adapter")
	}
}

// ── toItem ───────────────────────────────────────────────────────────────────

func TestToItem_FullDetailMapsFields(t *testing.T) {
	job := parsedJob{
		ID:          "google-123",
		Title:       "Software Engineer",
		URL:         "https://www.google.com/about/careers/applications/jobs/results/123-software-engineer",
		Location:    "Mountain View, CA, USA",
		Org:         "YouTube",
		Description: "Minimum qualifications:\n- 3 years experience",
	}
	item := toItem(job)
	if item.ID != "google-123" {
		t.Errorf("ID: got %q", item.ID)
	}
	if item.EmploymentType != "full_time" {
		t.Errorf("EmploymentType: got %q, want full_time", item.EmploymentType)
	}
	if item.FullContent == nil {
		t.Fatal("FullContent should not be nil for enriched job")
	}
	if item.FullContent.Department != "YouTube" {
		t.Errorf("Department: got %q, want YouTube", item.FullContent.Department)
	}
	if item.FullContent.Description == "" {
		t.Error("Description should not be empty")
	}
}

func TestToItem_ThinJobLeavesFullContentNil(t *testing.T) {
	// Empty Description signals a failed-or-skipped enrichment — analyzer's
	// HTTP fallback handles it. FullContent must be nil to trigger that path.
	job := parsedJob{
		ID:       "google-456",
		Title:    "Software Engineer",
		URL:      "https://www.google.com/about/careers/applications/jobs/results/456-software-engineer",
		Location: "Sunnyvale, CA, USA",
		Org:      "Google",
	}
	item := toItem(job)
	if item.FullContent != nil {
		t.Errorf("FullContent should be nil for thin job (empty Description), got %+v", item.FullContent)
	}
	if item.ID != "google-456" {
		t.Errorf("ID: got %q", item.ID)
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

// ── parseDetail ──────────────────────────────────────────────────────────────

const fixtureDetail = `<!doctype html><html><head>
<title>Software Engineer, Search — Google Careers</title>
</head><body>
<h2>job details</h2>
<h3>About the job</h3>
<p>Google is...</p>
<h3>Minimum qualifications:</h3>
<ul>
  <li>Bachelor's degree or equivalent practical experience.</li>
  <li>3 years of experience with software development.</li>
</ul>
<h3>Preferred qualifications:</h3>
<ul>
  <li>Master's degree.</li>
  <li>Experience with distributed systems.</li>
</ul>
<h3>Responsibilities</h3>
<ul>
  <li>Write code.</li>
  <li>Review designs.</li>
</ul>
</body></html>`

func TestParseDetail_BuildsDescriptionWithAllSections(t *testing.T) {
	desc := parseDetail(fixtureDetail, "FALLBACK TITLE", "FALLBACK LOC")

	if !strings.Contains(desc, "Software Engineer, Search") {
		t.Error("description missing title from <title> tag")
	}
	if !strings.Contains(desc, "FALLBACK LOC") {
		t.Error("description should include fallback location")
	}
	for _, header := range []string{
		"Minimum qualifications",
		"Preferred qualifications",
		"Responsibilities",
	} {
		if !strings.Contains(desc, header) {
			t.Errorf("description missing section header %q", header)
		}
	}
	for _, bullet := range []string{
		"- Bachelor's degree or equivalent practical experience.",
		"- Master's degree.",
		"- Write code.",
	} {
		if !strings.Contains(desc, bullet) {
			t.Errorf("description missing bullet %q", bullet)
		}
	}
}

func TestParseDetail_FallsBackToProvidedTitleWhenTagMissing(t *testing.T) {
	html := `<html><body><h3>Minimum qualifications</h3><ul><li>Foo</li></ul></body></html>`
	desc := parseDetail(html, "Backup Title", "Backup Location")
	if !strings.Contains(desc, "Backup Title") {
		t.Errorf("description should fall back to provided title; got: %q", desc)
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
