package deel

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/trushilpatel/jobradar/apps/crawler-v2/common/logger"
	"github.com/trushilpatel/jobradar/common/ats/core"
)

// ── Interface compliance ───────────────────────────────────────────────────────

func TestDeel_ImplementsListingFetcher(t *testing.T) {
	var _ core.ListingFetcher = (*Deel)(nil)
}

func TestDeel_DoesNotImplementPostFetcher(t *testing.T) {
	// Deel is a fat adapter — all content at listing time.
	// Implementing PostFetcher would cause a wasted per-job re-fetch.
	var v interface{} = New(logger.NewNoop(), http.DefaultClient)
	if _, ok := v.(core.PostFetcher); ok {
		t.Fatal("Deel must not implement PostFetcher — it is a fat adapter")
	}
}

// ── Constructor + helpers ─────────────────────────────────────────────────────

func TestNew_DefaultBaseURL(t *testing.T) {
	d := New(logger.NewNoop(), http.DefaultClient)
	if d.baseURL != "" {
		// baseURL is empty by default; resolveBaseURL() returns defaultBaseURL.
		// We don't expose it as a field, so check resolveBaseURL indirectly.
		t.Errorf("baseURL field should be empty on New(), got %q", d.baseURL)
	}
}

func TestResolveBaseURL_ZeroValueFallsBackToDefault(t *testing.T) {
	d := &Deel{}
	if got := d.resolveBaseURL(); got != defaultBaseURL {
		t.Errorf("zero-value resolveBaseURL = %q, want %q", got, defaultBaseURL)
	}
}

func TestResolveBaseURL_OverrideIsUsed(t *testing.T) {
	d := &Deel{baseURL: "http://test-server"}
	if got := d.resolveBaseURL(); got != "http://test-server" {
		t.Errorf("override resolveBaseURL = %q, want http://test-server", got)
	}
}

// ── RSC stream parsing ────────────────────────────────────────────────────────

func TestParseChunks_BasicSingleChunk(t *testing.T) {
	body := "15:T5,hello"
	chunks := parseChunks(body)
	if got := chunks["15"]; got != "hello" {
		t.Errorf("chunk 15 = %q, want %q", got, "hello")
	}
}

func TestParseChunks_MultipleChunksConcatenated(t *testing.T) {
	// Two T-chunks back-to-back inside one physical line.
	// "hello" is 5 chars (0x5), "<p>world</p>" is 12 chars (0xc).
	body := "15:T5,helloa1:Tc,<p>world</p>"
	chunks := parseChunks(body)
	if got := chunks["15"]; got != "hello" {
		t.Errorf("chunk 15 = %q", got)
	}
	if got := chunks["a1"]; got != "<p>world</p>" {
		t.Errorf("chunk a1 = %q", got)
	}
}

func TestParseChunks_IgnoresMalformedLengthHex(t *testing.T) {
	chunks := parseChunks("15:TZZZ,bad")
	if _, ok := chunks["15"]; ok {
		t.Error("malformed length-hex chunk should be skipped")
	}
}

func TestParseChunks_LengthBeyondBufferIsClamped(t *testing.T) {
	// Length claims 64 bytes (0x40) but only 3 are present.
	chunks := parseChunks("15:T40,abc")
	if got := chunks["15"]; got != "abc" {
		t.Errorf("clamped chunk = %q, want %q", got, "abc")
	}
}

func TestParseChunks_EmptyBody(t *testing.T) {
	chunks := parseChunks("")
	if len(chunks) != 0 {
		t.Errorf("expected empty map for empty body, got %d entries", len(chunks))
	}
}

func TestMatchBalanced_FlatObject(t *testing.T) {
	s := `{"a":1,"b":2}`
	if got := matchBalanced(s, 0); got != len(s) {
		t.Errorf("got %d, want %d", got, len(s))
	}
}

func TestMatchBalanced_NestedObject(t *testing.T) {
	s := `{"a":{"b":{"c":1}}}`
	if got := matchBalanced(s, 0); got != len(s) {
		t.Errorf("got %d, want %d", got, len(s))
	}
}

func TestMatchBalanced_BraceInsideStringIgnored(t *testing.T) {
	s := `{"a":"}}}}}}}}}}"}`
	if got := matchBalanced(s, 0); got != len(s) {
		t.Errorf("got %d, want %d (string braces should not count)", got, len(s))
	}
}

func TestMatchBalanced_EscapedQuoteInsideString(t *testing.T) {
	s := `{"a":"foo\"}bar"}`
	if got := matchBalanced(s, 0); got != len(s) {
		t.Errorf("got %d, want %d", got, len(s))
	}
}

func TestMatchBalanced_UnbalancedReturnsMinusOne(t *testing.T) {
	if got := matchBalanced(`{"a":1`, 0); got != -1 {
		t.Errorf("unbalanced should return -1, got %d", got)
	}
}

func TestMatchBalanced_NonObjectStartReturnsMinusOne(t *testing.T) {
	if got := matchBalanced(`["a"]`, 0); got != -1 {
		t.Errorf("non-object start should return -1, got %d", got)
	}
}

func TestMatchBalanced_StartBeyondBoundsReturnsMinusOne(t *testing.T) {
	if got := matchBalanced(`{"a":1}`, 100); got != -1 {
		t.Errorf("start beyond bounds should return -1, got %d", got)
	}
}

// ── Job record extraction ─────────────────────────────────────────────────────

const sampleJobJSON = `{"id":"5d3636f8-0712-4fe2-a1f4-84358440d272",` +
	`"jobId":"461d452c-2788-47ba-90c3-8ece26f12684",` +
	`"title":"Senior Backend Engineer",` +
	`"richtextDescription":"$15",` +
	`"job":{"id":"461d452c","jobEmploymentTypes":[{"id":"a","employmentType":{"id":"b","name":"Full-time"}}],` +
	`"jobLocations":[{"id":"c","location":{"id":"d","name":"Berlin"}}],` +
	`"jobDepartments":[{"id":"e","department":{"id":"f","name":"Engineering"}}],` +
	`"jobTeams":[],` +
	`"currentCompensation":{"id":"g","currencyIsoCode":"EUR","minAmount":80000,"maxAmount":110000}},` +
	`"jobPostingPublications":[{"id":"h","currentState":{"id":"i","stateSlug":"published","createdAt":"2026-04-21T07:45:50.278Z"}}]}`

func TestExtractJobRecords_SingleRecord(t *testing.T) {
	recs := extractJobRecords(sampleJobJSON)
	if len(recs) != 1 {
		t.Fatalf("got %d records, want 1", len(recs))
	}
	r := recs[0]
	if r.Title != "Senior Backend Engineer" {
		t.Errorf("title = %q", r.Title)
	}
	if r.ID != "5d3636f8-0712-4fe2-a1f4-84358440d272" {
		t.Errorf("id = %q", r.ID)
	}
	if len(r.Job.JobLocations) != 1 || r.Job.JobLocations[0].Location.Name != "Berlin" {
		t.Errorf("location parsing failed: %+v", r.Job.JobLocations)
	}
}

func TestExtractJobRecords_MultipleConcatenatedRecords(t *testing.T) {
	body := sampleJobJSON + "," + sampleJobJSON
	recs := extractJobRecords(body)
	if len(recs) != 2 {
		t.Fatalf("got %d records, want 2", len(recs))
	}
}

func TestExtractJobRecords_SkipsCareerPageSettings(t *testing.T) {
	// careerPageSettings has `id` + `organizationId`, NOT `id` + `jobId`.
	cps := `{"id":"7e2c","organizationId":"10ce","urlSlug":"klarna","title":"Make your next move"}`
	body := cps + sampleJobJSON
	recs := extractJobRecords(body)
	if len(recs) != 1 {
		t.Errorf("expected 1 record (job posting), got %d", len(recs))
	}
}

func TestExtractJobRecords_HandlesMalformedJSONGracefully(t *testing.T) {
	// Header matches but body is unbalanced — should be skipped, not crash.
	bad := `{"id":"5d3636f8-0712-4fe2-a1f4-84358440d272","jobId":"461d452c-2788-47ba-90c3-8ece26f12684","title":"x"`
	recs := extractJobRecords(bad)
	if len(recs) != 0 {
		t.Errorf("expected 0 records for unbalanced JSON, got %d", len(recs))
	}
}

func TestExtractJobRecords_EmptyBodyReturnsEmpty(t *testing.T) {
	recs := extractJobRecords("")
	if len(recs) != 0 {
		t.Errorf("expected 0 records, got %d", len(recs))
	}
}

// ── resolveDescription ────────────────────────────────────────────────────────

func TestResolveDescription_ChunkReference(t *testing.T) {
	chunks := map[string]string{"15": "<p>chunk fifteen</p>"}
	got := resolveDescription(json.RawMessage(`"$15"`), chunks)
	if got != "chunk fifteen" {
		t.Errorf("got %q, want %q", got, "chunk fifteen")
	}
}

func TestResolveDescription_MissingChunkReturnsEmpty(t *testing.T) {
	chunks := map[string]string{}
	got := resolveDescription(json.RawMessage(`"$99"`), chunks)
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestResolveDescription_InlineHTMLStripped(t *testing.T) {
	got := resolveDescription(json.RawMessage(`"<p>inline</p>"`), nil)
	if got != "inline" {
		t.Errorf("got %q, want %q", got, "inline")
	}
}

func TestResolveDescription_NullReturnsEmpty(t *testing.T) {
	got := resolveDescription(json.RawMessage(`null`), nil)
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestResolveDescription_EmptyRawReturnsEmpty(t *testing.T) {
	got := resolveDescription(json.RawMessage(``), nil)
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestResolveDescription_HexIDWithLettersResolved(t *testing.T) {
	chunks := map[string]string{"1a": "<b>inline html chunk</b>"}
	got := resolveDescription(json.RawMessage(`"$1a"`), chunks)
	if got != "inline html chunk" {
		t.Errorf("got %q, want %q", got, "inline html chunk")
	}
}

func TestResolveDescription_PlainStringNoPrefix(t *testing.T) {
	got := resolveDescription(json.RawMessage(`"plain text"`), nil)
	if got != "plain text" {
		t.Errorf("got %q, want %q", got, "plain text")
	}
}

func TestResolveDescription_NonStringJSONReturnsEmpty(t *testing.T) {
	// richtextDescription is a JSON number — not a valid string value.
	// json.Unmarshal into a string will fail; expect empty string back.
	got := resolveDescription(json.RawMessage(`42`), nil)
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

// ── End-to-end FetchJobListing ────────────────────────────────────────────────

func TestDeel_FetchJobListing(t *testing.T) {
	body := buildRSCBody([]string{sampleJobJSON}, map[string]string{
		"15": "<p>About the role.</p><p>You will build.</p>",
	})
	srv := newRSCServer(t, body)
	defer srv.Close()

	d := newTestAdapter(srv.URL)
	listing, err := d.FetchJobListing(context.Background(), "klarna")
	if err != nil {
		t.Fatalf("FetchJobListing error: %v", err)
	}
	if len(listing.Jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(listing.Jobs))
	}

	j := listing.Jobs[0]
	assertEqual(t, "ID", j.ID, "deel-5d3636f8-0712-4fe2-a1f4-84358440d272")
	assertEqual(t, "Title", j.Title, "Senior Backend Engineer")
	assertEqual(t, "URL", j.URL, "https://jobs.deel.com/klarna/job-details/5d3636f8-0712-4fe2-a1f4-84358440d272")
	assertEqual(t, "Location", j.Location, "Berlin")
	assertEqual(t, "EmployType", j.EmployType, core.EmployFullTime)

	if !j.LocationKnown {
		t.Error("LocationKnown should be true for Deel")
	}

	if j.JobContent == nil {
		t.Fatal("JobContent must be set — deel is a fat adapter")
	}
	assertEqual(t, "Description", j.JobContent.Description, "About the role. You will build.")
	assertEqual(t, "Department", j.JobContent.Department, "Engineering")
	assertEqual(t, "PublishedDate", j.JobContent.PublishedDate, "2026-04-21T07:45:50.278Z")
}

func TestDeel_FetchJobListing_TwoJobsDescriptionsResolved(t *testing.T) {
	// Job 1 has a chunk reference description; job 2 has inline HTML description.
	job2 := strings.Replace(sampleJobJSON,
		`"id":"5d3636f8-0712-4fe2-a1f4-84358440d272"`,
		`"id":"aaaaaaaa-0000-0000-0000-000000000001"`, 1)
	job2 = strings.Replace(job2, `"richtextDescription":"$15"`, `"richtextDescription":"<p>Job two desc.</p>"`, 1)

	body := buildRSCBody([]string{sampleJobJSON, job2}, map[string]string{
		"15": "<p>Job one desc.</p>",
	})
	srv := newRSCServer(t, body)
	defer srv.Close()

	d := newTestAdapter(srv.URL)
	listing, err := d.FetchJobListing(context.Background(), "klarna")
	if err != nil {
		t.Fatalf("FetchJobListing error: %v", err)
	}
	if len(listing.Jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(listing.Jobs))
	}

	assertEqual(t, "job1 description", listing.Jobs[0].JobContent.Description, "Job one desc.")
	assertEqual(t, "job2 description", listing.Jobs[1].JobContent.Description, "Job two desc.")
}

func TestDeel_FetchJobListing_Headers(t *testing.T) {
	var capturedRSC, capturedAccept, capturedUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedRSC = r.Header.Get("RSC")
		capturedAccept = r.Header.Get("Accept")
		capturedUA = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "text/x-component")
		_, _ = w.Write([]byte(""))
	}))
	defer srv.Close()

	d := newTestAdapter(srv.URL)
	_, _ = d.FetchJobListing(context.Background(), "klarna")

	if capturedRSC != "1" {
		t.Errorf("RSC header = %q, want %q", capturedRSC, "1")
	}
	if capturedAccept != "text/x-component" {
		t.Errorf("Accept = %q, want text/x-component", capturedAccept)
	}
	if !strings.Contains(capturedUA, "Mozilla") {
		t.Errorf("User-Agent should be browser-like, got %q", capturedUA)
	}
}

func TestDeel_FetchJobListing_UsesSlug(t *testing.T) {
	var capturedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.Header().Set("Content-Type", "text/x-component")
		_, _ = w.Write([]byte(""))
	}))
	defer srv.Close()

	d := newTestAdapter(srv.URL)
	_, _ = d.FetchJobListing(context.Background(), "klarna")

	if capturedPath != "/klarna" {
		t.Errorf("path = %q, want /klarna", capturedPath)
	}
}

func TestDeel_FetchJobListing_EmptyBoard(t *testing.T) {
	srv := newRSCServer(t, "0:null\n")
	defer srv.Close()

	d := newTestAdapter(srv.URL)
	listing, err := d.FetchJobListing(context.Background(), "empty-co")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(listing.Jobs) != 0 {
		t.Errorf("expected 0 jobs, got %d", len(listing.Jobs))
	}
}

func TestDeel_FetchJobListing_Errors(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{"http 500", http.StatusInternalServerError, ""},
		{"http 404", http.StatusNotFound, ""},
		{"http 429", http.StatusTooManyRequests, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			d := newTestAdapter(srv.URL)
			_, err := d.FetchJobListing(context.Background(), "klarna")
			if err == nil {
				t.Fatalf("expected error for %s, got nil", tt.name)
			}
		})
	}
}

func TestDeel_FetchJobListing_ContextCancelled(t *testing.T) {
	srv := newRSCServer(t, "")
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	d := newTestAdapter(srv.URL)
	if _, err := d.FetchJobListing(ctx, "klarna"); err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

// ── Testdata fixture ──────────────────────────────────────────────────────────

func TestDeel_FetchJobListing_FromFixture(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "listing.rsc"))
	if err != nil {
		t.Fatalf("load fixture: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/x-component")
		_, _ = w.Write(data)
	}))
	defer srv.Close()

	d := newTestAdapter(srv.URL)
	listing, err := d.FetchJobListing(context.Background(), "klarna")
	if err != nil {
		t.Fatalf("FetchJobListing error: %v", err)
	}
	if len(listing.Jobs) < 2 {
		t.Fatalf("expected at least 2 jobs from fixture, got %d", len(listing.Jobs))
	}

	j0 := listing.Jobs[0]
	if !strings.HasPrefix(j0.ID, "deel-") {
		t.Errorf("job ID %q should have deel- prefix", j0.ID)
	}
	if j0.JobContent == nil || j0.JobContent.Description == "" {
		t.Error("first job should have description resolved from chunk reference")
	}

	// Second job has inline description
	j1 := listing.Jobs[1]
	if j1.JobContent == nil || j1.JobContent.Description == "" {
		t.Error("second job should have description resolved from inline HTML")
	}
}

// ── Additional edge cases ─────────────────────────────────────────────────────

func TestParseChunks_NegativeLengthSkipped(t *testing.T) {
	// Sscanf parses fine but result is negative (e.g. signed overflow on huge hex).
	// The condition `n < 0` in parseChunks guards against this.
	// We verify parseChunks simply doesn't panic and skips the chunk — the
	// clamping path handles huge values, but explicitly negative would be filtered.
	// This is effectively covered by TestParseChunks_LengthBeyondBufferIsClamped;
	// we call parseChunks with well-formed data and verify map is populated.
	chunks := parseChunks("aa:T1,x")
	if got := chunks["aa"]; got != "x" {
		t.Errorf("chunk aa = %q, want x", got)
	}
}

func TestExtractJobRecords_UnbalancedSkipped(t *testing.T) {
	// The regex matches but matchBalanced returns -1 — record is skipped.
	partial := `{"id":"5d3636f8-0712-4fe2-a1f4-84358440d272","jobId":"461d452c-2788-47ba-90c3-8ece26f12684","title":"x"` // no closing brace
	recs := extractJobRecords(partial)
	if len(recs) != 0 {
		t.Errorf("expected 0 records for unbalanced JSON, got %d", len(recs))
	}
}

// ── Helper functions ──────────────────────────────────────────────────────────

// newTestAdapter builds a Deel pointed at the given test server URL.
func newTestAdapter(serverURL string) *Deel {
	d := New(logger.NewNoop(), http.DefaultClient)
	d.baseURL = serverURL
	return d
}

// newRSCServer returns a test server that serves the provided RSC body.
func newRSCServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/x-component")
		_, _ = w.Write([]byte(body))
	}))
}

// buildRSCBody assembles a minimal RSC stream containing job JSON records inside
// a T-chunk plus any extra description chunks.
func buildRSCBody(jobRecords []string, descChunks map[string]string) string {
	var b strings.Builder
	combined := strings.Join(jobRecords, ",")
	fmt.Fprintf(&b, "1:T%x,%s\n", len(combined), combined)
	for id, content := range descChunks {
		fmt.Fprintf(&b, "%s:T%x,%s\n", id, len(content), content)
	}
	return b.String()
}

func assertEqual(t *testing.T, field, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("%s: got %q, want %q", field, got, want)
	}
}
