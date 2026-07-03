package workday

import (
	"encoding/json"
	"strings"
	"testing"

	wdapi "github.com/trushilpatel/jobradar/apps/crawler-v2/adapters/ats/workday/api"
	"github.com/trushilpatel/jobradar/common/ats/core"
)

// ── adapterConfig.validate ────────────────────────────────────────────────────

func TestAdapterConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     adapterConfig
		wantErr string
	}{
		{
			name:    "valid",
			cfg:     adapterConfig{Host: "nvidia.wd5.myworkdayjobs.com", Site: "NVIDIAExternalCareerSite"},
			wantErr: "",
		},
		{
			name:    "valid with locale",
			cfg:     adapterConfig{Host: "polar-semi.wd501.myworkdayjobs.com", Site: "MySite", Locale: "en-US"},
			wantErr: "",
		},
		{
			name:    "missing host",
			cfg:     adapterConfig{Host: "", Site: "NVIDIAExternalCareerSite"},
			wantErr: "host is required",
		},
		{
			name:    "host with spaces",
			cfg:     adapterConfig{Host: "  ", Site: "NVIDIAExternalCareerSite"},
			wantErr: "host is required",
		},
		{
			name:    "bad host pattern — wrong TLD",
			cfg:     adapterConfig{Host: "nvidia.example.com", Site: "NVIDIAExternalCareerSite"},
			wantErr: "does not match pattern",
		},
		{
			name:    "bad host pattern — no wd segment",
			cfg:     adapterConfig{Host: "nvidia.myworkdayjobs.com", Site: "NVIDIAExternalCareerSite"},
			wantErr: "does not match pattern",
		},
		{
			name:    "bad host pattern — uppercase",
			cfg:     adapterConfig{Host: "NVIDIA.wd5.myworkdayjobs.com", Site: "NVIDIAExternalCareerSite"},
			wantErr: "does not match pattern",
		},
		{
			name:    "missing site",
			cfg:     adapterConfig{Host: "nvidia.wd5.myworkdayjobs.com", Site: ""},
			wantErr: "site is required",
		},
		{
			name:    "site with slash",
			cfg:     adapterConfig{Host: "nvidia.wd5.myworkdayjobs.com", Site: "foo/bar"},
			wantErr: "must not contain",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("expected no error, got: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestAdapterConfig_Tenant(t *testing.T) {
	tests := []struct {
		host       string
		wantTenant string
	}{
		{"nvidia.wd5.myworkdayjobs.com", "nvidia"},
		{"polar-semi.wd501.myworkdayjobs.com", "polar-semi"},
		{"jpmc.wd1.myworkdayjobs.com", "jpmc"},
	}
	for _, tt := range tests {
		cfg := adapterConfig{Host: tt.host}
		if got := cfg.tenant(); got != tt.wantTenant {
			t.Errorf("host=%q: tenant() = %q, want %q", tt.host, got, tt.wantTenant)
		}
	}
}

func TestAdapterConfig_Locale(t *testing.T) {
	tests := []struct {
		name       string
		cfg        adapterConfig
		wantLocale string
	}{
		{
			name:       "default locale when empty",
			cfg:        adapterConfig{Host: "nvidia.wd5.myworkdayjobs.com", Site: "NVIDIAExternalCareerSite"},
			wantLocale: "en-US",
		},
		{
			name:       "configured locale is used",
			cfg:        adapterConfig{Host: "nvidia.wd5.myworkdayjobs.com", Site: "NVIDIAExternalCareerSite", Locale: "fr-FR"},
			wantLocale: "fr-FR",
		},
		{
			name:       "en-US explicit",
			cfg:        adapterConfig{Host: "nvidia.wd5.myworkdayjobs.com", Site: "NVIDIAExternalCareerSite", Locale: "en-US"},
			wantLocale: "en-US",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.locale(); got != tt.wantLocale {
				t.Errorf("locale() = %q, want %q", got, tt.wantLocale)
			}
		})
	}
}

// ── mapJobSummary ─────────────────────────────────────────────────────────────

func TestMapJobSummary_AllFields(t *testing.T) {
	jp := wdapi.JobPosting{
		Title:         "Senior Software Engineer",
		ExternalPath:  "/job/San-Jose/Senior-Software-Engineer_R-12345",
		LocationsText: "San Jose, CA",
		PostedOn:      "Posted 5 Days Ago",
		BulletFields:  []string{"R-12345"},
	}
	cfg := adapterConfig{Host: "nvidia.wd5.myworkdayjobs.com", Site: "NVIDIAExternalCareerSite"}

	got := mapJobSummary(jp, cfg)

	want := core.JobSummary{
		ID:         "workday-R-12345",
		Title:      "Senior Software Engineer",
		Location:   "San Jose, CA",
		URL:        "https://nvidia.wd5.myworkdayjobs.com/NVIDIAExternalCareerSite/job/San-Jose/Senior-Software-Engineer_R-12345",
		EmployType: core.EmployFullTime,
		// LocationKnown is intentionally false: the CXS listing carries only a
		// coarse LocationsText that cannot be trusted for string-based non-USA
		// rejection. The real location decision is deferred to enrich-time, where
		// FetchJobPost supplies a reliable CountryCode.
		LocationKnown: false,
	}

	if got.ID != want.ID {
		t.Errorf("ID = %q, want %q", got.ID, want.ID)
	}
	if got.Title != want.Title {
		t.Errorf("Title = %q, want %q", got.Title, want.Title)
	}
	if got.Location != want.Location {
		t.Errorf("Location = %q, want %q", got.Location, want.Location)
	}
	if got.URL != want.URL {
		t.Errorf("URL = %q, want %q", got.URL, want.URL)
	}
	if got.EmployType != want.EmployType {
		t.Errorf("EmployType = %q, want %q", got.EmployType, want.EmployType)
	}
	if got.LocationKnown != want.LocationKnown {
		t.Errorf("LocationKnown = %v, want %v", got.LocationKnown, want.LocationKnown)
	}
}

// TestMapJobSummary_LocationDeferred locks the invariant that Workday listing
// summaries never claim a known location. Workday's CXS listing exposes only a
// coarse LocationsText (no structured country), so string-based non-USA
// rejection at filter-time would mis-fire. The decision is deferred to
// enrich-time via FetchJobPost.CountryCode. Matches v1 parity (jobToItem leaves
// FetchJobsItem.LocationKnown at its false zero value).
func TestMapJobSummary_LocationDeferred(t *testing.T) {
	jp := wdapi.JobPosting{
		Title:         "Engineer",
		ExternalPath:  "/job/Bengaluru/Engineer_R-99",
		LocationsText: "Bengaluru, India",
		BulletFields:  []string{"R-99"},
	}
	cfg := adapterConfig{Host: "nvidia.wd5.myworkdayjobs.com", Site: "NVIDIAExternalCareerSite"}

	got := mapJobSummary(jp, cfg)

	if got.LocationKnown {
		t.Error("LocationKnown must be false at listing time; Workday location is deferred to enrich-time")
	}
	if got.AllLocations != nil {
		t.Errorf("AllLocations = %v, want nil (no reliable structured locations at listing time)", got.AllLocations)
	}
}

func TestMapJobSummary_FallbackIDFromExternalPath(t *testing.T) {
	// No bulletFields → should fall back to trailing _<id> from externalPath.
	jp := wdapi.JobPosting{
		Title:        "Backend Engineer",
		ExternalPath: "/job/Remote/Backend-Engineer_R-99999",
		BulletFields: []string{},
	}
	cfg := adapterConfig{Host: "nvidia.wd5.myworkdayjobs.com", Site: "NVIDIAExternalCareerSite"}

	got := mapJobSummary(jp, cfg)
	if got.ID != "workday-R-99999" {
		t.Errorf("ID = %q, want workday-R-99999", got.ID)
	}
}

func TestMapJobSummary_FallbackIDUnknown(t *testing.T) {
	// No bulletFields, no underscore in externalPath → "unknown".
	jp := wdapi.JobPosting{
		Title:        "Engineer",
		ExternalPath: "",
		BulletFields: []string{},
	}
	cfg := adapterConfig{Host: "nvidia.wd5.myworkdayjobs.com", Site: "NVIDIAExternalCareerSite"}

	got := mapJobSummary(jp, cfg)
	if got.ID != "workday-unknown" {
		t.Errorf("ID = %q, want workday-unknown", got.ID)
	}
}

// ── mapJobPost ────────────────────────────────────────────────────────────────

func TestMapJobPost_AllFields(t *testing.T) {
	countryJSON := json.RawMessage(`{"descriptor":"United States of America","id":"bc33aa3152ec42d4995f4791a106ed09"}`)
	info := wdapi.JobPostingInfo{
		Title:          "Senior Software Engineer",
		JobDescription: "<p>Join our team.</p>",
		Location:       "San Jose, CA",
		StartDate:      "2026-05-01",
		PostedOn:       "Posted 5 Days Ago",
		TimeType:       "Full time",
		JobReqID:       "R-12345",
		Country:        countryJSON,
	}
	jobURL := "https://nvidia.wd5.myworkdayjobs.com/NVIDIAExternalCareerSite/job/San-Jose/Senior-SWE_R-12345"

	got := mapJobPost(info, jobURL)

	if got.ID != "workday-R-12345" {
		t.Errorf("ID = %q, want workday-R-12345", got.ID)
	}
	if got.Title != "Senior Software Engineer" {
		t.Errorf("Title = %q, want Senior Software Engineer", got.Title)
	}
	if got.Location != "San Jose, CA" {
		t.Errorf("Location = %q, want San Jose, CA", got.Location)
	}
	if got.URL != jobURL {
		t.Errorf("URL = %q, want %q", got.URL, jobURL)
	}
	if got.CountryCode != "US" {
		t.Errorf("CountryCode = %q, want US", got.CountryCode)
	}
	if got.PublishedDate != "2026-05-01" {
		t.Errorf("PublishedDate = %q, want 2026-05-01", got.PublishedDate)
	}
	if got.EmploymentType != core.EmployFullTime {
		t.Errorf("EmploymentType = %q, want %q", got.EmploymentType, core.EmployFullTime)
	}
}

func TestMapJobPost_DescriptionStripped(t *testing.T) {
	info := wdapi.JobPostingInfo{
		JobDescription: "<p>We are looking for a <strong>Senior</strong> engineer.</p><ul><li>Design systems</li></ul>",
		JobReqID:       "R-1",
	}
	got := mapJobPost(info, "https://nvidia.wd5.myworkdayjobs.com/Site/job/loc/title_R-1")

	if strings.Contains(got.Description, "<p>") {
		t.Error("Description should have HTML tags stripped")
	}
	if strings.Contains(got.Description, "<strong>") {
		t.Error("Description should have strong tags stripped")
	}
	if strings.Contains(got.Description, "<ul>") {
		t.Error("Description should have ul tags stripped")
	}
	if got.Description == "" {
		t.Error("Description should not be empty after stripping")
	}
	// Text content should remain.
	if !strings.Contains(got.Description, "Senior") {
		t.Error("Description should retain text content")
	}
}

func TestMapJobPost_StartDateFallback(t *testing.T) {
	// StartDate empty → fall back to PostedOn.
	info := wdapi.JobPostingInfo{
		StartDate: "",
		PostedOn:  "Posted Today",
		JobReqID:  "R-1",
	}
	got := mapJobPost(info, "https://nvidia.wd5.myworkdayjobs.com/Site/job/loc/title_R-1")
	if got.PublishedDate != "Posted Today" {
		t.Errorf("PublishedDate = %q, want Posted Today", got.PublishedDate)
	}
}

func TestMapJobPost_NullCountry(t *testing.T) {
	info := wdapi.JobPostingInfo{
		JobReqID: "R-1",
		Country:  json.RawMessage("null"),
	}
	got := mapJobPost(info, "https://nvidia.wd5.myworkdayjobs.com/Site/job/loc/title_R-1")
	if got.CountryCode != "" {
		t.Errorf("CountryCode = %q, want empty for null country", got.CountryCode)
	}
}

// ── mapEmploymentType ─────────────────────────────────────────────────────────

func TestMapEmploymentType(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Full time", core.EmployFullTime},
		{"Full-time", core.EmployFullTime},
		{"full_time", core.EmployFullTime},
		{"Regular", core.EmployFullTime},
		{"Part time", core.EmployPartTime},
		{"Part-time", core.EmployPartTime},
		{"Contract", core.EmployContract},
		{"Contractor", core.EmployContract},
		{"Consultant", core.EmployContract},
		{"Fixed term", core.EmployContract},
		{"Intern", core.EmployInternship},
		{"Internship", core.EmployInternship},
		{"", core.EmployFullTime},
		{"unknown value", core.EmployFullTime},
	}
	for _, tt := range tests {
		got := mapEmploymentType(tt.input)
		if got != tt.want {
			t.Errorf("mapEmploymentType(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// ── extractCountryCode ────────────────────────────────────────────────────────

func TestExtractCountryCode(t *testing.T) {
	tests := []struct {
		name string
		raw  json.RawMessage
		want string
	}{
		{
			// AC2.1: a real tenant-internal WID in `id` must NOT be used; the US
			// descriptor is the signal that yields "US".
			name: "US via descriptor with real WID id",
			raw:  json.RawMessage(`{"descriptor":"United States of America","id":"bc33aa3152ec42d4995f4791a106ed09"}`),
			want: "US",
		},
		{
			// AC2.6: descriptor variants are matched case-insensitively and trimmed.
			name: "US variant United States",
			raw:  json.RawMessage(`{"descriptor":"United States","id":"aaa"}`),
			want: "US",
		},
		{
			name: "US variant USA",
			raw:  json.RawMessage(`{"descriptor":"USA","id":"bbb"}`),
			want: "US",
		},
		{
			name: "US variant U.S.",
			raw:  json.RawMessage(`{"descriptor":"U.S.","id":"ccc"}`),
			want: "US",
		},
		{
			name: "US variant US",
			raw:  json.RawMessage(`{"descriptor":"US","id":"ddd"}`),
			want: "US",
		},
		{
			name: "US descriptor case-insensitive with surrounding whitespace",
			raw:  json.RawMessage(`{"descriptor":"  united states of AMERICA  ","id":"eee"}`),
			want: "US",
		},
		{
			// AC2.3: a non-empty non-US descriptor returns the normalized descriptor
			// name so the analyzer's country-code gate fires immediately, rather than
			// falling through to string matching which misses lesser-known cities.
			name: "Japan descriptor → normalized name",
			raw:  json.RawMessage(`{"descriptor":"Japan","id":"fff"}`),
			want: "japan",
		},
		{
			name: "United Kingdom descriptor → normalized name",
			raw:  json.RawMessage(`{"descriptor":"United Kingdom","id":"ggg"}`),
			want: "united kingdom",
		},
		{
			name: "India descriptor → normalized name",
			raw:  json.RawMessage(`{"descriptor":"India","id":"hhh"}`),
			want: "india",
		},
		{
			name: "Spain descriptor → normalized name",
			raw:  json.RawMessage(`{"descriptor":"Spain","id":"iii"}`),
			want: "spain",
		},
		{
			// AC2.4: missing / null / malformed / empty-object all return "".
			name: "null",
			raw:  json.RawMessage("null"),
			want: "",
		},
		{
			name: "empty",
			raw:  json.RawMessage(""),
			want: "",
		},
		{
			name: "malformed",
			raw:  json.RawMessage("not-json"),
			want: "",
		},
		{
			name: "empty object",
			raw:  json.RawMessage("{}"),
			want: "",
		},
		{
			name: "object with empty descriptor",
			raw:  json.RawMessage(`{"descriptor":"","id":"zzz"}`),
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractCountryCode(tt.raw)
			if got != tt.want {
				t.Errorf("extractCountryCode(%s) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

// ── buildID ───────────────────────────────────────────────────────────────────

func TestBuildID(t *testing.T) {
	tests := []struct {
		name          string
		requisitionID string
		externalURL   string
		want          string
	}{
		{
			name:          "explicit requisition ID",
			requisitionID: "R-12345",
			externalURL:   "https://nvidia.wd5.myworkdayjobs.com/Site/job/loc/title_R-12345",
			want:          "workday-R-12345",
		},
		{
			name:          "empty requisitionID — fallback to externalURL path segment",
			requisitionID: "",
			externalURL:   "https://nvidia.wd5.myworkdayjobs.com/Site/job/San-Jose/Title_R-99999",
			want:          "workday-R-99999",
		},
		{
			name:          "empty requisitionID and no underscore in path — unknown",
			requisitionID: "",
			externalURL:   "https://nvidia.wd5.myworkdayjobs.com/Site/job/somepath",
			want:          "workday-unknown",
		},
		{
			name:          "empty requisitionID and empty externalURL — unknown",
			requisitionID: "",
			externalURL:   "",
			want:          "workday-unknown",
		},
		{
			name:          "whitespace-only requisitionID treated as empty",
			requisitionID: "   ",
			externalURL:   "https://nvidia.wd5.myworkdayjobs.com/Site/job/loc/Title_R-555",
			want:          "workday-R-555",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildID(tt.requisitionID, tt.externalURL)
			if got != tt.want {
				t.Errorf("buildID(%q, %q) = %q, want %q", tt.requisitionID, tt.externalURL, got, tt.want)
			}
		})
	}
}

// ── tenantFromHost ────────────────────────────────────────────────────────────

func TestTenantFromHost(t *testing.T) {
	tests := []struct {
		host string
		want string
	}{
		{"nvidia.wd5.myworkdayjobs.com", "nvidia"},
		{"NVIDIA.WD5.MYWORKDAYJOBS.COM", "nvidia"}, // lowercased
		{"", ""},
		{"noDot", ""},
		{".startswithDot", ""}, // idx == 0
	}
	for _, tt := range tests {
		got := tenantFromHost(tt.host)
		if got != tt.want {
			t.Errorf("tenantFromHost(%q) = %q, want %q", tt.host, got, tt.want)
		}
	}
}

// ── parseJobURL ───────────────────────────────────────────────────────────────

func TestParseJobURL(t *testing.T) {
	tests := []struct {
		name             string
		url              string
		wantHost         string
		wantSite         string
		wantExternalPath string
		wantErr          bool
	}{
		{
			name:             "valid nvidia URL",
			url:              "https://nvidia.wd5.myworkdayjobs.com/NVIDIAExternalCareerSite/job/San-Jose/Senior-SWE_R-12345",
			wantHost:         "nvidia.wd5.myworkdayjobs.com",
			wantSite:         "NVIDIAExternalCareerSite",
			wantExternalPath: "/job/San-Jose/Senior-SWE_R-12345",
		},
		{
			name:             "valid jpmc URL",
			url:              "https://jpmc.wd1.myworkdayjobs.com/External/job/New-York/Analyst_R-001",
			wantHost:         "jpmc.wd1.myworkdayjobs.com",
			wantSite:         "External",
			wantExternalPath: "/job/New-York/Analyst_R-001",
		},
		{
			name:    "non-workday host",
			url:     "https://example.com/site/job/123/title",
			wantErr: true,
		},
		{
			name:    "missing job segment",
			url:     "https://nvidia.wd5.myworkdayjobs.com/NVIDIAExternalCareerSite/apply/12345",
			wantErr: true,
		},
		{
			name:    "empty url",
			url:     "",
			wantErr: true,
		},
		{
			name:    "site segment is empty (path starts with /job/)",
			url:     "https://nvidia.wd5.myworkdayjobs.com//job/San-Jose/Title_R-1",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host, site, externalPath, err := parseJobURL(tt.url)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (host=%q site=%q externalPath=%q)", host, site, externalPath)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if host != tt.wantHost {
				t.Errorf("host = %q, want %q", host, tt.wantHost)
			}
			if site != tt.wantSite {
				t.Errorf("site = %q, want %q", site, tt.wantSite)
			}
			if externalPath != tt.wantExternalPath {
				t.Errorf("externalPath = %q, want %q", externalPath, tt.wantExternalPath)
			}
		})
	}
}

// ── extractRequisitionID ──────────────────────────────────────────────────────

func TestExtractRequisitionID(t *testing.T) {
	tests := []struct {
		name string
		jp   wdapi.JobPosting
		want string
	}{
		{
			name: "bulletFields first non-empty",
			jp:   wdapi.JobPosting{BulletFields: []string{"R-12345"}, ExternalPath: "/job/x/y_other"},
			want: "R-12345",
		},
		{
			name: "empty bulletFields → trailing path segment",
			jp:   wdapi.JobPosting{BulletFields: []string{}, ExternalPath: "/job/Remote/Title_R-99999"},
			want: "R-99999",
		},
		{
			name: "no underscore in path → full path",
			jp:   wdapi.JobPosting{BulletFields: []string{}, ExternalPath: "/job/someID"},
			want: "/job/someID",
		},
		{
			name: "empty everything → unknown",
			jp:   wdapi.JobPosting{},
			want: "unknown",
		},
		{
			name: "blank bulletField falls through to path",
			jp:   wdapi.JobPosting{BulletFields: []string{" "}, ExternalPath: "/job/Loc/Title_R-555"},
			want: "R-555",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractRequisitionID(tt.jp)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
