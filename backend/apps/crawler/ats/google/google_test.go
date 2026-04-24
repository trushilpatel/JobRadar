package google

import (
	"testing"

	"github.com/trushilpatel/jobradar/apps/crawler/ats/types"
	"github.com/trushilpatel/jobradar/common/logger"
)

// ── CareersURL ───────────────────────────────────────────────────────────────

func TestRodGoogle_CareersURL(t *testing.T) {
	g := NewRodGoogleDefault()
	got := g.CareersURL("anything")
	if got != careersURL {
		t.Errorf("CareersURL() = %q, want %q", got, careersURL)
	}
}

// ── Constructor ──────────────────────────────────────────────────────────────

func TestNewRodGoogle_StoresConfig(t *testing.T) {
	cfg := Config{
		MinDelayMs:    5000,
		MaxDelayMs:    8000,
		TimeoutMs:     60000,
		MaxPages:      2,
		EnrichMinMs:   3000,
		EnrichMaxMs:   5000,
		MaxDetailJobs: 10,
	}
	g := NewRodGoogle(cfg, logger.NewNoop())
	if g.cfg.MinDelayMs != 5000 {
		t.Errorf("MinDelayMs: got %d, want 5000", g.cfg.MinDelayMs)
	}
	if g.cfg.MaxDetailJobs != 10 {
		t.Errorf("MaxDetailJobs: got %d, want 10", g.cfg.MaxDetailJobs)
	}
}

func TestNewRodGoogleDefault_UsesProductionDefaults(t *testing.T) {
	g := NewRodGoogleDefault()
	def := defaultConfig()
	if g.cfg.MinDelayMs != def.MinDelayMs {
		t.Errorf("MinDelayMs: got %d, want %d", g.cfg.MinDelayMs, def.MinDelayMs)
	}
	if g.cfg.TimeoutMs != def.TimeoutMs {
		t.Errorf("TimeoutMs: got %d, want %d", g.cfg.TimeoutMs, def.TimeoutMs)
	}
}

// ── Interface compliance ─────────────────────────────────────────────────────

func TestRodGoogle_ImplementsAdapter(t *testing.T) {
	var _ types.Adapter = (*RodGoogle)(nil)
}

func TestRodGoogle_ImplementsURLProvider(t *testing.T) {
	var _ types.URLProvider = (*RodGoogle)(nil)
}

func TestRodGoogle_ImplementsKnownJobsProvider(t *testing.T) {
	var _ types.KnownJobsProvider = (*RodGoogle)(nil)
}

func TestRodGoogle_DoesNotImplementContentFetcher(t *testing.T) {
	var g any = NewRodGoogleDefault()
	if _, ok := g.(types.ContentFetcher); ok {
		t.Error("RodGoogle should not implement ContentFetcher — it is a full-detail adapter")
	}
}

// ── SetKnownJobIDs ───────────────────────────────────────────────────────────

func TestSetKnownJobIDs_StoresIDs(t *testing.T) {
	g := NewRodGoogleDefault()
	ids := map[string]struct{}{"google-111": {}, "google-222": {}}
	g.SetKnownJobIDs(ids)
	if len(g.knownIDs) != 2 {
		t.Errorf("knownIDs len: got %d, want 2", len(g.knownIDs))
	}
	if _, ok := g.knownIDs["google-111"]; !ok {
		t.Error("knownIDs missing google-111")
	}
}

func TestSetKnownJobIDs_NilClears(t *testing.T) {
	g := NewRodGoogleDefault()
	g.SetKnownJobIDs(map[string]struct{}{"google-111": {}})
	g.SetKnownJobIDs(nil)
	if len(g.knownIDs) != 0 {
		t.Errorf("knownIDs should be empty after nil, got %d", len(g.knownIDs))
	}
}

// ── toRodItems ───────────────────────────────────────────────────────────────

func TestToRodItems_MapsFields(t *testing.T) {
	jobs := []jsJob{{
		ID:          "google-123",
		Title:       "Software Engineer",
		URL:         "https://careers.google.com/jobs/results/123",
		Location:    "Mountain View, CA, USA",
		Org:         "YouTube",
		Description: "Minimum qualifications:\n- 3 years experience",
	}}
	items := toRodItems(jobs)
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	item := items[0]
	if item.ID != "google-123" {
		t.Errorf("ID: got %q", item.ID)
	}
	if item.EmploymentType != "full_time" {
		t.Errorf("EmploymentType: got %q, want full_time", item.EmploymentType)
	}
	if item.FullContent == nil {
		t.Fatal("FullContent should not be nil")
	}
	if item.FullContent.Department != "YouTube" {
		t.Errorf("Department: got %q, want YouTube", item.FullContent.Department)
	}
	if item.FullContent.Description == "" {
		t.Error("Description should not be empty")
	}
}

func TestToRodItems_EmptySlice(t *testing.T) {
	if len(toRodItems(nil)) != 0 {
		t.Error("expected empty slice for nil input")
	}
}
