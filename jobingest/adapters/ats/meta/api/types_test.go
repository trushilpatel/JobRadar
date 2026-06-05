package api

import (
	"encoding/json"
	"testing"
)

// ── FlexString ─────────────────────────────────────────────────────────────

func TestFlexString_PlainString(t *testing.T) {
	var f FlexString
	if err := json.Unmarshal([]byte(`"Menlo Park"`), &f); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if string(f) != "Menlo Park" {
		t.Errorf("got %q, want Menlo Park", f)
	}
}

func TestFlexString_Null(t *testing.T) {
	var f FlexString
	if err := json.Unmarshal([]byte(`null`), &f); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if string(f) != "" {
		t.Errorf("got %q, want empty", f)
	}
}

func TestFlexString_ObjectWithName(t *testing.T) {
	var f FlexString
	if err := json.Unmarshal([]byte(`{"name":"New York"}`), &f); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if string(f) != "New York" {
		t.Errorf("got %q, want New York", f)
	}
}

func TestFlexString_ObjectWithoutName(t *testing.T) {
	var f FlexString
	if err := json.Unmarshal([]byte(`{"other":"value"}`), &f); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if string(f) != "" {
		t.Errorf("got %q, want empty", f)
	}
}

func TestFlexString_Array(t *testing.T) {
	var f FlexString
	if err := json.Unmarshal([]byte(`["Seattle","Bellevue","Seattle"]`), &f); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	// Should dedupe Seattle, join with "; "
	got := string(f)
	if got != "Seattle, Bellevue" {
		t.Errorf("got %q, want Seattle, Bellevue", got)
	}
}

func TestFlexString_ArrayEmptyItems(t *testing.T) {
	var f FlexString
	if err := json.Unmarshal([]byte(`["","Paris"]`), &f); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if string(f) != "Paris" {
		t.Errorf("got %q, want Paris", f)
	}
}

func TestFlexString_ArrayDeduplicates(t *testing.T) {
	var f FlexString
	// "Seattle" appears twice — dedup must skip second occurrence
	if err := json.Unmarshal([]byte(`["Seattle","Seattle","Portland"]`), &f); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if string(f) != "Seattle, Portland" {
		t.Errorf("got %q, want Seattle, Portland", f)
	}
}

func TestFlexString_String(t *testing.T) {
	f := FlexString("Austin, TX")
	if f.String() != "Austin, TX" {
		t.Errorf("String() = %q, want Austin, TX", f.String())
	}
}

func TestFlexString_UnknownShape(t *testing.T) {
	// A bare JSON number is not a string, object, or array — hits the unknown shape path.
	var f FlexString
	if err := json.Unmarshal([]byte(`42`), &f); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(f) != "" {
		t.Errorf("got %q, want empty for unknown shape", f)
	}
}

// ── JobLocations ───────────────────────────────────────────────────────────

func TestJobLocations_Array(t *testing.T) {
	var locs JobLocations
	data := `[{"@type":"Place","address":{"addressLocality":"SF"}},{"@type":"Place","address":{"addressLocality":"NY"}}]`
	if err := json.Unmarshal([]byte(data), &locs); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if len(locs) != 2 {
		t.Fatalf("got %d locations, want 2", len(locs))
	}
}

func TestJobLocations_SingleObject(t *testing.T) {
	var locs JobLocations
	data := `{"@type":"Place","address":{"addressLocality":"Menlo Park"}}`
	if err := json.Unmarshal([]byte(data), &locs); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if len(locs) != 1 {
		t.Fatalf("got %d locations, want 1", len(locs))
	}
}

func TestJobLocations_Empty(t *testing.T) {
	var locs JobLocations
	if err := locs.UnmarshalJSON([]byte{}); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if locs != nil {
		t.Errorf("expected nil, got %v", locs)
	}
}

func TestJobLocations_Null(t *testing.T) {
	var locs JobLocations
	if err := json.Unmarshal([]byte(`null`), &locs); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if locs != nil {
		t.Errorf("expected nil, got %v", locs)
	}
}

func TestJobLocations_InvalidArrayJSON(t *testing.T) {
	var locs JobLocations
	// Invalid JSON inside an array bracket triggers the Unmarshal error path
	err := json.Unmarshal([]byte(`[{invalid}]`), &locs)
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
}

func TestJobLocations_InvalidSingleJSON(t *testing.T) {
	var locs JobLocations
	// Not an array, not valid JSON object → triggers single-object error path
	err := json.Unmarshal([]byte(`not-json`), &locs)
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
}
