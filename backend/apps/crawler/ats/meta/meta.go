// Package meta is the Meta Careers adapter.
//
// v4 architecture: pure HTTP — no go-rod, no Chromium.
//
//   - Listing  : HTTP GET /jobsearch/sitemap.xml → ~895 numeric job IDs
//   - Detail   : HTTP GET /profile/job_details/{id} → schema.org JobPosting
//                JSON-LD embedded in <script type="application/ld+json">
//
// Meta v3 used go-rod to scroll Meta's React SPA listing because the
// /jobsearch HTML shell is empty until JavaScript bootstraps. v4 replaces
// that browser path with the public sitemap, which advertises every active
// job and is itself fetched over plain HTTP.
//
// The sitemap path is documented in https://www.metacareers.com/robots.txt
// under the `Sitemap:` directive. We treat it as a stable crawler contract.
package meta

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/trushilpatel/jobradar/apps/crawler/ats/types"
	"github.com/trushilpatel/jobradar/common/logger"
	companycommon "github.com/trushilpatel/jobradar/service/company/common"
	jobcommon "github.com/trushilpatel/jobradar/service/job/common"
)

// jobsearchURL is the human-facing careers search page. Returned by
// CareersURL — mostly used for display in the UI / Slack notifications.
const jobsearchURL = "https://www.metacareers.com/jobsearch"

// Config holds the Meta scraper settings sourced from the central config.
//
// All previous rod-related knobs (MinDelayMs, MaxDelayMs, MaxJobs) are gone.
// The sitemap is one HTTP call; per-detail enrichment is driven by the
// analyzer downstream and paced there.
type Config struct {
	TimeoutMs int // overall scan timeout in ms; 0 = no limit
}

// defaultConfig returns sensible production defaults.
func defaultConfig() Config {
	return Config{
		TimeoutMs: 600000, // 10 min — sitemap GET + thin-item publish loop
	}
}

// MetaAdapter implements types.Adapter (listing) + types.ContentFetcher
// (detail). Both paths are pure HTTP. See package comment for the v4
// design rationale.
type MetaAdapter struct {
	cfg           Config
	log           logger.Logger
	httpClient    *http.Client
	streamPublish func([]types.FetchJobsItem)
}

// New constructs a MetaAdapter with the given config.
func New(cfg Config, log logger.Logger) *MetaAdapter {
	return &MetaAdapter{
		cfg:        cfg,
		log:        log,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// NewDefault constructs a MetaAdapter with production defaults and a no-op
// logger. Convenient for tests and simple wiring.
func NewDefault() *MetaAdapter {
	return New(defaultConfig(), logger.NewNoop())
}

// CareersURL implements types.URLProvider — returns the public careers page.
func (m *MetaAdapter) CareersURL(_ string) string { return jobsearchURL }

// SetStreamPublish implements types.StreamPublisher.
func (m *MetaAdapter) SetStreamPublish(fn func([]types.FetchJobsItem)) {
	m.streamPublish = fn
}

// FetchJobs walks the sitemap and publishes one thin item per job ID.
// The analyzer downstream calls FetchJobContent for each item to enrich
// with the JSON-LD detail payload.
func (m *MetaAdapter) FetchJobs(ctx context.Context, _ string) (types.FetchJobsResponse, error) {
	if m.cfg.TimeoutMs > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(m.cfg.TimeoutMs)*time.Millisecond)
		defer cancel()
	}

	m.log.Info("meta/sitemap: fetching", logger.Str("url", sitemapURL))
	ids, err := m.fetchSitemap(ctx)
	if err != nil {
		return types.FetchJobsResponse{}, fmt.Errorf("meta: %w", err)
	}
	m.log.Info("meta/sitemap: parsed",
		logger.Int(logger.FieldFetched, len(ids)),
	)

	items := make([]types.FetchJobsItem, 0, len(ids))
	for _, numericID := range ids {
		items = append(items, types.FetchJobsItem{
			ID:             companycommon.AdapterMeta + "-" + numericID,
			URL:            detailURLForJobID(numericID),
			EmploymentType: jobcommon.EmployFullTime,
			// Title, Location, FullContent are populated by FetchJobContent
			// downstream — listing-time we only know the ID + URL.
		})
	}

	if m.streamPublish != nil && len(items) > 0 {
		m.streamPublish(items)
	}
	m.log.Info("meta/sitemap: published",
		logger.Int("committed", len(items)),
	)
	return types.FetchJobsResponse{Jobs: items}, nil
}

// FetchJobContent implements types.ContentFetcher.
//
// Pulls Meta's detail page over plain HTTP and parses the inline schema.org
// JobPosting JSON-LD block. Same code path used in v3 — extracted to a
// reusable helper here.
//
// Verified across 11+10 diverse jobs (engineering / PM / design / energy
// infra / spread across the sitemap): 100% coverage of title, description,
// responsibilities, qualifications, employmentType, datePosted, validThrough,
// hiringOrganization, jobLocation.
func (m *MetaAdapter) FetchJobContent(ctx context.Context, job types.RawJob) (types.FetchJobContentResponse, error) {
	url := job.URL
	if url == "" {
		url = detailURLForJobID(stripMetaPrefix(job.ID))
	}
	jp, err := m.fetchAndParseDetail(ctx, url)
	if err != nil {
		return types.FetchJobContentResponse{}, err
	}
	resp := jp.toFetchJobContentResponse(job.ID)
	resp.URL = url
	return resp, nil
}

// Compile-time interface checks.
var (
	_ types.Adapter         = (*MetaAdapter)(nil)
	_ types.URLProvider     = (*MetaAdapter)(nil)
	_ types.StreamPublisher = (*MetaAdapter)(nil)
	_ types.ContentFetcher  = (*MetaAdapter)(nil)
)
