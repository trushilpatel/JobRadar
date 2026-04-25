package meta

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
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

// RodMeta implements types.Adapter using go-rod for in-process browser automation.
// It replaces the Node.js/Playwright subprocess. WithBrowser allows sharing a
// Chromium process if other rod-based adapters are added later (Google has
// since moved to a pure-HTTP adapter — see ats/google/http.go).
type RodMeta struct {
	cfg           Config
	browser       *rod.Browser              // shared browser; nil = launch own per scan
	streamPublish func([]types.FetchJobsItem) // set by engine via SetStreamPublish before FetchJobs
	log           logger.Logger
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
	return &RodMeta{cfg: cfg, log: log}
}

func NewRodMetaDefault() *RodMeta {
	return &RodMeta{cfg: defaultConfig(), log: logger.NewNoop()}
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

// FetchJobs opens two tabs: one holds the listing page while the other visits
// detail pages. Each job is streamed to Kafka immediately after enrichment so
// jobs enter the pipeline without waiting for the full scan to finish.
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

	// Listing tab — stays on metacareers.com/jobsearch throughout scrolling so
	// scroll position is never lost when navigating to detail pages.
	listingPage, err := stealth.Page(browser)
	if err != nil {
		return types.FetchJobsResponse{}, fmt.Errorf("meta/rod: stealth listing page: %w", err)
	}
	defer func() { _ = listingPage.Close() }()
	m.applyFingerprint(listingPage)

	// Detail tab — navigates to each job URL without disturbing the listing tab.
	detailPage, err := stealth.Page(browser)
	if err != nil {
		return types.FetchJobsResponse{}, fmt.Errorf("meta/rod: stealth detail page: %w", err)
	}
	defer func() { _ = detailPage.Close() }()
	m.applyFingerprint(detailPage)

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

	var published []types.FetchJobsItem

	for i := range allJobs {
		select {
		case <-ctx.Done():
			m.log.Warn("meta/rod: context cancelled during enrichment")
			goto done
		default:
		}

		if i > 0 {
			if err := m.sleep(ctx, m.cfg.MinDelayMs, m.cfg.MaxDelayMs); err != nil {
				goto done
			}
		}

		m.log.Info("meta/rod: enriching detail page",
			logger.Int("index", i+1),
			logger.Int("total", len(allJobs)),
			logger.Str(logger.FieldJobID, allJobs[i].ID),
		)

		desc, err := m.fetchDetailPage(ctx, detailPage, allJobs[i])
		if err != nil {
			m.log.Warn("meta/rod: detail page failed",
				logger.Str(logger.FieldJobID, allJobs[i].ID),
				logger.Str("error", err.Error()),
			)
			continue
		}
		allJobs[i].Description = desc

		items := toMetaItems([]metaJob{allJobs[i]})
		if m.streamPublish != nil {
			m.streamPublish(items) // publish to Kafka + ledger immediately
		}
		published = append(published, items...)
	}

done:
	m.log.Info("meta/rod: enrichment complete",
		logger.Int("enriched", len(published)),
		logger.Int("total", len(allJobs)),
	)
	return types.FetchJobsResponse{Jobs: published}, nil
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

// fetchDetailPage navigates to a job's URL and extracts its full description.
func (m *RodMeta) fetchDetailPage(ctx context.Context, page *rod.Page, job metaJob) (string, error) {
	if err := page.Navigate(job.URL); err != nil {
		return "", fmt.Errorf("navigate: %w", err)
	}
	if err := page.WaitLoad(); err != nil {
		m.log.Warn("meta/rod: detail WaitLoad timed out, continuing")
	}
	if err := m.sleep(ctx, 1500, 2500); err != nil {
		return "", err
	}
	return m.extractJobDetail(page, job.Title, job.Location)
}

// extractJobDetail extracts full job content from the current detail page.
// Ported from scrape.js extractJobDetail.
func (m *RodMeta) extractJobDetail(page *rod.Page, title, location string) (string, error) {
	res, err := page.Eval(`(title, location) => {
		const lines = [title, '', location, ''];
		const main = document.querySelector('[role="main"]') ||
		             document.querySelector('main') || document.body;
		const text = (main.innerText || main.textContent || '').trim();
		lines.push(text.substring(0, 5000));
		return lines.join('\n').trim();
	}`, title, location)
	if err != nil {
		return "", fmt.Errorf("detail eval: %w", err)
	}
	return res.Value.Str(), nil
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

// toMetaItems converts raw job data to pipeline types.
// FullContent is set only when Description is populated; nil means the detail
// page wasn't visited (context cancelled) and the analyzer will HTTP-fetch it.
func toMetaItems(jobs []metaJob) []types.FetchJobsItem {
	items := make([]types.FetchJobsItem, len(jobs))
	for i, j := range jobs {
		var fc *types.FullJobContent
		if j.Description != "" {
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

func countEnriched(jobs []metaJob) int {
	n := 0
	for _, j := range jobs {
		if j.Description != "" {
			n++
		}
	}
	return n
}

// Compile-time interface checks.
var (
	_ types.Adapter         = (*RodMeta)(nil)
	_ types.URLProvider     = (*RodMeta)(nil)
	_ types.StreamPublisher = (*RodMeta)(nil)
)
