package deel

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/trushilpatel/jobradar/apps/crawler/ats/types"
	companycommon "github.com/trushilpatel/jobradar/service/company/common"
	jobcommon "github.com/trushilpatel/jobradar/service/job/common"
)

// ── Interface compliance ────────────────────────────────────────────────────

func TestDeel_ImplementsAdapter(t *testing.T) {
	var _ types.Adapter = (*Deel)(nil)
}

func TestDeel_ImplementsURLProvider(t *testing.T) {
	var _ types.URLProvider = (*Deel)(nil)
}

func TestDeel_DoesNotImplementContentFetcher(t *testing.T) {
	// Full-detail adapter: FullContent is set at scan time, so the analyzer
	// must NOT call FetchJobContent for deel — implementing ContentFetcher
	// would cause a wasted per-job re-fetch.
	var v interface{} = New()
	if _, ok := v.(types.ContentFetcher); ok {
		t.Fatal("Deel must not implement ContentFetcher — it is a full-detail adapter")
	}
}

// ── Constructor + helpers ───────────────────────────────────────────────────

func TestNew_DefaultBaseURL(t *testing.T) {
	d := New()
	if d.baseURL != defaultBaseURL {
		t.Errorf("baseURL = %q, want %q", d.baseURL, defaultBaseURL)
	}
}

func TestCareersURL(t *testing.T) {
	d := New()
	if got := d.CareersURL("klarna"); got != "https://jobs.deel.com/klarna" {
		t.Errorf("CareersURL = %q", got)
	}
}

func TestResolveBaseURL_ZeroValueFallsBackToDefault(t *testing.T) {
	d := &Deel{}
	if got := d.resolveBaseURL(); got != defaultBaseURL {
		t.Errorf("zero-value resolveBaseURL = %q, want %q", got, defaultBaseURL)
	}
}

// ── RSC parsing primitives ──────────────────────────────────────────────────

func TestParseChunks_BasicSingleChunk(t *testing.T) {
	body := "15:T5,hello"
	chunks := parseChunks(body)
	if got := chunks["15"]; got != "hello" {
		t.Errorf("chunk 15 = %q, want %q", got, "hello")
	}
}

func TestParseChunks_MultipleChunksConcatenated(t *testing.T) {
	// Two T-chunks back-to-back inside one physical line. Lengths in hex —
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

func TestParseChunks_LengthBeyondBufferIsClampedNotPanic(t *testing.T) {
	// Length claims 64 bytes but only 3 are present.
	chunks := parseChunks("15:T40,abc")
	if got := chunks["15"]; got != "abc" {
		t.Errorf("clamped chunk = %q, want %q", got, "abc")
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

// ── Job record extraction ──────────────────────────────────────────────────

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
	// Two records back-to-back, as they appear in a real RSC text chunk.
	body := sampleJobJSON + "," + sampleJobJSON
	recs := extractJobRecords(body)
	if len(recs) != 2 {
		t.Fatalf("got %d records, want 2", len(recs))
	}
}

func TestExtractJobRecords_SkipsCareerPageSettings(t *testing.T) {
	// The careerPageSettings record has `id` + `organizationId`, NOT `id` + `jobId`.
	// Our regex must match only the job-posting prefix.
	cps := `{"id":"7e2c","organizationId":"10ce","urlSlug":"klarna","title":"Make your next move"}`
	body := cps + sampleJobJSON
	recs := extractJobRecords(body)
	if len(recs) != 1 {
		t.Errorf("expected 1 record (the job-posting), got %d", len(recs))
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

// ── End-to-end FetchJobs ────────────────────────────────────────────────────

func TestFetchJobs_HappyPath(t *testing.T) {
	body := buildRSCBody([]string{sampleJobJSON}, map[string]string{
		"15": "<p>About the role.</p><p>You will build.</p>",
	})
	srv := newRSCServer(t, body)
	defer srv.Close()

	d := &Deel{baseURL: srv.URL}
	resp, err := d.FetchJobs(context.Background(), "klarna")
	if err != nil {
		t.Fatalf("FetchJobs error: %v", err)
	}
	if len(resp.Jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(resp.Jobs))
	}

	j := resp.Jobs[0]
	assertEqual(t, "ID", j.ID, fmt.Sprintf("%s-5d3636f8-0712-4fe2-a1f4-84358440d272", companycommon.AdapterDeel))
	assertEqual(t, "Title", j.Title, "Senior Backend Engineer")
	assertEqual(t, "URL", j.URL, "https://jobs.deel.com/klarna/job-details/5d3636f8-0712-4fe2-a1f4-84358440d272")
	assertEqual(t, "Location", j.Location, "Berlin")
	assertEqual(t, "EmploymentType", j.EmploymentType, jobcommon.EmployFullTime)

	if j.FullContent == nil {
		t.Fatal("FullContent must be set — deel is a full-detail adapter")
	}
	assertEqual(t, "Description", j.FullContent.Description, "About the role. You will build.")
	assertEqual(t, "Department", j.FullContent.Department, "Engineering")
	assertEqual(t, "PublishedDate", j.FullContent.PublishedDate, "2026-04-21T07:45:50.278Z")
	assertEqual(t, "Compensation", j.FullContent.Compensation, "80000 – 110000 EUR")
	if j.FullContent.WorkplaceType != "" {
		t.Errorf("WorkplaceType should be empty (not in Deel schema), got %q", j.FullContent.WorkplaceType)
	}
}

func TestFetchJobs_MultipleJobs(t *testing.T) {
	// Two distinct postings sharing the same description ref.
	other := strings.Replace(sampleJobJSON,
		`"id":"5d3636f8-0712-4fe2-a1f4-84358440d272"`,
		`"id":"aaaaaaaa-0000-0000-0000-000000000001"`, 1)
	body := buildRSCBody([]string{sampleJobJSON, other}, map[string]string{
		"15": "<p>desc</p>",
	})
	srv := newRSCServer(t, body)
	defer srv.Close()

	d := &Deel{baseURL: srv.URL}
	resp, err := d.FetchJobs(context.Background(), "klarna")
	if err != nil {
		t.Fatalf("FetchJobs error: %v", err)
	}
	if len(resp.Jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(resp.Jobs))
	}
	if resp.Jobs[1].ID != "deel-aaaaaaaa-0000-0000-0000-000000000001" {
		t.Errorf("second job ID = %q", resp.Jobs[1].ID)
	}
}

func TestFetchJobs_EmptyBoard(t *testing.T) {
	srv := newRSCServer(t, "0:null\n")
	defer srv.Close()
	d := &Deel{baseURL: srv.URL}
	resp, err := d.FetchJobs(context.Background(), "empty")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Jobs) != 0 {
		t.Errorf("expected 0 jobs, got %d", len(resp.Jobs))
	}
}

func TestFetchJobs_EmptySlugIsRejected(t *testing.T) {
	d := New()
	if _, err := d.FetchJobs(context.Background(), ""); err == nil {
		t.Fatal("expected error for empty slug")
	}
	if _, err := d.FetchJobs(context.Background(), "   "); err == nil {
		t.Fatal("expected error for whitespace-only slug")
	}
}

func TestFetchJobs_404UnknownSlug(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	d := &Deel{baseURL: srv.URL}
	_, err := d.FetchJobs(context.Background(), "no-such-board")
	if err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("expected 404 error, got %v", err)
	}
}

func TestFetchJobs_5xxServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	d := &Deel{baseURL: srv.URL}
	_, err := d.FetchJobs(context.Background(), "klarna")
	if err == nil || !strings.Contains(err.Error(), "502") {
		t.Fatalf("expected 502 error, got %v", err)
	}
}

func TestFetchJobs_ContextCancelled(t *testing.T) {
	srv := newRSCServer(t, "")
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	d := &Deel{baseURL: srv.URL}
	if _, err := d.FetchJobs(ctx, "klarna"); err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

// ── Headers + URL ──────────────────────────────────────────────────────────

func TestFetchJobs_SendsRSCHeader(t *testing.T) {
	var capturedRSC, capturedAccept, capturedUA, capturedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedRSC = r.Header.Get("RSC")
		capturedAccept = r.Header.Get("Accept")
		capturedUA = r.Header.Get("User-Agent")
		capturedPath = r.URL.Path
		_, _ = w.Write([]byte(""))
	}))
	defer srv.Close()
	d := &Deel{baseURL: srv.URL}
	_, _ = d.FetchJobs(context.Background(), "klarna")
	if capturedRSC != "1" {
		t.Errorf("RSC header = %q, want %q", capturedRSC, "1")
	}
	if capturedAccept != "text/x-component" {
		t.Errorf("Accept = %q", capturedAccept)
	}
	if !strings.Contains(capturedUA, "Mozilla") {
		t.Errorf("UA should be browser-like, got %q", capturedUA)
	}
	if capturedPath != "/klarna" {
		t.Errorf("path = %q, want /klarna", capturedPath)
	}
}

// ── Converter helpers ──────────────────────────────────────────────────────

func TestFormatLocations(t *testing.T) {
	tests := []struct {
		name string
		in   []rawJobLocation
		want string
	}{
		{"nil", nil, ""},
		{"empty", []rawJobLocation{}, ""},
		{"single", []rawJobLocation{{Location: rawNamed{Name: "Berlin"}}}, "Berlin"},
		{"multiple", []rawJobLocation{
			{Location: rawNamed{Name: "Berlin"}},
			{Location: rawNamed{Name: "London"}},
		}, "Berlin; London"},
		{"blank-skipped", []rawJobLocation{
			{Location: rawNamed{Name: ""}},
			{Location: rawNamed{Name: "Paris"}},
		}, "Paris"},
		{"whitespace-trimmed-and-skipped", []rawJobLocation{
			{Location: rawNamed{Name: "  "}},
			{Location: rawNamed{Name: " Tokyo "}},
		}, "Tokyo"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := formatLocations(tc.in); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFirstHelpers(t *testing.T) {
	if got := firstEmploymentTypeName(nil); got != "" {
		t.Errorf("nil EmploymentType = %q", got)
	}
	if got := firstDepartmentName(nil); got != "" {
		t.Errorf("nil Department = %q", got)
	}
	if got := firstPublishedAt(nil); got != "" {
		t.Errorf("nil PublishedAt = %q", got)
	}
	if got := firstEmploymentTypeName([]rawEmploymentType{{EmploymentType: rawNamed{Name: "Full-time"}}}); got != "Full-time" {
		t.Errorf("first EmploymentType = %q", got)
	}
}

func TestFormatCompensation(t *testing.T) {
	tests := []struct {
		name string
		in   *rawCompensation
		want string
	}{
		{"nil", nil, ""},
		{"zero-range", &rawCompensation{}, ""},
		{"eur-range", &rawCompensation{CurrencyIsoCode: "EUR", MinAmount: 60000, MaxAmount: 80000}, "60000 – 80000 EUR"},
		{"no-currency-defaults-to-usd", &rawCompensation{MinAmount: 100000, MaxAmount: 150000}, "100000 – 150000 USD"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := formatCompensation(tc.in); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestResolveDescription(t *testing.T) {
	chunks := map[string]string{
		"15": "<p>chunk fifteen</p>",
		"1a": "<b>inline html chunk</b>",
	}
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"reference-resolved", `"$15"`, "chunk fifteen"},
		{"reference-missing-chunk", `"$99"`, ""},
		{"inline-html-fallback-stripped", `"<p>inline</p>"`, "inline"},
		{"null-string-returns-empty", `null`, ""},
		{"empty-raw-returns-empty", ``, ""},
		{"hex-id-with-letters-resolved", `"$1a"`, "inline html chunk"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveDescription(json.RawMessage(tc.raw), chunks)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestMapEmploymentType(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"Full-time", jobcommon.EmployFullTime},
		{"full time", jobcommon.EmployFullTime},
		{"FULL_TIME", jobcommon.EmployFullTime},
		{"Part-time", jobcommon.EmployPartTime},
		{"part time", jobcommon.EmployPartTime},
		{"Contract", jobcommon.EmployContract},
		{"Contractor", jobcommon.EmployContract},
		{"Consultant", jobcommon.EmployContract},
		{"Freelance", jobcommon.EmployContract},
		{"freelancer", jobcommon.EmployContract},
		{"Intern", jobcommon.EmployInternship},
		{"Internship", jobcommon.EmployInternship},
		{" ", jobcommon.EmployFullTime},       // empty / whitespace → default
		{"Unknown", jobcommon.EmployFullTime}, // unknown → default
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			if got := mapEmploymentType(tc.in); got != tc.want {
				t.Errorf("mapEmploymentType(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// ── Real integration test (gated on TEST_REAL=1) ────────────────────────────

func TestFetchJobs_Real(t *testing.T) {
	if os.Getenv("TEST_REAL") == "" {
		t.Skip("skipping real API test; set TEST_REAL=1 to run")
	}
	d := New()
	resp, err := d.FetchJobs(context.Background(), "klarna")
	if err != nil {
		t.Fatalf("FetchJobs error: %v", err)
	}
	if len(resp.Jobs) == 0 {
		t.Fatal("expected at least one job from live jobs.deel.com/klarna")
	}
	j := resp.Jobs[0]
	if !strings.HasPrefix(j.ID, "deel-") {
		t.Errorf("ID %q should have 'deel-' prefix", j.ID)
	}
	if j.Title == "" {
		t.Error("title should not be empty")
	}
	if !strings.HasPrefix(j.URL, "https://jobs.deel.com/klarna/job-details/") {
		t.Errorf("URL %q should match the public detail-page pattern", j.URL)
	}
	if j.FullContent == nil || j.FullContent.Description == "" {
		t.Error("FullContent.Description should not be empty (RSC stream carries it)")
	}
	t.Logf("fetched %d jobs; first: %q (%s) at %q", len(resp.Jobs), j.Title, j.EmploymentType, j.Location)
}

// ── Test helpers ───────────────────────────────────────────────────────────

// newRSCServer returns a test server that serves the provided RSC body for
// any GET request. The body is returned verbatim, so callers can craft the
// exact stream they want to parse against.
func newRSCServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/x-component")
		_, _ = w.Write([]byte(body))
	}))
}

// buildRSCBody assembles a minimal RSC stream that contains the supplied job
// JSON records inside a T-chunk plus any extra description chunks. Mirrors
// the shape used by jobs.deel.com closely enough that the parser exercises
// the same code paths as production.
func buildRSCBody(jobRecords []string, descChunks map[string]string) string {
	var b strings.Builder
	// One big T-chunk carrying all the concatenated job records.
	combined := strings.Join(jobRecords, ",")
	fmt.Fprintf(&b, "1:T%x,%s\n", len(combined), combined)
	for id, body := range descChunks {
		fmt.Fprintf(&b, "%s:T%x,%s\n", id, len(body), body)
	}
	return b.String()
}

func assertEqual(t *testing.T, field, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("%s: got %q, want %q", field, got, want)
	}
}
