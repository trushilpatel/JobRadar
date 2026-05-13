package services

import (
	"testing"
)

// parseExperienceRange is an internal handler helper. The following cases
// guard the parse rules in JobRestService.go so a wrong fix doesn't quietly
// silence input the UI is depending on (or accept bogus ranges that hit the
// DB layer).
func TestParseExperienceRange(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantOK  bool
		wantMin int
		wantMax *int // nil means unbounded
	}{
		{name: "empty", input: "", wantOK: false},
		{name: "open ended valid", input: "5+", wantOK: true, wantMin: 5, wantMax: nil},
		{name: "open ended zero", input: "0+", wantOK: true, wantMin: 0, wantMax: nil},
		{name: "open ended negative rejected", input: "-1+", wantOK: false},
		{name: "open ended non numeric", input: "abc+", wantOK: false},
		{name: "open ended just plus", input: "+", wantOK: false},

		{name: "closed range valid", input: "0-2", wantOK: true, wantMin: 0, wantMax: intPtr(2)},
		{name: "closed range valid mid", input: "5-10", wantOK: true, wantMin: 5, wantMax: intPtr(10)},
		{name: "closed range max equals min rejected", input: "5-5", wantOK: false},
		{name: "closed range max less than min rejected", input: "5-3", wantOK: false},
		{name: "closed range negative min rejected", input: "-1-5", wantOK: false},
		{name: "closed range negative max", input: "5--3", wantOK: false},
		{name: "closed range empty max", input: "5-", wantOK: false},
		{name: "closed range empty min", input: "-5", wantOK: false},
		{name: "closed range non numeric", input: "abc-def", wantOK: false},
		{name: "closed range non numeric max", input: "5-abc", wantOK: false},
		{name: "no separator", input: "10", wantOK: false},
		{name: "extra separators", input: "5-10-15", wantOK: false},
		{name: "leading whitespace rejected", input: " 5+", wantOK: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rng, ok := parseExperienceRange(tc.input)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v (range = %+v)", ok, tc.wantOK, rng)
			}
			if !ok {
				return
			}
			if rng.Min != tc.wantMin {
				t.Errorf("Min = %d, want %d", rng.Min, tc.wantMin)
			}
			if (rng.Max == nil) != (tc.wantMax == nil) {
				t.Fatalf("Max nil mismatch: got %v want %v", rng.Max, tc.wantMax)
			}
			if rng.Max != nil && *rng.Max != *tc.wantMax {
				t.Errorf("Max = %d, want %d", *rng.Max, *tc.wantMax)
			}
		})
	}
}

func intPtr(n int) *int { return &n }
