package google

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
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

// ── FetchJobs — end-to-end against a fake listing server ───────────────────

// makeListingPage builds a single listing-page HTML body with the supplied
// jobs and "Showing X to Y of N" total — synthesises the exact card shape
// the live Google page emits so parseListing returns a non-empty slice.
func makeListingPage(total int, jobs ...parsedJob) string {
	var sb strings.Builder
	sb.WriteString(`<!doctype html><html><body>`)
	sb.WriteString(`<div class="header">Showing 1 to 20 of `)
	sb.WriteString(strconv.Itoa(total))
	sb.WriteString(` rows</div><ul class="search-results">`)
	for _, j := range jobs {
		id := strings.TrimPrefix(j.ID, "google-")
		sb.WriteString(`<li class="card"><h3>` + j.Title + `</h3>`)
		sb.WriteString(`<i class="google-material-icons">corporate_fare</i>`)
		sb.WriteString(`<span><span>` + j.Org + `</span></span>`)
		sb.WriteString(`<i class="google-material-icons">place</i>`)
		sb.WriteString(`<span><span>` + j.Location + `</span></span>`)
		sb.WriteString(`<button>Save</button>`)
		sb.WriteString(`<a class="link" href="jobs/results/` + id + `-test-slug?q=us" aria-label="Learn more about ` + j.Title + `"></a>`)
		sb.WriteString(`</li>`)
	}
	sb.WriteString(`</ul></body></html>`)
	return sb.String()
}

// TestFetchJobs_HappyPath_SinglePage verifies FetchJobs walks a synthetic
// listing page, parses cards, and returns FetchJobsItem values via the
// streaming callback. Sets MaxPages=1 and zero delays to keep the test fast.
func TestFetchJobs_HappyPath_SinglePage(t *testing.T) {
	page := makeListingPage(2,
		parsedJob{ID: "google-100", Title: "SWE A", Org: "Search", Location: "Mountain View"},
		parsedJob{ID: "google-200", Title: "SWE B", Org: "Cloud", Location: "Kirkland"},
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(page))
	}))
	defer srv.Close()

	cfg := Config{MinDelayMs: 1, MaxDelayMs: 2, TimeoutMs: 5000, MaxPages: 1}
	g := NewGoogleAdapter(cfg, logger.NewNoop())
	g.listingURLOverride = srv.URL + "?"

	var streamed []types.FetchJobsItem
	g.SetStreamPublish(func(items []types.FetchJobsItem) {
		streamed = append(streamed, items...)
	})

	resp, err := g.FetchJobs(context.Background(), "google")
	if err != nil {
		t.Fatalf("FetchJobs: %v", err)
	}
	if len(resp.Jobs) != 2 {
		t.Errorf("FetchJobs returned %d jobs, want 2", len(resp.Jobs))
	}
	if len(streamed) != 2 {
		t.Errorf("stream callback fired %d times, want 2", len(streamed))
	}
}

// TestFetchJobs_StopsAtMaxPages verifies the engine honours cfg.MaxPages —
// without this guard, a misconfigured scan could walk every page forever.
func TestFetchJobs_StopsAtMaxPages(t *testing.T) {
	page := makeListingPage(1000,
		parsedJob{ID: "google-1", Title: "SWE", Org: "Co", Location: "MV"},
	)
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		_, _ = w.Write([]byte(page))
	}))
	defer srv.Close()

	cfg := Config{MinDelayMs: 1, MaxDelayMs: 2, TimeoutMs: 5000, MaxPages: 1}
	g := NewGoogleAdapter(cfg, logger.NewNoop())
	g.listingURLOverride = srv.URL + "?"
	g.SetStreamPublish(func(_ []types.FetchJobsItem) {})

	if _, err := g.FetchJobs(context.Background(), "google"); err != nil {
		t.Fatalf("FetchJobs: %v", err)
	}
	if hits != 1 {
		t.Errorf("expected 1 page fetch (MaxPages=1), got %d", hits)
	}
}

// TestFetchJobs_SeenCheckerSkipsKnownJobs verifies the seen-check filters
// previously-known cards before publishing — a regression here would re-spam
// NATS JetStream with already-tracked jobs on every scan.
func TestFetchJobs_SeenCheckerSkipsKnownJobs(t *testing.T) {
	page := makeListingPage(2,
		parsedJob{ID: "google-100", Title: "SWE A", Org: "X", Location: "MV"},
		parsedJob{ID: "google-200", Title: "SWE B", Org: "Y", Location: "KL"},
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(page))
	}))
	defer srv.Close()

	cfg := Config{MinDelayMs: 1, MaxDelayMs: 2, TimeoutMs: 5000, MaxPages: 1}
	g := NewGoogleAdapter(cfg, logger.NewNoop())
	g.listingURLOverride = srv.URL + "?"

	var streamed []types.FetchJobsItem
	g.SetStreamPublish(func(items []types.FetchJobsItem) {
		streamed = append(streamed, items...)
	})
	g.SetSeenChecker(func(_ context.Context, ids []string) (map[string]bool, error) {
		out := map[string]bool{}
		for _, id := range ids {
			// pretend google-100 is already in the ledger
			if id == "google-100" {
				out[id] = true
			}
		}
		return out, nil
	})

	resp, err := g.FetchJobs(context.Background(), "google")
	if err != nil {
		t.Fatalf("FetchJobs: %v", err)
	}
	if len(resp.Jobs) != 1 || resp.Jobs[0].ID != "google-200" {
		t.Errorf("expected only google-200 published, got %+v", resp.Jobs)
	}
	if len(streamed) != 1 {
		t.Errorf("streamed: got %d, want 1", len(streamed))
	}
}

// TestFetchJobs_SeenCheckerErrorContinuesPagination verifies a flapping
// seen-check causes FetchJobs to skip enrichment for that page and advance —
// without this the scan loop would either stall or re-publish duplicates.
func TestFetchJobs_SeenCheckerErrorContinuesPagination(t *testing.T) {
	page := makeListingPage(2,
		parsedJob{ID: "google-100", Title: "SWE A", Org: "X", Location: "MV"},
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(page))
	}))
	defer srv.Close()

	cfg := Config{MinDelayMs: 1, MaxDelayMs: 2, TimeoutMs: 5000, MaxPages: 1}
	g := NewGoogleAdapter(cfg, logger.NewNoop())
	g.listingURLOverride = srv.URL + "?"
	g.SetStreamPublish(func(_ []types.FetchJobsItem) {})
	g.SetSeenChecker(func(_ context.Context, _ []string) (map[string]bool, error) {
		return nil, errSeenCheckFail
	})

	// Should not return an error — failure to check is treated as
	// "everything seen" so we don't blast NATS JetStream.
	resp, err := g.FetchJobs(context.Background(), "google")
	if err != nil {
		t.Fatalf("FetchJobs returned error on seen-check failure: %v", err)
	}
	if len(resp.Jobs) != 0 {
		t.Errorf("expected 0 jobs on seen-check failure (defensive skip), got %d", len(resp.Jobs))
	}
}

var errSeenCheckFail = errors.New("seen-check transient error")

// TestFetchJobs_ListingFetchErrorStopsScan verifies an HTTP fetch failure on
// the first page terminates the loop cleanly — no panic, returns whatever
// was published so far (zero items).
func TestFetchJobs_ListingFetchErrorStopsScan(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	cfg := Config{MinDelayMs: 1, MaxDelayMs: 2, TimeoutMs: 5000, MaxPages: 1}
	g := NewGoogleAdapter(cfg, logger.NewNoop())
	g.listingURLOverride = srv.URL + "?"

	resp, err := g.FetchJobs(context.Background(), "google")
	if err != nil {
		t.Fatalf("FetchJobs should swallow HTTP errors (logged + break), got %v", err)
	}
	if len(resp.Jobs) != 0 {
		t.Errorf("expected 0 jobs after fetch error, got %d", len(resp.Jobs))
	}
}

// TestFetchJobs_TotalCoveredStopsLoop verifies the engine stops paginating
// once pageNum*pageSize >= total. The pageSize constant is 20.
func TestFetchJobs_TotalCoveredStopsLoop(t *testing.T) {
	// total=2 means after page 1 (page 1 * 20 = 20 >= 2) the loop should stop.
	page := makeListingPage(2,
		parsedJob{ID: "google-100", Title: "SWE A", Org: "X", Location: "MV"},
	)
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		_, _ = w.Write([]byte(page))
	}))
	defer srv.Close()

	cfg := Config{MinDelayMs: 1, MaxDelayMs: 2, TimeoutMs: 5000, MaxPages: 0}
	g := NewGoogleAdapter(cfg, logger.NewNoop())
	g.listingURLOverride = srv.URL + "?"
	g.SetStreamPublish(func(_ []types.FetchJobsItem) {})

	if _, err := g.FetchJobs(context.Background(), "google"); err != nil {
		t.Fatalf("FetchJobs: %v", err)
	}
	if hits != 1 {
		t.Errorf("expected 1 page fetch (total=2 < 20), got %d", hits)
	}
}

// TestFetchJobs_ContextCancelledTerminatesEarly verifies a cancelled context
// stops the scan loop without panicking or returning a stale error.
func TestFetchJobs_ContextCancelledTerminatesEarly(t *testing.T) {
	page := makeListingPage(1000,
		parsedJob{ID: "google-1", Title: "SWE", Org: "X", Location: "MV"},
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(page))
	}))
	defer srv.Close()

	cfg := Config{MinDelayMs: 1, MaxDelayMs: 2, TimeoutMs: 5000, MaxPages: 0}
	g := NewGoogleAdapter(cfg, logger.NewNoop())
	g.listingURLOverride = srv.URL + "?"
	g.SetStreamPublish(func(_ []types.FetchJobsItem) {})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before invoking
	_, err := g.FetchJobs(ctx, "google")
	// FetchJobs swallows context errors and returns whatever was committed.
	if err != nil {
		t.Errorf("FetchJobs should not return error on context cancel; got %v", err)
	}
}

// TestSetStreamPublish_StoresCallback verifies SetStreamPublish wires the
// callback into the adapter so streaming publishes route through the engine.
func TestSetStreamPublish_StoresCallback(t *testing.T) {
	g := NewGoogleAdapterDefault()
	called := false
	g.SetStreamPublish(func(_ []types.FetchJobsItem) { called = true })
	g.streamPublish([]types.FetchJobsItem{{ID: "x"}})
	if !called {
		t.Error("SetStreamPublish did not store the callback")
	}
}

// TestSleep_ContextCancellationReturnsErr verifies the inter-page pacing
// helper returns ctx.Err() promptly when cancelled, instead of blocking
// for the full random interval — required so a SIGTERM during a long
// 20-40 s pause does not stall shutdown.
func TestSleep_ContextCancellationReturnsErr(t *testing.T) {
	g := NewGoogleAdapter(Config{MinDelayMs: 10000, MaxDelayMs: 20000}, logger.NewNoop())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := g.sleep(ctx, 10000, 20000); err == nil {
		t.Error("expected ctx.Err on cancelled context")
	}
}

// TestSleep_MinEqualsMaxStillTerminates verifies the off-by-one guard:
// when minMs == maxMs, the helper bumps maxMs to minMs+1 so rand.Intn does
// not panic on zero. A direct equal-bounds call must still return cleanly.
func TestSleep_MinEqualsMaxStillTerminates(t *testing.T) {
	g := NewGoogleAdapter(Config{}, logger.NewNoop())
	if err := g.sleep(context.Background(), 1, 1); err != nil {
		t.Errorf("sleep with min==max: %v", err)
	}
}

// ── FetchJobs — inter-page pagination and publish-loop cancellation ──────────

// TestFetchJobs_MultiPagePaginates verifies pageNum advances across pages when
// total > pageSize (20) and MaxPages is 0. Two page fetches must occur.
func TestFetchJobs_MultiPagePaginates(t *testing.T) {
	// total=21: page 1 * 20 = 20 < 21, so a second page is requested.
	// Page 2 also returns total=21; 2*20=40 >= 21, so loop ends.
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		page := makeListingPage(21,
			parsedJob{ID: "google-" + strconv.Itoa(hits*100), Title: "SWE", Org: "X", Location: "MV"},
		)
		_, _ = w.Write([]byte(page))
	}))
	defer srv.Close()

	// MinDelayMs/MaxDelayMs both 1 ms so inter-page sleep is near-instant.
	cfg := Config{MinDelayMs: 1, MaxDelayMs: 2, TimeoutMs: 5000, MaxPages: 0}
	g := NewGoogleAdapter(cfg, logger.NewNoop())
	g.listingURLOverride = srv.URL + "?"
	g.SetStreamPublish(func(_ []types.FetchJobsItem) {})

	resp, err := g.FetchJobs(context.Background(), "google")
	if err != nil {
		t.Fatalf("FetchJobs: %v", err)
	}
	if hits < 2 {
		t.Errorf("expected at least 2 page fetches (total=21 > pageSize=20), got %d", hits)
	}
	if len(resp.Jobs) < 2 {
		t.Errorf("expected at least 2 jobs across pages, got %d", len(resp.Jobs))
	}
}

// TestFetchJobs_SleepCancelledBreaksLoop verifies that when the context is
// cancelled during the inter-page sleep (between page 1 and page 2), FetchJobs
// returns cleanly without error instead of hanging for the full delay duration.
func TestFetchJobs_SleepCancelledBreaksLoop(t *testing.T) {
	// total=21 so a second page would normally be requested after sleep.
	page := makeListingPage(21,
		parsedJob{ID: "google-1", Title: "SWE", Org: "X", Location: "MV"},
	)
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		_, _ = w.Write([]byte(page))
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())

	// MinDelayMs=500 ms — long enough that we can cancel during sleep.
	cfg := Config{MinDelayMs: 500, MaxDelayMs: 1000, TimeoutMs: 10000, MaxPages: 0}
	g := NewGoogleAdapter(cfg, logger.NewNoop())
	g.listingURLOverride = srv.URL + "?"
	// seenChecker cancels ctx after returning, so by the time FetchJobs reaches
	// sleep() the context is already done — sleep returns ctx.Err() immediately.
	g.SetSeenChecker(func(_ context.Context, ids []string) (map[string]bool, error) {
		cancel()
		out := map[string]bool{}
		for _, id := range ids {
			out[id] = false
		}
		return out, nil
	})
	g.SetStreamPublish(func(_ []types.FetchJobsItem) {})

	resp, err := g.FetchJobs(ctx, "google")
	if err != nil {
		t.Fatalf("FetchJobs should not return error on sleep cancellation; got %v", err)
	}
	// Only one page was fetched before the cancel took effect.
	if hits != 1 {
		t.Errorf("expected 1 page fetch before cancel, got %d", hits)
	}
	_ = resp
}

// TestFetchJobs_ContextCancelledDuringPublish verifies that cancelling the
// context mid-publish (after the page is fetched and filtered but while
// iterating cards to publish) breaks out of the inner loop via break scanLoop
// rather than continuing to publish remaining cards.
func TestFetchJobs_ContextCancelledDuringPublish(t *testing.T) {
	// Build a page with 5 cards — we cancel after the first publish call.
	jobs := make([]parsedJob, 5)
	for i := range jobs {
		jobs[i] = parsedJob{ID: "google-" + strconv.Itoa(i+1), Title: "SWE", Org: "X", Location: "MV"}
	}
	page := makeListingPage(5, jobs...)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(page))
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())

	cfg := Config{MinDelayMs: 1, MaxDelayMs: 2, TimeoutMs: 5000, MaxPages: 1}
	g := NewGoogleAdapter(cfg, logger.NewNoop())
	g.listingURLOverride = srv.URL + "?"

	var publishCount int
	g.SetStreamPublish(func(_ []types.FetchJobsItem) {
		publishCount++
		// Cancel after the first publish so the inner loop hits ctx.Err() != nil.
		cancel()
	})

	resp, err := g.FetchJobs(ctx, "google")
	if err != nil {
		t.Fatalf("FetchJobs should not return error on mid-publish cancel; got %v", err)
	}
	// At least one job was published before cancel.
	if publishCount == 0 {
		t.Error("expected at least one publish before cancel")
	}
	// We should NOT have published all 5 cards — cancel interrupted the loop.
	if publishCount == 5 {
		t.Errorf("context cancel during publish had no effect — all 5 items published")
	}
	_ = resp
}

// ── fetch — error branches ───────────────────────────────────────────────────

// TestFetch_RequestBuildError verifies fetch returns a "new request" error
// when the URL contains a control character that http.NewRequestWithContext
// rejects before making any network call.
func TestFetch_RequestBuildError(t *testing.T) {
	g := NewGoogleAdapterDefault()
	// A null byte in the URL causes http.NewRequestWithContext to return an error.
	_, err := g.fetch(context.Background(), "http://example.com/\x00bad")
	if err == nil {
		t.Fatal("expected error from invalid URL (null byte), got nil")
	}
}

// TestFetch_NetworkError verifies fetch returns an "http do" error when the
// server closes the connection before responding (simulates a network failure).
func TestFetch_NetworkError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Hijack the connection and close it immediately — causes client.Do to fail.
		hj, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "no hijack", http.StatusInternalServerError)
			return
		}
		conn, _, _ := hj.Hijack()
		_ = conn.Close()
	}))
	defer srv.Close()

	g := NewGoogleAdapterDefault()
	_, err := g.fetch(context.Background(), srv.URL+"/test")
	if err == nil {
		t.Fatal("expected network error from hijacked connection, got nil")
	}
}

// TestFetch_Non200Status verifies fetch returns an "http status N" error
// when the server returns a non-200 status code. This complements the existing
// TestFetchJobContent_ReturnsErrorOnNon200 by testing the fetch helper directly.
func TestFetch_Non200Status(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	g := NewGoogleAdapterDefault()
	_, err := g.fetch(context.Background(), srv.URL+"/jobs")
	if err == nil {
		t.Fatal("expected error on 503, got nil")
	}
}

// TestFetch_BodyReadError verifies fetch returns a "read body" error when the
// server closes the TCP connection in the middle of writing the body. This
// triggers io.ReadAll to return an unexpected-EOF error after receiving the
// response headers but before the full body is delivered.
func TestFetch_BodyReadError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Error("server does not support hijacking")
			return
		}
		conn, buf, err := hj.Hijack()
		if err != nil {
			t.Errorf("hijack: %v", err)
			return
		}
		defer func() { _ = conn.Close() }()
		// Write a valid 200 OK with Content-Length larger than the actual body
		// so the client's io.ReadAll blocks waiting for more bytes, then close
		// the connection — producing an unexpected-EOF read error.
		_, _ = buf.WriteString("HTTP/1.1 200 OK\r\nContent-Length: 10000\r\n\r\npartial")
		_ = buf.Flush()
		// Closing conn here causes the client to receive EOF before Content-Length bytes.
	}))
	defer srv.Close()

	g := NewGoogleAdapterDefault()
	_, err := g.fetch(context.Background(), srv.URL+"/test")
	if err == nil {
		t.Fatal("expected error from truncated response body, got nil")
	}
}

// ── parseListing — degenerate HTML branches ─────────────────────────────────

// TestParseListing_SliceEndBeforeSliceStart verifies the guard that skips
// blocks where sliceEnd <= sliceStart. This fires when two aria-label matches
// are immediately character-adjacent so the inter-label block is zero-length.
func TestParseListing_SliceEndBeforeSliceStart(t *testing.T) {
	// Two aria-labels that are literally touching: the closing quote of the
	// first aria-label attribute value is immediately followed by the opening
	// of the second, leaving a zero-length block for card[1].
	// This is the exact byte shape that triggers sliceEnd == sliceStart.
	//   titleIdx[0][1] is the byte after "Job One"'s closing quote.
	//   titleIdx[1][0] is the byte of "Job Two"'s opening 'a' in aria-label.
	// When the two matches are adjacent, those two values are equal.
	html := `<html><body>` +
		`<div>Showing 1 to 20 of 100 rows</div>` +
		`<a href="jobs/results/111-job-one?q=x" aria-label="Learn more about Job One"` +
		// No whitespace or characters between the two aria-label attributes:
		`aria-label="Learn more about Job Two"></a>` +
		`</body></html>`

	cards, total := parseListing(html)
	if total != 100 {
		t.Errorf("total: got %d, want 100", total)
	}
	// The guard fires for card[1] (zero-length block) — must not panic,
	// and must not emit more cards than there are valid anchors.
	if len(cards) > 2 {
		t.Errorf("expected at most 2 cards, got %d", len(cards))
	}
}

// TestParseListing_ExtraAriaLabelNoHref verifies the "21st anchor" case:
// an aria-label anchor whose preceding block contains no href is silently
// skipped — matching the live Google page behavior where the search input
// emits a "Learn more" anchor with no card content.
func TestParseListing_ExtraAriaLabelNoHref(t *testing.T) {
	html := `<html><body>
<div>Showing 1 to 20 of 20 rows</div>
<ul>
  <li>
    <i class="google-material-icons">corporate_fare</i><span><span>Org</span></span>
    <i class="google-material-icons">place</i><span><span>Location</span></span>
    <button>Save</button>
    <a href="jobs/results/123-software-engineer?q=x" aria-label="Learn more about Software Engineer"></a>
  </li>
  <li>
    <!-- No href anchor before this aria-label — the 21st-anchor case -->
    <a aria-label="Learn more about Phantom Job"></a>
  </li>
</ul>
</body></html>`
	cards, _ := parseListing(html)
	// The phantom job (no href in its block) must be silently skipped.
	for _, c := range cards {
		if c.Title == "Phantom Job" {
			t.Errorf("phantom job with no href should be skipped, but was emitted: %+v", c)
		}
	}
}

// ── extractDescription — error and dedup branches ───────────────────────────

// TestExtractDescription_JSONDecodeError verifies that strings containing the
// < escape sequence but with surrounding malformed JSON are silently
// skipped (json.Unmarshal fails → continue).
func TestExtractDescription_JSONDecodeError(t *testing.T) {
	// Craft a ds:0 block that contains an unterminated string with <.
	// The state machine identifies it as a candidate but json.Unmarshal fails.
	// Build the string as two halves so the literal six-byte escape survives.
	const escapedLT = `\` + `u003c`
	// An opening quote, the escape sequence, some content, and NO closing quote.
	badString := `"` + escapedLT + `p>Unterminated`
	html := `<html><body><script>AF_initDataCallback({key: 'ds:0', hash: '1', data:[` +
		badString +
		`]});</script></body></html>`

	// Must not panic; result may be empty.
	got := extractDescription(html)
	_ = got
}

// TestExtractDescription_EmptyAfterStripHTML verifies that a decoded string
// whose content is entirely HTML tags (no visible text) is skipped via the
// `plain == ""` guard, so no empty entry appears in the output.
func TestExtractDescription_EmptyAfterStripHTML(t *testing.T) {
	// json.Marshal of "<br/><hr/>" → JSON string containing <br/><hr/>
	tagsOnly := "<br/><hr/>"
	quoted, _ := json.Marshal(tagsOnly)

	html := `<html><body><script>AF_initDataCallback({key: 'ds:0', hash: '1', data:[[` +
		string(quoted) +
		`]]});</script></body></html>`

	got := extractDescription(html)
	if got != "" {
		t.Errorf("expected empty description for tag-only content, got %q", got)
	}
}

// TestExtractDescription_DuplicateSegmentDeduped verifies that when the same
// content string appears more than once in the ds:0 block, only one copy is
// included in the output (the `seen` map deduplication guard).
func TestExtractDescription_DuplicateSegmentDeduped(t *testing.T) {
	repeated := "<p>Minimum qualifications: 5 years of experience.</p>"
	q1, _ := json.Marshal(repeated)
	q2, _ := json.Marshal(repeated)

	html := `<html><body><script>AF_initDataCallback({key: 'ds:0', hash: '1', data:[[` +
		string(q1) + `,` + string(q2) +
		`]]});</script></body></html>`

	got := extractDescription(html)
	expected := "Minimum qualifications: 5 years of experience."

	if !strings.Contains(got, expected) {
		t.Errorf("description missing expected content; got %q", got)
	}
	// Count occurrences — should appear exactly once after deduplication.
	if count := strings.Count(got, expected); count != 1 {
		t.Errorf("expected deduped content to appear exactly once, got %d occurrences", count)
	}
}
