package lever

import (
	"strings"
	"testing"

	leverapi "github.com/trushilpatel/jobradar/apps/crawler-v2/adapters/ats/lever/api"
	"github.com/trushilpatel/jobradar/apps/crawler-v2/core"
)

// ── mapJobSummary ─────────────────────────────────────────────────────────────

func TestMapJobSummary_AllFields(t *testing.T) {
	input := leverapi.JobSummary{
		ID:        "abc-111-def",
		Text:      "Backend Engineer",
		HostedURL: "https://jobs.lever.co/netlify/abc-111-def",
	}
	input.Categories.Location = "Remote"
	input.Categories.Commitment = "Full-time"
	input.Categories.AllLocations = []string{"Remote", "San Francisco, CA"}

	got := mapJobSummary(input)

	if got.ID != "lever-abc-111-def" {
		t.Errorf("ID = %q, want lever-abc-111-def", got.ID)
	}
	if got.Title != "Backend Engineer" {
		t.Errorf("Title = %q, want Backend Engineer", got.Title)
	}
	if got.Location != "Remote" {
		t.Errorf("Location = %q, want Remote", got.Location)
	}
	if got.URL != "https://jobs.lever.co/netlify/abc-111-def" {
		t.Errorf("URL = %q, want correct hosted URL", got.URL)
	}
	if got.EmployType != core.EmployFullTime {
		t.Errorf("EmployType = %q, want %q", got.EmployType, core.EmployFullTime)
	}
	if !got.LocationKnown {
		t.Error("LocationKnown should be true")
	}
	if len(got.AllLocations) != 2 {
		t.Errorf("AllLocations length = %d, want 2", len(got.AllLocations))
	}
	if got.JobContent != nil {
		t.Error("JobContent should be nil for thin adapter")
	}
}

func TestMapJobSummary_EmptyLocation(t *testing.T) {
	input := leverapi.JobSummary{
		ID:   "xyz-000",
		Text: "Engineer",
	}
	got := mapJobSummary(input)
	if got.Location != "" {
		t.Errorf("expected empty location, got %q", got.Location)
	}
	if !got.LocationKnown {
		t.Error("LocationKnown should always be true for Lever")
	}
}

// ── mapJobListing ─────────────────────────────────────────────────────────────

func TestMapJobListing_Multiple(t *testing.T) {
	j1 := leverapi.JobSummary{ID: "aaa-001", Text: "Engineer A"}
	j2 := leverapi.JobSummary{ID: "bbb-002", Text: "Engineer B"}
	input := []leverapi.JobSummary{j1, j2}

	got := mapJobListing(input)
	if len(got.Jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(got.Jobs))
	}
	if got.Jobs[0].ID != "lever-aaa-001" {
		t.Errorf("jobs[0].ID = %q, want lever-aaa-001", got.Jobs[0].ID)
	}
	if got.Jobs[1].ID != "lever-bbb-002" {
		t.Errorf("jobs[1].ID = %q, want lever-bbb-002", got.Jobs[1].ID)
	}
}

func TestMapJobListing_Empty(t *testing.T) {
	got := mapJobListing([]leverapi.JobSummary{})
	if len(got.Jobs) != 0 {
		t.Errorf("expected empty jobs, got %d", len(got.Jobs))
	}
}

// ── mapJobPost ────────────────────────────────────────────────────────────────

func TestMapJobPost_AllFields(t *testing.T) {
	input := leverapi.JobPost{
		ID:               "abc-111-def",
		Text:             "Backend Engineer",
		HostedURL:        "https://jobs.lever.co/netlify/abc-111-def",
		DescriptionPlain: "We are building the future.\nJoin our team.",
		AdditionalPlain:  "About Netlify: great company.",
		SalaryRange: &leverapi.SalaryRange{
			Currency: "USD",
			Interval: "per-year-salary",
			Min:      120000,
			Max:      160000,
		},
		SalaryDescriptionPlain: "$120K – $160K depending on experience",
		WorkplaceType:          "remote",
		CreatedAt:              1685000000000,
	}
	input.Categories.Location = "Remote"
	input.Categories.Commitment = "Full-time"
	input.Categories.Department = "Product & Engineering"
	input.Categories.Team = "Engineering"
	input.Lists = []struct {
		Text    string `json:"text"`
		Content string `json:"content"`
	}{
		{Text: "Requirements", Content: "<li>5+ years Go</li><li>Strong backend</li>"},
		{Text: "Benefits", Content: "<li>Health insurance</li>"},
	}

	got := mapJobPost(input)

	if got.ID != "lever-abc-111-def" {
		t.Errorf("ID = %q, want lever-abc-111-def", got.ID)
	}
	if got.Title != "Backend Engineer" {
		t.Errorf("Title = %q, want Backend Engineer", got.Title)
	}
	if got.Location != "Remote" {
		t.Errorf("Location = %q, want Remote", got.Location)
	}
	if got.URL != "https://jobs.lever.co/netlify/abc-111-def" {
		t.Errorf("URL = %q", got.URL)
	}
	if !strings.Contains(got.Description, "We are building the future") {
		t.Errorf("Description should contain main body, got: %q", got.Description)
	}
	if !strings.Contains(got.Description, "About Netlify") {
		t.Errorf("Description should contain additionalPlain, got: %q", got.Description)
	}
	// salaryDescriptionPlain takes priority over salaryRange
	if got.Compensation != "$120K – $160K depending on experience" {
		t.Errorf("Compensation = %q, want salaryDescriptionPlain", got.Compensation)
	}
	if got.Department != "Product & Engineering" {
		t.Errorf("Department = %q, want Product & Engineering", got.Department)
	}
	if got.EmploymentType != core.EmployFullTime {
		t.Errorf("EmploymentType = %q, want %q", got.EmploymentType, core.EmployFullTime)
	}
	if got.WorkplaceType != "remote" {
		t.Errorf("WorkplaceType = %q, want remote", got.WorkplaceType)
	}
	if got.PublishedDate != "1685000000000" {
		t.Errorf("PublishedDate = %q, want 1685000000000", got.PublishedDate)
	}
	if got.Requirements == "" {
		t.Error("Requirements is empty")
	}
	if !strings.Contains(got.Requirements, "Requirements:") {
		t.Errorf("Requirements should contain 'Requirements:', got: %q", got.Requirements)
	}
}

func TestMapJobPost_NoSalaryRange(t *testing.T) {
	input := leverapi.JobPost{
		ID:   "abc-111",
		Text: "Engineer",
	}
	got := mapJobPost(input)
	if got.Compensation != "" {
		t.Errorf("expected empty Compensation, got %q", got.Compensation)
	}
}

func TestMapJobPost_SalaryRangeFallback(t *testing.T) {
	input := leverapi.JobPost{
		ID:   "abc-111",
		Text: "Engineer",
		SalaryRange: &leverapi.SalaryRange{
			Currency: "USD",
			Interval: "per-year-salary",
			Min:      100000,
			Max:      140000,
		},
		// SalaryDescriptionPlain is empty — should fall back to SalaryRange
	}
	got := mapJobPost(input)
	want := "100000 – 140000 USD / per-year-salary"
	if got.Compensation != want {
		t.Errorf("Compensation = %q, want %q", got.Compensation, want)
	}
}

func TestMapJobPost_DepartmentFallback(t *testing.T) {
	input := leverapi.JobPost{
		ID:   "abc-111",
		Text: "Engineer",
	}
	// Department is empty, Team is set — should fall back to team
	input.Categories.Department = ""
	input.Categories.Team = "Platform Engineering"

	got := mapJobPost(input)
	if got.Department != "Platform Engineering" {
		t.Errorf("Department = %q, want Platform Engineering (team fallback)", got.Department)
	}
}

func TestMapJobPost_RequirementsBuilding(t *testing.T) {
	input := leverapi.JobPost{
		ID:   "abc-111",
		Text: "Engineer",
	}
	input.Lists = []struct {
		Text    string `json:"text"`
		Content string `json:"content"`
	}{
		{Text: "Requirements", Content: "<li>Go experience</li><li>5+ years backend</li>"},
		{Text: "Nice to have", Content: "<li>Kubernetes</li>"},
		{Text: "Empty section", Content: "   "}, // should be omitted
	}

	got := mapJobPost(input)
	// Two non-empty sections → joined with "\n"
	if !strings.Contains(got.Requirements, "Requirements:") {
		t.Errorf("Requirements should contain 'Requirements:', got: %q", got.Requirements)
	}
	if !strings.Contains(got.Requirements, "Nice to have:") {
		t.Errorf("Requirements should contain 'Nice to have:', got: %q", got.Requirements)
	}
	if strings.Contains(got.Requirements, "Empty section") {
		t.Errorf("Requirements should not contain empty section, got: %q", got.Requirements)
	}
	// Should be joined by newline
	parts := strings.Split(got.Requirements, "\n")
	if len(parts) != 2 {
		t.Errorf("Requirements should have 2 sections joined by newline, got %d parts: %q", len(parts), got.Requirements)
	}
}

func TestMapJobPost_PublishedDateZero(t *testing.T) {
	input := leverapi.JobPost{ID: "abc-111", Text: "Engineer", CreatedAt: 0}
	got := mapJobPost(input)
	if got.PublishedDate != "" {
		t.Errorf("PublishedDate should be empty for zero CreatedAt, got %q", got.PublishedDate)
	}
}

func TestMapJobPost_DescriptionAdditionalEmpty(t *testing.T) {
	input := leverapi.JobPost{
		ID:               "abc-111",
		Text:             "Engineer",
		DescriptionPlain: "Main description here.",
		AdditionalPlain:  "", // empty — should not add trailing newline
	}
	got := mapJobPost(input)
	if got.Description != "Main description here." {
		t.Errorf("Description = %q, want 'Main description here.'", got.Description)
	}
}

// ── normalizeLeverCommitment ──────────────────────────────────────────────────

func TestNormalizeLeverCommitment(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Full-time", core.EmployFullTime},
		{"Fulltime", core.EmployFullTime},
		{"Full time", core.EmployFullTime},
		{"full-time", core.EmployFullTime},
		{"FULL-TIME", core.EmployFullTime},
		{"Part-time", core.EmployPartTime},
		{"Parttime", core.EmployPartTime},
		{"Part time", core.EmployPartTime},
		{"Contract", core.EmployContract},
		{"Contractor", core.EmployContract},
		{"contract", core.EmployContract},
		{"Internship", core.EmployInternship},
		{"Intern", core.EmployInternship},
		{"intern", core.EmployInternship},
		{"", ""},
		{"unknown", ""},
		{"Temporary", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeLeverCommitment(tt.input)
			if got != tt.want {
				t.Errorf("normalizeLeverCommitment(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
