package meta

import (
	"testing"

	metaapi "github.com/trushilpatel/jobradar/apps/crawler-v2/adapters/ats/meta/api"
)

// ── extractJobPosting ─────────────────────────────────────────────────────────

func TestExtractJobPosting_PlainObject(t *testing.T) {
	body := []byte(`<html><head>
<script type="application/ld+json">
{"@context":"http://schema.org","@type":"JobPosting","title":"Software Engineer","datePosted":"2024-01-15"}
</script>
</head></html>`)

	jp, err := extractJobPosting(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if jp.Type != "JobPosting" {
		t.Errorf("Type = %q, want JobPosting", jp.Type)
	}
	if jp.Title != "Software Engineer" {
		t.Errorf("Title = %q, want Software Engineer", jp.Title)
	}
	if jp.DatePosted != "2024-01-15" {
		t.Errorf("DatePosted = %q, want 2024-01-15", jp.DatePosted)
	}
}

func TestExtractJobPosting_NotFound(t *testing.T) {
	body := []byte(`<html><head>
<script type="application/ld+json">{"@type":"WebSite","name":"Meta"}</script>
</head></html>`)

	_, err := extractJobPosting(body)
	if err == nil {
		t.Fatal("expected error when no JobPosting found, got nil")
	}
}

func TestExtractJobPosting_NoScriptBlocks(t *testing.T) {
	body := []byte(`<html><head><title>Meta Careers</title></head><body></body></html>`)
	_, err := extractJobPosting(body)
	if err == nil {
		t.Fatal("expected error for HTML with no JSON-LD blocks, got nil")
	}
}

func TestExtractJobPosting_MultipleScripts_JobPostingIsSecond(t *testing.T) {
	body := []byte(`<html><head>
<script type="application/ld+json">
{"@context":"http://schema.org","@type":"BreadcrumbList","itemListElement":[]}
</script>
<script type="application/ld+json">
{"@context":"http://schema.org","@type":"JobPosting","title":"Data Engineer","datePosted":"2024-02-01"}
</script>
</head></html>`)

	jp, err := extractJobPosting(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if jp.Title != "Data Engineer" {
		t.Errorf("Title = %q, want Data Engineer", jp.Title)
	}
}

func TestExtractJobPosting_ArrayShape(t *testing.T) {
	body := []byte(`<html><head>
<script type="application/ld+json">
[{"@type":"WebSite"},{"@type":"JobPosting","title":"PM","datePosted":"2024-03-01"}]
</script>
</head></html>`)

	jp, err := extractJobPosting(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if jp.Title != "PM" {
		t.Errorf("Title = %q, want PM", jp.Title)
	}
}

func TestExtractJobPosting_GraphShape(t *testing.T) {
	body := []byte(`<html><head>
<script type="application/ld+json">
{"@graph":[{"@type":"WebPage"},{"@type":"JobPosting","title":"Designer","datePosted":"2024-04-01"}]}
</script>
</head></html>`)

	jp, err := extractJobPosting(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if jp.Title != "Designer" {
		t.Errorf("Title = %q, want Designer", jp.Title)
	}
}

func TestExtractJobPosting_SingleQuoteAttribute(t *testing.T) {
	body := []byte(`<html><head>
<script type='application/ld+json'>
{"@type":"JobPosting","title":"QA Engineer"}
</script>
</head></html>`)

	jp, err := extractJobPosting(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if jp.Title != "QA Engineer" {
		t.Errorf("Title = %q, want QA Engineer", jp.Title)
	}
}

// ── extractCountryCode ────────────────────────────────────────────────────────

func TestExtractCountryCode(t *testing.T) {
	tests := []struct {
		name    string
		country string
		want    string
	}{
		{"USA string", "USA", "US"},
		{"United States", "United States", "US"},
		{"United States of America", "United States of America", "US"},
		{"lowercase us", "us", "US"},
		{"2-char GB", "GB", "GB"},
		{"2-char in", "in", "IN"},
		{"empty string", "", ""},
		{"unknown long", "Germany", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := metaCountryToCode(tt.country)
			if got != tt.want {
				t.Errorf("metaCountryToCode(%q) = %q, want %q", tt.country, got, tt.want)
			}
		})
	}
}

func TestExtractCountryCode_FromJobPosting(t *testing.T) {
	jp := &metaapi.JobPosting{}
	_ = jp.JobLocation.UnmarshalJSON([]byte(`[{"address":{"addressCountry":"USA","addressLocality":"Menlo Park"}}]`))

	code := extractCountryCode(jp)
	if code != "US" {
		t.Errorf("extractCountryCode = %q, want US", code)
	}
}

func TestExtractCountryCode_Empty(t *testing.T) {
	jp := &metaapi.JobPosting{}
	code := extractCountryCode(jp)
	if code != "" {
		t.Errorf("extractCountryCode on empty posting = %q, want empty", code)
	}
}

// ── extractLocation ───────────────────────────────────────────────────────────

func TestExtractLocation(t *testing.T) {
	jp := &metaapi.JobPosting{}
	_ = jp.JobLocation.UnmarshalJSON([]byte(`{"address":{"addressLocality":"Menlo Park","addressRegion":"CA","addressCountry":"USA"}}`))

	loc := extractLocation(jp)
	if loc != "Menlo Park, CA, USA" {
		t.Errorf("extractLocation = %q, want Menlo Park, CA, USA", loc)
	}
}

func TestExtractLocation_Empty(t *testing.T) {
	jp := &metaapi.JobPosting{}
	loc := extractLocation(jp)
	if loc != "" {
		t.Errorf("extractLocation on empty posting = %q, want empty", loc)
	}
}

func TestExtractLocation_LocalityOnly(t *testing.T) {
	jp := &metaapi.JobPosting{}
	_ = jp.JobLocation.UnmarshalJSON([]byte(`{"address":{"addressLocality":"Austin"}}`))

	loc := extractLocation(jp)
	if loc != "Austin" {
		t.Errorf("extractLocation = %q, want Austin", loc)
	}
}

// ── FlexString unmarshalling ──────────────────────────────────────────────────

func TestFlexString_PlainString(t *testing.T) {
	var f metaapi.FlexString
	if err := f.UnmarshalJSON([]byte(`"USA"`)); err != nil {
		t.Fatal(err)
	}
	if f.String() != "USA" {
		t.Errorf("got %q, want USA", f.String())
	}
}

func TestFlexString_ObjectWithName(t *testing.T) {
	var f metaapi.FlexString
	if err := f.UnmarshalJSON([]byte(`{"@type":"Country","name":"USA"}`)); err != nil {
		t.Fatal(err)
	}
	if f.String() != "USA" {
		t.Errorf("got %q, want USA", f.String())
	}
}

func TestFlexString_ArrayDeduped(t *testing.T) {
	var f metaapi.FlexString
	if err := f.UnmarshalJSON([]byte(`["USA","USA","Canada"]`)); err != nil {
		t.Fatal(err)
	}
	if f.String() != "USA, Canada" {
		t.Errorf("got %q, want USA, Canada", f.String())
	}
}

func TestFlexString_NestedObjectWithListName(t *testing.T) {
	var f metaapi.FlexString
	if err := f.UnmarshalJSON([]byte(`{"@type":"Country","name":["USA","USA"]}`)); err != nil {
		t.Fatal(err)
	}
	if f.String() != "USA" {
		t.Errorf("got %q, want USA", f.String())
	}
}

func TestFlexString_Null(t *testing.T) {
	var f metaapi.FlexString
	if err := f.UnmarshalJSON([]byte("null")); err != nil {
		t.Fatal(err)
	}
	if f.String() != "" {
		t.Errorf("got %q, want empty for null", f.String())
	}
}

func TestFlexString_Empty(t *testing.T) {
	var f metaapi.FlexString
	if err := f.UnmarshalJSON([]byte("")); err != nil {
		t.Fatal(err)
	}
	if f.String() != "" {
		t.Errorf("got %q, want empty for empty input", f.String())
	}
}

// ── JobLocations unmarshalling ────────────────────────────────────────────────

func TestJobLocations_SingleObject(t *testing.T) {
	var jl metaapi.JobLocations
	if err := jl.UnmarshalJSON([]byte(`{"address":{"addressLocality":"NY"}}`)); err != nil {
		t.Fatal(err)
	}
	if len(jl) != 1 {
		t.Fatalf("got %d locations, want 1", len(jl))
	}
	if jl[0].Address.Locality.String() != "NY" {
		t.Errorf("locality = %q, want NY", jl[0].Address.Locality)
	}
}

func TestJobLocations_Array(t *testing.T) {
	var jl metaapi.JobLocations
	if err := jl.UnmarshalJSON([]byte(`[{"address":{"addressLocality":"NY"}},{"address":{"addressLocality":"SF"}}]`)); err != nil {
		t.Fatal(err)
	}
	if len(jl) != 2 {
		t.Fatalf("got %d locations, want 2", len(jl))
	}
}

func TestJobLocations_Null(t *testing.T) {
	var jl metaapi.JobLocations
	if err := jl.UnmarshalJSON([]byte("null")); err != nil {
		t.Fatal(err)
	}
	if jl != nil {
		t.Errorf("expected nil for null input, got %v", jl)
	}
}

// ── locations (detail-package internal) ──────────────────────────────────────

func TestLocations_EmptyAddressSkipped(t *testing.T) {
	// A jobLocation entry with all empty address fields should be skipped.
	jp := &metaapi.JobPosting{}
	_ = jp.JobLocation.UnmarshalJSON([]byte(`[{"address":{}},{"address":{"addressLocality":"Austin"}}]`))
	locs := locations(jp)
	if len(locs) != 1 {
		t.Fatalf("expected 1 location, got %d: %v", len(locs), locs)
	}
	if locs[0] != "Austin" {
		t.Errorf("got %q, want Austin", locs[0])
	}
}

func TestLocations_DuplicatesDeduped(t *testing.T) {
	jp := &metaapi.JobPosting{}
	_ = jp.JobLocation.UnmarshalJSON([]byte(`[
		{"address":{"addressLocality":"Menlo Park","addressRegion":"CA","addressCountry":"USA"}},
		{"address":{"addressLocality":"Menlo Park","addressRegion":"CA","addressCountry":"USA"}}
	]`))
	locs := locations(jp)
	if len(locs) != 1 {
		t.Fatalf("duplicates should be deduped, got %d: %v", len(locs), locs)
	}
}

// ── extractLocation fallback ──────────────────────────────────────────────────

func TestExtractLocation_FallbackToLocality(t *testing.T) {
	// Confirm normal path where location has only Locality.
	jp := &metaapi.JobPosting{}
	_ = jp.JobLocation.UnmarshalJSON([]byte(`{"address":{"addressLocality":"Dallas"}}`))
	loc := extractLocation(jp)
	if loc != "Dallas" {
		t.Errorf("extractLocation = %q, want Dallas", loc)
	}
}

func TestExtractLocation_FallbackRawLocality(t *testing.T) {
	// Trigger the fallback path: locations() returns empty (all address parts
	// are blank), but jobLocation is non-empty. Achieved by providing a
	// jobLocation with no address fields at all but a non-empty Locality that
	// we inject directly.
	//
	// Because FlexString unmarshals "" to "", we simulate this by constructing
	// a JobPosting whose locations() call returns empty (all-empty parts)
	// while jobLocation has a Locality set. The only way this can happen is
	// through the struct directly.
	jp := &metaapi.JobPosting{
		JobLocation: metaapi.JobLocations{
			{Address: metaapi.Address{
				// All three parts are empty → locations() skips it.
				// But Locality is still accessible for the fallback.
			}},
		},
	}
	// After JSON round-trip the empty FlexString Locality returns "".
	// So the fallback returns "".
	loc := extractLocation(jp)
	if loc != "" {
		t.Errorf("extractLocation with all-empty address = %q, want empty", loc)
	}
}
