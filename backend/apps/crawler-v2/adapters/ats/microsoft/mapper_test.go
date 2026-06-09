package microsoft

import (
	"testing"

	msapi "github.com/trushilpatel/jobradar/apps/crawler-v2/adapters/ats/microsoft/api"
	"github.com/trushilpatel/jobradar/apps/crawler-v2/core"
)

// ── stripReqID ────────────────────────────────────────────────────────────────

func TestStripReqID(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Software Engineer - 17248K", "Software Engineer"},
		{"Principal Engineer - MS-12345", "Principal Engineer"},
		{"Senior Program Manager - 1234567890", "Senior Program Manager"},
		{"Principal Engineer", "Principal Engineer"},
		{"Product Manager", "Product Manager"},
		{"Engineer - ABC", "Engineer - ABC"},                   // no digit → no strip
		{"Engineer - 1234", "Engineer"},                        // digits only → strip
		{"  Software Engineer - 1234K  ", "Software Engineer"}, // trim whitespace
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := stripReqID(tt.input)
			if got != tt.want {
				t.Errorf("stripReqID(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ── extractPositionID ─────────────────────────────────────────────────────────

func TestExtractPositionID(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want int64
	}{
		{
			name: "valid full URL",
			url:  "https://apply.careers.microsoft.com/careers/job/1812345678",
			want: 1812345678,
		},
		{
			name: "trailing slash",
			url:  "https://apply.careers.microsoft.com/careers/job/1812345678/",
			want: 1812345678,
		},
		{
			name: "short path",
			url:  "https://apply.careers.microsoft.com/careers/job/999",
			want: 999,
		},
		{
			name: "non-numeric last segment",
			url:  "https://apply.careers.microsoft.com/careers/job/senior-engineer",
			want: 0,
		},
		{
			name: "empty URL",
			url:  "",
			want: 0,
		},
		{
			name: "no path segments",
			url:  "https://example.com",
			want: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractPositionID(tt.url)
			if got != tt.want {
				t.Errorf("extractPositionID(%q) = %d, want %d", tt.url, got, tt.want)
			}
		})
	}
}

// ── mapEmploymentType ─────────────────────────────────────────────────────────

func TestMapEmploymentType(t *testing.T) {
	tests := []struct {
		input []string
		want  string
	}{
		{[]string{"Full-Time"}, core.EmployFullTime},
		{[]string{"Part-Time"}, core.EmployPartTime},
		{[]string{"Contract"}, core.EmployContract},
		{[]string{"Intern"}, core.EmployInternship},
		{[]string{"Internship"}, core.EmployInternship},
		{[]string{"Unknown"}, core.EmployFullTime},
		{[]string{}, core.EmployFullTime},
		{nil, core.EmployFullTime},
		{[]string{"  Full-Time  "}, core.EmployFullTime},          // trim whitespace
		{[]string{"Part-Time", "Full-Time"}, core.EmployPartTime}, // first wins
	}
	for _, tt := range tests {
		got := mapEmploymentType(tt.input)
		if got != tt.want {
			t.Errorf("mapEmploymentType(%v) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// ── mapJobSummary ─────────────────────────────────────────────────────────────

func TestMapJobSummary_AllFields(t *testing.T) {
	input := msapi.Position{
		ID:           1812345678,
		DisplayJobID: "1812345678",
		Name:         "Senior Software Engineer - 1812345678",
		Locations:    []string{"Redmond, Washington, United States", "Remote"},
		PostedTs:     1712678400,
		Department:   "Engineering",
	}

	got := mapJobSummary(input)

	if got.ID != "microsoft-1812345678" {
		t.Errorf("ID = %q, want microsoft-1812345678", got.ID)
	}
	if got.Title != "Senior Software Engineer" {
		t.Errorf("Title = %q, want Senior Software Engineer", got.Title)
	}
	if got.Location != "Redmond, Washington, United States; Remote" {
		t.Errorf("Location = %q, want joined locations", got.Location)
	}
	if got.URL != "https://apply.careers.microsoft.com/careers/job/1812345678" {
		t.Errorf("URL = %q, want internal URL with int64 ID", got.URL)
	}
	if got.EmployType != core.EmployFullTime {
		t.Errorf("EmployType = %q, want full_time", got.EmployType)
	}
	if !got.LocationKnown {
		t.Error("LocationKnown should be true")
	}
	if len(got.AllLocations) != 2 {
		t.Errorf("AllLocations len = %d, want 2", len(got.AllLocations))
	}
}

func TestMapJobSummary_NoLocations(t *testing.T) {
	input := msapi.Position{
		ID:           1,
		DisplayJobID: "1",
		Name:         "Engineer",
		Locations:    nil,
	}
	got := mapJobSummary(input)
	if got.Location != "" {
		t.Errorf("Location = %q, want empty string", got.Location)
	}
	if len(got.AllLocations) != 0 {
		t.Errorf("AllLocations should be empty, got %v", got.AllLocations)
	}
}

// ── mapJobPost ────────────────────────────────────────────────────────────────

func TestMapJobPost_AllFields(t *testing.T) {
	input := msapi.PositionDetail{
		ID:                         1812345678,
		DisplayJobID:               "1812345678",
		Name:                       "Senior Software Engineer - 1812345678",
		Location:                   "Redmond, Washington, United States",
		Locations:                  []string{"Redmond, Washington, United States"},
		JobDescription:             "<p>Join our <b>team</b>.</p>",
		Department:                 "Engineering",
		WorkLocationOption:         "Up to 50% work from home",
		EfcustomTextEmploymentType: []string{"Full-Time"},
		PostedTs:                   1712678400,
		PositionURL:                "https://careers.microsoft.com/v2/global/en/job/1812345678/senior-software-engineer",
	}

	got := mapJobPost(input)

	if got.ID != "microsoft-1812345678" {
		t.Errorf("ID = %q, want microsoft-1812345678", got.ID)
	}
	if got.Title != "Senior Software Engineer" {
		t.Errorf("Title = %q, want Senior Software Engineer", got.Title)
	}
	if got.Location != "Redmond, Washington, United States" {
		t.Errorf("Location = %q, want primary location", got.Location)
	}
	if got.URL != "https://careers.microsoft.com/v2/global/en/job/1812345678/senior-software-engineer" {
		t.Errorf("URL = %q, want positionUrl", got.URL)
	}
	if got.Description == "" {
		t.Error("Description should not be empty")
	}
	if got.Department != "Engineering" {
		t.Errorf("Department = %q, want Engineering", got.Department)
	}
	if got.WorkplaceType != "Up to 50% work from home" {
		t.Errorf("WorkplaceType = %q, want 'Up to 50%% work from home'", got.WorkplaceType)
	}
	if got.EmploymentType != core.EmployFullTime {
		t.Errorf("EmploymentType = %q, want full_time", got.EmploymentType)
	}
	if got.PublishedDate == "" {
		t.Error("PublishedDate should not be empty")
	}
}

func TestMapJobPost_LocationFallbackToLocations(t *testing.T) {
	input := msapi.PositionDetail{
		DisplayJobID: "123",
		Name:         "Engineer",
		Location:     "",
		Locations:    []string{"Seattle, WA", "Remote"},
		PositionURL:  "https://careers.microsoft.com/v2/global/en/job/123/engineer",
	}
	got := mapJobPost(input)
	if got.Location != "Seattle, WA; Remote" {
		t.Errorf("Location = %q, want joined fallback locations", got.Location)
	}
}

func TestMapJobPost_URLAbsoluteConversion(t *testing.T) {
	input := msapi.PositionDetail{
		DisplayJobID: "1812345678",
		Name:         "Engineer",
		PositionURL:  "/careers/job/1812345678",
	}
	got := mapJobPost(input)
	want := "https://apply.careers.microsoft.com/careers/job/1812345678"
	if got.URL != want {
		t.Errorf("URL = %q, want %q (relative path made absolute)", got.URL, want)
	}
}

func TestMapJobPost_URLFallbackWhenEmpty(t *testing.T) {
	input := msapi.PositionDetail{
		DisplayJobID: "1812345678",
		Name:         "Engineer",
		PositionURL:  "",
	}
	got := mapJobPost(input)
	want := "https://careers.microsoft.com/v2/global/en/job/1812345678"
	if got.URL != want {
		t.Errorf("URL = %q, want %q (fallback URL)", got.URL, want)
	}
}

func TestMapJobPost_HTMLStripped(t *testing.T) {
	input := msapi.PositionDetail{
		DisplayJobID:   "1",
		Name:           "Engineer",
		JobDescription: "<p>Build <b>great</b> things &amp; beyond.</p>",
		PositionURL:    "https://careers.microsoft.com/v2/global/en/job/1/engineer",
	}
	got := mapJobPost(input)
	want := "Build great things & beyond."
	if got.Description != want {
		t.Errorf("Description = %q, want %q", got.Description, want)
	}
}

func TestMapJobPost_ZeroTimestamp(t *testing.T) {
	input := msapi.PositionDetail{
		DisplayJobID: "1",
		Name:         "Engineer",
		PostedTs:     0,
		PositionURL:  "https://careers.microsoft.com/v2/global/en/job/1/engineer",
	}
	got := mapJobPost(input)
	if got.PublishedDate != "" {
		t.Errorf("PublishedDate = %q, want empty for zero timestamp", got.PublishedDate)
	}
}
