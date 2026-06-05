package deel

import (
	"testing"

	deelapi "github.com/trushilpatel/jobradar/jobingest/adapters/ats/deel/api"
	"github.com/trushilpatel/jobradar/jobingest/core"
)

// ── mapJobSummary ─────────────────────────────────────────────────────────────

func TestMapJobSummary_AllFields(t *testing.T) {
	rec := deelapi.RawJobPosting{
		ID:    "abc-123",
		Title: "Backend Engineer",
		Job: deelapi.RawJob{
			JobEmploymentTypes: []deelapi.RawEmploymentType{{
				EmploymentType: deelapi.RawNamed{Name: "Full-time"},
			}},
			JobLocations: []deelapi.RawJobLocation{{
				Location: deelapi.RawNamed{Name: "Berlin"},
			}},
			JobDepartments: []deelapi.RawJobDepartment{{
				Department: deelapi.RawNamed{Name: "Engineering"},
			}},
			CurrentCompensation: &deelapi.RawCompensation{
				CurrencyIsoCode: "EUR",
				MinAmount:       80000,
				MaxAmount:       120000,
			},
		},
		JobPostingPubs: []deelapi.RawJobPostingPub{{
			CurrentState: deelapi.RawPubState{CreatedAt: "2025-03-01T10:00:00Z"},
		}},
	}
	content := &core.JobContent{
		Description:   "About the role.",
		Department:    "Engineering",
		PublishedDate: "2025-03-01T10:00:00Z",
	}

	got := mapJobSummary(rec, "klarna", content)

	if got.ID != "deel-abc-123" {
		t.Errorf("ID = %q, want deel-abc-123", got.ID)
	}
	if got.Title != "Backend Engineer" {
		t.Errorf("Title = %q, want Backend Engineer", got.Title)
	}
	if got.Location != "Berlin" {
		t.Errorf("Location = %q, want Berlin", got.Location)
	}
	if got.URL != "https://jobs.deel.com/klarna/job-details/abc-123" {
		t.Errorf("URL = %q", got.URL)
	}
	if got.EmployType != core.EmployFullTime {
		t.Errorf("EmployType = %q, want %q", got.EmployType, core.EmployFullTime)
	}
	if !got.LocationKnown {
		t.Error("LocationKnown should be true")
	}
	if got.JobContent == nil {
		t.Fatal("JobContent must be set")
	}
	if got.JobContent.Description != "About the role." {
		t.Errorf("Description = %q", got.JobContent.Description)
	}
}

func TestMapJobSummary_URLFallback(t *testing.T) {
	// When no explicit URL in the record, the adapter constructs one from slug+id.
	rec := deelapi.RawJobPosting{ID: "xyz-789", Title: "Designer"}
	got := mapJobSummary(rec, "dott", nil)
	want := "https://jobs.deel.com/dott/job-details/xyz-789"
	if got.URL != want {
		t.Errorf("URL = %q, want %q", got.URL, want)
	}
}

func TestMapJobSummary_MultipleLocationsJoined(t *testing.T) {
	rec := deelapi.RawJobPosting{
		ID:    "m1",
		Title: "SRE",
		Job: deelapi.RawJob{
			JobLocations: []deelapi.RawJobLocation{
				{Location: deelapi.RawNamed{Name: "Berlin"}},
				{Location: deelapi.RawNamed{Name: "London"}},
			},
		},
	}
	got := mapJobSummary(rec, "acme", nil)
	if got.Location != "Berlin; London" {
		t.Errorf("Location = %q, want %q", got.Location, "Berlin; London")
	}
}

func TestMapJobSummary_EmptyLocationsResultsInEmpty(t *testing.T) {
	rec := deelapi.RawJobPosting{ID: "e1", Title: "PM"}
	got := mapJobSummary(rec, "co", nil)
	if got.Location != "" {
		t.Errorf("expected empty location, got %q", got.Location)
	}
}

func TestMapJobSummary_NilJobContentPassedThrough(t *testing.T) {
	rec := deelapi.RawJobPosting{ID: "n1", Title: "Dev"}
	got := mapJobSummary(rec, "co", nil)
	if got.JobContent != nil {
		t.Error("expected nil JobContent when passed nil")
	}
}

// ── normalizeDeelEmployment ───────────────────────────────────────────────────

func TestNormalizeDeelEmployment(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"Full-time", core.EmployFullTime},
		{"full time", core.EmployFullTime},
		{"FULL_TIME", core.EmployFullTime},
		{"Part-time", core.EmployPartTime},
		{"part time", core.EmployPartTime},
		{"PART_TIME", core.EmployPartTime},
		{"Contract", core.EmployContract},
		{"Contractor", core.EmployContract},
		{"Consultant", core.EmployContract},
		{"Freelance", core.EmployContract},
		{"freelancer", core.EmployContract},
		{"Intern", core.EmployInternship},
		{"Internship", core.EmployInternship},
		{"INTERNSHIP", core.EmployInternship},
		{" ", core.EmployFullTime},       // whitespace → default
		{"", core.EmployFullTime},        // empty → default
		{"Unknown", core.EmployFullTime}, // unrecognized → default
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			if got := normalizeDeelEmployment(tc.in); got != tc.want {
				t.Errorf("normalizeDeelEmployment(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// ── formatLocations ───────────────────────────────────────────────────────────

func TestFormatLocations(t *testing.T) {
	tests := []struct {
		name string
		in   []deelapi.RawJobLocation
		want string
	}{
		{"nil", nil, ""},
		{"empty", []deelapi.RawJobLocation{}, ""},
		{"single", []deelapi.RawJobLocation{{Location: deelapi.RawNamed{Name: "Berlin"}}}, "Berlin"},
		{"multiple", []deelapi.RawJobLocation{
			{Location: deelapi.RawNamed{Name: "Berlin"}},
			{Location: deelapi.RawNamed{Name: "London"}},
		}, "Berlin; London"},
		{"blank-skipped", []deelapi.RawJobLocation{
			{Location: deelapi.RawNamed{Name: ""}},
			{Location: deelapi.RawNamed{Name: "Paris"}},
		}, "Paris"},
		{"whitespace-trimmed-and-skipped", []deelapi.RawJobLocation{
			{Location: deelapi.RawNamed{Name: "  "}},
			{Location: deelapi.RawNamed{Name: " Tokyo "}},
		}, "Tokyo"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := formatLocations(tc.in); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// ── formatCompensation ────────────────────────────────────────────────────────

func TestFormatCompensation(t *testing.T) {
	tests := []struct {
		name string
		in   *deelapi.RawCompensation
		want string
	}{
		{"nil", nil, ""},
		{"zero-range", &deelapi.RawCompensation{}, ""},
		{"eur-range", &deelapi.RawCompensation{CurrencyIsoCode: "EUR", MinAmount: 60000, MaxAmount: 80000}, "60000 – 80000 EUR"},
		{"no-currency-defaults-usd", &deelapi.RawCompensation{MinAmount: 100000, MaxAmount: 150000}, "100000 – 150000 USD"},
		// Only one bound is zero — still rendered (only both-zero → empty).
		{"only-max-zero", &deelapi.RawCompensation{CurrencyIsoCode: "USD", MinAmount: 50000, MaxAmount: 0}, "50000 – 0 USD"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := formatCompensation(tc.in); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// ── firstHelpers ──────────────────────────────────────────────────────────────

func TestFirstHelpers(t *testing.T) {
	if got := firstEmploymentTypeName(nil); got != "" {
		t.Errorf("nil EmploymentType = %q", got)
	}
	if got := firstDepartmentName(nil); got != "" {
		t.Errorf("nil Department = %q", got)
	}
	if got := firstPublishedAt(nil); got != "" {
		t.Errorf("nil PublishedAt = %q", got)
	}
	if got := firstEmploymentTypeName([]deelapi.RawEmploymentType{{EmploymentType: deelapi.RawNamed{Name: "Full-time"}}}); got != "Full-time" {
		t.Errorf("first EmploymentType = %q", got)
	}
	if got := firstDepartmentName([]deelapi.RawJobDepartment{{Department: deelapi.RawNamed{Name: "Eng"}}}); got != "Eng" {
		t.Errorf("first Department = %q", got)
	}
	if got := firstPublishedAt([]deelapi.RawJobPostingPub{{CurrentState: deelapi.RawPubState{CreatedAt: "2025-01-01"}}}); got != "2025-01-01" {
		t.Errorf("first PublishedAt = %q", got)
	}
}
