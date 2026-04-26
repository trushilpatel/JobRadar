package meta

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/trushilpatel/jobradar/apps/crawler/ats/types"
	"github.com/trushilpatel/jobradar/common/logger"
)

// ── CareersURL ───────────────────────────────────────────────────────────────

func TestMetaAdapter_CareersURL(t *testing.T) {
	m := NewDefault()
	got := m.CareersURL("anything")
	if got != jobsearchURL {
		t.Errorf("CareersURL() = %q, want %q", got, jobsearchURL)
	}
}

// ── Constructor ──────────────────────────────────────────────────────────────

func TestNew_StoresConfig(t *testing.T) {
	cfg := Config{TimeoutMs: 60000}
	m := New(cfg, logger.NewNoop())
	if m.cfg.TimeoutMs != 60000 {
		t.Errorf("TimeoutMs: got %d, want 60000", m.cfg.TimeoutMs)
	}
	if m.httpClient == nil {
		t.Error("httpClient must be set")
	}
}

func TestNewDefault_UsesProductionDefaults(t *testing.T) {
	m := NewDefault()
	def := defaultConfig()
	if m.cfg.TimeoutMs != def.TimeoutMs {
		t.Errorf("TimeoutMs: got %d, want %d", m.cfg.TimeoutMs, def.TimeoutMs)
	}
}

// ── Interface compliance ─────────────────────────────────────────────────────

func TestMetaAdapter_ImplementsAdapter(t *testing.T) {
	var _ types.Adapter = (*MetaAdapter)(nil)
}

func TestMetaAdapter_ImplementsURLProvider(t *testing.T) {
	var _ types.URLProvider = (*MetaAdapter)(nil)
}

func TestMetaAdapter_ImplementsStreamPublisher(t *testing.T) {
	var _ types.StreamPublisher = (*MetaAdapter)(nil)
}

func TestMetaAdapter_ImplementsContentFetcher(t *testing.T) {
	var _ types.ContentFetcher = (*MetaAdapter)(nil)
}

// ── SetStreamPublish ─────────────────────────────────────────────────────────

func TestSetStreamPublish_StoresCallback(t *testing.T) {
	m := NewDefault()
	called := false
	m.SetStreamPublish(func(items []types.FetchJobsItem) { called = true })
	if m.streamPublish == nil {
		t.Fatal("streamPublish must be stored")
	}
	m.streamPublish(nil)
	if !called {
		t.Error("stored callback should be callable")
	}
}

func TestSetStreamPublish_NilClearsCallback(t *testing.T) {
	m := NewDefault()
	m.SetStreamPublish(func(_ []types.FetchJobsItem) {})
	m.SetStreamPublish(nil)
	if m.streamPublish != nil {
		t.Error("streamPublish should be nil after SetStreamPublish(nil)")
	}
}

// ── FetchJobs end-to-end against fake server ─────────────────────────────────

const fixtureSitemap = `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>https://www.metacareers.com/profile/job_details/100001</loc></url>
  <url><loc>https://www.metacareers.com/profile/job_details/100002</loc></url>
  <url><loc>https://www.metacareers.com/profile/job_details/100003</loc></url>
  <url><loc>https://www.metacareers.com/jobs/some-other-page</loc></url>
  <url><loc>https://www.metacareers.com/profile/job_details/100002</loc></url>
</urlset>`

func TestFetchJobs_PublishesOneItemPerSitemapID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/jobsearch/sitemap.xml" {
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(fixtureSitemap))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	// Point fetchSitemap at the test server. overrideSitemapURL is a
	// test-only seam declared in sitemap.go.
	originalURL := sitemapURL
	overrideSitemapURL(srv.URL + "/jobsearch/sitemap.xml")
	defer overrideSitemapURL(originalURL)

	m := NewDefault()
	var published [][]types.FetchJobsItem
	m.SetStreamPublish(func(items []types.FetchJobsItem) {
		published = append(published, items)
	})

	resp, err := m.FetchJobs(context.Background(), "")
	if err != nil {
		t.Fatalf("FetchJobs: %v", err)
	}
	if len(resp.Jobs) != 3 {
		t.Errorf("Jobs: got %d, want 3 (deduped sitemap)", len(resp.Jobs))
	}
	if len(published) != 1 {
		t.Errorf("streamPublish should be called once per scan, got %d", len(published))
	}
	for _, item := range resp.Jobs {
		if !strings.HasPrefix(item.ID, "meta-") {
			t.Errorf("ID must start with meta-: got %q", item.ID)
		}
		if item.EmploymentType != "full_time" {
			t.Errorf("EmploymentType: got %q, want full_time", item.EmploymentType)
		}
		if !strings.HasPrefix(item.URL, "https://www.metacareers.com/profile/job_details/") {
			t.Errorf("URL: got %q", item.URL)
		}
		if item.FullContent != nil {
			t.Errorf("FullContent must be nil at listing time (analyzer fills it via FetchJobContent)")
		}
	}
}

func TestFetchJobs_SurfacesSitemap404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	originalURL := sitemapURL
	overrideSitemapURL(srv.URL + "/jobsearch/sitemap.xml")
	defer overrideSitemapURL(originalURL)

	m := NewDefault()
	_, err := m.FetchJobs(context.Background(), "")
	if err == nil {
		t.Error("expected error on sitemap 404")
	}
}
