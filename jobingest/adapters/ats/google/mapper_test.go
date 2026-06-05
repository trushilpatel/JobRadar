package google

import (
	"strings"
	"testing"

	googleapi "github.com/trushilpatel/jobradar/jobingest/adapters/ats/google/api"
	"github.com/trushilpatel/jobradar/jobingest/core"
)

// ── mapParsedJobToSummary ─────────────────────────────────────────────────────

func TestMapJobSummary_AllFields(t *testing.T) {
	input := googleapi.ParsedJob{
		ID:       "12345",
		Slug:     "software-engineer-search",
		Title:    "Software Engineer, Search",
		Location: "Mountain View, CA, USA",
		Team:     "Google",
		URL:      "https://www.google.com/about/careers/applications/jobs/results/12345-software-engineer-search",
	}
	got := mapParsedJobToSummary(input)

	if got.ID != "google-12345" {
		t.Errorf("ID = %q, want google-12345", got.ID)
	}
	if got.Title != "Software Engineer, Search" {
		t.Errorf("Title = %q", got.Title)
	}
	if got.Location != "Mountain View, CA, USA" {
		t.Errorf("Location = %q", got.Location)
	}
	if got.URL != "https://www.google.com/about/careers/applications/jobs/results/12345-software-engineer-search" {
		t.Errorf("URL = %q", got.URL)
	}
	if got.EmployType != core.EmployFullTime {
		t.Errorf("EmployType = %q, want %q", got.EmployType, core.EmployFullTime)
	}
	if !got.LocationKnown {
		t.Error("LocationKnown should be true")
	}
}

func TestMapJobSummary_IDPrefixed(t *testing.T) {
	input := googleapi.ParsedJob{ID: "99999"}
	got := mapParsedJobToSummary(input)
	if got.ID != "google-99999" {
		t.Errorf("ID = %q, want google-99999", got.ID)
	}
}

func TestMapJobSummary_AlwaysFullTime(t *testing.T) {
	input := googleapi.ParsedJob{ID: "1"}
	got := mapParsedJobToSummary(input)
	if got.EmployType != core.EmployFullTime {
		t.Errorf("EmployType = %q, want %q", got.EmployType, core.EmployFullTime)
	}
}

func TestMapJobSummary_AlwaysLocationKnown(t *testing.T) {
	input := googleapi.ParsedJob{ID: "1"}
	got := mapParsedJobToSummary(input)
	if !got.LocationKnown {
		t.Error("LocationKnown should be true even when Location is empty")
	}
}

// ── mapDetailToJobPost ────────────────────────────────────────────────────────

func TestMapJobPost_AllFields(t *testing.T) {
	summary := core.JobSummary{
		ID:       "google-12345",
		Title:    "Software Engineer, Search",
		Location: "Mountain View, CA, USA",
		URL:      "https://www.google.com/about/careers/applications/jobs/results/12345-software-engineer-search",
	}
	detail := googleapi.JobDetail{
		Title:       "Software Engineer, Search",
		Description: "<p>Build cool systems.</p>",
		Location:    "Mountain View, CA, USA",
		Department:  "Core Infrastructure",
		PostedDate:  "2024-03-01",
	}
	got := mapDetailToJobPost(summary, detail)

	if got.ID != "google-12345" {
		t.Errorf("ID = %q, want google-12345", got.ID)
	}
	if got.Title != "Software Engineer, Search" {
		t.Errorf("Title = %q", got.Title)
	}
	if got.Location != "Mountain View, CA, USA" {
		t.Errorf("Location = %q", got.Location)
	}
	if got.URL != summary.URL {
		t.Errorf("URL = %q, want %q", got.URL, summary.URL)
	}
	if got.Department != "Core Infrastructure" {
		t.Errorf("Department = %q, want Core Infrastructure", got.Department)
	}
	if got.PublishedDate != "2024-03-01" {
		t.Errorf("PublishedDate = %q, want 2024-03-01", got.PublishedDate)
	}
	// Description should have HTML stripped
	if strings.Contains(got.Description, "<p>") {
		t.Errorf("Description should have HTML stripped; got: %q", got.Description)
	}
	if got.Description == "" {
		t.Error("Description should not be empty after stripping")
	}
}

func TestMapJobPost_DescriptionStripped(t *testing.T) {
	summary := core.JobSummary{ID: "google-1"}
	detail := googleapi.JobDetail{
		Description: "<h3>Minimum qualifications:</h3><ul><li>Bachelor's degree.</li><li>5 years experience.</li></ul>",
	}
	got := mapDetailToJobPost(summary, detail)

	if strings.Contains(got.Description, "<h3>") || strings.Contains(got.Description, "<li>") {
		t.Errorf("Description should have HTML stripped; got: %q", got.Description)
	}
	if !strings.Contains(got.Description, "Minimum qualifications") {
		t.Errorf("Description should retain text content; got: %q", got.Description)
	}
	if !strings.Contains(got.Description, "Bachelor's degree") {
		t.Errorf("Description should retain list items; got: %q", got.Description)
	}
}

func TestMapJobPost_FallsBackToSummaryTitle(t *testing.T) {
	summary := core.JobSummary{ID: "google-1", Title: "Summary Title", Location: "Seattle"}
	detail := googleapi.JobDetail{} // empty — no detail page data
	got := mapDetailToJobPost(summary, detail)

	if got.Title != "Summary Title" {
		t.Errorf("Title = %q, want Summary Title (fallback)", got.Title)
	}
	if got.Location != "Seattle" {
		t.Errorf("Location = %q, want Seattle (fallback)", got.Location)
	}
}

func TestMapJobPost_FallsBackToSummaryLocation(t *testing.T) {
	summary := core.JobSummary{ID: "google-1", Title: "Engineer", Location: "Austin, TX"}
	detail := googleapi.JobDetail{Title: "Engineer"} // no location in detail
	got := mapDetailToJobPost(summary, detail)

	if got.Location != "Austin, TX" {
		t.Errorf("Location = %q, want Austin, TX (fallback)", got.Location)
	}
}

func TestMapJobPost_DetailTitleTakesPrecedence(t *testing.T) {
	summary := core.JobSummary{ID: "google-1", Title: "Summary Title"}
	detail := googleapi.JobDetail{Title: "Detail Title"}
	got := mapDetailToJobPost(summary, detail)

	if got.Title != "Detail Title" {
		t.Errorf("Title = %q, want Detail Title", got.Title)
	}
}

func TestMapJobPost_DetailLocationTakesPrecedence(t *testing.T) {
	summary := core.JobSummary{ID: "google-1", Location: "Summary Location"}
	detail := googleapi.JobDetail{Location: "Detail Location"}
	got := mapDetailToJobPost(summary, detail)

	if got.Location != "Detail Location" {
		t.Errorf("Location = %q, want Detail Location", got.Location)
	}
}

func TestMapJobPost_IDPassedThrough(t *testing.T) {
	summary := core.JobSummary{ID: "google-99999"}
	got := mapDetailToJobPost(summary, googleapi.JobDetail{})
	if got.ID != "google-99999" {
		t.Errorf("ID = %q, want google-99999", got.ID)
	}
}
