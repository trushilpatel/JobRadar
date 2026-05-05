package services

import (
	"testing"
	"time"
)

// TestCursor_RoundTrip verifies the opaque keyset cursor encodes a (time,
// id) pair and decodes back to the same values. Bound to the package so
// callers never need to know the encoding format — only that it is
// stable across encode→decode.
func TestCursor_RoundTrip(t *testing.T) {
	want := time.Date(2024, 6, 15, 12, 30, 0, 123, time.UTC)
	wantID := "job-abc"

	c := encodeCursor(want, wantID)
	if c == "" {
		t.Fatal("encodeCursor returned empty string")
	}

	got, gotID, err := decodeCursor(c)
	if err != nil {
		t.Fatalf("decodeCursor err = %v", err)
	}
	if !got.Equal(want) {
		t.Errorf("time = %v, want %v", got, want)
	}
	if gotID != wantID {
		t.Errorf("id = %q, want %q", gotID, wantID)
	}
}

func TestCursor_EmptyMeansFirstPage(t *testing.T) {
	got, gotID, err := decodeCursor("")
	if err != nil {
		t.Fatalf("decodeCursor empty err = %v", err)
	}
	if !got.IsZero() || gotID != "" {
		t.Errorf("empty cursor must decode to zero values, got (%v, %q)", got, gotID)
	}
}

func TestCursor_InvalidRejected(t *testing.T) {
	if _, _, err := decodeCursor("!@#not-base64!@#"); err == nil {
		t.Error("expected error for malformed cursor")
	}
}
