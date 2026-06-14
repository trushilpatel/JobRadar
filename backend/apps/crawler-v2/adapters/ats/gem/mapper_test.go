package gem

import (
	"testing"

	gemapi "github.com/trushilpatel/jobradar/apps/crawler-v2/adapters/ats/gem/api"
	"github.com/trushilpatel/jobradar/common/ats/core"
)

// ── mapJobSummary ─────────────────────────────────────────────────────────────

func TestMapJobSummary_AllFields(t *testing.T) {
	input := gemapi.JobPost{
		ID:             "abc123",
		Title:          "Backend Engineer",
		AbsoluteURL:    "https://jobs.gem.com/inception/posts/abc123",
		ContentPlain:   "Great role.",
		Location:       gemapi.Location{Name: "San Francisco, CA"},
		LocationType:   "hybrid",
		EmploymentType: "full_time",
		Departments: []gemapi.Taxonomy{
			{ID: "d1", Name: "Engineering"},
		},
		FirstPublishedAt: "2025-01-16T09:00:00Z",
		UpdatedAt:        "2025-03-01T08:30:00Z",
		CreatedAt:        "2025-01-15T10:00:00Z",
	}

	got := mapJobSummary(input)

	if got.ID != "gem-abc123" {
		t.Errorf("ID = %q, want gem-abc123", got.ID)
	}
	if got.Title != "Backend Engineer" {
		t.Errorf("Title = %q, want Backend Engineer", got.Title)
	}
	if got.URL != "https://jobs.gem.com/inception/posts/abc123" {
		t.Errorf("URL = %q", got.URL)
	}
	if got.Location != "San Francisco, CA" {
		t.Errorf("Location = %q, want San Francisco, CA", got.Location)
	}
	if got.EmployType != core.EmployFullTime {
		t.Errorf("EmployType = %q, want %q", got.EmployType, core.EmployFullTime)
	}
	if !got.LocationKnown {
		t.Error("LocationKnown should be true")
	}
	if len(got.AllLocations) != 1 || got.AllLocations[0] != "San Francisco, CA" {
		t.Errorf("AllLocations = %v, want [San Francisco, CA]", got.AllLocations)
	}
	if got.JobContent == nil {
		t.Fatal("JobContent is nil")
	}
	if got.JobContent.Department != "Engineering" {
		t.Errorf("JobContent.Department = %q, want Engineering", got.JobContent.Department)
	}
	if got.JobContent.WorkplaceType != "hybrid" {
		t.Errorf("JobContent.WorkplaceType = %q, want hybrid", got.JobContent.WorkplaceType)
	}
}

func TestMapJobSummary_OfficeFallback(t *testing.T) {
	// Empty location.name — should fall back to joining office location names.
	input := gemapi.JobPost{
		ID:       "xyz999",
		Title:    "Designer",
		Location: gemapi.Location{Name: ""},
		Offices: []gemapi.Office{
			{Location: gemapi.Location{Name: "New York, NY"}},
			{Location: gemapi.Location{Name: "Austin, TX"}},
		},
	}

	got := mapJobSummary(input)

	want := "New York, NY; Austin, TX"
	if got.Location != want {
		t.Errorf("Location = %q, want %q", got.Location, want)
	}
	if len(got.AllLocations) != 1 || got.AllLocations[0] != want {
		t.Errorf("AllLocations = %v, want [%s]", got.AllLocations, want)
	}
}

func TestMapJobSummary_JobContentPopulated(t *testing.T) {
	input := gemapi.JobPost{
		ID:           "j1",
		ContentPlain: "Plain description.",
		Departments:  []gemapi.Taxonomy{{ID: "d1", Name: "Product"}},
		LocationType: "remote",
	}

	got := mapJobSummary(input)

	if got.JobContent == nil {
		t.Fatal("JobContent must not be nil for fat adapter")
	}
	if got.JobContent.Description != "Plain description." {
		t.Errorf("Description = %q, want Plain description.", got.JobContent.Description)
	}
	if got.JobContent.Department != "Product" {
		t.Errorf("Department = %q, want Product", got.JobContent.Department)
	}
	if got.JobContent.WorkplaceType != "remote" {
		t.Errorf("WorkplaceType = %q, want remote", got.JobContent.WorkplaceType)
	}
}

func TestMapJobSummary_PublishedDatePriority(t *testing.T) {
	tests := []struct {
		name             string
		firstPublishedAt string
		updatedAt        string
		createdAt        string
		want             string
	}{
		{
			name:             "first_published_at preferred",
			firstPublishedAt: "2025-01-16T09:00:00Z",
			updatedAt:        "2025-03-01T08:30:00Z",
			createdAt:        "2025-01-15T10:00:00Z",
			want:             "2025-01-16T09:00:00Z",
		},
		{
			name:             "falls back to updated_at when first_published_at empty",
			firstPublishedAt: "",
			updatedAt:        "2025-03-01T08:30:00Z",
			createdAt:        "2025-01-15T10:00:00Z",
			want:             "2025-03-01T08:30:00Z",
		},
		{
			name:             "falls back to created_at when both empty",
			firstPublishedAt: "",
			updatedAt:        "",
			createdAt:        "2025-01-15T10:00:00Z",
			want:             "2025-01-15T10:00:00Z",
		},
		{
			name:             "empty when all missing",
			firstPublishedAt: "",
			updatedAt:        "",
			createdAt:        "",
			want:             "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := gemapi.JobPost{
				ID:               "j1",
				FirstPublishedAt: tt.firstPublishedAt,
				UpdatedAt:        tt.updatedAt,
				CreatedAt:        tt.createdAt,
			}
			got := mapJobSummary(input)
			if got.JobContent == nil {
				t.Fatal("JobContent is nil")
			}
			if got.JobContent.PublishedDate != tt.want {
				t.Errorf("PublishedDate = %q, want %q", got.JobContent.PublishedDate, tt.want)
			}
		})
	}
}

// ── mapEmploymentType ─────────────────────────────────────────────────────────

func TestMapEmploymentType(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"full_time", core.EmployFullTime},
		{"FULL_TIME", core.EmployFullTime},
		{"part_time", core.EmployPartTime},
		{"contract", core.EmployContract},
		{"intern", core.EmployInternship},
		{"internship", core.EmployInternship},
		{"INTERNSHIP", core.EmployInternship},
		{"", core.EmployFullTime},             // unknown → default full_time
		{"unknown_type", core.EmployFullTime}, // unknown → default full_time
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := mapEmploymentType(tt.input)
			if got != tt.want {
				t.Errorf("mapEmploymentType(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ── mapWorkplaceType ──────────────────────────────────────────────────────────

func TestMapWorkplaceType(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"remote", "remote"},
		{"REMOTE", "remote"},
		{"hybrid", "hybrid"},
		{"Hybrid", "hybrid"},
		{"in_office", "onsite"},
		{"onsite", "onsite"},
		{"on_site", "onsite"},
		{"ON_SITE", "onsite"},
		{"", ""},
		{"unknown", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := mapWorkplaceType(tt.input)
			if got != tt.want {
				t.Errorf("mapWorkplaceType(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ── mapJobListing ─────────────────────────────────────────────────────────────

func TestMapJobListing_MultipleJobs(t *testing.T) {
	posts := []gemapi.JobPost{
		{ID: "j1", Title: "Engineer A"},
		{ID: "j2", Title: "Engineer B"},
	}
	got := mapJobListing(posts)
	if len(got.Jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(got.Jobs))
	}
	if got.Jobs[0].ID != "gem-j1" {
		t.Errorf("jobs[0].ID = %q, want gem-j1", got.Jobs[0].ID)
	}
	if got.Jobs[1].ID != "gem-j2" {
		t.Errorf("jobs[1].ID = %q, want gem-j2", got.Jobs[1].ID)
	}
}

func TestMapJobListing_Empty(t *testing.T) {
	got := mapJobListing(nil)
	if len(got.Jobs) != 0 {
		t.Errorf("expected empty jobs, got %d", len(got.Jobs))
	}
}
