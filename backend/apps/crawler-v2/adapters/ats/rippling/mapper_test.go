package rippling

import (
	"testing"

	rapi "github.com/trushilpatel/jobradar/apps/crawler-v2/adapters/ats/rippling/api"
	"github.com/trushilpatel/jobradar/common/ats/core"
)

const testATSBase = "https://ats.rippling.com"

// ── mapJobSummary ─────────────────────────────────────────────────────────────

func TestMapJobSummary_AllFields(t *testing.T) {
	item := rapi.ListingJob{
		ID:         "550e8400",
		Name:       "Backend Engineer",
		URL:        "https://ats.rippling.com/rippling/jobs/550e8400",
		Department: rapi.Department{Name: "Engineering"},
		Locations: []rapi.Location{
			{CountryCode: "US", City: "San Francisco", StateCode: "CA"},
		},
	}
	got := mapJobSummary(item, testATSBase)

	if got.ID != "rippling-550e8400" {
		t.Errorf("ID = %q, want rippling-550e8400", got.ID)
	}
	if got.Title != "Backend Engineer" {
		t.Errorf("Title = %q, want Backend Engineer", got.Title)
	}
	if got.Location != "San Francisco, CA" {
		t.Errorf("Location = %q, want San Francisco, CA", got.Location)
	}
	if got.URL != "https://ats.rippling.com/rippling/jobs/550e8400" {
		t.Errorf("URL = %q, want original URL", got.URL)
	}
	if got.Adapter != "rippling" {
		t.Errorf("Adapter = %q, want rippling", got.Adapter)
	}
	if !got.LocationKnown {
		t.Error("LocationKnown should be true")
	}
	if got.EmployType != core.EmployFullTime {
		t.Errorf("EmployType = %q, want full_time", got.EmployType)
	}
	if len(got.AllLocations) != 1 || got.AllLocations[0] != "San Francisco, CA" {
		t.Errorf("AllLocations = %v, want [San Francisco, CA]", got.AllLocations)
	}
}

func TestMapJobSummary_URLFallback(t *testing.T) {
	// When job has no URL, construct from ats base + id.
	item := rapi.ListingJob{
		ID:        "no-url-id",
		Name:      "Eng",
		URL:       "",
		Locations: []rapi.Location{{CountryCode: "US", City: "SF", StateCode: "CA"}},
	}
	got := mapJobSummary(item, testATSBase)
	want := "https://ats.rippling.com/rippling/jobs/no-url-id"
	if got.URL != want {
		t.Errorf("URL = %q, want %q", got.URL, want)
	}
}

func TestMapJobSummary_MultipleUSLocations(t *testing.T) {
	item := rapi.ListingJob{
		ID:   "multi-loc",
		Name: "Eng",
		URL:  "https://ats.rippling.com/rippling/jobs/multi-loc",
		Locations: []rapi.Location{
			{CountryCode: "US", City: "San Francisco", StateCode: "CA"},
			{CountryCode: "US", City: "New York", StateCode: "NY"},
		},
	}
	got := mapJobSummary(item, testATSBase)
	if got.Location != "San Francisco, CA" {
		t.Errorf("Location = %q, want first US location", got.Location)
	}
	if len(got.AllLocations) != 2 {
		t.Errorf("AllLocations len = %d, want 2", len(got.AllLocations))
	}
}

// ── mapJobListing (US filter) ─────────────────────────────────────────────────

func TestMapJobListing_USFilter(t *testing.T) {
	items := []rapi.ListingJob{
		{
			ID:        "us-001",
			Name:      "US Job",
			URL:       "https://ats.rippling.com/rippling/jobs/us-001",
			Locations: []rapi.Location{{CountryCode: "US", City: "SF", StateCode: "CA"}},
		},
		{
			ID:        "gb-001",
			Name:      "UK Job",
			URL:       "https://ats.rippling.com/rippling/jobs/gb-001",
			Locations: []rapi.Location{{CountryCode: "GB", City: "London", StateCode: "ENG"}},
		},
	}
	got := mapJobListing(items, testATSBase)
	if len(got.Jobs) != 1 {
		t.Fatalf("expected 1 US job, got %d", len(got.Jobs))
	}
	if got.Jobs[0].ID != "rippling-us-001" {
		t.Errorf("job.ID = %q, want rippling-us-001", got.Jobs[0].ID)
	}
}

func TestMapJobListing_Empty(t *testing.T) {
	got := mapJobListing(nil, testATSBase)
	if len(got.Jobs) != 0 {
		t.Errorf("expected empty jobs, got %d", len(got.Jobs))
	}
}

func TestMapJobListing_AllNonUS(t *testing.T) {
	items := []rapi.ListingJob{
		{ID: "a", Locations: []rapi.Location{{CountryCode: "GB"}}},
		{ID: "b", Locations: []rapi.Location{{CountryCode: "DE"}}},
	}
	got := mapJobListing(items, testATSBase)
	if len(got.Jobs) != 0 {
		t.Errorf("expected 0 jobs, got %d", len(got.Jobs))
	}
}

func TestMapJobListing_Dedup(t *testing.T) {
	items := []rapi.ListingJob{
		{ID: "dup", Name: "First", URL: "u1", Locations: []rapi.Location{{CountryCode: "US", City: "SF", StateCode: "CA"}}},
		{ID: "dup", Name: "Second", URL: "u2", Locations: []rapi.Location{{CountryCode: "US", City: "NY", StateCode: "NY"}}},
	}
	got := mapJobListing(items, testATSBase)
	if len(got.Jobs) != 1 {
		t.Errorf("expected 1 after dedup, got %d", len(got.Jobs))
	}
	// First occurrence wins.
	if got.Jobs[0].Title != "First" {
		t.Errorf("Title = %q, want First (first occurrence wins)", got.Jobs[0].Title)
	}
}

// ── mapJobPost ────────────────────────────────────────────────────────────────

func TestMapJobPost_AllFields(t *testing.T) {
	detail := rapi.DetailJob{
		ID:              "aaa-111",
		Title:           "Software Engineer",
		DescriptionHTML: "<h2>About</h2><p>Join us.</p>",
		Compensation:    "$180,000 - $245,000 USD/year",
		WorkplaceType:   "ON_SITE",
		Department:      rapi.Department{Name: "Engineering"},
		Locations:       []rapi.Location{{CountryCode: "US", City: "San Francisco", StateCode: "CA"}},
		CreatedAt:       "2026-03-01T00:00:00.000Z",
	}
	got := mapJobPost(detail, "https://ats.rippling.com/rippling/jobs/aaa-111")

	if got.ID != "rippling-aaa-111" {
		t.Errorf("ID = %q, want rippling-aaa-111", got.ID)
	}
	if got.Title != "Software Engineer" {
		t.Errorf("Title = %q, want Software Engineer", got.Title)
	}
	if got.Description == "" {
		t.Error("Description is empty")
	}
	// HTML should be stripped.
	if got.Description == "<h2>About</h2><p>Join us.</p>" {
		t.Error("Description should not contain raw HTML")
	}
	if got.Compensation != "$180,000 - $245,000 USD/year" {
		t.Errorf("Compensation = %q, want $180,000 - $245,000 USD/year", got.Compensation)
	}
	if got.WorkplaceType != "ON_SITE" {
		t.Errorf("WorkplaceType = %q, want ON_SITE", got.WorkplaceType)
	}
	if got.Department != "Engineering" {
		t.Errorf("Department = %q, want Engineering", got.Department)
	}
	if got.PublishedDate != "2026-03-01T00:00:00.000Z" {
		t.Errorf("PublishedDate = %q, want 2026-03-01T00:00:00.000Z", got.PublishedDate)
	}
	if got.Location != "San Francisco, CA" {
		t.Errorf("Location = %q, want San Francisco, CA", got.Location)
	}
}

func TestMapJobPost_DescriptionFallback(t *testing.T) {
	tests := []struct {
		name   string
		detail rapi.DetailJob
		wantIn string // substring that should appear in Description
	}{
		{
			name:   "descriptionHtml first",
			detail: rapi.DetailJob{ID: "x", DescriptionHTML: "<p>html</p>", Description: "plain", Body: "body"},
			wantIn: "html",
		},
		{
			name:   "description when no html",
			detail: rapi.DetailJob{ID: "x", Description: "plain desc", Body: "body"},
			wantIn: "plain desc",
		},
		{
			name:   "jobDescription fallback",
			detail: rapi.DetailJob{ID: "x", JobDescription: "job desc", Body: "body"},
			wantIn: "job desc",
		},
		{
			name:   "body as last resort",
			detail: rapi.DetailJob{ID: "x", Body: "body text"},
			wantIn: "body text",
		},
		{
			name:   "empty when all empty",
			detail: rapi.DetailJob{ID: "x"},
			wantIn: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapJobPost(tt.detail, "url")
			if tt.wantIn == "" {
				if got.Description != "" {
					t.Errorf("expected empty description, got %q", got.Description)
				}
				return
			}
			if !containsStr(got.Description, tt.wantIn) {
				t.Errorf("Description = %q, want to contain %q", got.Description, tt.wantIn)
			}
		})
	}
}

func TestMapJobPost_CompensationFallback(t *testing.T) {
	tests := []struct {
		name   string
		detail rapi.DetailJob
		want   string
	}{
		{
			name:   "compensation first",
			detail: rapi.DetailJob{ID: "x", Compensation: "comp", CompensationRng: "rng", Salary: "sal"},
			want:   "comp",
		},
		{
			name:   "compensationRange fallback",
			detail: rapi.DetailJob{ID: "x", CompensationRng: "rng", Salary: "sal"},
			want:   "rng",
		},
		{
			name:   "salary last resort",
			detail: rapi.DetailJob{ID: "x", Salary: "sal"},
			want:   "sal",
		},
		{
			name:   "empty when all empty",
			detail: rapi.DetailJob{ID: "x"},
			want:   "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapJobPost(tt.detail, "url")
			if got.Compensation != tt.want {
				t.Errorf("Compensation = %q, want %q", got.Compensation, tt.want)
			}
		})
	}
}

func TestMapJobPost_WorkplaceTypeFallback(t *testing.T) {
	tests := []struct {
		name   string
		detail rapi.DetailJob
		want   string
	}{
		{
			name:   "workplaceType first",
			detail: rapi.DetailJob{ID: "x", WorkplaceType: "REMOTE", WorkplaceTypAlt: "alt"},
			want:   "REMOTE",
		},
		{
			name:   "workplace_type fallback",
			detail: rapi.DetailJob{ID: "x", WorkplaceTypAlt: "HYBRID"},
			want:   "HYBRID",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapJobPost(tt.detail, "url")
			if got.WorkplaceType != tt.want {
				t.Errorf("WorkplaceType = %q, want %q", got.WorkplaceType, tt.want)
			}
		})
	}
}

func TestMapJobPost_DepartmentFallback(t *testing.T) {
	tests := []struct {
		name   string
		detail rapi.DetailJob
		want   string
	}{
		{
			name:   "department struct first",
			detail: rapi.DetailJob{ID: "x", Department: rapi.Department{Name: "Eng"}, DepartmentName: "Other"},
			want:   "Eng",
		},
		{
			name:   "departmentName fallback",
			detail: rapi.DetailJob{ID: "x", DepartmentName: "HR"},
			want:   "HR",
		},
		{
			name:   "empty when both empty",
			detail: rapi.DetailJob{ID: "x"},
			want:   "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapJobPost(tt.detail, "url")
			if got.Department != tt.want {
				t.Errorf("Department = %q, want %q", got.Department, tt.want)
			}
		})
	}
}

// ── hasUSLocation ─────────────────────────────────────────────────────────────

func TestHasUSLocation(t *testing.T) {
	tests := []struct {
		name      string
		locations []rapi.Location
		want      bool
	}{
		{
			name:      "US only",
			locations: []rapi.Location{{CountryCode: "US"}},
			want:      true,
		},
		{
			name:      "mixed US and non-US",
			locations: []rapi.Location{{CountryCode: "GB"}, {CountryCode: "US"}},
			want:      true,
		},
		{
			name:      "non-US only",
			locations: []rapi.Location{{CountryCode: "GB"}, {CountryCode: "DE"}},
			want:      false,
		},
		{
			name:      "empty",
			locations: nil,
			want:      false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasUSLocation(tt.locations); got != tt.want {
				t.Errorf("hasUSLocation = %v, want %v", got, tt.want)
			}
		})
	}
}

// ── locationDisplayName ───────────────────────────────────────────────────────

func TestLocationDisplayName(t *testing.T) {
	tests := []struct {
		name string
		loc  rapi.Location
		want string
	}{
		{
			name: "city and stateCode",
			loc:  rapi.Location{City: "San Francisco", StateCode: "CA"},
			want: "San Francisco, CA",
		},
		{
			name: "city only",
			loc:  rapi.Location{City: "London"},
			want: "London",
		},
		{
			name: "name fallback",
			loc:  rapi.Location{Name: "Remote"},
			want: "Remote",
		},
		{
			name: "country fallback",
			loc:  rapi.Location{Country: "Germany"},
			want: "Germany",
		},
		{
			name: "prefers city+state over name",
			loc:  rapi.Location{City: "NYC", StateCode: "NY", Name: "New York Metro"},
			want: "NYC, NY",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := locationDisplayName(tt.loc); got != tt.want {
				t.Errorf("locationDisplayName = %q, want %q", got, tt.want)
			}
		})
	}
}

// ── usLocationDisplayNames ────────────────────────────────────────────────────

func TestUSLocationDisplayNames_NoUSLocations(t *testing.T) {
	locs := []rapi.Location{
		{CountryCode: "GB", City: "London", StateCode: "ENG"},
		{CountryCode: "DE", City: "Berlin"},
	}
	got := usLocationDisplayNames(locs)
	if len(got) != 0 {
		t.Errorf("expected empty slice for non-US locations, got %v", got)
	}
}

func TestUSLocationDisplayNames_Mixed(t *testing.T) {
	locs := []rapi.Location{
		{CountryCode: "US", City: "SF", StateCode: "CA"},
		{CountryCode: "GB", City: "London"},
		{CountryCode: "US", City: "NY", StateCode: "NY"},
	}
	got := usLocationDisplayNames(locs)
	if len(got) != 2 {
		t.Fatalf("expected 2 US locations, got %d: %v", len(got), got)
	}
	if got[0] != "SF, CA" {
		t.Errorf("got[0] = %q, want SF, CA", got[0])
	}
	if got[1] != "NY, NY" {
		t.Errorf("got[1] = %q, want NY, NY", got[1])
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func containsStr(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && func() bool {
		for i := 0; i <= len(s)-len(sub); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	}())
}
