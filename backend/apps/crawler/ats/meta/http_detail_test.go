package meta

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/trushilpatel/jobradar/apps/crawler/ats/types"
	"github.com/trushilpatel/jobradar/common/logger"
)

// ── flexString — every shape of address field Meta has been observed to emit ─

func TestFlexString_PlainString(t *testing.T) {
	var f flexString
	if err := f.UnmarshalJSON([]byte(`"USA"`)); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if f != "USA" {
		t.Errorf("got %q, want USA", f)
	}
}

func TestFlexString_ObjectWithNameString(t *testing.T) {
	// {"@type":"Country","name":"USA"} — schema.org's verbose form.
	var f flexString
	if err := f.UnmarshalJSON([]byte(`{"@type":"Country","name":"USA"}`)); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if f != "USA" {
		t.Errorf("got %q, want USA", f)
	}
}

func TestFlexString_ObjectWithNameList(t *testing.T) {
	// The real Meta case: {"@type":"Country","name":["USA","USA",…]}.
	// Should dedupe to one "USA".
	var f flexString
	raw := `{"@type":"Country","name":["USA","USA","USA"]}`
	if err := f.UnmarshalJSON([]byte(raw)); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if f != "USA" {
		t.Errorf("got %q, want deduped 'USA'", f)
	}
}

func TestFlexString_ArrayOfStrings(t *testing.T) {
	var f flexString
	if err := f.UnmarshalJSON([]byte(`["USA","Canada","USA"]`)); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// Dedup preserves order; first "USA" then "Canada".
	if f != "USA, Canada" {
		t.Errorf("got %q, want 'USA, Canada'", f)
	}
}

func TestFlexString_Null(t *testing.T) {
	var f flexString
	if err := f.UnmarshalJSON([]byte(`null`)); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if f != "" {
		t.Errorf("got %q, want empty", f)
	}
}

// ── jobLocations — single object vs array ────────────────────────────────────

func TestJobLocations_SingleObject(t *testing.T) {
	var jl jobLocations
	raw := `{"@type":"Place","address":{"addressLocality":"Menlo Park"}}`
	if err := jl.UnmarshalJSON([]byte(raw)); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(jl) != 1 {
		t.Fatalf("got %d entries, want 1", len(jl))
	}
	if jl[0].Address.Locality != "Menlo Park" {
		t.Errorf("locality: got %q", jl[0].Address.Locality)
	}
}

func TestJobLocations_Array(t *testing.T) {
	var jl jobLocations
	raw := `[
		{"@type":"Place","address":{"addressLocality":"Menlo Park"}},
		{"@type":"Place","address":{"addressLocality":"Sunnyvale"}}
	]`
	if err := jl.UnmarshalJSON([]byte(raw)); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(jl) != 2 {
		t.Fatalf("got %d entries, want 2", len(jl))
	}
}

// ── extractJobPosting — happy path + edge cases ──────────────────────────────

const fixtureJSONLD = `<!doctype html><html><head>
<title>Software Engineer (Test) — Meta Careers</title>
<script type="application/ld+json">
{
  "@context": "http://schema.org/",
  "@type": "JobPosting",
  "title": "Software Engineer (Test)",
  "description": "Meta is seeking…",
  "responsibilities": "Drive technical direction.",
  "qualifications": "Bachelor's degree in CS.",
  "employmentType": "Full-time",
  "datePosted": "2025-04-11T09:54:55-07:00",
  "validThrough": "2026-05-24T20:31:26-07:00",
  "directApply": true,
  "hiringOrganization": {"@type":"Organization","name":"Meta","logo":"https://example.com/logo.png"},
  "jobLocation": [
    {"@type":"Place","address":{"addressLocality":"Menlo Park","addressRegion":"CA","addressCountry":{"@type":"Country","name":["USA","USA"]}}},
    {"@type":"Place","address":{"addressLocality":"Menlo Park","addressRegion":"CA","addressCountry":{"@type":"Country","name":["USA","USA"]}}},
    {"@type":"Place","address":{"addressLocality":"Sunnyvale","addressRegion":"CA","addressCountry":{"@type":"Country","name":["USA","USA"]}}}
  ]
}
</script>
</head><body>shell</body></html>`

func TestExtractJobPosting_HappyPath(t *testing.T) {
	jp, err := extractJobPosting([]byte(fixtureJSONLD))
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if jp.Type != "JobPosting" {
		t.Errorf("Type: got %q", jp.Type)
	}
	if jp.Title != "Software Engineer (Test)" {
		t.Errorf("Title: got %q", jp.Title)
	}
	if jp.HiringOrg.Name != "Meta" {
		t.Errorf("Org.Name: got %q", jp.HiringOrg.Name)
	}
	if !jp.DirectApply {
		t.Error("DirectApply: got false")
	}
	if len(jp.JobLocation) != 3 {
		t.Errorf("JobLocation count: got %d, want 3", len(jp.JobLocation))
	}
}

func TestExtractJobPosting_DedupedLocations(t *testing.T) {
	jp, err := extractJobPosting([]byte(fixtureJSONLD))
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	locs := jp.Locations()
	// 3 entries → 2 deduped: "Menlo Park, CA, USA" and "Sunnyvale, CA, USA".
	if len(locs) != 2 {
		t.Fatalf("got %d deduped locations, want 2: %v", len(locs), locs)
	}
	if locs[0] != "Menlo Park, CA, USA" {
		t.Errorf("locs[0]: got %q", locs[0])
	}
	if locs[1] != "Sunnyvale, CA, USA" {
		t.Errorf("locs[1]: got %q", locs[1])
	}
}

func TestExtractJobPosting_NoJSONLD(t *testing.T) {
	_, err := extractJobPosting([]byte(`<html><body>nothing here</body></html>`))
	if err == nil {
		t.Error("expected error when no JSON-LD block present")
	}
}

func TestExtractJobPosting_NonJobPostingJSONLD(t *testing.T) {
	html := `<html><head>
<script type="application/ld+json">{"@type":"BreadcrumbList","itemListElement":[]}</script>
</head></html>`
	_, err := extractJobPosting([]byte(html))
	if err == nil {
		t.Error("expected error when JSON-LD has no JobPosting")
	}
}

func TestExtractJobPosting_GraphWrappedJobPosting(t *testing.T) {
	// schema.org allows {"@graph": [JobPosting, ...]} wrapping.
	html := `<html><head>
<script type="application/ld+json">{"@graph":[
  {"@type":"BreadcrumbList","itemListElement":[]},
  {"@type":"JobPosting","title":"Inside Graph","employmentType":"Full-time"}
]}</script>
</head></html>`
	jp, err := extractJobPosting([]byte(html))
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if jp.Title != "Inside Graph" {
		t.Errorf("Title: got %q", jp.Title)
	}
}

// ── Compose — full description string composition ────────────────────────────

func TestCompose_IncludesAllSections(t *testing.T) {
	jp, err := extractJobPosting([]byte(fixtureJSONLD))
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	desc := jp.Compose()

	for _, s := range []string{
		"Software Engineer (Test)",
		"Menlo Park, CA, USA",
		"Sunnyvale, CA, USA",
		"Organization: Meta",
		"Employment type: Full-time",
		"Posted: 2025-04-11T09:54:55-07:00",
		"Description",
		"Meta is seeking",
		"Responsibilities",
		"Drive technical direction",
		"Qualifications",
		"Bachelor's degree in CS",
	} {
		if !strings.Contains(desc, s) {
			t.Errorf("description missing %q\n--- full ---\n%s", s, desc)
		}
	}
}

func TestCompose_StripsHTMLAndEntities(t *testing.T) {
	jp := &JobPosting{
		Title:       "Test",
		Description: "Lead the <b>team</b> &amp; build <a href='#'>features</a>.",
	}
	desc := jp.Compose()
	if strings.Contains(desc, "<b>") || strings.Contains(desc, "&amp;") {
		t.Errorf("HTML/entities not stripped: %q", desc)
	}
	if !strings.Contains(desc, "Lead the team & build features") {
		t.Errorf("expected stripped text in description: %q", desc)
	}
}

func TestCompose_HandlesMissingOptionalFields(t *testing.T) {
	// Minimum viable posting — every optional field absent.
	jp := &JobPosting{Title: "Bare", EmploymentType: "Full-time"}
	desc := jp.Compose()
	if !strings.Contains(desc, "Bare") {
		t.Errorf("Title missing: %q", desc)
	}
	// Should not crash and should not emit empty section headers.
	if strings.Contains(desc, "Description\n\n") || strings.Contains(desc, "Responsibilities\n\n") {
		t.Errorf("empty sections leaked: %q", desc)
	}
}

// ── toFetchJobContentResponse — engine-facing contract ───────────────────────

func TestToFetchJobContentResponse_PopulatesAllFields(t *testing.T) {
	jp, err := extractJobPosting([]byte(fixtureJSONLD))
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	resp := jp.toFetchJobContentResponse("meta-12345")
	if resp.ID != "meta-12345" {
		t.Errorf("ID: got %q", resp.ID)
	}
	if resp.Title != "Software Engineer (Test)" {
		t.Errorf("Title: got %q", resp.Title)
	}
	if resp.Location != "Menlo Park, CA, USA" {
		t.Errorf("Location: got %q (want first deduped entry)", resp.Location)
	}
	if resp.EmploymentType != "full_time" {
		t.Errorf("EmploymentType: got %q (want full_time constant)", resp.EmploymentType)
	}
	if resp.Department != "Meta" {
		t.Errorf("Department: got %q", resp.Department)
	}
	if resp.PublishedDate != "2025-04-11T09:54:55-07:00" {
		t.Errorf("PublishedDate: got %q", resp.PublishedDate)
	}
	if resp.Description == "" {
		t.Error("Description must not be empty")
	}
}

// ── stripMetaPrefix — id → numeric job id ────────────────────────────────────

func TestStripMetaPrefix_RemovesPrefix(t *testing.T) {
	if got := stripMetaPrefix("meta-12345"); got != "12345" {
		t.Errorf("got %q, want 12345", got)
	}
}

func TestStripMetaPrefix_PassesThroughBareID(t *testing.T) {
	if got := stripMetaPrefix("99999"); got != "99999" {
		t.Errorf("got %q, want 99999", got)
	}
}

// ── FetchJobContent — end-to-end with httptest server ────────────────────────

func TestFetchJobContent_EndToEndAgainstFakeServer(t *testing.T) {
	// Spin up a fake server that returns our JSON-LD fixture.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/profile/job_details/") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		// Sanity-check the request shape Meta's edge expects.
		if r.Header.Get("User-Agent") == "" {
			t.Errorf("User-Agent header missing")
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(fixtureJSONLD))
	}))
	defer srv.Close()

	m := New(defaultConfig(), logger.NewNoop())
	resp, err := m.FetchJobContent(context.Background(), types.RawJob{
		ID:  "meta-12345",
		URL: srv.URL + "/profile/job_details/12345",
	})
	if err != nil {
		t.Fatalf("FetchJobContent: %v", err)
	}
	if resp.Title != "Software Engineer (Test)" {
		t.Errorf("Title: got %q", resp.Title)
	}
	if !strings.Contains(resp.Description, "Bachelor") {
		t.Errorf("Description missing qualifications: %q", resp.Description)
	}
	if resp.URL != srv.URL+"/profile/job_details/12345" {
		t.Errorf("URL: got %q", resp.URL)
	}
}

func TestFetchJobContent_Surfaces404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	m := New(defaultConfig(), logger.NewNoop())
	_, err := m.FetchJobContent(context.Background(), types.RawJob{
		ID:  "meta-99",
		URL: srv.URL + "/profile/job_details/99",
	})
	if err == nil {
		t.Error("expected error on 404")
	}
}

// ── extractMetaTag ───────────────────────────────────────────────────────────

func TestExtractMetaTag_OgTitle(t *testing.T) {
	html := []byte(`<html><head>
<meta property="og:title" content="UX Researcher, Qualitative" />
<meta name="description" content="We design for humans." />
</head></html>`)
	if got := extractMetaTag(html, "og:title"); got != "UX Researcher, Qualitative" {
		t.Errorf("og:title: got %q", got)
	}
}

func TestExtractMetaTag_NameAttribute(t *testing.T) {
	html := []byte(`<meta name="title" content="Software Engineer" />`)
	if got := extractMetaTag(html, "title"); got != "Software Engineer" {
		t.Errorf("name=title: got %q", got)
	}
}

func TestExtractMetaTag_MissingReturnsEmpty(t *testing.T) {
	if got := extractMetaTag([]byte(`<html></html>`), "og:title"); got != "" {
		t.Errorf("missing tag: got %q, want empty", got)
	}
}

// ── FetchJobContent — meta-tag fallback when JSON-LD absent ─────────────────

const fixtureNoJSONLD = `<!DOCTYPE html><html><head>
<meta name="title" content="UX Researcher, Qualitative" />
<meta property="og:title" content="UX Researcher, Qualitative" />
<meta property="og:description" content="Our UX Research team is designing for the global spectrum of human needs." />
</head><body><div>Page content loaded by React.</div></body></html>`

func TestFetchJobContent_FallsBackToMetaTagsWhenNoJSONLD(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(fixtureNoJSONLD))
	}))
	defer srv.Close()

	m := New(defaultConfig(), logger.NewNoop())
	resp, err := m.FetchJobContent(context.Background(), types.RawJob{
		ID:  "meta-2396505620550988",
		URL: srv.URL + "/profile/job_details/2396505620550988",
	})
	if err != nil {
		t.Fatalf("FetchJobContent: expected fallback success, got error: %v", err)
	}
	if resp.Title != "UX Researcher, Qualitative" {
		t.Errorf("Title: got %q, want 'UX Researcher, Qualitative'", resp.Title)
	}
	if !strings.Contains(resp.Description, "UX Research") {
		t.Errorf("Description: expected og:description content, got %q", resp.Description)
	}
}

func TestFetchJobContent_ErrorHasNoDoublePrefix(t *testing.T) {
	// When the page has no JSON-LD and no meta tags, error should say
	// "meta/http: ..." exactly once, not "meta/http: meta/http: ...".
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body>nothing</body></html>`))
	}))
	defer srv.Close()

	m := New(defaultConfig(), logger.NewNoop())
	_, err := m.FetchJobContent(context.Background(), types.RawJob{
		ID:  "meta-1",
		URL: srv.URL + "/profile/job_details/1",
	})
	if err == nil {
		t.Fatal("expected error when page has no JSON-LD and no meta tags")
	}
	msg := err.Error()
	first := strings.Index(msg, "meta/http:")
	if first == -1 {
		t.Errorf("error missing 'meta/http:' prefix: %q", msg)
		return
	}
	if strings.Contains(msg[first+1:], "meta/http:") {
		t.Errorf("double 'meta/http:' prefix in error: %q", msg)
	}
}

// ── Sanity check: JSON tags produce expected wire format ─────────────────────

func TestJobPosting_RoundTripsThroughJSON(t *testing.T) {
	in := JobPosting{
		Type:           "JobPosting",
		Title:          "X",
		EmploymentType: "Full-time",
		DatePosted:     "2025-01-01",
	}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out JobPosting
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Title != "X" || out.DatePosted != "2025-01-01" {
		t.Errorf("round-trip mismatch: %+v", out)
	}
}

// TestFlexString_StringMethod verifies the String() pass-through used by
// fmt.Stringer-aware callers (logger fields, error messages).
func TestFlexString_StringMethod(t *testing.T) {
	f := flexString("hello")
	if got := f.String(); got != "hello" {
		t.Errorf("flexString.String(): got %q, want %q", got, "hello")
	}
	// Zero value renders as empty string — required for omitempty-aware logging.
	var zero flexString
	if got := zero.String(); got != "" {
		t.Errorf("zero flexString.String(): got %q, want empty", got)
	}
}

// ── flexString — additional edge cases ───────────────────────────────────────

func TestFlexString_JSONNumber_FallsThrough(t *testing.T) {
	// A JSON number is not a string, object, or array — must produce empty
	// without error rather than crashing the parser.
	var f flexString
	if err := f.UnmarshalJSON([]byte(`42`)); err != nil {
		t.Fatalf("unexpected error on number input: %v", err)
	}
	if f != "" {
		t.Errorf("got %q, want empty for unknown shape", f)
	}
}

func TestFlexString_EmptyInput(t *testing.T) {
	var f flexString
	if err := f.UnmarshalJSON([]byte{}); err != nil {
		t.Fatalf("unexpected error on empty input: %v", err)
	}
	if f != "" {
		t.Errorf("got %q, want empty", f)
	}
}

func TestFlexString_ObjectWithNoNameField(t *testing.T) {
	// Object that has no "name" key — should become empty without error.
	var f flexString
	if err := f.UnmarshalJSON([]byte(`{"@type":"Country","code":"US"}`)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f != "" {
		t.Errorf("got %q, want empty for object without name field", f)
	}
}

func TestFlexString_ArrayWithEmptyElements(t *testing.T) {
	// All elements are empty strings — result must be empty (no leading ", ").
	var f flexString
	if err := f.UnmarshalJSON([]byte(`["",""]`)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f != "" {
		t.Errorf("got %q, want empty when all array elements are empty", f)
	}
}

// ── jobLocations.UnmarshalJSON — additional edge cases ──────────────────────

func TestJobLocations_Null(t *testing.T) {
	var jl jobLocations
	if err := jl.UnmarshalJSON([]byte(`null`)); err != nil {
		t.Fatalf("unexpected error on null: %v", err)
	}
	if jl != nil {
		t.Errorf("null should produce nil slice, got %v", jl)
	}
}

func TestJobLocations_InvalidArray(t *testing.T) {
	// Array whose elements are not valid jobLocation objects.
	var jl jobLocations
	// A valid JSON array of scalars — the json.Unmarshal into []jobLocation
	// will error because elements are not objects.
	err := jl.UnmarshalJSON([]byte(`[1,2,3]`))
	if err == nil {
		t.Fatal("expected error when array elements are not jobLocation objects, got nil")
	}
}

// ── extractJobPosting — array-of-objects form ────────────────────────────────

func TestExtractJobPosting_ArrayOfObjects(t *testing.T) {
	// Multiple JSON-LD blocks in a single <script> as an array — not @graph.
	html := `<html><head>
<script type="application/ld+json">[
  {"@type":"BreadcrumbList","itemListElement":[]},
  {"@type":"JobPosting","title":"Array Item Engineer","employmentType":"Full-time"}
]</script>
</head></html>`
	jp, err := extractJobPosting([]byte(html))
	if err != nil {
		t.Fatalf("extract from array form: %v", err)
	}
	if jp.Title != "Array Item Engineer" {
		t.Errorf("Title: got %q, want Array Item Engineer", jp.Title)
	}
}

// ── Locations() — location with all empty address parts ──────────────────────

func TestLocations_AllPartsEmpty(t *testing.T) {
	// A jobLocation with no locality, region, or country produces no output.
	jp := &JobPosting{
		JobLocation: jobLocations{
			{Address: address{}},
		},
	}
	locs := jp.Locations()
	if len(locs) != 0 {
		t.Errorf("expected no locations when all parts are empty, got %v", locs)
	}
}

// ── Compose() — location truncation at >5 entries ────────────────────────────

func TestCompose_TruncatesLocationsAboveFive(t *testing.T) {
	// 7 distinct locations — Compose must show 5 and emit "(+2 more)".
	locs := make(jobLocations, 7)
	for i := range locs {
		locs[i] = jobLocation{
			Address: address{
				Locality: flexString(fmt.Sprintf("City%d", i)),
				Region:   "CA",
				Country:  "USA",
			},
		}
	}
	jp := &JobPosting{Title: "T", JobLocation: locs}
	desc := jp.Compose()
	if !strings.Contains(desc, "(+2 more)") {
		t.Errorf("Compose should truncate after 5 locations with '+N more', got:\n%s", desc)
	}
}

// ── metaCountryToCode — uncovered variants ────────────────────────────────────

func TestMetaCountryToCode(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"USA", "US"},
		{"United States", "US"},
		{"United States of America", "US"},
		{"US", "US"},
		{"us", "US"},      // lowercase normalised
		{"IN", "IN"},      // 2-char ISO pass-through
		{"GB", "GB"},      // 2-char ISO pass-through
		{"in", "IN"},      // lowercase 2-char
		{"", ""},          // empty stays empty
		{"Germany", ""},   // unknown non-2-char country name
		{"  USA  ", "US"}, // leading/trailing whitespace trimmed
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got := metaCountryToCode(tc.input)
			if got != tc.want {
				t.Errorf("metaCountryToCode(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// ── toFetchJobContentResponse — fallback when Locations() is empty ────────────

func TestToFetchJobContentResponse_FallsBackToLocalityWhenLocationsEmpty(t *testing.T) {
	// JobPosting with a location that has no region/country — Locations()
	// returns a non-empty joined string, so let us test the path where all
	// address fields are empty (Locality only).
	jp := &JobPosting{
		Title: "Bare Location Test",
		JobLocation: jobLocations{
			{
				Address: address{
					Locality: "Menlo Park",
					// Region and Country both empty — "Menlo Park" is
					// the only component, Locations() returns ["Menlo Park"].
				},
			},
		},
	}
	resp := jp.toFetchJobContentResponse("meta-99")
	if resp.Location != "Menlo Park" {
		t.Errorf("Location: got %q, want Menlo Park", resp.Location)
	}
}

func TestToFetchJobContentResponse_NoLocations(t *testing.T) {
	// No jobLocation entries at all — falls back to empty locality.
	jp := &JobPosting{Title: "No Location"}
	resp := jp.toFetchJobContentResponse("meta-0")
	if resp.Location != "" {
		t.Errorf("Location: got %q, want empty when no jobLocation", resp.Location)
	}
}

func TestToFetchJobContentResponse_LocationsEmptyJoinedFallback(t *testing.T) {
	// jobLocation present but ALL address parts empty → Locations() returns [].
	// Falls back to raw Locality (also empty) → location must be "".
	jp := &JobPosting{
		Title:       "Empty Address",
		JobLocation: jobLocations{{Address: address{}}},
	}
	resp := jp.toFetchJobContentResponse("meta-1")
	if resp.Location != "" {
		t.Errorf("Location: got %q, want empty when all address parts are empty", resp.Location)
	}
}

func TestToFetchJobContentResponse_CountryCodeExtracted(t *testing.T) {
	jp := &JobPosting{
		Title: "US Engineer",
		JobLocation: jobLocations{
			{
				Address: address{
					Locality: flexString("Austin"),
					Region:   flexString("TX"),
					Country:  flexString("USA"),
				},
			},
		},
	}
	resp := jp.toFetchJobContentResponse("meta-42")
	if resp.CountryCode != "US" {
		t.Errorf("CountryCode: got %q, want US", resp.CountryCode)
	}
}

func TestToFetchJobContentResponse_CountryCodeEmptyWhenUnknown(t *testing.T) {
	jp := &JobPosting{
		Title: "Unknown Country",
		JobLocation: jobLocations{
			{
				Address: address{
					Country: flexString("Narnia"),
				},
			},
		},
	}
	resp := jp.toFetchJobContentResponse("meta-43")
	if resp.CountryCode != "" {
		t.Errorf("CountryCode: got %q, want empty for unknown country", resp.CountryCode)
	}
}

// ── extractMetaTag — tag found but missing content attribute ─────────────────

func TestExtractMetaTag_TagPresentButNoContent(t *testing.T) {
	// <meta> tag matches by name but has no content= attribute.
	html := []byte(`<meta name="og:title" />`)
	got := extractMetaTag(html, "og:title")
	if got != "" {
		t.Errorf("expected empty when content attribute absent, got %q", got)
	}
}

// ── FetchJobContent — URL derived from job.ID when job.URL is empty ──────────

func TestFetchJobContent_DerivedURLFromID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Expect the canonical detail path for job id "12345".
		if r.URL.Path != "/profile/job_details/12345" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(fixtureJSONLD))
	}))
	defer srv.Close()

	// Override the detail URL prefix so the adapter hits the test server.
	old := detailURLPrefix
	// We can't override detailURLPrefix directly (it's a const), so we set
	// job.URL explicitly to the test server path — this exercises the
	// job.URL == "" branch indirectly via providing an empty URL and letting
	// the adapter build it. Since detailURLPrefix is a const we test the
	// non-empty URL branch here and cover the empty URL path via the
	// deriveURL helper test below.
	_ = old

	m := New(defaultConfig(), logger.NewNoop())
	// Pass URL explicitly pointing at test server — covers the non-empty URL
	// branch of FetchJobContent.
	resp, err := m.FetchJobContent(context.Background(), types.RawJob{
		ID:  "meta-12345",
		URL: srv.URL + "/profile/job_details/12345",
	})
	if err != nil {
		t.Fatalf("FetchJobContent: %v", err)
	}
	if resp.Title != "Software Engineer (Test)" {
		t.Errorf("Title: got %q", resp.Title)
	}
}

// ── fetchSitemap — non-200 status ────────────────────────────────────────────

func TestFetchSitemap_Non200Status(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	origURL := sitemapURL
	overrideSitemapURL(srv.URL + "/jobsearch/sitemap.xml")
	defer overrideSitemapURL(origURL)

	m := NewDefault()
	_, err := m.fetchSitemap(context.Background())
	if err == nil {
		t.Fatal("expected error on non-200 sitemap response")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("error should mention 403, got: %v", err)
	}
}

// ── fetchSitemap — HTTP client Do error ──────────────────────────────────────

// errTransport is an http.RoundTripper that always returns an error.
// Used to test the httpClient.Do error path without a real network call.
type errTransport struct{ err error }

func (e errTransport) RoundTrip(*http.Request) (*http.Response, error) { return nil, e.err }

func TestFetchSitemap_ClientDoError(t *testing.T) {
	origURL := sitemapURL
	overrideSitemapURL("http://127.0.0.1:0/sitemap.xml") // unreachable port
	defer overrideSitemapURL(origURL)

	m := NewDefault()
	m.httpClient = &http.Client{Transport: errTransport{err: fmt.Errorf("dial tcp: connection refused")}}

	_, err := m.fetchSitemap(context.Background())
	if err == nil {
		t.Fatal("expected error when httpClient.Do fails")
	}
	if !strings.Contains(err.Error(), "do") && !strings.Contains(err.Error(), "connection") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// ── fetchAndParseDetail — HTTP client Do error ────────────────────────────────

func TestFetchAndParseDetail_ClientDoError(t *testing.T) {
	m := NewDefault()
	m.httpClient = &http.Client{Transport: errTransport{err: fmt.Errorf("connection refused")}}

	_, err := m.fetchAndParseDetail(context.Background(), "http://127.0.0.1:0/profile/job_details/1")
	if err == nil {
		t.Fatal("expected error when httpClient.Do fails in fetchAndParseDetail")
	}
}

// ── FetchJobContent — URL derived from job.ID when job.URL is empty ──────────

func TestFetchJobContent_URLDerivedFromIDWhenURLEmpty(t *testing.T) {
	// When job.URL is empty the adapter builds a URL from job.ID.
	// We inject a custom RoundTripper that validates the built URL and returns
	// the JSON-LD fixture — no real network needed.
	var capturedURL string
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		capturedURL = req.URL.String()
		return newHTMLResponse(fixtureJSONLD), nil
	})
	m := NewDefault()
	m.httpClient = &http.Client{Transport: transport}

	_, err := m.FetchJobContent(context.Background(), types.RawJob{
		ID:  "meta-12345",
		URL: "", // empty — adapter must derive URL from ID
	})
	if err != nil {
		t.Fatalf("FetchJobContent with empty URL: %v", err)
	}
	// The derived URL must contain the numeric job id from "meta-12345".
	if !strings.Contains(capturedURL, "12345") {
		t.Errorf("derived URL should contain the job numeric id 12345, got: %q", capturedURL)
	}
}

// ── jobLocations.UnmarshalJSON — single-object decode error path ──────────────

func TestJobLocations_SingleObjectDecodeError(t *testing.T) {
	// A JSON string that starts with '"' (not '[') is not a valid jobLocation
	// object — should return a decode error.
	var jl jobLocations
	err := jl.UnmarshalJSON([]byte(`"not-an-object"`))
	if err == nil {
		t.Fatal("expected error when single-object unmarshal receives a string, got nil")
	}
}

// ── fetchAndParseDetail — io.ReadAll error ────────────────────────────────────

func TestFetchAndParseDetail_ReadBodyError(t *testing.T) {
	// Return a 200 response whose body errors on Read.
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/html"}},
			Body:       io.NopCloser(&errReader{err: fmt.Errorf("read error")}),
		}, nil
	})
	m := NewDefault()
	m.httpClient = &http.Client{Transport: transport}

	_, err := m.fetchAndParseDetail(context.Background(), "http://example.com/profile/job_details/1")
	if err == nil {
		t.Fatal("expected error when body Read fails in fetchAndParseDetail")
	}
}

// ── fetchSitemap — io.ReadAll error ──────────────────────────────────────────

func TestFetchSitemap_ReadBodyError(t *testing.T) {
	origURL := sitemapURL
	overrideSitemapURL("http://127.0.0.1:0/sitemap.xml")
	defer overrideSitemapURL(origURL)

	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/xml"}},
			Body:       io.NopCloser(&errReader{err: fmt.Errorf("read error")}),
		}, nil
	})
	m := NewDefault()
	m.httpClient = &http.Client{Transport: transport}

	_, err := m.fetchSitemap(context.Background())
	if err == nil {
		t.Fatal("expected error when body Read fails in fetchSitemap")
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

// roundTripFunc is a functional http.RoundTripper.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

// newHTMLResponse wraps an HTML string into a minimal *http.Response.
func newHTMLResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/html"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

// errReader always returns an error on Read.
type errReader struct{ err error }

func (e *errReader) Read(_ []byte) (int, error) { return 0, e.err }
