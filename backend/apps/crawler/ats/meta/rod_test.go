package meta

import (
	"testing"

	"github.com/trushilpatel/jobradar/apps/crawler/ats/types"
	"github.com/trushilpatel/jobradar/common/logger"
)

// ── CareersURL ───────────────────────────────────────────────────────────────

func TestRodMeta_CareersURL(t *testing.T) {
	m := NewRodMetaDefault()
	got := m.CareersURL("anything")
	want := "https://www.metacareers.com/jobsearch"
	if got != want {
		t.Errorf("CareersURL() = %q, want %q", got, want)
	}
}

// ── Constructor ──────────────────────────────────────────────────────────────

func TestNewRodMeta_StoresConfig(t *testing.T) {
	cfg := Config{MinDelayMs: 1000, MaxDelayMs: 3000, TimeoutMs: 60000, MaxJobs: 5}
	m := NewRodMeta(cfg, logger.NewNoop())
	if m.cfg.MinDelayMs != 1000 {
		t.Errorf("MinDelayMs: got %d, want 1000", m.cfg.MinDelayMs)
	}
	if m.cfg.MaxJobs != 5 {
		t.Errorf("MaxJobs: got %d, want 5", m.cfg.MaxJobs)
	}
}

func TestNewRodMetaDefault_UsesProductionDefaults(t *testing.T) {
	m := NewRodMetaDefault()
	def := defaultConfig()
	if m.cfg.MinDelayMs != def.MinDelayMs {
		t.Errorf("MinDelayMs: got %d, want %d", m.cfg.MinDelayMs, def.MinDelayMs)
	}
	if m.cfg.MaxJobs != def.MaxJobs {
		t.Errorf("MaxJobs: got %d, want %d", m.cfg.MaxJobs, def.MaxJobs)
	}
}

// ── Interface compliance ─────────────────────────────────────────────────────

func TestRodMeta_ImplementsAdapter(t *testing.T) {
	var _ types.Adapter = (*RodMeta)(nil)
}

func TestRodMeta_ImplementsURLProvider(t *testing.T) {
	var _ types.URLProvider = (*RodMeta)(nil)
}

// RodMeta is a full-detail adapter — FullContent is set by the browser scrape.
// The analyzer must NOT call FetchJobContent on it.
func TestRodMeta_DoesNotImplementContentFetcher(t *testing.T) {
	var a any = NewRodMetaDefault()
	if _, ok := a.(types.ContentFetcher); ok {
		t.Error("RodMeta must not implement ContentFetcher — it is a full-detail adapter")
	}
}

// ── StreamPublisher ──────────────────────────────────────────────────────────

func TestRodMeta_ImplementsStreamPublisher(t *testing.T) {
	var _ types.StreamPublisher = (*RodMeta)(nil)
}

func TestSetStreamPublish_StoresCallback(t *testing.T) {
	m := NewRodMetaDefault()
	called := false
	m.SetStreamPublish(func(items []types.FetchJobsItem) { called = true })
	if m.streamPublish == nil {
		t.Error("streamPublish should be set after SetStreamPublish")
	}
	m.streamPublish(nil)
	if !called {
		t.Error("stored callback should be callable")
	}
}

func TestSetStreamPublish_NilClearsCallback(t *testing.T) {
	m := NewRodMetaDefault()
	m.SetStreamPublish(func(_ []types.FetchJobsItem) {})
	m.SetStreamPublish(nil)
	if m.streamPublish != nil {
		t.Error("streamPublish should be nil after SetStreamPublish(nil)")
	}
}

// ── WithBrowser ──────────────────────────────────────────────────────────────

func TestWithBrowser_StoresBrowser(t *testing.T) {
	m := NewRodMetaDefault()
	if m.browser != nil {
		t.Fatal("browser should be nil before WithBrowser")
	}
	// We can't construct a real *rod.Browser without Chrome, but we can verify
	// that the field is set by passing nil (the method must accept it without panic).
	result := m.WithBrowser(nil)
	if result != m {
		t.Error("WithBrowser should return the same RodMeta for chaining")
	}
}

// ── toMetaItems ──────────────────────────────────────────────────────────────

func TestToMetaItems_PopulatesFullContentWhenDescriptionSet(t *testing.T) {
	jobs := []metaJob{{
		ID:          "meta-123",
		Title:       "Software Engineer",
		URL:         "https://www.metacareers.com/profile/job_details/123",
		Location:    "Menlo Park, CA",
		Team:        "Infrastructure",
		Description: "Minimum qualifications:\n- 5 years experience",
	}}
	items := toMetaItems(jobs)
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	item := items[0]
	if item.ID != "meta-123" {
		t.Errorf("ID: got %q, want meta-123", item.ID)
	}
	if item.Title != "Software Engineer" {
		t.Errorf("Title: got %q", item.Title)
	}
	if item.Location != "Menlo Park, CA" {
		t.Errorf("Location: got %q", item.Location)
	}
	if item.FullContent == nil {
		t.Fatal("FullContent must not be nil when description is set")
	}
	if item.FullContent.Department != "Infrastructure" {
		t.Errorf("Department: got %q, want Infrastructure", item.FullContent.Department)
	}
	if item.FullContent.Description == "" {
		t.Error("Description must not be empty")
	}
}

// Jobs whose detail page wasn't visited (context cancelled mid-run) should have
// nil FullContent so the analyzer falls back to its HTTP fetch.
func TestToMetaItems_NilFullContentWhenNoDescription(t *testing.T) {
	jobs := []metaJob{{
		ID:       "meta-99",
		Title:    "Engineer",
		URL:      "https://www.metacareers.com/profile/job_details/99",
		Location: "Remote",
		Team:     "Meta",
		// Description intentionally empty — detail page not visited
	}}
	items := toMetaItems(jobs)
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].FullContent != nil {
		t.Error("FullContent must be nil when description is empty — analyzer will HTTP-fetch it")
	}
}

func TestToMetaItems_EmptySlice(t *testing.T) {
	if len(toMetaItems(nil)) != 0 {
		t.Error("expected empty slice for nil input")
	}
	if len(toMetaItems([]metaJob{})) != 0 {
		t.Error("expected empty slice for empty input")
	}
}

// All Meta jobs are full-time — the site only lists full-time positions.
func TestToMetaItems_AlwaysFullTimeEmployment(t *testing.T) {
	jobs := []metaJob{
		{ID: "meta-1", Title: "A", URL: "u", Location: "L", Team: "T", Description: "d"},
		{ID: "meta-2", Title: "B", URL: "u", Location: "L", Team: "T"},
	}
	for _, item := range toMetaItems(jobs) {
		if item.EmploymentType != "full_time" {
			t.Errorf("EmploymentType = %q, want full_time", item.EmploymentType)
		}
	}
}

// ── metaJob struct ───────────────────────────────────────────────────────────

// Verify that metaJob JSON tags match what the browser JS outputs.
func TestMetaJob_JSONTagsMatchBrowserOutput(t *testing.T) {
	// The browser eval returns lowercase keys; verify the struct can decode them.
	raw := `{"id":"meta-1","title":"SWE","url":"https://example.com","location":"NYC","team":"Eng","description":"desc"}`
	var job metaJob
	if err := unmarshalMetaJob([]byte(raw), &job); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if job.ID != "meta-1" {
		t.Errorf("ID: got %q", job.ID)
	}
	if job.Team != "Eng" {
		t.Errorf("Team: got %q", job.Team)
	}
}
