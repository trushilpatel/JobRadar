package google

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

// Google detail pages are ~1.2MB and embed the description as JSON-quoted
// escaped-HTML strings inside `AF_initDataCallback({key:'ds:0',…})` past the
// 1MB mark — well outside the analyzer's 50KB HTTP fallback read window. The
// adapter must therefore implement ContentFetcher and own its own detail
// extraction so the LLM receives a non-empty Description.
func TestGoogleAdapter_ImplementsContentFetcher(t *testing.T) {
	var g any = NewGoogleAdapterDefault()
	if _, ok := g.(types.ContentFetcher); !ok {
		t.Error("GoogleAdapter must implement ContentFetcher — the analyzer's 50KB HTTP fallback cannot reach Google's description region")
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

// ── extractDescription ───────────────────────────────────────────────────────
//
// Google detail pages embed job data inside `AF_initDataCallback({key:'ds:N',
// hash:'…', data:[…]});` script blocks. The focal job lives in `ds:0` and the
// ~20 search-context siblings live in `ds:1`. Inside those data arrays the
// description text is encoded as JSON-quoted strings with HTML angle brackets
// emitted as the literal six-byte escapes `<` / `>` (not the
// rendered runes). The fixture below mirrors that exact byte shape so
// extractDescription is verified against the structure the live page emits.

// fixtureDetailFocalID is the canonical job id the fixture's ds:0 block holds.
const fixtureDetailFocalID = "999000111000"

// fixtureDetailSiblingSentinel must NEVER appear in extracted output — it
// lives in ds:1 (sibling jobs) and would only leak if the extractor failed to
// scope itself to the focal callback.
const fixtureDetailSiblingSentinel = "SIBLING_JOB_LEAK_SENTINEL"

// mustJSONQuote returns the JSON-encoded form of s, including surrounding
// quotes. Go's json.Marshal escapes `<`, `>`, and `&` as `<` / `>`
// / `&` for HTML safety — the exact six-byte sequences that appear in
// Google's embedded `AF_initDataCallback` data strings.
func mustJSONQuote(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// fixtureDetailHTML synthesizes a Google careers detail page. Padding before
// the ds:0 callback places the description region past 50KB so the fixture
// also documents the reason the analyzer's HTTP fallback could not recover
// this content.
var fixtureDetailHTML = `<!doctype html><html><head><title>Test Job</title></head><body>` +
	strings.Repeat("<!-- padding to push ds:0 past the 50KB read window -->", 2000) +
	`<script>AF_initDataCallback({key: 'ds:0', hash: '1', data:[["` + fixtureDetailFocalID + `","Test Software Engineer","https://example.com/apply",null,null,null,` +
	mustJSONQuote(`<p>About the company. We build cool distributed systems.</p>`) + `,` +
	mustJSONQuote(`<ul><li>Design reliable services.</li><li>Mentor other engineers.</li></ul>`) + `,` +
	mustJSONQuote(`<h3>Minimum qualifications:</h3><ul><li>Bachelor's degree or equivalent practical experience.</li><li>2 years of software engineering experience.</li></ul><h3>Preferred qualifications:</h3><ul><li>Experience with Go.</li></ul>`) +
	`]]});</script>` +
	`<script>AF_initDataCallback({key: 'ds:1', hash: '2', data:[[["sibling-id-222","Sibling Job","https://example.com/sibling",null,null,null,` +
	mustJSONQuote(`<p>`+fixtureDetailSiblingSentinel+`</p>`) + `]]]});</script>` +
	`</body></html>`

// TestFixtureUsesGoogleEscapeBytes guards the fixture itself — without it the
// rest of the extractDescription tests can silently pass on plain `<` runes
// while the production code only matches the literal six-byte `<` escape
// that the live page emits.
func TestFixtureUsesGoogleEscapeBytes(t *testing.T) {
	// The fixture must contain the six-byte sequence `<` (backslash, u,
	// 0, 0, 3, c) — not the single-byte `<` rune.
	wantBytes := []byte{'\\', 'u', '0', '0', '3', 'c'}
	if !strings.Contains(fixtureDetailHTML, string(wantBytes)) {
		t.Fatalf("fixture has no literal \\u003c escape sequence — tests would silently bypass the production decoder")
	}
}

func TestExtractDescription_DecodesEscapedHTMLFromDS0(t *testing.T) {
	got := extractDescription(fixtureDetailHTML)
	if got == "" {
		t.Fatalf("extractDescription returned empty; expected focal-job description")
	}
	for _, want := range []string{
		"About the company",
		"distributed systems",
		"Design reliable services",
		"Mentor other engineers",
		"Minimum qualifications",
		"Bachelor's degree",
		"2 years of software engineering experience",
		"Preferred qualifications",
		"Experience with Go",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("description missing %q\nfull:\n%s", want, got)
		}
	}
	// HTML tags should be stripped — no encoded escapes survive in output.
	if strings.Contains(got, `<`) || strings.Contains(got, "<h3>") || strings.Contains(got, "<li>") {
		t.Errorf("description leaked HTML or encoded markup: %q", got)
	}
}

func TestExtractDescription_IgnoresSiblingJobsInDS1(t *testing.T) {
	got := extractDescription(fixtureDetailHTML)
	if strings.Contains(got, fixtureDetailSiblingSentinel) {
		t.Errorf("description leaked ds:1 sibling-job content; got: %s", got)
	}
}

func TestExtractDescription_ReturnsEmptyWhenNoDS0(t *testing.T) {
	got := extractDescription(`<html><body>no callbacks here</body></html>`)
	if got != "" {
		t.Errorf("expected empty description when ds:0 absent; got: %q", got)
	}
}

func TestExtractDescription_DemonstratesBugWithFirst50KBOnly(t *testing.T) {
	// Regression marker: the analyzer's HTTP fallback used to read at most
	// 50KB of the response body, then run StripHTML. The fixture's padding
	// is sized so the description region lives PAST byte 50000 — proving
	// the fallback could not recover any description from a Google page.
	if len(fixtureDetailHTML) <= 50*1024 {
		t.Fatalf("fixture too short to exercise the 50KB-cap regression: %d bytes", len(fixtureDetailHTML))
	}
	truncated := fixtureDetailHTML[:50*1024]
	plain := types.StripHTML(truncated)
	for _, marker := range []string{"Minimum qualifications", "About the company", "Design reliable services"} {
		if strings.Contains(plain, marker) {
			t.Fatalf("regression: %q survived the 50KB read window — fixture no longer reproduces the bug", marker)
		}
	}
}

// ── FetchJobContent ──────────────────────────────────────────────────────────

func TestFetchJobContent_ReturnsPlainDescriptionFromGoogleStylePage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(fixtureDetailHTML))
	}))
	defer srv.Close()

	g := NewGoogleAdapter(defaultConfig(), logger.NewNoop())
	job := types.RawJob{
		ID:       "google-" + fixtureDetailFocalID,
		Title:    "Test Software Engineer",
		Location: "Mountain View, CA, USA",
		URL:      srv.URL + "/about/careers/applications/jobs/results/" + fixtureDetailFocalID + "-test-software-engineer",
		Adapter:  "google",
	}

	resp, err := g.FetchJobContent(context.Background(), job)
	if err != nil {
		t.Fatalf("FetchJobContent: %v", err)
	}
	if resp.ID != job.ID {
		t.Errorf("ID: got %q, want %q", resp.ID, job.ID)
	}
	if resp.Title != job.Title {
		t.Errorf("Title: got %q, want %q", resp.Title, job.Title)
	}
	if resp.Location != job.Location {
		t.Errorf("Location: got %q, want %q", resp.Location, job.Location)
	}
	if resp.URL != job.URL {
		t.Errorf("URL: got %q, want %q", resp.URL, job.URL)
	}
	if !strings.Contains(resp.Description, "Minimum qualifications") {
		t.Errorf("description missing core section; got: %s", resp.Description)
	}
	if strings.Contains(resp.Description, fixtureDetailSiblingSentinel) {
		t.Errorf("description leaked sibling-job text; got: %s", resp.Description)
	}
}

func TestFetchJobContent_ReturnsErrorOnNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "gone", http.StatusGone)
	}))
	defer srv.Close()

	g := NewGoogleAdapter(defaultConfig(), logger.NewNoop())
	_, err := g.FetchJobContent(context.Background(), types.RawJob{
		ID:  "google-1",
		URL: srv.URL + "/missing",
	})
	if err == nil {
		t.Fatal("expected error on non-200 response")
	}
}

func TestFetchJobContent_ReturnsErrorOnEmptyURL(t *testing.T) {
	g := NewGoogleAdapter(defaultConfig(), logger.NewNoop())
	_, err := g.FetchJobContent(context.Background(), types.RawJob{
		ID:  "google-1",
		URL: "",
	})
	if err == nil {
		t.Fatal("expected error when RawJob.URL is empty")
	}
}
