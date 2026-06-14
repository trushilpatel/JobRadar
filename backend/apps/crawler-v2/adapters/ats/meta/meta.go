// Package meta is the Meta Careers adapter for the crawler-v2 module.
//
// Pure HTTP — no go-rod, no Chromium, no JavaScript execution.
//
//   - Listing  : GET https://www.metacareers.com/jobsearch/sitemap.xml
//     → parse <loc> tags → numeric job IDs → thin JobSummary list
//   - Detail   : GET https://www.metacareers.com/profile/job_details/{id}
//     → parse <script type="application/ld+json"> → schema.org JobPosting
//
// The sitemap is the official crawler contract: advertised in robots.txt under
// the Sitemap: directive. Stable URL, public XML, no auth, no JavaScript.
package meta

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/trushilpatel/jobradar/apps/crawler-v2/common/logger"
	"github.com/trushilpatel/jobradar/common/ats/core"
)

// sitemapURL is the public sitemap of every active Meta job posting.
// Declared as var (not const) so tests can override via the test seam.
var sitemapURL = "https://www.metacareers.com/jobsearch/sitemap.xml"

// detailURLPrefix is the canonical detail-page URL prefix for a Meta job.
const detailURLPrefix = "https://www.metacareers.com/profile/job_details/"

// userAgent — pinned Chrome 121 on macOS. Meta validates the request header
// set; a bare urllib User-Agent gets a 400.
const userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) " +
	"AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.0.0 Safari/537.36"

const acceptHeader = "text/html,application/xhtml+xml,application/xml;q=0.9," +
	"image/webp,*/*;q=0.8"
const acceptLangHeader = "en-US,en;q=0.9"

// Meta implements core.ListingFetcher and core.PostFetcher.
//
// companyID is ignored for listing — Meta's sitemap is company-global.
type Meta struct {
	baseURL    string // overridden in tests; empty means use production URLs
	log        logger.Logger
	httpClient *http.Client
}

// New constructs a Meta adapter with the given logger and HTTP client.
func New(log logger.Logger, httpClient *http.Client) *Meta {
	return &Meta{
		log:        log.With(logger.Str(logger.FieldAdapter, "meta")),
		httpClient: httpClient,
	}
}

// ── ListingFetcher ────────────────────────────────────────────────────────────

// FetchJobListing fetches the Meta careers sitemap and returns one thin
// JobSummary per numeric job ID. Title and Location are empty — not available
// at sitemap time; callers must use FetchJobPost to enrich them.
func (m *Meta) FetchJobListing(ctx context.Context, _ string) (core.JobListing, error) {
	log := m.log.WithContext(ctx)
	start := time.Now()

	targetURL := m.resolveSitemapURL()
	log.Debug("fetching sitemap", logger.Str(logger.FieldURL, targetURL))

	body, err := m.fetchBytes(ctx, targetURL)
	if err != nil {
		log.Error("sitemap fetch failed", err, logger.Str(logger.FieldURL, targetURL))
		return core.JobListing{}, fmt.Errorf("meta: listing: %w", err)
	}

	ids := parseSitemap(body)
	log.Info("sitemap parsed",
		logger.Int(logger.FieldJobCount, len(ids)),
		logger.Dur(logger.FieldDuration, time.Since(start)),
	)

	jobs := make([]core.JobSummary, 0, len(ids))
	for _, id := range ids {
		jobs = append(jobs, mapJobSummary(id))
	}
	return core.JobListing{Jobs: jobs}, nil
}

// ── PostFetcher ───────────────────────────────────────────────────────────────

// FetchJobPost fetches the detail page for the given job summary and returns
// a fully-populated core.JobPost. summary.ID is passed through unchanged.
// If summary.URL is empty the canonical URL is constructed from the ID.
func (m *Meta) FetchJobPost(ctx context.Context, summary core.JobSummary) (core.JobPost, error) {
	log := m.log.WithContext(ctx).With(logger.Str(logger.FieldJobID, summary.ID))
	start := time.Now()

	targetURL := m.resolveDetailURL(summary)
	log.Debug("fetching job post", logger.Str(logger.FieldURL, targetURL))

	body, err := m.fetchBytes(ctx, targetURL)
	if err != nil {
		log.Error("post fetch failed", err, logger.Str(logger.FieldURL, targetURL))
		return core.JobPost{}, fmt.Errorf("meta: post: %w", err)
	}

	jp, err := extractJobPosting(body)
	if err != nil {
		log.Error("JSON-LD extraction failed", err)
		return core.JobPost{}, fmt.Errorf("meta: post: %w", err)
	}

	post := mapJobPost(jp, summary.ID)
	log.Info("job post fetched", logger.Dur(logger.FieldDuration, time.Since(start)))
	return post, nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// fetchBytes performs a GET request with the full browser header set Meta
// requires and returns the response body. Non-200 responses return an error.
func (m *Meta) fetchBytes(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", acceptHeader)
	req.Header.Set("Accept-Language", acceptLangHeader)
	// Meta's CDN validates Sec-Fetch/sec-ch-ua headers; requests without them
	// (Chrome UA + basic headers only) may get 400.
	req.Header.Set("sec-ch-ua", `"Chromium";v="121", "Not A(Brand";v="99", "Google Chrome";v="121"`)
	req.Header.Set("sec-ch-ua-mobile", "?0")
	req.Header.Set("sec-ch-ua-platform", `"macOS"`)
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Sec-Fetch-User", "?1")
	req.Header.Set("Upgrade-Insecure-Requests", "1")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	return body, nil
}

// resolveSitemapURL returns the test-overridden URL or the production default.
func (m *Meta) resolveSitemapURL() string {
	if m.baseURL != "" {
		return m.baseURL + "/jobsearch/sitemap.xml"
	}
	return sitemapURL
}

// resolveDetailURL returns the detail URL for the summary. Falls back to
// constructing it from the ID when URL is empty.
func (m *Meta) resolveDetailURL(summary core.JobSummary) string {
	if summary.URL != "" {
		return summary.URL
	}
	// Strip "meta-" prefix to get the numeric ID.
	id := summary.ID
	const prefix = "meta-"
	if len(id) > len(prefix) && id[:len(prefix)] == prefix {
		id = id[len(prefix):]
	}
	return detailURLPrefix + id
}

// ── Interface checks ──────────────────────────────────────────────────────────

var (
	_ core.ListingFetcher = (*Meta)(nil)
	_ core.PostFetcher    = (*Meta)(nil)
)
