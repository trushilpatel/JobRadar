package request

import (
	"net/http/httptest"
	"testing"
)

// TestDefaultPagination asserts the documented defaults: page=1, limit=20.
// These values are consumed by every list endpoint; changing them is a
// breaking client contract.
func TestDefaultPagination(t *testing.T) {
	p := DefaultPagination()
	if p.Page != 1 {
		t.Errorf("Page=%d want 1", p.Page)
	}
	if p.Limit != 20 {
		t.Errorf("Limit=%d want 20", p.Limit)
	}
}

func TestParsePagination_DefaultsWhenMissing(t *testing.T) {
	r := httptest.NewRequest("GET", "/x", nil)
	p := ParsePagination(r)
	if p.Page != 1 || p.Limit != 20 {
		t.Errorf("got page=%d limit=%d, want defaults 1/20", p.Page, p.Limit)
	}
}

func TestParsePagination_ValidParams(t *testing.T) {
	r := httptest.NewRequest("GET", "/x?page=3&limit=50", nil)
	p := ParsePagination(r)
	if p.Page != 3 {
		t.Errorf("Page=%d want 3", p.Page)
	}
	if p.Limit != 50 {
		t.Errorf("Limit=%d want 50", p.Limit)
	}
}

func TestParsePagination_LimitClampedToMax(t *testing.T) {
	r := httptest.NewRequest("GET", "/x?limit=999", nil)
	p := ParsePagination(r)
	if p.Limit != 100 {
		t.Errorf("Limit=%d want 100 (clamp)", p.Limit)
	}
}

func TestParsePagination_LimitClampedToOneWhenZero(t *testing.T) {
	r := httptest.NewRequest("GET", "/x?limit=0", nil)
	p := ParsePagination(r)
	if p.Limit != 1 {
		t.Errorf("Limit=%d want 1 (clamp)", p.Limit)
	}
}

func TestParsePagination_LimitClampedToOneWhenNegative(t *testing.T) {
	r := httptest.NewRequest("GET", "/x?limit=-5", nil)
	p := ParsePagination(r)
	if p.Limit != 1 {
		t.Errorf("Limit=%d want 1 (clamp)", p.Limit)
	}
}

func TestParsePagination_PageBelowOneIgnored(t *testing.T) {
	// page<1 must NOT overwrite the default — keeps Page=1.
	cases := []string{"/x?page=0", "/x?page=-3"}
	for _, url := range cases {
		t.Run(url, func(t *testing.T) {
			r := httptest.NewRequest("GET", url, nil)
			p := ParsePagination(r)
			if p.Page != 1 {
				t.Errorf("Page=%d want 1 for %q", p.Page, url)
			}
		})
	}
}

func TestParsePagination_NonIntegerIgnored(t *testing.T) {
	r := httptest.NewRequest("GET", "/x?page=abc&limit=xyz", nil)
	p := ParsePagination(r)
	if p.Page != 1 || p.Limit != 20 {
		t.Errorf("non-int values must be ignored; got page=%d limit=%d", p.Page, p.Limit)
	}
}

func TestParsePagination_EmptyValuesIgnored(t *testing.T) {
	// `?page=&limit=` — the keys are present but values are empty; the parser
	// treats this identically to missing.
	r := httptest.NewRequest("GET", "/x?page=&limit=", nil)
	p := ParsePagination(r)
	if p.Page != 1 || p.Limit != 20 {
		t.Errorf("empty values must keep defaults; got page=%d limit=%d", p.Page, p.Limit)
	}
}

// TestParseLimit_MatchesParsePaginationBehavior locks the raw-string variant
// (used by Huma input Resolvers, see apihuma) to the exact same clamp/
// fallback semantics as ParsePagination's inline limit handling.
func TestParseLimit_MatchesParsePaginationBehavior(t *testing.T) {
	cases := []struct {
		raw  string
		want int
	}{
		{"", 20},
		{"50", 50},
		{"999", 100},
		{"0", 1},
		{"-5", 1},
		{"xyz", 20},
		{"100", 100},
		{"1", 1},
	}
	for _, c := range cases {
		t.Run(c.raw, func(t *testing.T) {
			if got := ParseLimit(c.raw, 20); got != c.want {
				t.Errorf("ParseLimit(%q, 20)=%d want %d", c.raw, got, c.want)
			}
		})
	}
}

// TestParsePage_MatchesParsePaginationBehavior locks the raw-string variant
// to ParsePagination's exact page fallback semantics.
func TestParsePage_MatchesParsePaginationBehavior(t *testing.T) {
	cases := []struct {
		raw  string
		want int
	}{
		{"", 1},
		{"3", 3},
		{"0", 1},
		{"-3", 1},
		{"abc", 1},
		{"2147483647", 2147483647},
	}
	for _, c := range cases {
		t.Run(c.raw, func(t *testing.T) {
			if got := ParsePage(c.raw, 1); got != c.want {
				t.Errorf("ParsePage(%q, 1)=%d want %d", c.raw, got, c.want)
			}
		})
	}
}

func TestParsePagination_BoundaryValues(t *testing.T) {
	// Boundary cases at the documented edges.
	cases := []struct {
		url       string
		wantPage  int
		wantLimit int
	}{
		{"/x?page=1&limit=1", 1, 1},
		{"/x?page=1&limit=100", 1, 100},
		{"/x?page=2147483647&limit=100", 2147483647, 100},
	}
	for _, c := range cases {
		t.Run(c.url, func(t *testing.T) {
			r := httptest.NewRequest("GET", c.url, nil)
			p := ParsePagination(r)
			if p.Page != c.wantPage || p.Limit != c.wantLimit {
				t.Errorf("got page=%d limit=%d, want %d/%d", p.Page, p.Limit, c.wantPage, c.wantLimit)
			}
		})
	}
}
