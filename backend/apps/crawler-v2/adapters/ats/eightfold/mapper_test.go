package eightfold

import (
	"strings"
	"testing"

	efapi "github.com/trushilpatel/jobradar/apps/crawler-v2/adapters/ats/eightfold/api"
	"github.com/trushilpatel/jobradar/apps/crawler-v2/core"
)

// ── mapJobSummary ──────────────────────────────────────────────────────────────

func TestMapJobSummary_LocationFallback(t *testing.T) {
	const testHost = "explore.jobs.example.net"
	tests := []struct {
		name      string
		location  string
		locations []string
		wantLoc   string
	}{
		{
			name:      "explicit location used",
			location:  "San Francisco, CA",
			locations: []string{"New York, NY"},
			wantLoc:   "San Francisco, CA",
		},
		{
			name:      "empty location falls back to joined locations",
			location:  "",
			locations: []string{"NYC", "Remote"},
			wantLoc:   "NYC; Remote",
		},
		{
			name:      "single fallback location",
			location:  "",
			locations: []string{"Chicago"},
			wantLoc:   "Chicago",
		},
		{
			name:      "both empty",
			location:  "",
			locations: nil,
			wantLoc:   "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := efapi.Position{
				ID:           1,
				Name:         "Engineer",
				Location:     tt.location,
				Locations:    tt.locations,
				CanonicalURL: "https://explore.jobs.example.net/careers/job/1",
			}
			got := mapJobSummary(p, testHost)
			if got.Location != tt.wantLoc {
				t.Errorf("Location = %q, want %q", got.Location, tt.wantLoc)
			}
		})
	}
}

func TestMapJobSummary_IDFormat(t *testing.T) {
	p := efapi.Position{
		ID:           790315382430,
		Name:         "Engineer",
		CanonicalURL: "https://explore.jobs.example.net/careers/job/790315382430",
	}
	got := mapJobSummary(p, "explore.jobs.example.net")
	want := "eightfold-790315382430"
	if got.ID != want {
		t.Errorf("ID = %q, want %q", got.ID, want)
	}
}

func TestMapJobSummary_URLFallback(t *testing.T) {
	const host = "explore.jobs.example.net"
	tests := []struct {
		name         string
		canonicalURL string
		id           int64
		wantURL      string
	}{
		{
			name:         "canonical URL used when present",
			canonicalURL: "https://explore.jobs.example.net/careers/job/42",
			id:           42,
			wantURL:      "https://explore.jobs.example.net/careers/job/42",
		},
		{
			name:         "fallback constructed when canonical empty",
			canonicalURL: "",
			id:           99,
			wantURL:      "https://" + host + "/careers/job/99",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := efapi.Position{
				ID:           tt.id,
				Name:         "Engineer",
				Location:     "NYC",
				CanonicalURL: tt.canonicalURL,
			}
			got := mapJobSummary(p, host)
			if got.URL != tt.wantURL {
				t.Errorf("URL = %q, want %q", got.URL, tt.wantURL)
			}
		})
	}
}

func TestMapJobSummary_EmployType(t *testing.T) {
	p := efapi.Position{ID: 1, Name: "Engineer", CanonicalURL: "https://example.com/careers/job/1"}
	got := mapJobSummary(p, "example.com")
	if got.EmployType != core.EmployFullTime {
		t.Errorf("EmployType = %q, want %q", got.EmployType, core.EmployFullTime)
	}
}

func TestMapJobSummary_LocationKnown(t *testing.T) {
	p := efapi.Position{ID: 1, Name: "Engineer", CanonicalURL: "https://example.com/careers/job/1"}
	got := mapJobSummary(p, "example.com")
	if !got.LocationKnown {
		t.Error("LocationKnown should always be true")
	}
}

func TestMapJobSummary_AllLocations(t *testing.T) {
	t.Run("single location wraps in slice", func(t *testing.T) {
		p := efapi.Position{
			ID:           1,
			Name:         "Engineer",
			Location:     "LA",
			Locations:    []string{"NYC"},
			CanonicalURL: "https://example.com/careers/job/1",
		}
		got := mapJobSummary(p, "example.com")
		if len(got.AllLocations) != 1 || got.AllLocations[0] != "LA" {
			t.Errorf("AllLocations = %v, want [LA]", got.AllLocations)
		}
	})
	t.Run("fallback locations propagated", func(t *testing.T) {
		p := efapi.Position{
			ID:        2,
			Name:      "Engineer",
			Location:  "",
			Locations: []string{"NYC", "Remote"},
		}
		got := mapJobSummary(p, "example.com")
		if len(got.AllLocations) != 2 {
			t.Errorf("AllLocations len = %d, want 2", len(got.AllLocations))
		}
	})
}

// ── mapJobPost ────────────────────────────────────────────────────────────────

func TestMapJobPost_AllFields(t *testing.T) {
	p := efapi.Position{
		ID:                 790315382430,
		Name:               "Senior Software Engineer",
		Location:           "Los Angeles, CA",
		Locations:          []string{"Los Angeles, CA"},
		Department:         "Engineering",
		TCreate:            1775779200,
		WorkLocationOption: "onsite",
		CanonicalURL:       "https://explore.jobs.example.net/careers/job/790315382430",
		JobDescription:     "<p>Build great things.</p>",
	}
	got := mapJobPost(p)

	if got.ID != "eightfold-790315382430" {
		t.Errorf("ID = %q, want eightfold-790315382430", got.ID)
	}
	if got.Description == "" {
		t.Error("Description is empty")
	}
	if strings.Contains(got.Description, "<") {
		t.Errorf("Description should have HTML stripped, got %q", got.Description)
	}
	if got.Department != "Engineering" {
		t.Errorf("Department = %q, want Engineering", got.Department)
	}
	if got.WorkplaceType != "onsite" {
		t.Errorf("WorkplaceType = %q, want onsite", got.WorkplaceType)
	}
	if got.PublishedDate == "" {
		t.Error("PublishedDate is empty")
	}
}

func TestMapJobPost_ZeroTCreate(t *testing.T) {
	p := efapi.Position{ID: 1, TCreate: 0}
	got := mapJobPost(p)
	if got.PublishedDate != "" {
		t.Errorf("PublishedDate should be empty for zero TCreate, got %q", got.PublishedDate)
	}
}

func TestMapJobPost_NegativeTCreate(t *testing.T) {
	p := efapi.Position{ID: 1, TCreate: -1}
	got := mapJobPost(p)
	if got.PublishedDate != "" {
		t.Errorf("PublishedDate should be empty for negative TCreate, got %q", got.PublishedDate)
	}
}

func TestMapJobPost_IDFormat(t *testing.T) {
	tests := []struct {
		id   int64
		want string
	}{
		{1, "eightfold-1"},
		{790315382430, "eightfold-790315382430"},
		{9999999999999, "eightfold-9999999999999"},
	}
	for _, tt := range tests {
		p := efapi.Position{ID: tt.id}
		got := mapJobPost(p)
		if got.ID != tt.want {
			t.Errorf("id=%d: got %q, want %q", tt.id, got.ID, tt.want)
		}
	}
}

// ── AdapterConfig.Validate ────────────────────────────────────────────────────

func TestAdapterConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     adapterConfig
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid config",
			cfg:     adapterConfig{Host: "explore.jobs.netflix.net", Domain: "netflix.com"},
			wantErr: false,
		},
		{
			name:    "valid with locale and max_jobs",
			cfg:     adapterConfig{Host: "careers.company.com", Domain: "company.com", Locale: "fr", MaxJobsPerScan: 100},
			wantErr: false,
		},
		{
			name:    "empty host",
			cfg:     adapterConfig{Host: "", Domain: "netflix.com"},
			wantErr: true,
			errMsg:  "host is required",
		},
		{
			name:    "host with scheme",
			cfg:     adapterConfig{Host: "https://explore.jobs.netflix.net", Domain: "netflix.com"},
			wantErr: true,
			errMsg:  "no scheme",
		},
		{
			name:    "empty domain",
			cfg:     adapterConfig{Host: "explore.jobs.netflix.net", Domain: ""},
			wantErr: true,
			errMsg:  "domain is required",
		},
		{
			name:    "host without dot",
			cfg:     adapterConfig{Host: "localhost", Domain: "example.com"},
			wantErr: true,
			errMsg:  "FQDN",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.validate()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("validate() error = nil, want error containing %q", tt.errMsg)
				}
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("validate() error = %q, want to contain %q", err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("validate() unexpected error: %v", err)
				}
			}
		})
	}
}

// ── unixToRFC3339 ─────────────────────────────────────────────────────────────

func TestUnixToRFC3339(t *testing.T) {
	t.Run("zero returns empty", func(t *testing.T) {
		if got := unixToRFC3339(0); got != "" {
			t.Errorf("unixToRFC3339(0) = %q, want empty string", got)
		}
	})
	t.Run("negative returns empty", func(t *testing.T) {
		if got := unixToRFC3339(-1); got != "" {
			t.Errorf("unixToRFC3339(-1) = %q, want empty string", got)
		}
	})
	t.Run("known timestamp", func(t *testing.T) {
		got := unixToRFC3339(1)
		want := "1970-01-01T00:00:01Z"
		if got != want {
			t.Errorf("unixToRFC3339(1) = %q, want %q", got, want)
		}
	})
	t.Run("non-zero returns RFC3339", func(t *testing.T) {
		got := unixToRFC3339(1775779200)
		if got == "" {
			t.Fatal("unixToRFC3339(1775779200) returned empty string")
		}
		if !strings.Contains(got, "T") || !strings.HasSuffix(got, "Z") {
			t.Errorf("unixToRFC3339(1775779200) = %q, does not look like RFC3339", got)
		}
	})
}
