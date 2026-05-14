package meta

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
