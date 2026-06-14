package meta

import (
	"encoding/json"
	"strings"
	"testing"

	metaapi "github.com/trushilpatel/jobradar/apps/crawler-v2/adapters/ats/meta/api"
	"github.com/trushilpatel/jobradar/common/ats/core"
)

// ── mapJobSummary ─────────────────────────────────────────────────────────────

func TestMapJobSummary(t *testing.T) {
	s := mapJobSummary("1234567890")
	if s.ID != "meta-1234567890" {
		t.Errorf("ID = %q, want meta-1234567890", s.ID)
	}
	if s.URL != "https://www.metacareers.com/profile/job_details/1234567890" {
		t.Errorf("URL = %q", s.URL)
	}
	if s.Title != "" {
		t.Errorf("Title should be empty at listing time, got %q", s.Title)
	}
	if s.Location != "" {
		t.Errorf("Location should be empty at listing time, got %q", s.Location)
	}
}

// ── mapJobPost ────────────────────────────────────────────────────────────────

func makeJobPosting(t *testing.T) *metaapi.JobPosting {
	t.Helper()
	jp := &metaapi.JobPosting{
		Type:             "JobPosting",
		Title:            "Software Engineer, Infrastructure",
		Description:      "<p>We build large-scale systems.</p>",
		Responsibilities: "<p>Design distributed systems.</p>",
		Qualifications:   "<p>BS/MS Computer Science.</p>",
		EmploymentType:   "Full-time",
		DatePosted:       "2024-01-15",
		HiringOrg:        metaapi.HiringOrganization{Name: "Meta"},
	}
	_ = jp.JobLocation.UnmarshalJSON([]byte(
		`{"address":{"addressLocality":"Menlo Park","addressRegion":"CA","addressCountry":"USA"}}`,
	))
	return jp
}

func TestMapJobPost_AllFields(t *testing.T) {
	jp := makeJobPosting(t)
	post := mapJobPost(jp, "meta-1234567890")

	if post.ID != "meta-1234567890" {
		t.Errorf("ID = %q, want meta-1234567890", post.ID)
	}
	if post.Title != "Software Engineer, Infrastructure" {
		t.Errorf("Title = %q", post.Title)
	}
	if post.Location == "" {
		t.Error("Location is empty")
	}
	if post.Description == "" {
		t.Error("Description is empty")
	}
	if post.EmploymentType != core.EmployFullTime {
		t.Errorf("EmploymentType = %q, want %q", post.EmploymentType, core.EmployFullTime)
	}
	if post.Department != "Meta" {
		t.Errorf("Department = %q, want Meta", post.Department)
	}
	if post.PublishedDate != "2024-01-15" {
		t.Errorf("PublishedDate = %q, want 2024-01-15", post.PublishedDate)
	}
}

func TestMapJobPost_CountryCode(t *testing.T) {
	jp := makeJobPosting(t)
	post := mapJobPost(jp, "meta-1234567890")
	if post.CountryCode != "US" {
		t.Errorf("CountryCode = %q, want US", post.CountryCode)
	}
}

func TestMapJobPost_LocationFormatting(t *testing.T) {
	jp := makeJobPosting(t)
	post := mapJobPost(jp, "meta-1234567890")
	// Should contain city and region
	if !strings.Contains(post.Location, "Menlo Park") {
		t.Errorf("Location %q should contain Menlo Park", post.Location)
	}
	if !strings.Contains(post.Location, "CA") {
		t.Errorf("Location %q should contain CA", post.Location)
	}
}

func TestMapJobPost_Compensation_Present(t *testing.T) {
	jp := makeJobPosting(t)
	raw := `{"currency":"USD","value":{"minValue":150000,"maxValue":250000,"unitText":"YEAR"}}`
	jp.BaseSalary = json.RawMessage(raw)

	post := mapJobPost(jp, "meta-1234567890")
	if post.Compensation == "" {
		t.Error("Compensation is empty when baseSalary present")
	}
	if !strings.Contains(post.Compensation, "150000") {
		t.Errorf("Compensation %q should contain min value 150000", post.Compensation)
	}
	if !strings.Contains(post.Compensation, "250000") {
		t.Errorf("Compensation %q should contain max value 250000", post.Compensation)
	}
}

func TestMapJobPost_Compensation_Absent(t *testing.T) {
	jp := makeJobPosting(t)
	post := mapJobPost(jp, "meta-1234567890")
	if post.Compensation != "" {
		t.Errorf("Compensation should be empty when no baseSalary, got %q", post.Compensation)
	}
}

func TestMapJobPost_Compensation_MaxOnly(t *testing.T) {
	jp := makeJobPosting(t)
	jp.BaseSalary = json.RawMessage(`{"currency":"USD","value":{"minValue":0,"maxValue":200000,"unitText":"YEAR"}}`)

	post := mapJobPost(jp, "meta-1234567890")
	if post.Compensation == "" {
		t.Error("Compensation empty when maxValue is set")
	}
	if !strings.Contains(post.Compensation, "200000") {
		t.Errorf("Compensation %q should contain 200000", post.Compensation)
	}
}

func TestMapJobPost_DescriptionStripsHTML(t *testing.T) {
	jp := makeJobPosting(t)
	post := mapJobPost(jp, "meta-1234567890")
	if strings.Contains(post.Description, "<p>") {
		t.Errorf("Description should have HTML stripped, got: %s", post.Description)
	}
	if strings.Contains(post.Description, "<ul>") {
		t.Errorf("Description should have HTML stripped, got: %s", post.Description)
	}
}

func TestMapJobPost_EmptyJobPosting(t *testing.T) {
	jp := &metaapi.JobPosting{}
	post := mapJobPost(jp, "meta-999")
	if post.ID != "meta-999" {
		t.Errorf("ID = %q, want meta-999", post.ID)
	}
	if post.Title != "" {
		t.Errorf("Title should be empty for empty posting, got %q", post.Title)
	}
	if post.EmploymentType != core.EmployFullTime {
		t.Errorf("EmploymentType = %q, want %q", post.EmploymentType, core.EmployFullTime)
	}
}

// ── buildDescription ──────────────────────────────────────────────────────────

func TestBuildDescription_MultiSection(t *testing.T) {
	jp := &metaapi.JobPosting{
		Title:            "Engineer",
		Description:      "<p>We build things.</p>",
		Responsibilities: "<p>Build more things.</p>",
		Qualifications:   "<p>Know things.</p>",
	}
	desc := buildDescription(jp)
	if !strings.Contains(desc, "Engineer") {
		t.Errorf("description should contain title")
	}
	if !strings.Contains(desc, "We build things") {
		t.Errorf("description should contain stripped description text")
	}
	if !strings.Contains(desc, "Build more things") {
		t.Errorf("description should contain responsibilities text")
	}
	if !strings.Contains(desc, "Know things") {
		t.Errorf("description should contain qualifications text")
	}
}

func TestBuildDescription_LocationLine(t *testing.T) {
	jp := &metaapi.JobPosting{Title: "Eng"}
	_ = jp.JobLocation.UnmarshalJSON([]byte(`{"address":{"addressLocality":"Austin","addressRegion":"TX","addressCountry":"USA"}}`))
	desc := buildDescription(jp)
	if !strings.Contains(desc, "Austin") {
		t.Errorf("description should contain location city, got: %s", desc)
	}
}

// ── buildCompensation ─────────────────────────────────────────────────────────

func TestBuildCompensation_Empty(t *testing.T) {
	jp := &metaapi.JobPosting{}
	if got := buildCompensation(jp); got != "" {
		t.Errorf("buildCompensation on nil BaseSalary = %q, want empty", got)
	}
}

func TestBuildCompensation_ZeroValues(t *testing.T) {
	jp := &metaapi.JobPosting{
		BaseSalary: json.RawMessage(`{"currency":"USD","value":{"minValue":0,"maxValue":0}}`),
	}
	if got := buildCompensation(jp); got != "" {
		t.Errorf("buildCompensation with zero values = %q, want empty", got)
	}
}

func TestBuildCompensation_InvalidJSON(t *testing.T) {
	jp := &metaapi.JobPosting{BaseSalary: json.RawMessage(`not-json`)}
	if got := buildCompensation(jp); got != "" {
		t.Errorf("buildCompensation with invalid JSON = %q, want empty", got)
	}
}

func TestBuildCompensation_DefaultCurrencyAndUnit(t *testing.T) {
	// Missing currency and unitText — should default to "USD" and "YEAR".
	jp := &metaapi.JobPosting{
		BaseSalary: json.RawMessage(`{"value":{"minValue":100000,"maxValue":150000}}`),
	}
	got := buildCompensation(jp)
	if got == "" {
		t.Error("buildCompensation with missing currency/unit should still produce output")
	}
	if !strings.Contains(got, "USD") {
		t.Errorf("buildCompensation = %q, should contain default currency USD", got)
	}
	if !strings.Contains(got, "YEAR") {
		t.Errorf("buildCompensation = %q, should contain default unit YEAR", got)
	}
}

func TestBuildCompensation_MinValueOnly(t *testing.T) {
	// maxValue == 0 but minValue > 0 — only min value printed.
	jp := &metaapi.JobPosting{
		BaseSalary: json.RawMessage(`{"currency":"USD","value":{"minValue":80000,"maxValue":0,"unitText":"YEAR"}}`),
	}
	got := buildCompensation(jp)
	if got == "" {
		t.Error("buildCompensation with only minValue should produce output")
	}
	if !strings.Contains(got, "80000") {
		t.Errorf("buildCompensation = %q, should contain min value", got)
	}
}

func TestBuildDescription_TruncatesMoreThan5Locations(t *testing.T) {
	jp := &metaapi.JobPosting{Title: "Multi-location Eng"}
	// Create 7 locations to trigger the cap=5 truncation.
	_ = jp.JobLocation.UnmarshalJSON([]byte(`[
		{"address":{"addressLocality":"City1"}},
		{"address":{"addressLocality":"City2"}},
		{"address":{"addressLocality":"City3"}},
		{"address":{"addressLocality":"City4"}},
		{"address":{"addressLocality":"City5"}},
		{"address":{"addressLocality":"City6"}},
		{"address":{"addressLocality":"City7"}}
	]`))
	desc := buildDescription(jp)
	if !strings.Contains(desc, "+2 more") {
		t.Errorf("description should contain '+2 more' when 7 locations, got: %s", desc)
	}
}
