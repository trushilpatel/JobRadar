package greenhouse

import (
	"reflect"
	"testing"

	ghapi "github.com/trushilpatel/jobradar/jobingest/adapters/ats/greenhouse/api"
	"github.com/trushilpatel/jobradar/jobingest/core"
)

// ── mapJobSummary ─────────────────────────────────────────────────────────────

func TestMapJobSummary_AllFields(t *testing.T) {
	input := ghapi.JobSummary{
		ID:    12345,
		Title: "Backend Engineer",
		Location: struct {
			Name string `json:"name"`
		}{Name: "San Francisco, CA"},
		AbsoluteURL: "https://job-boards.greenhouse.io/stripe/jobs/12345",
	}
	want := core.JobSummary{
		ID:       "greenhouse-12345",
		Title:    "Backend Engineer",
		Location: "San Francisco, CA",
		URL:      "https://job-boards.greenhouse.io/stripe/jobs/12345",
	}
	if got := mapJobSummary(input); !reflect.DeepEqual(got, want) {
		t.Errorf("mapJobSummary:\n got  %+v\n want %+v", got, want)
	}
}

func TestMapJobSummary_EmptyLocation(t *testing.T) {
	input := ghapi.JobSummary{ID: 1, Title: "Engineer"}
	got := mapJobSummary(input)
	if got.Location != "" {
		t.Errorf("expected empty location, got %q", got.Location)
	}
}

// ── mapJobListing ─────────────────────────────────────────────────────────────

func TestMapJobListing_MultipleJobs(t *testing.T) {
	input := ghapi.JobListing{
		Jobs: []ghapi.JobSummary{
			{ID: 1, Title: "Engineer A"},
			{ID: 2, Title: "Engineer B"},
		},
	}
	got := mapJobListing(input)
	if len(got.Jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(got.Jobs))
	}
	if got.Jobs[0].ID != "greenhouse-1" {
		t.Errorf("jobs[0].ID = %q, want greenhouse-1", got.Jobs[0].ID)
	}
	if got.Jobs[1].ID != "greenhouse-2" {
		t.Errorf("jobs[1].ID = %q, want greenhouse-2", got.Jobs[1].ID)
	}
}

func TestMapJobListing_Empty(t *testing.T) {
	got := mapJobListing(ghapi.JobListing{})
	if len(got.Jobs) != 0 {
		t.Errorf("expected empty jobs, got %d", len(got.Jobs))
	}
}

// ── mapJobPost ────────────────────────────────────────────────────────────────

func TestMapJobPost_AllFields(t *testing.T) {
	input := ghapi.JobPost{
		ID:    12345,
		Title: "Backend Engineer",
		Location: struct {
			Name string `json:"name"`
		}{Name: "San Francisco, CA"},
		AbsoluteURL: "https://job-boards.greenhouse.io/stripe/jobs/12345",
		Content:     "<p>Join our team.</p>",
		Departments: []struct {
			Name string `json:"name"`
		}{{Name: "Engineering"}},
		UpdatedAt: "2024-01-15T10:00:00Z",
		PayInputRanges: []struct {
			MinCents     int    `json:"min_cents"`
			MaxCents     int    `json:"max_cents"`
			CurrencyType string `json:"currency_type"`
			Title        string `json:"title"`
			Blurb        string `json:"blurb"`
		}{{MinCents: 5000000, MaxCents: 7500000, CurrencyType: "USD"}},
	}
	want := core.JobPost{
		ID:            "greenhouse-12345",
		Title:         "Backend Engineer",
		Location:      "San Francisco, CA",
		URL:           "https://job-boards.greenhouse.io/stripe/jobs/12345",
		Description:   "<p>Join our team.</p>",
		Department:    "Engineering",
		Compensation:  "$50000 – $75000 USD",
		PublishedDate: "2024-01-15T10:00:00Z",
	}
	if got := mapJobPost(input); got != want {
		t.Errorf("mapJobPost:\n got  %+v\n want %+v", got, want)
	}
}

func TestMapJobPost_NoDepartment(t *testing.T) {
	input := ghapi.JobPost{ID: 1, Title: "Engineer"}
	got := mapJobPost(input)
	if got.Department != "" {
		t.Errorf("expected empty department, got %q", got.Department)
	}
}

func TestMapJobPost_NoCompensation(t *testing.T) {
	input := ghapi.JobPost{ID: 1, Title: "Engineer"}
	got := mapJobPost(input)
	if got.Compensation != "" {
		t.Errorf("expected empty compensation, got %q", got.Compensation)
	}
}

func TestMapJobPost_OnlyFirstDepartment(t *testing.T) {
	input := ghapi.JobPost{
		ID: 1,
		Departments: []struct {
			Name string `json:"name"`
		}{
			{Name: "Engineering"},
			{Name: "Product"},
		},
	}
	got := mapJobPost(input)
	if got.Department != "Engineering" {
		t.Errorf("expected first department Engineering, got %q", got.Department)
	}
}

func TestMapJobPost_OnlyFirstPayRange(t *testing.T) {
	input := ghapi.JobPost{
		ID: 1,
		PayInputRanges: []struct {
			MinCents     int    `json:"min_cents"`
			MaxCents     int    `json:"max_cents"`
			CurrencyType string `json:"currency_type"`
			Title        string `json:"title"`
			Blurb        string `json:"blurb"`
		}{
			{MinCents: 10000000, MaxCents: 15000000, CurrencyType: "USD"},
			{MinCents: 1000000, MaxCents: 2000000, CurrencyType: "EUR"},
		},
	}
	got := mapJobPost(input)
	if got.Compensation != "$100000 – $150000 USD" {
		t.Errorf("compensation = %q, want $100000 – $150000 USD", got.Compensation)
	}
}

func TestMapJobPost_IDFormat(t *testing.T) {
	tests := []struct {
		id   int
		want string
	}{
		{1, "greenhouse-1"},
		{12345, "greenhouse-12345"},
		{9999999, "greenhouse-9999999"},
	}
	for _, tt := range tests {
		got := mapJobPost(ghapi.JobPost{ID: tt.id})
		if got.ID != tt.want {
			t.Errorf("id=%d: got %q, want %q", tt.id, got.ID, tt.want)
		}
	}
}
