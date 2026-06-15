package ashby

import (
	"reflect"
	"strings"
	"testing"

	ashbyapi "github.com/trushilpatel/jobradar/apps/crawler-v2/adapters/ats/ashby/api"
	"github.com/trushilpatel/jobradar/common/ats/core"
)

// ── mapJobSummary ─────────────────────────────────────────────────────────────

func TestMapJobSummary_AllFields(t *testing.T) {
	input := ashbyapi.JobPosting{
		ID:                      "abc-123",
		Title:                   "Backend Engineer",
		LocationName:            "San Francisco, CA",
		WorkplaceType:           "Hybrid",
		EmploymentType:          "FullTime",
		CompensationTierSummary: "$180K – $240K",
	}
	want := core.JobSummary{
		ID:            "ashby-abc-123",
		Title:         "Backend Engineer",
		Location:      "San Francisco, CA (Hybrid)",
		URL:           "https://jobs.ashbyhq.com/netlify/abc-123",
		EmployType:    core.EmployFullTime,
		LocationKnown: true,
		AllLocations:  []string{"San Francisco, CA (Hybrid)"},
	}
	got := mapJobSummary(input, "netlify")
	if !reflect.DeepEqual(got, want) {
		t.Errorf("mapJobSummary:\n got  %+v\n want %+v", got, want)
	}
}

func TestMapJobSummary_LocationWithWorkplaceType(t *testing.T) {
	input := ashbyapi.JobPosting{
		ID:            "id-1",
		LocationName:  "New York, NY",
		WorkplaceType: "Onsite",
	}
	got := mapJobSummary(input, "acme")
	want := "New York, NY (Onsite)"
	if got.Location != want {
		t.Errorf("Location = %q, want %q", got.Location, want)
	}
	if !reflect.DeepEqual(got.AllLocations, []string{want}) {
		t.Errorf("AllLocations = %v, want [%q]", got.AllLocations, want)
	}
}

func TestMapJobSummary_LocationWithoutWorkplaceType(t *testing.T) {
	input := ashbyapi.JobPosting{
		ID:           "id-2",
		LocationName: "Remote",
	}
	got := mapJobSummary(input, "acme")
	if got.Location != "Remote" {
		t.Errorf("Location = %q, want Remote", got.Location)
	}
	if !reflect.DeepEqual(got.AllLocations, []string{"Remote"}) {
		t.Errorf("AllLocations = %v, want [Remote]", got.AllLocations)
	}
}

func TestMapJobSummary_URLFormat(t *testing.T) {
	input := ashbyapi.JobPosting{ID: "xyz-789", Title: "Engineer"}
	got := mapJobSummary(input, "openai")
	want := "https://jobs.ashbyhq.com/openai/xyz-789"
	if got.URL != want {
		t.Errorf("URL = %q, want %q", got.URL, want)
	}
}

func TestMapJobSummary_LocationKnownAlwaysTrue(t *testing.T) {
	got := mapJobSummary(ashbyapi.JobPosting{ID: "1"}, "acme")
	if !got.LocationKnown {
		t.Error("LocationKnown should always be true")
	}
}

// ── mapJobPost ────────────────────────────────────────────────────────────────

func TestMapJobPost_AllFields(t *testing.T) {
	input := ashbyapi.JobPostingDetail{
		ID:                      "abc-123",
		Title:                   "Backend Engineer",
		LocationName:            "San Francisco, CA",
		WorkplaceType:           "Hybrid",
		EmploymentType:          "FullTime",
		DescriptionHtml:         "<p>Join our <strong>team</strong>.</p>",
		CompensationTierSummary: "$180K – $240K",
		PublishedDate:           "2024-01-15",
	}
	want := core.JobPost{
		ID:             "ashby-abc-123",
		Title:          "Backend Engineer",
		Location:       "San Francisco, CA (Hybrid)",
		Description:    "Join our team .",
		EmploymentType: core.EmployFullTime,
		Compensation:   "$180K – $240K",
		WorkplaceType:  "Hybrid",
		PublishedDate:  "2024-01-15",
	}
	got := mapJobPost(input)
	if got != want {
		t.Errorf("mapJobPost:\n got  %+v\n want %+v", got, want)
	}
}

func TestMapJobPost_EmptyDescription(t *testing.T) {
	input := ashbyapi.JobPostingDetail{ID: "x", DescriptionHtml: ""}
	got := mapJobPost(input)
	if got.Description != "" {
		t.Errorf("Description = %q, want empty string", got.Description)
	}
}

func TestMapJobPost_HTMLStripped(t *testing.T) {
	input := ashbyapi.JobPostingDetail{
		ID:              "x",
		DescriptionHtml: "<ul><li>Build systems</li><li>Ship code</li></ul>",
	}
	got := mapJobPost(input)
	if strings.Contains(got.Description, "<") || strings.Contains(got.Description, ">") {
		t.Errorf("Description still contains HTML tags: %q", got.Description)
	}
	if got.Description == "" {
		t.Error("Description should not be empty after stripping HTML")
	}
}

// ── normalizeAshbyEmploymentType ──────────────────────────────────────────────

func TestNormalizeAshbyEmploymentType(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"FullTime", core.EmployFullTime},
		{"PartTime", core.EmployPartTime},
		{"Contract", core.EmployContract},
		{"Contractor", core.EmployContract},
		{"Temporary", core.EmployContract},
		{"Intern", core.EmployInternship},
		{"Internship", core.EmployInternship},
		{"", ""},
		{"Unknown", ""},
		{"fulltime", ""}, // case-sensitive
		{"FULLTIME", ""}, // case-sensitive
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeAshbyEmploymentType(tt.input)
			if got != tt.want {
				t.Errorf("normalizeAshbyEmploymentType(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ── formatLocation ────────────────────────────────────────────────────────────

func TestFormatLocation_WithWorkplaceType(t *testing.T) {
	got := formatLocation("Seattle, WA", "Remote")
	want := "Seattle, WA (Remote)"
	if got != want {
		t.Errorf("formatLocation = %q, want %q", got, want)
	}
}

func TestFormatLocation_WithoutWorkplaceType(t *testing.T) {
	got := formatLocation("Seattle, WA", "")
	want := "Seattle, WA"
	if got != want {
		t.Errorf("formatLocation = %q, want %q", got, want)
	}
}

func TestFormatLocation_BothEmpty(t *testing.T) {
	got := formatLocation("", "")
	if got != "" {
		t.Errorf("formatLocation(\"\",\"\") = %q, want empty string", got)
	}
}
