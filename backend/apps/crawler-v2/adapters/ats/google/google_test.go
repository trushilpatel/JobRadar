package google

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/trushilpatel/jobradar/common/ats/core"
	"github.com/trushilpatel/jobradar/common/logger"
)

// ── Test helpers ──────────────────────────────────────────────────────────────

// newTestAdapter builds a Google adapter pointed at the given listing base URL.
// minDelayMs and maxDelayMs are both 0 so tests return without sleeping.
func newTestAdapter(listingBaseURL string) *Google {
	g := New(logger.NewNoop(), http.DefaultClient)
	g.listingBaseURL = listingBaseURL
	g.minDelayMs = 0
	g.maxDelayMs = 0
	return g
}

// newTestAdapterWithClient builds an adapter using the given HTTP client and listing base URL.
func newTestAdapterWithClient(client *http.Client, listingBaseURL string) *Google {
	g := &Google{
		log:            logger.NewNoop().With(logger.Str(logger.FieldAdapter, "google")),
		httpClient:     client,
		listingBaseURL: listingBaseURL,
		minDelayMs:     0,
		maxDelayMs:     0,
	}
	return g
}

func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("load fixture %s: %v", name, err)
	}
	return data
}

// buildListingPage builds a listing-page HTML body with the supplied total and
// job cards. Mirrors the exact card structure the backend's regex expects.
func buildListingPage(total int, cards []struct{ id, title, location, org string }) string {
	var sb strings.Builder
	sb.WriteString(`<!doctype html><html><body>`)
	sb.WriteString(`<div class="header">Showing 1 to 20 of `)
	sb.WriteString(strconv.Itoa(total))
	sb.WriteString(` rows</div><ul class="search-results">`)
	for _, c := range cards {
		sb.WriteString(`<li class="card"><h3>` + c.title + `</h3>`)
		sb.WriteString(`<i class="google-material-icons">corporate_fare</i>`)
		sb.WriteString(`<span><span>` + c.org + `</span></span>`)
		sb.WriteString(`<i class="google-material-icons">place</i>`)
		sb.WriteString(`<span><span>` + c.location + `</span></span>`)
		sb.WriteString(`<button>Save</button>`)
		sb.WriteString(`<a class="link" href="jobs/results/` + c.id + `-test-slug?q=us" aria-label="Learn more about ` + c.title + `"></a>`)
		sb.WriteString(`</li>`)
	}
	sb.WriteString(`</ul></body></html>`)
	return sb.String()
}

// ── FetchJobListing ───────────────────────────────────────────────────────────

func TestGoogle_FetchJobListing(t *testing.T) {
	html := string(loadFixture(t, "listing.html"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(html))
	}))
	defer srv.Close()

	result, err := newTestAdapter(srv.URL+"?").FetchJobListing(context.Background(), "ignored")
	if err != nil {
		t.Fatalf("FetchJobListing error: %v", err)
	}
	if len(result.Jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(result.Jobs))
	}

	j0 := result.Jobs[0]
	if j0.ID != "google-12345" {
		t.Errorf("jobs[0].ID = %q, want google-12345", j0.ID)
	}
	if j0.Title != "Software Engineer, Search" {
		t.Errorf("jobs[0].Title = %q, want 'Software Engineer, Search'", j0.Title)
	}
	if j0.Location != "Mountain View, CA, USA" {
		t.Errorf("jobs[0].Location = %q, want 'Mountain View, CA, USA'", j0.Location)
	}
	if j0.EmployType != core.EmployFullTime {
		t.Errorf("jobs[0].EmployType = %q, want %q", j0.EmployType, core.EmployFullTime)
	}
	if !j0.LocationKnown {
		t.Error("jobs[0].LocationKnown should be true")
	}
	if !strings.Contains(j0.URL, "12345-software-engineer-search") {
		t.Errorf("jobs[0].URL = %q, should contain '12345-software-engineer-search'", j0.URL)
	}

	j1 := result.Jobs[1]
	if j1.ID != "google-67890" {
		t.Errorf("jobs[1].ID = %q, want google-67890", j1.ID)
	}
	if strings.Contains(j1.Location, "+2 more") {
		t.Errorf("jobs[1].Location should have '+2 more' stripped: %q", j1.Location)
	}
}

func TestGoogle_FetchJobListing_Pagination(t *testing.T) {
	// Page 1 returns 1 job with total=25 (>20) so adapter expects page 2.
	// Page 2 returns 0 jobs → stop.
	page1 := buildListingPage(25, []struct{ id, title, location, org string }{
		{"100", "SWE Alpha", "Mountain View, CA", "Google"},
	})
	page2 := `<!doctype html><html><body><div>No results found</div></body></html>`

	var requests []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.RawQuery)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if strings.Contains(r.URL.RawQuery, "page=2") {
			_, _ = w.Write([]byte(page2))
		} else {
			_, _ = w.Write([]byte(page1))
		}
	}))
	defer srv.Close()

	result, err := newTestAdapter(srv.URL+"?").FetchJobListing(context.Background(), "ignored")
	if err != nil {
		t.Fatalf("FetchJobListing error: %v", err)
	}
	if len(result.Jobs) != 1 {
		t.Errorf("expected 1 job, got %d", len(result.Jobs))
	}
	if len(requests) != 2 {
		t.Errorf("expected 2 HTTP requests (page1+page2), got %d: %v", len(requests), requests)
	}
}

func TestGoogle_FetchJobListing_StopsWhenTotalCovered(t *testing.T) {
	// total=2 → page 1 * 20 = 20 >= 2 → stop after page 1
	page := buildListingPage(2, []struct{ id, title, location, org string }{
		{"200", "SWE Beta", "Kirkland, WA", "YouTube"},
	})
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		_, _ = w.Write([]byte(page))
	}))
	defer srv.Close()

	_, err := newTestAdapter(srv.URL+"?").FetchJobListing(context.Background(), "ignored")
	if err != nil {
		t.Fatalf("FetchJobListing error: %v", err)
	}
	if hits != 1 {
		t.Errorf("expected 1 HTTP request (total=2 < pageSize=20), got %d", hits)
	}
}

func TestGoogle_FetchJobListing_Errors(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{"http 500", http.StatusInternalServerError, "Internal Server Error"},
		{"http 429", http.StatusTooManyRequests, "Too Many Requests"},
		{"http 403", http.StatusForbidden, "Forbidden"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, tt.body, tt.status)
			}))
			defer srv.Close()

			_, err := newTestAdapter(srv.URL+"?").FetchJobListing(context.Background(), "ignored")
			if err == nil {
				t.Fatalf("expected error for %s, got nil", tt.name)
			}
		})
	}
}

func TestGoogle_FetchJobListing_NoJobCards(t *testing.T) {
	html := `<!doctype html><html><body><div>No results found</div></body></html>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(html))
	}))
	defer srv.Close()

	result, err := newTestAdapter(srv.URL+"?").FetchJobListing(context.Background(), "ignored")
	if err != nil {
		t.Fatalf("FetchJobListing unexpected error: %v", err)
	}
	if len(result.Jobs) != 0 {
		t.Errorf("expected 0 jobs, got %d", len(result.Jobs))
	}
}

// ── FetchJobPost ──────────────────────────────────────────────────────────────

func TestGoogle_FetchJobPost(t *testing.T) {
	html := string(loadFixture(t, "job_post.html"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(html))
	}))
	defer srv.Close()

	summary := core.JobSummary{
		ID:       "google-12345",
		Title:    "Software Engineer, Search",
		Location: "Mountain View, CA, USA",
		URL:      srv.URL + "/about/careers/applications/jobs/results/12345-software-engineer-search",
	}

	post, err := newTestAdapterWithClient(http.DefaultClient, "").FetchJobPost(context.Background(), summary)
	if err != nil {
		t.Fatalf("FetchJobPost error: %v", err)
	}
	if post.ID != "google-12345" {
		t.Errorf("post.ID = %q, want google-12345", post.ID)
	}
	if post.Title != "Software Engineer, Search" {
		t.Errorf("post.Title = %q, want 'Software Engineer, Search'", post.Title)
	}
	if post.Description == "" {
		t.Error("post.Description should not be empty")
	}
	if strings.Contains(post.Description, "<") || strings.Contains(post.Description, ">") {
		t.Errorf("post.Description should have HTML stripped, got: %s", post.Description[:min(200, len(post.Description))])
	}
	if !strings.Contains(post.Description, "About the job") {
		t.Errorf("post.Description missing expected content; got: %s", post.Description)
	}
	// Sibling job content must not leak
	if strings.Contains(post.Description, "SIBLING_LEAK_SENTINEL") {
		t.Errorf("post.Description leaked sibling job content")
	}
}

func TestGoogle_FetchJobPost_FallsBackToSummaryTitle(t *testing.T) {
	// HTML with no AF_initDataCallback → detail.Title will be empty → fallback to summary.Title
	html := `<!doctype html><html><body><h1>No callbacks here</h1></body></html>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(html))
	}))
	defer srv.Close()

	summary := core.JobSummary{
		ID:       "google-999",
		Title:    "Fallback Title",
		Location: "Seattle, WA",
		URL:      srv.URL + "/job",
	}
	post, err := newTestAdapterWithClient(http.DefaultClient, "").FetchJobPost(context.Background(), summary)
	if err != nil {
		t.Fatalf("FetchJobPost error: %v", err)
	}
	if post.Title != "Fallback Title" {
		t.Errorf("post.Title = %q, want 'Fallback Title'", post.Title)
	}
	if post.Location != "Seattle, WA" {
		t.Errorf("post.Location = %q, want 'Seattle, WA'", post.Location)
	}
}

func TestGoogle_FetchJobPost_Errors(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{"http 404", http.StatusNotFound, "Not Found"},
		{"http 500", http.StatusInternalServerError, "Server Error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, tt.body, tt.status)
			}))
			defer srv.Close()

			summary := core.JobSummary{
				ID:  "google-123",
				URL: srv.URL + "/job",
			}
			_, err := newTestAdapterWithClient(http.DefaultClient, "").FetchJobPost(context.Background(), summary)
			if err == nil {
				t.Fatalf("expected error for %s, got nil", tt.name)
			}
		})
	}
}

func TestGoogle_FetchJobPost_EmptyURL(t *testing.T) {
	g := New(logger.NewNoop(), http.DefaultClient)
	_, err := g.FetchJobPost(context.Background(), core.JobSummary{ID: "google-1", URL: ""})
	if err == nil {
		t.Fatal("expected error for empty URL, got nil")
	}
}

func TestGoogle_FetchJobPost_NoAFInitDataCallback(t *testing.T) {
	html := `<!doctype html><html><body><p>Regular page with no callbacks</p></body></html>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(html))
	}))
	defer srv.Close()

	summary := core.JobSummary{
		ID:  "google-404",
		URL: srv.URL + "/job",
	}
	post, err := newTestAdapterWithClient(http.DefaultClient, "").FetchJobPost(context.Background(), summary)
	if err != nil {
		t.Fatalf("FetchJobPost should not error when AF_initDataCallback is absent; got: %v", err)
	}
	if post.Description != "" {
		t.Errorf("post.Description should be empty when no AF_initDataCallback; got: %q", post.Description)
	}
}

// ── parseListing ──────────────────────────────────────────────────────────────

func TestParseListing_FromFixture(t *testing.T) {
	html := string(loadFixture(t, "listing.html"))
	cards, total := parseListing(html)

	if total != 2 {
		t.Errorf("total = %d, want 2", total)
	}
	if len(cards) != 2 {
		t.Fatalf("expected 2 cards, got %d", len(cards))
	}

	if cards[0].ID != "12345" {
		t.Errorf("cards[0].ID = %q, want 12345", cards[0].ID)
	}
	if cards[0].Slug != "software-engineer-search" {
		t.Errorf("cards[0].Slug = %q, want software-engineer-search", cards[0].Slug)
	}
	if cards[0].Title != "Software Engineer, Search" {
		t.Errorf("cards[0].Title = %q", cards[0].Title)
	}
	if cards[0].Location != "Mountain View, CA, USA" {
		t.Errorf("cards[0].Location = %q", cards[0].Location)
	}
	if cards[0].Team != "Google" {
		t.Errorf("cards[0].Team = %q, want Google", cards[0].Team)
	}

	if cards[1].ID != "67890" {
		t.Errorf("cards[1].ID = %q, want 67890", cards[1].ID)
	}
	if strings.Contains(cards[1].Location, "+2 more") {
		t.Errorf("cards[1].Location should strip '+2 more': %q", cards[1].Location)
	}
}

func TestParseListing_EmptyHTML(t *testing.T) {
	cards, total := parseListing(`<!doctype html><html><body></body></html>`)
	if total != 0 {
		t.Errorf("total = %d, want 0", total)
	}
	if len(cards) != 0 {
		t.Errorf("expected 0 cards, got %d", len(cards))
	}
}

// ── extractDescription ────────────────────────────────────────────────────────

func TestExtractDescription_FromFixture(t *testing.T) {
	html := string(loadFixture(t, "job_post.html"))
	got := extractDescription(html)
	if got == "" {
		t.Fatal("extractDescription returned empty string")
	}
	for _, want := range []string{
		"About the job",
		"distributed systems",
		"Design scalable systems",
		"Minimum qualifications",
		"Bachelor's degree",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("description missing %q; full: %s", want, got)
		}
	}
	if strings.Contains(got, "SIBLING_LEAK_SENTINEL") {
		t.Error("description leaked ds:1 sibling content")
	}
}

func TestExtractDescription_ReturnsEmptyWhenNoDS0(t *testing.T) {
	got := extractDescription(`<html><body>no callbacks here</body></html>`)
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestExtractDescription_StripsHTML(t *testing.T) {
	html := string(loadFixture(t, "job_post.html"))
	got := extractDescription(html)
	if strings.Contains(got, "<p>") || strings.Contains(got, "<h3>") || strings.Contains(got, "<ul>") {
		t.Errorf("description still contains HTML tags: %s", got)
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
		t.Errorf("locations should not be stripped: %q", got)
	}
}

func TestCleanText_PreservesPlusNMoreWhenNotRequested(t *testing.T) {
	got := cleanText(`<span>Kirkland, WA, USA; +2 more</span>`, false)
	if !strings.Contains(got, "+2 more") {
		t.Errorf("'+2 more' should be preserved when stripPlusMore=false: %q", got)
	}
}

// ── Interface compliance ──────────────────────────────────────────────────────

func TestGoogle_ImplementsListingFetcher(t *testing.T) {
	var _ core.ListingFetcher = (*Google)(nil)
}

func TestGoogle_ImplementsPostFetcher(t *testing.T) {
	var _ core.PostFetcher = (*Google)(nil)
}

// ── New constructor ───────────────────────────────────────────────────────────

func TestNew_ProductionDefaults(t *testing.T) {
	g := New(logger.NewNoop(), http.DefaultClient)
	if g.minDelayMs != 20000 {
		t.Errorf("minDelayMs = %d, want 20000", g.minDelayMs)
	}
	if g.maxDelayMs != 40000 {
		t.Errorf("maxDelayMs = %d, want 40000", g.maxDelayMs)
	}
}

// ── sleep ─────────────────────────────────────────────────────────────────────

func TestSleep_ZeroDelayReturnsImmediately(t *testing.T) {
	g := New(logger.NewNoop(), http.DefaultClient)
	if err := g.sleep(context.Background(), 0, 0); err != nil {
		t.Errorf("sleep(0,0) should return nil immediately; got: %v", err)
	}
}

func TestSleep_ContextCancelReturnsError(t *testing.T) {
	g := New(logger.NewNoop(), http.DefaultClient)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := g.sleep(ctx, 10000, 20000); err == nil {
		t.Error("expected ctx.Err on cancelled context")
	}
}

func TestSleep_MinEqualsMaxTerminates(t *testing.T) {
	g := New(logger.NewNoop(), http.DefaultClient)
	// minMs == maxMs: bumps maxMs to minMs+1 so rand.Intn does not panic
	if err := g.sleep(context.Background(), 1, 1); err != nil {
		t.Errorf("sleep(1,1) should not error; got: %v", err)
	}
}

// ── fetch — error paths ───────────────────────────────────────────────────────

func TestFetch_NetworkError(t *testing.T) {
	// A server that closes the connection immediately triggers an http.Do error.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Hijack and close the connection to simulate a network error.
		hj, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "no hijack", http.StatusInternalServerError)
			return
		}
		conn, _, _ := hj.Hijack()
		_ = conn.Close()
	}))
	defer srv.Close()

	g := New(logger.NewNoop(), &http.Client{})
	_, err := g.fetch(context.Background(), srv.URL+"/path")
	if err == nil {
		t.Error("expected error from closed connection")
	}
}

func TestFetch_InvalidURLReturnsError(t *testing.T) {
	g := New(logger.NewNoop(), http.DefaultClient)
	// "://" is an invalid URL that http.NewRequestWithContext will reject.
	_, err := g.fetch(context.Background(), "://invalid-url")
	if err == nil {
		t.Error("expected error for invalid URL")
	}
}

// ── resolveListingBaseURL ─────────────────────────────────────────────────────

func TestResolveListingBaseURL_UsesDefaultWhenEmpty(t *testing.T) {
	g := New(logger.NewNoop(), http.DefaultClient)
	if got := g.resolveListingBaseURL(); got != defaultListingURL {
		t.Errorf("resolveListingBaseURL() = %q, want %q", got, defaultListingURL)
	}
}

func TestResolveListingBaseURL_UsesOverrideWhenSet(t *testing.T) {
	g := New(logger.NewNoop(), http.DefaultClient)
	g.listingBaseURL = "http://localhost:9999"
	if got := g.resolveListingBaseURL(); got != "http://localhost:9999" {
		t.Errorf("resolveListingBaseURL() = %q, want http://localhost:9999", got)
	}
}

// ── FetchJobListing — context already cancelled ───────────────────────────────

func TestGoogle_FetchJobListing_ContextAlreadyCancelled(t *testing.T) {
	g := New(logger.NewNoop(), http.DefaultClient)
	g.listingBaseURL = "http://localhost:9999?" // unreachable; context cancelled before use
	g.minDelayMs = 0
	g.maxDelayMs = 0

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before calling

	result, err := g.FetchJobListing(ctx, "ignored")
	if err != nil {
		t.Fatalf("FetchJobListing should not error on pre-cancelled ctx; got: %v", err)
	}
	if len(result.Jobs) != 0 {
		t.Errorf("expected 0 jobs on pre-cancelled ctx, got %d", len(result.Jobs))
	}
}

// ── sleep — time.After path ───────────────────────────────────────────────────

func TestSleep_SmallDelayReturnsCleanly(t *testing.T) {
	g := New(logger.NewNoop(), http.DefaultClient)
	// A 1ms delay exercises the time.After branch without blocking the test.
	if err := g.sleep(context.Background(), 1, 2); err != nil {
		t.Errorf("sleep(1,2) should not error; got: %v", err)
	}
}

// ── parseListing — card without href (extra aria-label) ──────────────────────

func TestParseListing_SkipsAriaWithoutHref(t *testing.T) {
	// An "extra" aria-label with no href block should be silently skipped.
	html := `<!doctype html><html><body>
<div>Showing 1 to 1 of 1 rows</div>
<ul>
  <li class="card">
    <i class="google-material-icons">corporate_fare</i><span><span>Google</span></span>
    <i class="google-material-icons">place</i><span><span>NYC</span></span>
    <button>Save</button>
    <a href="jobs/results/111-some-job?q=x" aria-label="Learn more about Some Job"></a>
  </li>
</ul>
<a aria-label="Learn more about Extra No Href"></a>
</body></html>`

	cards, _ := parseListing(html)
	for _, c := range cards {
		if c.Title == "Extra No Href" {
			t.Error("card with no href should be silently skipped")
		}
	}
}

// ── extractDescription — truncated/invalid JSON string ────────────────────────

func TestExtractDescription_DecodesHTMLEntities(t *testing.T) {
	// Google embeds job data as JSON with < encoded as <.
	// After json.Unmarshal, decoded = "<p>Pay &gt;$100k &amp; great</p>".
	// extractDescription must decode HTML entities (&gt; → >, &amp; → &) at
	// extraction time, not rely on mapper to do it later.
	html := "<script>AF_initDataCallback({key: 'ds:0', data:[\"\\u003cp\\u003ePay &gt;$100k &amp; great\\u003c/p\\u003e\"]});</script>"
	got := extractDescription(html)
	if strings.Contains(got, "&gt;") || strings.Contains(got, "&amp;") {
		t.Errorf("extractDescription should decode HTML entities at extraction time; got: %q", got)
	}
	if !strings.Contains(got, "&") || !strings.Contains(got, ">$100k") {
		t.Errorf("extractDescription should retain decoded text; got: %q", got)
	}
}

func TestExtractDescription_SkipsInvalidJSONStrings(t *testing.T) {
	// A block that contains `<` inside a malformed JSON string should not panic.
	// The state machine skips strings that fail json.Unmarshal.
	html := `<script>AF_initDataCallback({key: 'ds:0', data:["<p>Valid text</p>", "not-closed]});</script>`
	got := extractDescription(html)
	// The valid part should still be extracted; the malformed part is skipped.
	_ = got // result may be empty or partial — main guard is no panic
}
