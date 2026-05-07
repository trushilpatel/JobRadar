package services

import (
	"testing"
	"time"
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
