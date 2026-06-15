package apple

import (
	"strings"
	"testing"

	appleapi "github.com/trushilpatel/jobradar/apps/crawler-v2/adapters/ats/apple/api"
	"github.com/trushilpatel/jobradar/common/ats/core"
)

// ── mapJobSummary ─────────────────────────────────────────────────────────────

func TestMapJobSummary_AllFields(t *testing.T) {
	baseURL := "https://jobs.apple.com"
	input := appleapi.Job{
		ID:                      "200647207",
		PositionID:              "P1234",
		PostingTitle:            "Software Engineer",
		TransformedPostingTitle: "software-engineer",
		WorkHours:               "FULL_TIME",
		Locations: []appleapi.Location{
			{
				City:          "Cupertino",
				StateProvince: "California",
				CountryID:     "iso-country-USA",
				CountryCode:   "US",
			},
		},
	}

	got := mapJobSummary(input, baseURL)

	if got.ID != "apple-200647207" {
		t.Errorf("ID = %q, want apple-200647207", got.ID)
	}
	if got.Title != "Software Engineer" {
		t.Errorf("Title = %q, want Software Engineer", got.Title)
	}
	if got.Location != "Cupertino, California" {
		t.Errorf("Location = %q, want Cupertino, California", got.Location)
	}
	wantURL := "https://jobs.apple.com/en-us/details/200647207/software-engineer"
	if got.URL != wantURL {
		t.Errorf("URL = %q, want %q", got.URL, wantURL)
	}
	if got.EmployType != core.EmployFullTime {
		t.Errorf("EmployType = %q, want %q", got.EmployType, core.EmployFullTime)
	}
	if !got.LocationKnown {
		t.Error("LocationKnown should be true for US location")
	}
	if len(got.AllLocations) == 0 {
		t.Error("AllLocations should not be empty")
	}
}

func TestMapJobSummary_PartTimeWorkHours(t *testing.T) {
	input := appleapi.Job{
		ID:                      "200647210",
		PostingTitle:            "Part-Time Specialist",
		TransformedPostingTitle: "part-time-specialist",
		WorkHours:               "PART_TIME",
	}
	got := mapJobSummary(input, "https://jobs.apple.com")
	if got.EmployType != core.EmployPartTime {
		t.Errorf("EmployType = %q, want %q", got.EmployType, core.EmployPartTime)
	}
}

func TestMapJobSummary_InternshipWorkHours(t *testing.T) {
	input := appleapi.Job{
		ID:                      "200647211",
		PostingTitle:            "Engineering Intern",
		TransformedPostingTitle: "engineering-intern",
		WorkHours:               "INTERNSHIP",
	}
	got := mapJobSummary(input, "https://jobs.apple.com")
	if got.EmployType != core.EmployInternship {
		t.Errorf("EmployType = %q, want %q", got.EmployType, core.EmployInternship)
	}
}

func TestMapJobSummary_NonUSLocation(t *testing.T) {
	input := appleapi.Job{
		ID:                      "200647212",
		PostingTitle:            "Engineer",
		TransformedPostingTitle: "engineer",
		Locations: []appleapi.Location{
			{
				City:          "London",
				StateProvince: "England",
				CountryID:     "iso-country-GBR",
				CountryCode:   "GBR",
			},
		},
	}
	got := mapJobSummary(input, "https://jobs.apple.com")
	if got.LocationKnown {
		t.Error("LocationKnown should be false for non-US location")
	}
}

// ── mapJobPost ────────────────────────────────────────────────────────────────

func TestMapJobPost_DescriptionSections(t *testing.T) {
	input := appleapi.JobDetail{
		ID:                      "200647207",
		PositionID:              "200647207",
		PostingTitle:            "Software Engineer",
		JobSummary:              "<p>Join the team.</p>",
		BoardDescription:        "<p>We create great products.</p>",
		Responsibilities:        "<ul><li>Build things</li></ul>",
		MinimumQualifications:   "<ul><li>5 years experience</li></ul>",
		PreferredQualifications: "<ul><li>Go expertise</li></ul>",
		Locations: []appleapi.Location{
			{City: "Cupertino", StateProvince: "California", CountryID: "iso-country-USA"},
		},
	}

	got := mapJobPost(input)

	if got.ID != "apple-200647207" {
		t.Errorf("ID = %q, want apple-200647207", got.ID)
	}
	if !strings.Contains(got.Description, "Summary:") {
		t.Errorf("Description missing Summary section: %q", got.Description)
	}
	if !strings.Contains(got.Description, "Join the team.") {
		t.Errorf("Description missing jobSummary content: %q", got.Description)
	}
	if !strings.Contains(got.Description, "Description:") {
		t.Errorf("Description missing Description section: %q", got.Description)
	}
	if !strings.Contains(got.Description, "We create great products.") {
		t.Errorf("Description missing boardDescription content: %q", got.Description)
	}
	if !strings.Contains(got.Description, "Responsibilities:") {
		t.Errorf("Description missing Responsibilities section: %q", got.Description)
	}
	if !strings.Contains(got.Description, "Build things") {
		t.Errorf("Description missing responsibilities content: %q", got.Description)
	}
	if !strings.Contains(got.Description, "Requirements:") {
		t.Errorf("Description missing Requirements section: %q", got.Description)
	}
	if !strings.Contains(got.Description, "5 years experience") {
		t.Errorf("Description missing minimumQualifications content: %q", got.Description)
	}
	if !strings.Contains(got.Description, "Preferred:") {
		t.Errorf("Description missing Preferred section: %q", got.Description)
	}
	if !strings.Contains(got.Description, "Go expertise") {
		t.Errorf("Description missing preferredQualifications content: %q", got.Description)
	}
}

func TestMapJobPost_SkipsEmptySections(t *testing.T) {
	input := appleapi.JobDetail{
		ID:           "200647207",
		PositionID:   "200647207",
		PostingTitle: "Software Engineer",
		JobSummary:   "<p>Summary text.</p>",
		// BoardDescription empty
		// Responsibilities empty
		// MinimumQualifications empty
		// PreferredQualifications empty
	}

	got := mapJobPost(input)

	if strings.Contains(got.Description, "Description:") {
		t.Error("Description should not contain empty Description section")
	}
	if strings.Contains(got.Description, "Responsibilities:") {
		t.Error("Description should not contain empty Responsibilities section")
	}
	if strings.Contains(got.Description, "Requirements:") {
		t.Error("Description should not contain empty Requirements section")
	}
	if strings.Contains(got.Description, "Preferred:") {
		t.Error("Description should not contain empty Preferred section")
	}
	if !strings.Contains(got.Description, "Summary text.") {
		t.Error("Description should contain summary text")
	}
}

func TestMapJobPost_PayRangeExtraction(t *testing.T) {
	input := appleapi.JobDetail{
		ID:           "200647207",
		PositionID:   "200647207",
		PostingTitle: "Software Engineer",
		PostingFooters: []appleapi.Footer{
			{
				Type:    "POSTING_FOOTER_RULES_PAY",
				Content: "<p>The base pay range for this role is between $150,000 and $200,000.</p>",
			},
		},
	}

	got := mapJobPost(input)

	if got.Compensation == "" {
		t.Fatal("Compensation is empty, expected pay range")
	}
	if !strings.Contains(got.Compensation, "$150,000") {
		t.Errorf("Compensation %q should contain $150,000", got.Compensation)
	}
	if !strings.Contains(got.Compensation, "$200,000") {
		t.Errorf("Compensation %q should contain $200,000", got.Compensation)
	}
}

func TestMapJobPost_NoPayRange(t *testing.T) {
	input := appleapi.JobDetail{
		ID:           "200647207",
		PositionID:   "200647207",
		PostingTitle: "Software Engineer",
		PostingFooters: []appleapi.Footer{
			{
				Type:    "POSTING_FOOTER_RULES_EOE",
				Content: "<p>Apple is an equal opportunity employer.</p>",
			},
		},
	}

	got := mapJobPost(input)

	if got.Compensation != "" {
		t.Errorf("Compensation should be empty when no pay range, got %q", got.Compensation)
	}
}

func TestMapJobPost_CountryCode(t *testing.T) {
	input := appleapi.JobDetail{
		ID:           "200647207",
		PositionID:   "200647207",
		PostingTitle: "Software Engineer",
		Locations: []appleapi.Location{
			{City: "Cupertino", StateProvince: "California", CountryID: "iso-country-USA"},
		},
	}
	got := mapJobPost(input)
	if got.CountryCode != "US" {
		t.Errorf("CountryCode = %q, want US", got.CountryCode)
	}
}

func TestMapJobPost_NoCountryCode(t *testing.T) {
	input := appleapi.JobDetail{
		ID:           "200647209",
		PositionID:   "200647209",
		PostingTitle: "Senior Engineer",
		Locations: []appleapi.Location{
			{City: "London", StateProvince: "England", CountryID: "iso-country-GBR"},
		},
	}
	got := mapJobPost(input)
	if got.CountryCode == "US" {
		t.Errorf("CountryCode should not be US for non-US location, got %q", got.CountryCode)
	}
}

// ── isUSALocation via countryName ─────────────────────────────────────────────

func TestMapJobSummary_USAByCountryName(t *testing.T) {
	input := appleapi.Job{
		ID:                      "200647220",
		PostingTitle:            "Engineer",
		TransformedPostingTitle: "engineer",
		Locations: []appleapi.Location{
			{
				City:        "Austin",
				CountryName: "United States",
				// No CountryID or CountryCode set
			},
		},
	}
	got := mapJobSummary(input, "https://jobs.apple.com")
	if !got.LocationKnown {
		t.Error("LocationKnown should be true when countryName is United States")
	}
}

func TestMapJobSummary_USAByCountryNameFull(t *testing.T) {
	input := appleapi.Job{
		ID:                      "200647221",
		PostingTitle:            "Engineer",
		TransformedPostingTitle: "engineer",
		Locations: []appleapi.Location{
			{
				City:        "Seattle",
				CountryName: "United States of America",
			},
		},
	}
	got := mapJobSummary(input, "https://jobs.apple.com")
	if !got.LocationKnown {
		t.Error("LocationKnown should be true when countryName is United States of America")
	}
}

// ── primaryCountryCode via iso-country prefix ─────────────────────────────────

func TestMapJobPost_PrimaryCountryCodeFromISOPrefix(t *testing.T) {
	input := appleapi.JobDetail{
		ID:           "200647230",
		PositionID:   "200647230",
		PostingTitle: "Engineer",
		Locations: []appleapi.Location{
			{
				City:      "Toronto",
				CountryID: "iso-country-CAN",
			},
		},
	}
	got := mapJobPost(input)
	if got.CountryCode != "CAN" {
		t.Errorf("CountryCode = %q, want CAN", got.CountryCode)
	}
}

func TestMapJobPost_PrimaryCountryCodeFromCountryCodeField(t *testing.T) {
	input := appleapi.JobDetail{
		ID:           "200647231",
		PositionID:   "200647231",
		PostingTitle: "Engineer",
		Locations: []appleapi.Location{
			{
				City:        "Tokyo",
				CountryCode: "JPN",
			},
		},
	}
	got := mapJobPost(input)
	if got.CountryCode != "JPN" {
		t.Errorf("CountryCode = %q, want JPN", got.CountryCode)
	}
}

func TestMapJobPost_FirstUSLocationFallbackToFirst(t *testing.T) {
	// No US location: falls back to first location
	input := appleapi.JobDetail{
		ID:           "200647232",
		PositionID:   "200647232",
		PostingTitle: "Engineer",
		Locations: []appleapi.Location{
			{Name: "London", CountryID: "iso-country-GBR"},
		},
	}
	got := mapJobPost(input)
	// Should fall back to first location
	if got.Location != "London" {
		t.Errorf("Location = %q, want London (fallback to first)", got.Location)
	}
}

func TestFormatSingleLocation_NameFallback(t *testing.T) {
	// When City is empty, Name is used
	loc := appleapi.Location{Name: "Cupertino Area", StateCode: "CA"}
	got := formatSingleLocation(loc)
	if got != "Cupertino Area, CA" {
		t.Errorf("formatSingleLocation = %q, want Cupertino Area, CA", got)
	}
}

func TestFormatSingleLocation_NoStateOrCity(t *testing.T) {
	// When both city and state are empty, result is empty
	loc := appleapi.Location{}
	got := formatSingleLocation(loc)
	if got != "" {
		t.Errorf("formatSingleLocation empty loc = %q, want empty", got)
	}
}

// ── normalizeAppleWorkHours ───────────────────────────────────────────────────

func TestNormalizeAppleWorkHours(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"FULL_TIME", core.EmployFullTime},
		{"full_time", core.EmployFullTime},
		{"PART_TIME", core.EmployPartTime},
		{"part_time", core.EmployPartTime},
		{"INTERNSHIP", core.EmployInternship},
		{"internship", core.EmployInternship},
		{"", core.EmployFullTime},
		{"UNKNOWN_VALUE", core.EmployFullTime},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeAppleWorkHours(tt.input)
			if got != tt.want {
				t.Errorf("normalizeAppleWorkHours(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
