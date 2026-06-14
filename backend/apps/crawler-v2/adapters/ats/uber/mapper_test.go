package uber

import (
	"reflect"
	"strings"
	"testing"

	uberapi "github.com/trushilpatel/jobradar/apps/crawler-v2/adapters/ats/uber/api"
	"github.com/trushilpatel/jobradar/common/ats/core"
)

const testBaseURL = "https://www.uber.com"

// ── mapJobSummary ─────────────────────────────────────────────────────────────

func TestMapJobSummary_AllFields(t *testing.T) {
	job := uberapi.UberJob{
		ID:          157927,
		Title:       "Software Engineer II",
		Description: "**About the Role**\n\nBuild scalable systems.",
		Department:  "Engineering",
		TimeType:    "Full-Time",
		Location: uberapi.UberLocation{
			Country:     "CAN",
			CountryName: "Canada",
			City:        "Toronto",
			Region:      "Ontario",
		},
		AllLocations: []uberapi.UberLocation{
			{Country: "CAN", CountryName: "Canada", City: "Toronto", Region: "Ontario"},
		},
		CreationDate: "2026-03-10T00:00:00.000Z",
	}

	got := mapJobSummary(job, testBaseURL)

	if got.ID != "uber-157927" {
		t.Errorf("ID = %q, want uber-157927", got.ID)
	}
	if got.Title != "Software Engineer II" {
		t.Errorf("Title = %q", got.Title)
	}
	if got.URL != "https://www.uber.com/careers/list/157927" {
		t.Errorf("URL = %q", got.URL)
	}
	if got.EmployType != core.EmployFullTime {
		t.Errorf("EmployType = %q, want %q", got.EmployType, core.EmployFullTime)
	}
	if !got.LocationKnown {
		t.Error("LocationKnown should be true")
	}
	if got.JobContent == nil {
		t.Fatal("JobContent is nil")
	}
	if got.JobContent.Description != job.Description {
		t.Errorf("JobContent.Description mismatch")
	}
	if got.JobContent.Department != "Engineering" {
		t.Errorf("JobContent.Department = %q", got.JobContent.Department)
	}
	if got.JobContent.PublishedDate != "2026-03-10T00:00:00.000Z" {
		t.Errorf("JobContent.PublishedDate = %q", got.JobContent.PublishedDate)
	}
}

func TestMapJobSummary_LocationFormatting(t *testing.T) {
	tests := []struct {
		name         string
		job          uberapi.UberJob
		wantLocation string
		wantAllLocs  []string
	}{
		{
			name: "city+region+country",
			job: uberapi.UberJob{
				ID: 1,
				Location: uberapi.UberLocation{
					City: "Toronto", Region: "Ontario", CountryName: "Canada", Country: "CAN",
				},
				AllLocations: []uberapi.UberLocation{
					{City: "Toronto", Region: "Ontario", CountryName: "Canada", Country: "CAN"},
				},
			},
			wantLocation: "Toronto, Ontario, Canada",
			wantAllLocs:  []string{"Toronto, Ontario, Canada"},
		},
		{
			name: "country-only fallback (no city)",
			job: uberapi.UberJob{
				ID: 2,
				Location: uberapi.UberLocation{
					CountryName: "United States", Country: "USA",
				},
				AllLocations: []uberapi.UberLocation{
					{CountryName: "United States", Country: "USA"},
				},
			},
			wantLocation: "United States",
			wantAllLocs:  []string{"United States"},
		},
		{
			name: "ISO alpha-3 fallback (no city, no countryName)",
			job: uberapi.UberJob{
				ID:       3,
				Location: uberapi.UberLocation{Country: "GBR"},
				AllLocations: []uberapi.UberLocation{
					{Country: "GBR"},
				},
			},
			wantLocation: "GBR",
			wantAllLocs:  []string{"GBR"},
		},
		{
			name: "multi-location joins with semicolon",
			job: uberapi.UberJob{
				ID: 4,
				Location: uberapi.UberLocation{
					City: "San Francisco", Region: "California", CountryName: "United States",
				},
				AllLocations: []uberapi.UberLocation{
					{City: "San Francisco", Region: "California", CountryName: "United States"},
					{City: "New York", Region: "New York", CountryName: "United States"},
				},
			},
			wantLocation: "San Francisco, California, United States; New York, New York, United States",
			wantAllLocs: []string{
				"San Francisco, California, United States",
				"New York, New York, United States",
			},
		},
		{
			name: "no allLocations falls back to primary location",
			job: uberapi.UberJob{
				ID: 5,
				Location: uberapi.UberLocation{
					City: "Seattle", Region: "Washington", CountryName: "United States",
				},
			},
			wantLocation: "Seattle, Washington, United States",
			wantAllLocs:  []string{"Seattle, Washington, United States"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapJobSummary(tt.job, testBaseURL)
			if got.Location != tt.wantLocation {
				t.Errorf("Location = %q, want %q", got.Location, tt.wantLocation)
			}
			if !reflect.DeepEqual(got.AllLocations, tt.wantAllLocs) {
				t.Errorf("AllLocations = %v, want %v", got.AllLocations, tt.wantAllLocs)
			}
		})
	}
}

func TestMapJobSummary_JobContentSet(t *testing.T) {
	// Fat adapter — JobContent must never be nil.
	job := uberapi.UberJob{
		ID:           42,
		Description:  "Some description",
		Department:   "Product",
		CreationDate: "2026-01-01T00:00:00.000Z",
	}
	got := mapJobSummary(job, testBaseURL)
	if got.JobContent == nil {
		t.Fatal("JobContent is nil — fat adapter must always set it")
	}
	if got.JobContent.Description != "Some description" {
		t.Errorf("JobContent.Description = %q", got.JobContent.Description)
	}
	if got.JobContent.Department != "Product" {
		t.Errorf("JobContent.Department = %q", got.JobContent.Department)
	}
	if got.JobContent.PublishedDate != "2026-01-01T00:00:00.000Z" {
		t.Errorf("JobContent.PublishedDate = %q", got.JobContent.PublishedDate)
	}
}

// ── normalizeUberTimeType ─────────────────────────────────────────────────────

func TestNormalizeUberTimeType(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		// Standard values
		{"Full-Time", core.EmployFullTime},
		{"Part-Time", core.EmployPartTime},
		{"Contract", core.EmployContract},
		{"Internship", core.EmployInternship},
		// Case-insensitive
		{"full-time", core.EmployFullTime},
		{"FULL-TIME", core.EmployFullTime},
		{"PART-TIME", core.EmployPartTime},
		{"contract", core.EmployContract},
		{"internship", core.EmployInternship},
		// With surrounding whitespace
		{"  Full-Time  ", core.EmployFullTime},
		// Unknown → default to full_time
		{"", core.EmployFullTime},
		{"Unknown", core.EmployFullTime},
		{"Temporary", core.EmployFullTime},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeUberTimeType(tt.input)
			if got != tt.want {
				t.Errorf("normalizeUberTimeType(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ── formatSingleLocation ──────────────────────────────────────────────────────

func TestFormatSingleLocation(t *testing.T) {
	tests := []struct {
		name string
		loc  uberapi.UberLocation
		want string
	}{
		{
			name: "city region countryName",
			loc:  uberapi.UberLocation{City: "Toronto", Region: "Ontario", CountryName: "Canada"},
			want: "Toronto, Ontario, Canada",
		},
		{
			name: "city no region",
			loc:  uberapi.UberLocation{City: "London", CountryName: "United Kingdom"},
			want: "London, United Kingdom",
		},
		{
			name: "city no countryName falls back to country code",
			loc:  uberapi.UberLocation{City: "Mumbai", Country: "IND"},
			want: "Mumbai, IND",
		},
		{
			name: "no city, has countryName",
			loc:  uberapi.UberLocation{CountryName: "Australia", Country: "AUS"},
			want: "Australia",
		},
		{
			name: "no city, no countryName, has country code",
			loc:  uberapi.UberLocation{Country: "SGP"},
			want: "SGP",
		},
		{
			name: "empty location",
			loc:  uberapi.UberLocation{},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatSingleLocation(tt.loc)
			if got != tt.want {
				t.Errorf("formatSingleLocation = %q, want %q", got, tt.want)
			}
		})
	}
}

// ── buildPrimaryLocation ──────────────────────────────────────────────────────

func TestBuildPrimaryLocation_MultipleLocations(t *testing.T) {
	job := uberapi.UberJob{
		ID: 1,
		Location: uberapi.UberLocation{
			City: "San Francisco", CountryName: "United States",
		},
		AllLocations: []uberapi.UberLocation{
			{City: "San Francisco", CountryName: "United States"},
			{City: "New York", CountryName: "United States"},
		},
	}
	got := buildPrimaryLocation(job)
	if !strings.Contains(got, ";") {
		t.Errorf("multi-location string %q should contain ';'", got)
	}
}

func TestBuildPrimaryLocation_SingleAllLocation(t *testing.T) {
	job := uberapi.UberJob{
		ID:       1,
		Location: uberapi.UberLocation{City: "Toronto", CountryName: "Canada"},
		AllLocations: []uberapi.UberLocation{
			{City: "Toronto", Region: "Ontario", CountryName: "Canada"},
		},
	}
	got := buildPrimaryLocation(job)
	if got != "Toronto, Ontario, Canada" {
		t.Errorf("got %q, want Toronto, Ontario, Canada", got)
	}
}

func TestBuildPrimaryLocation_FallbackToPrimaryLocation(t *testing.T) {
	job := uberapi.UberJob{
		ID:           1,
		Location:     uberapi.UberLocation{City: "Seattle", CountryName: "United States"},
		AllLocations: nil,
	}
	got := buildPrimaryLocation(job)
	if got != "Seattle, United States" {
		t.Errorf("got %q, want Seattle, United States", got)
	}
}
