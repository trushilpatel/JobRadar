package netflix

import (
	"strings"
	"testing"

	nfapi "github.com/trushilpatel/jobradar/apps/crawler-v2/adapters/ats/netflix/api"
	"github.com/trushilpatel/jobradar/common/ats/core"
)

// ── mapJobSummary ─────────────────────────────────────────────────────────────

func TestMapJobSummary_LocationFallback(t *testing.T) {
	tests := []struct {
		name      string
		location  string
		locations []string
		wantLoc   string
	}{
		{
			name:      "explicit location used",
			location:  "San Francisco, CA",
			locations: []string{"New York, NY"},
			wantLoc:   "San Francisco, CA",
		},
		{
			name:      "empty location falls back to joined locations",
			location:  "",
			locations: []string{"NYC", "Remote"},
			wantLoc:   "NYC; Remote",
		},
		{
			name:      "single fallback location",
			location:  "",
			locations: []string{"Chicago"},
			wantLoc:   "Chicago",
		},
		{
			name:      "both empty",
			location:  "",
			locations: nil,
			wantLoc:   "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := nfapi.Position{
				ID:           1,
				Name:         "Engineer",
				Location:     tt.location,
				Locations:    tt.locations,
				CanonicalURL: "https://explore.jobs.netflix.net/careers/job/1",
			}
			got := mapJobSummary(p)
			if got.Location != tt.wantLoc {
				t.Errorf("Location = %q, want %q", got.Location, tt.wantLoc)
			}
		})
	}
}

func TestMapJobSummary_URLFallback(t *testing.T) {
	tests := []struct {
		name         string
		canonicalURL string
		id           int64
		wantURL      string
	}{
		{
			name:         "canonical URL used when present",
			canonicalURL: "https://explore.jobs.netflix.net/careers/job/42",
			id:           42,
			wantURL:      "https://explore.jobs.netflix.net/careers/job/42",
		},
		{
			name:         "fallback constructed when canonical empty",
			canonicalURL: "",
			id:           99,
			wantURL:      defaultBaseURL + "/careers/job/99",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := nfapi.Position{
				ID:           tt.id,
				Name:         "Engineer",
				Location:     "NYC",
				CanonicalURL: tt.canonicalURL,
			}
			got := mapJobSummary(p)
			if got.URL != tt.wantURL {
				t.Errorf("URL = %q, want %q", got.URL, tt.wantURL)
			}
		})
	}
}

func TestMapJobSummary_AllLocationsSlice(t *testing.T) {
	t.Run("single location wraps in slice", func(t *testing.T) {
		p := nfapi.Position{
			ID:           1,
			Name:         "Engineer",
			Location:     "LA",
			Locations:    []string{"NYC"},
			CanonicalURL: "https://example.com/job/1",
		}
		got := mapJobSummary(p)
		if len(got.AllLocations) != 1 || got.AllLocations[0] != "LA" {
			t.Errorf("AllLocations = %v, want [LA]", got.AllLocations)
		}
	})
	t.Run("fallback locations propagated", func(t *testing.T) {
		p := nfapi.Position{
			ID:           2,
			Name:         "Engineer",
			Location:     "",
			Locations:    []string{"NYC", "Remote"},
			CanonicalURL: "https://example.com/job/2",
		}
		got := mapJobSummary(p)
		if len(got.AllLocations) != 2 {
			t.Errorf("AllLocations len = %d, want 2", len(got.AllLocations))
		}
	})
}

func TestMapJobSummary_EmployType(t *testing.T) {
	p := nfapi.Position{ID: 1, Name: "Engineer", CanonicalURL: "https://example.com/job/1"}
	got := mapJobSummary(p)
	if got.EmployType != core.EmployFullTime {
		t.Errorf("EmployType = %q, want %q", got.EmployType, core.EmployFullTime)
	}
}

func TestMapJobSummary_LocationKnown(t *testing.T) {
	p := nfapi.Position{ID: 1, Name: "Engineer", CanonicalURL: "https://example.com/job/1"}
	got := mapJobSummary(p)
	if !got.LocationKnown {
		t.Error("LocationKnown should always be true")
	}
}

// ── mapJobPost ────────────────────────────────────────────────────────────────

func TestMapJobPost_AllFields(t *testing.T) {
	p := nfapi.Position{
		ID:                 790315382430,
		Name:               "Senior Software Engineer",
		Location:           "Los Angeles, CA",
		Locations:          []string{"Los Angeles, CA"},
		Department:         "Engineering",
		TCreate:            1775779200,
		WorkLocationOption: "onsite",
		CanonicalURL:       "https://explore.jobs.netflix.net/careers/job/790315382430",
		JobDescription:     "<p>Build great things.</p>",
	}
	got := mapJobPost(p)

	if got.ID != "netflix-790315382430" {
		t.Errorf("ID = %q, want netflix-790315382430", got.ID)
	}
	if got.Title != "Senior Software Engineer" {
		t.Errorf("Title = %q, want Senior Software Engineer", got.Title)
	}
	if got.Location != "Los Angeles, CA" {
		t.Errorf("Location = %q, want Los Angeles, CA", got.Location)
	}
	if got.URL != "https://explore.jobs.netflix.net/careers/job/790315382430" {
		t.Errorf("URL = %q, want canonical URL", got.URL)
	}
	if got.Department != "Engineering" {
		t.Errorf("Department = %q, want Engineering", got.Department)
	}
	if got.WorkplaceType != "onsite" {
		t.Errorf("WorkplaceType = %q, want onsite", got.WorkplaceType)
	}
	if got.PublishedDate == "" {
		t.Error("PublishedDate is empty")
	}
	if got.Description == "" {
		t.Error("Description is empty")
	}
}

func TestMapJobPost_DescriptionStripped(t *testing.T) {
	tests := []struct {
		name     string
		html     string
		wantFree bool // true = no HTML tags should remain
	}{
		{
			name:     "paragraph and list",
			html:     "<p>Join our team.</p><ul><li>Build things</li></ul>",
			wantFree: true,
		},
		{
			name:     "empty description",
			html:     "",
			wantFree: true,
		},
		{
			name:     "plain text passthrough",
			html:     "No HTML here",
			wantFree: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := nfapi.Position{
				ID:             1,
				CanonicalURL:   "https://example.com/job/1",
				JobDescription: tt.html,
			}
			got := mapJobPost(p)
			if tt.wantFree && strings.Contains(got.Description, "<") {
				t.Errorf("Description contains HTML tags: %q", got.Description)
			}
		})
	}
}

func TestMapJobPost_ZeroTCreate(t *testing.T) {
	p := nfapi.Position{ID: 1, CanonicalURL: "https://example.com/job/1", TCreate: 0}
	got := mapJobPost(p)
	if got.PublishedDate != "" {
		t.Errorf("PublishedDate should be empty for zero TCreate, got %q", got.PublishedDate)
	}
}

func TestMapJobPost_IDFormat(t *testing.T) {
	tests := []struct {
		id   int64
		want string
	}{
		{1, "netflix-1"},
		{790315382430, "netflix-790315382430"},
		{9999999999999, "netflix-9999999999999"},
	}
	for _, tt := range tests {
		p := nfapi.Position{ID: tt.id, CanonicalURL: "https://example.com/job/1"}
		got := mapJobPost(p)
		if got.ID != tt.want {
			t.Errorf("id=%d: got %q, want %q", tt.id, got.ID, tt.want)
		}
	}
}
