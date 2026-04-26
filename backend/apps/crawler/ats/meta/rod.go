package meta

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/go-rod/stealth"
	"github.com/trushilpatel/jobradar/apps/crawler/ats/types"
	"github.com/trushilpatel/jobradar/common/logger"
	jobcommon "github.com/trushilpatel/jobradar/service/job/common"
)

const metaCareersURL = "https://www.metacareers.com/jobsearch"

// Config holds Meta scraper settings sourced from the central config.
type Config struct {
	MinDelayMs int // min delay between page navigations in ms
	MaxDelayMs int // max delay between page navigations in ms
	TimeoutMs  int // overall scan timeout in ms; 0 = no limit
	MaxJobs    int // max jobs to fetch; 0 = all
}

func defaultConfig() Config {
	return Config{
		MinDelayMs: 3000,
		MaxDelayMs: 8000,
		TimeoutMs:  3600000, // 1 hour
		MaxJobs:    0,
	}
}

// RodMeta implements types.Adapter (listing) + types.ContentFetcher (detail).
//
// Hybrid design: Meta's listing page is fully client-rendered (React +
// GraphQL with rotating CSRF tokens), so we still drive a headless browser
// to load it. But every detail page embeds a complete schema.org JobPosting
// JSON-LD payload, so detail enrichment runs over plain HTTP — no second
// browser tab required. See ats/meta/http_detail.go for the JSON-LD parser.
//
// WithBrowser allows sharing a Chromium process if other rod-based adapters
// are added later (Google has since moved to a pure-HTTP adapter — see
// ats/google/http.go).
type RodMeta struct {
	cfg           Config
	browser       *rod.Browser                // shared browser; nil = launch own per scan
	streamPublish func([]types.FetchJobsItem) // set by engine via SetStreamPublish before FetchJobs
	log           logger.Logger
	httpClient    *http.Client // used by FetchJobContent for JSON-LD detail pages
}

// metaJob holds raw data extracted from Meta's career pages via rod page.Eval.
// JSON tags match the keys returned by the browser JS.
type metaJob struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	URL         string `json:"url"`
	Location    string `json:"location"`
	Team        string `json:"team"`
	Description string `json:"description"`
}

// unmarshalMetaJob decodes a JSON object into a metaJob. Exposed so tests can
// verify the struct's JSON tags match the browser output format.
func unmarshalMetaJob(data []byte, dst *metaJob) error {
	return json.Unmarshal(data, dst)
}

func NewRodMeta(cfg Config, log logger.Logger) *RodMeta {
	return &RodMeta{
		cfg:        cfg,
		log:        log,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func NewRodMetaDefault() *RodMeta {
	return &RodMeta{
		cfg:        defaultConfig(),
		log:        logger.NewNoop(),
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// WithBrowser sets a shared browser so FetchJobs opens a new tab instead of
// launching a fresh Chromium process. Useful if multiple rod-based adapters
// run in the same crawler binary. Returns the receiver for chaining.
func (m *RodMeta) WithBrowser(b *rod.Browser) *RodMeta {
	m.browser = b
	return m
}

// SetStreamPublish implements types.StreamPublisher.
// The engine calls this before FetchJobs. After each detail page is scraped
// the adapter calls fn immediately — jobs enter Kafka without waiting for the
// full scan to finish.
func (m *RodMeta) SetStreamPublish(fn func([]types.FetchJobsItem)) {
	m.streamPublish = fn
}

// CareersURL returns the Meta Careers jobs search page URL.
func (m *RodMeta) CareersURL(_ string) string { return metaCareersURL }

// FetchJobs scrolls Meta's careers listing in a single rod tab, extracts
// every job card, and returns them as thin items (no FullContent). Detail
// enrichment is delegated to FetchJobContent (HTTP + JSON-LD), which the
// analyzer calls on each thin item downstream.
//
// Single-tab design: the original two-tab arrangement existed because the
// rod adapter visited each job's detail page in a second tab. That work
// now lives in FetchJobContent, so we save a tab + ~all per-job rod time.
//
// Implements types.Adapter.
func (m *RodMeta) FetchJobs(ctx context.Context, _ string) (types.FetchJobsResponse, error) {
	if m.cfg.TimeoutMs > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(m.cfg.TimeoutMs)*time.Millisecond)
		defer cancel()
	}

	browser, releaseBrowser, err := m.acquireBrowser()
	if err != nil {
		return types.FetchJobsResponse{}, fmt.Errorf("meta/rod: %w", err)
	}
	defer releaseBrowser()

	listingPage, err := stealth.Page(browser)
	if err != nil {
		return types.FetchJobsResponse{}, fmt.Errorf("meta/rod: stealth listing page: %w", err)
	}
	defer func() { _ = listingPage.Close() }()
	m.applyFingerprint(listingPage)

	m.log.Info("meta/rod: navigating to careers page")
	if err := listingPage.Navigate(metaCareersURL); err != nil {
		return types.FetchJobsResponse{}, fmt.Errorf("meta/rod: navigate: %w", err)
	}
	if err := listingPage.WaitLoad(); err != nil {
		m.log.Warn("meta/rod: WaitLoad timed out, continuing")
	}
	if err := m.sleep(ctx, 3000, 5000); err != nil {
		return types.FetchJobsResponse{}, err
	}

	allJobs, err := m.scrollToLoadAllJobs(ctx, listingPage, m.cfg.MaxJobs)
	if err != nil {
		return types.FetchJobsResponse{}, fmt.Errorf("meta/rod: scroll: %w", err)
	}

	if len(allJobs) == 0 {
		m.log.Warn("meta/rod: no jobs found — page structure may have changed")
		return types.FetchJobsResponse{}, nil
	}

	m.log.Info("meta/rod: listing complete", logger.Int(logger.FieldFetched, len(allJobs)))

	// Publish all jobs as thin items. Description is intentionally left empty
	// so toMetaItems leaves FullContent nil; the analyzer sees nil and calls
	// our FetchJobContent (HTTP + JSON-LD) downstream.
	items := toMetaItems(allJobs)
	if m.streamPublish != nil {
		m.streamPublish(items)
	}
	return types.FetchJobsResponse{Jobs: items}, nil
}

// FetchJobContent implements types.ContentFetcher.
//
// Pulls Meta's detail page over plain HTTP and parses the inline schema.org
// JobPosting JSON-LD block. Replaces the previous rod-based detail fetch —
// no browser, no DOM scraping, ~10x faster per job.
//
// Verified across 11 diverse jobs (engineering / PM / design / energy infra):
// 100% coverage of title, description, responsibilities, qualifications,
// employmentType, datePosted, validThrough, hiringOrganization, jobLocation.
func (m *RodMeta) FetchJobContent(ctx context.Context, job types.RawJob) (types.FetchJobContentResponse, error) {
	url := job.URL
	if url == "" {
		// Reconstruct from job.ID when the listing didn't carry the URL
		// (every Meta job ID maps to the canonical detail path).
		url = detailURLForJobID(stripMetaPrefix(job.ID))
	}
	jp, err := m.fetchAndParseDetail(ctx, url)
	if err != nil {
		return types.FetchJobContentResponse{}, fmt.Errorf("meta/http: %w", err)
	}
	resp := jp.toFetchJobContentResponse(job.ID)
	resp.URL = url
	return resp, nil
}

// acquireBrowser returns the shared browser when set, otherwise launches a fresh one.
// The returned releaseFn closes the browser for standalone launches; is a no-op for shared.
func (m *RodMeta) acquireBrowser() (*rod.Browser, func(), error) {
	if m.browser != nil {
		return m.browser, func() {}, nil
	}
	l := launcher.New().
		Headless(true).
		Set("no-sandbox", "").
		Set("disable-setuid-sandbox", "").
		Set("disable-dev-shm-usage", "")
	controlURL, err := l.Launch()
	if err != nil {
		return nil, nil, fmt.Errorf("launch browser: %w", err)
	}
	browser := rod.New().ControlURL(controlURL)
	if err := browser.Connect(); err != nil {
		return nil, nil, fmt.Errorf("connect to browser: %w", err)
	}
	return browser, func() { _ = browser.Close() }, nil
}

// scrollToLoadAllJobs scrolls until no new job cards appear (infinite scroll)
// or maxJobs is reached.
func (m *RodMeta) scrollToLoadAllJobs(ctx context.Context, page *rod.Page, maxJobs int) ([]metaJob, error) {
	prev := 0
	stale := 0
	const maxStale = 5

	for scrolls := 0; scrolls < 100; scrolls++ {
		select {
		case <-ctx.Done():
			jobs, _ := m.extractJobsFromPage(page)
			return jobs, nil
		default:
		}

		jobs, err := m.extractJobsFromPage(page)
		if err != nil {
			m.log.Warn("meta/rod: extraction error during scroll", logger.Str("error", err.Error()))
		}

		m.log.Info("meta/rod: scroll check",
			logger.Int("scroll", scrolls),
			logger.Int("jobs", len(jobs)),
		)

		if maxJobs > 0 && len(jobs) >= maxJobs {
			if len(jobs) > maxJobs {
				jobs = jobs[:maxJobs]
			}
			return jobs, nil
		}

		if len(jobs) == prev {
			stale++
			if stale >= maxStale {
				return jobs, nil
			}
		} else {
			stale = 0
			prev = len(jobs)
		}

		if _, err := page.Eval(`() => window.scrollBy(0, 600)`); err != nil {
			m.log.Warn("meta/rod: scroll eval failed", logger.Str("error", err.Error()))
		}
		if err := m.sleep(ctx, 1000, 2000); err != nil {
			jobs, _ := m.extractJobsFromPage(page)
			return jobs, nil
		}
	}

	return m.extractJobsFromPage(page)
}

// extractJobsFromPage runs DOM extraction on the current listing page and
// returns all job cards found. Ported from scrape.js extractJobsFromPage.
func (m *RodMeta) extractJobsFromPage(page *rod.Page) ([]metaJob, error) {
	res, err := page.Eval(`() => {
		const jobs = [];
		const seenIDs = new Set();
		const links = document.querySelectorAll('a[href*="/profile/job_details/"]');
		for (const link of links) {
			try {
				const href = link.getAttribute('href') || '';
				const idMatch = href.match(/\/profile\/job_details\/(\d+)/);
				if (!idMatch) continue;
				const jobID = idMatch[1];
				if (seenIDs.has(jobID)) continue;
				seenIDs.add(jobID);
				const absoluteURL = href.startsWith('http')
					? href
					: 'https://www.metacareers.com' + (href.startsWith('/') ? href : '/' + href);
				const lines = (link.innerText || link.textContent || '')
					.split('\n').map(l => l.trim()).filter(l => l && l !== '⋅');
				if (!lines.length) continue;
				const title = lines[0];
				let location = '';
				for (let i = 1; i < Math.min(lines.length, 6); i++) {
					const l = lines[i];
					if (l.includes(',') || l === 'Remote' || l === 'Hybrid' ||
						l === 'Multiple Locations' || /\+\d+ locations?/.test(l)) {
						location = l; break;
					}
				}
				let team = '';
				for (let i = 1; i < Math.min(lines.length, 6); i++) {
					const l = lines[i];
					if (l !== location && !l.includes(',') && l !== 'Remote' &&
						l !== 'Hybrid' && l !== 'Multiple Locations' && !/\+\d+ locations?/.test(l)) {
						team = l; break;
					}
				}
				jobs.push({ id: 'meta-' + jobID, title: title || 'Unknown Position',
					url: absoluteURL, location: location || '', team: team || 'Meta' });
			} catch(_) {}
		}
		return JSON.stringify(jobs);
	}`)
	if err != nil {
		return nil, fmt.Errorf("page eval: %w", err)
	}
	var jobs []metaJob
	if err := json.Unmarshal([]byte(res.Value.Str()), &jobs); err != nil {
		return nil, fmt.Errorf("parse jobs: %w (raw: %.200s)", err, res.Value.Str())
	}
	return jobs, nil
}

func (m *RodMeta) applyFingerprint(page *rod.Page) {
	if err := page.SetUserAgent(&proto.NetworkSetUserAgentOverride{
		UserAgent:      "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.0.0 Safari/537.36",
		AcceptLanguage: "en-US,en;q=0.9",
		Platform:       "MacIntel",
	}); err != nil {
		m.log.Warn("meta/rod: set user agent failed", logger.Str("error", err.Error()))
	}
	if err := (proto.EmulationSetTimezoneOverride{TimezoneID: "America/New_York"}).Call(page); err != nil {
		m.log.Warn("meta/rod: set timezone failed", logger.Str("error", err.Error()))
	}
	if err := (proto.EmulationSetLocaleOverride{Locale: "en-US"}).Call(page); err != nil {
		m.log.Warn("meta/rod: set locale failed", logger.Str("error", err.Error()))
	}
	if err := page.SetViewport(&proto.EmulationSetDeviceMetricsOverride{
		Width: 1366, Height: 768, DeviceScaleFactor: 1,
	}); err != nil {
		m.log.Warn("meta/rod: set viewport failed", logger.Str("error", err.Error()))
	}
}

func (m *RodMeta) sleep(ctx context.Context, minMs, maxMs int) error {
	if minMs >= maxMs {
		maxMs = minMs + 1
	}
	d := time.Duration(minMs+rand.Intn(maxMs-minMs)) * time.Millisecond
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

// toMetaItems converts raw listing-card data to pipeline types. FullContent
// is left nil — the analyzer downstream calls FetchJobContent (HTTP +
// JSON-LD) to enrich each job. This matches the Greenhouse/Lever/Ashby
// pattern: thin listing items, structured detail fetch on demand.
//
// Description / Team are still accepted on the metaJob struct for backward
// compatibility with the in-package extractor, but FetchJobs no longer
// populates them.
func toMetaItems(jobs []metaJob) []types.FetchJobsItem {
	items := make([]types.FetchJobsItem, len(jobs))
	for i, j := range jobs {
		var fc *types.FullJobContent
		if j.Description != "" {
			// Belt-and-braces: if a future caller pre-populates Description
			// (e.g. a fixture or a one-off backfill), still respect it.
			fc = &types.FullJobContent{
				Description: j.Description,
				Department:  j.Team,
			}
		}
		items[i] = types.FetchJobsItem{
			ID:             j.ID,
			Title:          j.Title,
			URL:            j.URL,
			Location:       j.Location,
			EmploymentType: jobcommon.EmployFullTime,
			FullContent:    fc,
		}
	}
	return items
}

// Compile-time interface checks.
var (
	_ types.Adapter         = (*RodMeta)(nil)
	_ types.URLProvider     = (*RodMeta)(nil)
	_ types.StreamPublisher = (*RodMeta)(nil)
	_ types.ContentFetcher  = (*RodMeta)(nil)
)
