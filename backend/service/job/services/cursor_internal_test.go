package services

import (
	"strings"
	"testing"
	"time"

	jobmodel "github.com/trushilpatel/jobradar/service/job/models"
)

// TestCursor_RoundTrip verifies the opaque keyset cursor round-trips through
// encode → decode preserving sort field, direction, primary value, and
// tiebreakers.
func TestCursor_RoundTrip(t *testing.T) {
	want := time.Date(2024, 6, 15, 12, 30, 0, 123, time.UTC)
	wantID := "job-abc"
	wantPrimVal := "some-title"

	c := encodeCursor("title", "desc", wantPrimVal, want, wantID)
	if c == "" {
		t.Fatal("encodeCursor returned empty string")
	}

	p, err := decodeCursor(c)
	if err != nil {
		t.Fatalf("decodeCursor err = %v", err)
	}
	if p == nil {
		t.Fatal("expected non-nil payload for valid cursor")
	}
	if p.ID != wantID {
		t.Errorf("id = %q, want %q", p.ID, wantID)
	}
	got := time.Unix(0, p.AddedAt)
	if !got.Equal(want) {
		t.Errorf("time = %v, want %v", got, want)
	}
	if p.PrimVal != wantPrimVal {
		t.Errorf("primVal = %q, want %q", p.PrimVal, wantPrimVal)
	}
	if p.SortBy != "title" {
		t.Errorf("sortBy = %q, want %q", p.SortBy, "title")
	}
	if p.SortDir != "desc" {
		t.Errorf("sortDir = %q, want %q", p.SortDir, "desc")
	}
}

func TestCursor_DefaultSort_RoundTrip(t *testing.T) {
	want := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	wantID := "job-xyz"

	c := encodeCursor("added_at", "desc", "", want, wantID)
	p, err := decodeCursor(c)
	if err != nil || p == nil {
		t.Fatalf("decodeCursor err = %v, p = %v", err, p)
	}
	if p.ID != wantID {
		t.Errorf("id = %q, want %q", p.ID, wantID)
	}
}

func TestCursor_EmptyMeansFirstPage(t *testing.T) {
	p, err := decodeCursor("")
	if err != nil {
		t.Fatalf("decodeCursor empty err = %v", err)
	}
	if p != nil {
		t.Errorf("empty cursor must decode to nil, got %+v", p)
	}
}

func TestCursor_UnknownFormatTreatedAsFirstPage(t *testing.T) {
	// Old-format cursor (base64 of "nano|id") should silently become first page.
	oldFmt := "MTcxNzQxMjgwMDAwMDAwMDAwMHxqb2ItYWJj" // base64url of "1717412800000000000|job-abc"
	p, err := decodeCursor(oldFmt)
	if err != nil {
		t.Errorf("unexpected error for old cursor format: %v", err)
	}
	if p != nil {
		t.Errorf("old cursor format must decode to nil (first page), got %+v", p)
	}
}

func TestCursor_MalformedBase64TreatedAsFirstPage(t *testing.T) {
	p, err := decodeCursor("!@#not-base64!@#")
	if err != nil {
		t.Errorf("unexpected error for malformed cursor: %v", err)
	}
	if p != nil {
		t.Errorf("malformed cursor must decode to nil (first page), got %+v", p)
	}
}

// TestCursor_RoundTrip_AllSortColumns exercises encode/decode for every
// whitelisted sort column. A regression where buildCursor or decodeCursor
// drops a field for any one column would silently break pagination on that
// tab — the bug class that motivated this audit.
func TestCursor_RoundTrip_AllSortColumns(t *testing.T) {
	added := time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		sortBy  string
		sortDir string
		primVal string
	}{
		{sortBy: "added_at", sortDir: "desc", primVal: ""},
		{sortBy: "added_at", sortDir: "asc", primVal: ""},
		{sortBy: "title", sortDir: "asc", primVal: "Software Engineer"},
		{sortBy: "title", sortDir: "desc", primVal: "Z-titled job"},
		{sortBy: "company", sortDir: "asc", primVal: "Google"},
		{sortBy: "company", sortDir: "desc", primVal: "Zoom"},
		{sortBy: "experience_years", sortDir: "asc", primVal: "5"},
		{sortBy: "experience_years", sortDir: "desc", primVal: "10"},
		{sortBy: "experience_years", sortDir: "asc", primVal: ""}, // null cursor
	}
	for _, tc := range tests {
		t.Run(tc.sortBy+"_"+tc.sortDir+"_"+tc.primVal, func(t *testing.T) {
			c := encodeCursor(tc.sortBy, tc.sortDir, tc.primVal, added, "job-1")
			p, err := decodeCursor(c)
			if err != nil || p == nil {
				t.Fatalf("decode err = %v, p = %v", err, p)
			}
			if p.SortBy != tc.sortBy {
				t.Errorf("SortBy = %q, want %q", p.SortBy, tc.sortBy)
			}
			if p.SortDir != tc.sortDir {
				t.Errorf("SortDir = %q, want %q", p.SortDir, tc.sortDir)
			}
			if p.PrimVal != tc.primVal {
				t.Errorf("PrimVal = %q, want %q", p.PrimVal, tc.primVal)
			}
			if p.ID != "job-1" {
				t.Errorf("ID = %q, want job-1", p.ID)
			}
			if time.Unix(0, p.AddedAt).UTC() != added {
				t.Errorf("AddedAt = %v, want %v", time.Unix(0, p.AddedAt).UTC(), added)
			}
		})
	}
}

// TestBuildCursor_ForEachSortColumn asserts buildCursor pulls the right
// primary-sort field for each sort column from a Job — the cursor must
// embed the value that the next page's predicate filters on.
func TestBuildCursor_ForEachSortColumn(t *testing.T) {
	expYears := 7
	job := jobmodel.Job{
		ID:              "job-x",
		Title:           "Senior Engineer",
		AddedAt:         time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		ExperienceYears: &expYears,
	}
	// Embedded Company comes from companymodel; we can't construct it directly
	// here without importing it, but Company.Name on a zero Company is "" which
	// is exactly what buildCursor uses for the "company" sort below — covering
	// the empty-company-name edge case.

	tests := []struct {
		name        string
		sortBy      string
		wantPrimVal string
	}{
		{name: "title sort embeds title", sortBy: "title", wantPrimVal: "Senior Engineer"},
		{name: "company sort embeds company name (empty here)", sortBy: "company", wantPrimVal: ""},
		{name: "experience sort embeds years", sortBy: "experience_years", wantPrimVal: "7"},
		{name: "added_at sort embeds empty primVal (tiebreaker carries)", sortBy: "added_at", wantPrimVal: ""},
		{name: "unrecognised sort falls back to empty primVal", sortBy: "bogus", wantPrimVal: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := buildCursor(tc.sortBy, "desc", job)
			p, err := decodeCursor(c)
			if err != nil || p == nil {
				t.Fatalf("decode err = %v, p = %v", err, p)
			}
			if p.PrimVal != tc.wantPrimVal {
				t.Errorf("PrimVal = %q, want %q", p.PrimVal, tc.wantPrimVal)
			}
			if p.ID != "job-x" {
				t.Errorf("ID = %q, want job-x", p.ID)
			}
		})
	}
}

// TestBuildCursor_NilExperienceYears — when sorting by experience_years and
// the row has a NULL value, the cursor encodes an empty PrimVal which the
// applyExperienceCursorPredicate path treats as a NULL anchor.
func TestBuildCursor_NilExperienceYears(t *testing.T) {
	job := jobmodel.Job{
		ID:              "job-null",
		AddedAt:         time.Now(),
		ExperienceYears: nil, // NULL row
	}
	c := buildCursor("experience_years", "asc", job)
	p, err := decodeCursor(c)
	if err != nil || p == nil {
		t.Fatalf("decode err = %v, p = %v", err, p)
	}
	if p.PrimVal != "" {
		t.Errorf("PrimVal = %q, want empty (signals NULL anchor)", p.PrimVal)
	}
}

// TestSortOrderClause_Whitelist asserts sortOrderClause never returns a
// clause whose primary column was caller-supplied without being on the
// whitelist. Defence-in-depth against an upstream parser miss.
func TestSortOrderClause_Whitelist(t *testing.T) {
	tests := []struct {
		sortBy   string
		sortDir  string
		wantPrim string
	}{
		{sortBy: "title", sortDir: "asc", wantPrim: "jobs.title ASC"},
		{sortBy: "title", sortDir: "desc", wantPrim: "jobs.title DESC"},
		{sortBy: "company", sortDir: "asc", wantPrim: `"Company".name ASC`},
		{sortBy: "company", sortDir: "desc", wantPrim: `"Company".name DESC`},
		{sortBy: "experience_years", sortDir: "asc", wantPrim: "jobs.experience_years ASC NULLS LAST"},
		{sortBy: "experience_years", sortDir: "desc", wantPrim: "jobs.experience_years DESC NULLS LAST"},
		{sortBy: "added_at", sortDir: "desc", wantPrim: "jobs.added_at DESC"},
		{sortBy: "added_at", sortDir: "asc", wantPrim: "jobs.added_at ASC"},
		{sortBy: "bogus_column", sortDir: "desc", wantPrim: "jobs.added_at DESC"},      // falls back to added_at
		{sortBy: "; DROP TABLE jobs", sortDir: "desc", wantPrim: "jobs.added_at DESC"}, // injection neutralised
	}
	for _, tc := range tests {
		t.Run(tc.sortBy+"/"+tc.sortDir, func(t *testing.T) {
			clause := sortOrderClause(tc.sortBy, tc.sortDir)
			if !strings.HasPrefix(clause, tc.wantPrim) {
				t.Errorf("clause = %q, want prefix %q", clause, tc.wantPrim)
			}
			// Tiebreaker invariant: every clause must end with id DESC so
			// keyset pagination remains stable across pages.
			if !strings.HasSuffix(clause, "jobs.id DESC") {
				t.Errorf("clause %q missing id DESC tiebreaker", clause)
			}
		})
	}
}

// TestSortOrderClause_DefaultDirection — an empty or unrecognised sort_dir
// defaults to DESC.
func TestSortOrderClause_DefaultDirection(t *testing.T) {
	if got := sortOrderClause("title", ""); !strings.HasPrefix(got, "jobs.title DESC") {
		t.Errorf("empty dir → %q, want DESC", got)
	}
	if got := sortOrderClause("title", "garbage"); !strings.HasPrefix(got, "jobs.title DESC") {
		t.Errorf("garbage dir → %q, want DESC", got)
	}
}
