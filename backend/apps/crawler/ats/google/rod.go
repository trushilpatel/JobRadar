package google

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/go-rod/stealth"
	"github.com/trushilpatel/jobradar/apps/crawler/ats/types"
	"github.com/trushilpatel/jobradar/common/logger"
)

const careersURL = "https://www.google.com/about/careers/applications/jobs/results?" +
	"location=United%20States&target_level=EARLY&target_level=MID" +
	"&employment_type=FULL_TIME&q=Software%20Engineering"

// RodGoogle implements types.Adapter using go-rod instead of the Node.js subprocess.
type RodGoogle struct {
	cfg           Config
	log           logger.Logger
	knownIDs      map[string]struct{}        // set by engine via SetKnownJobIDs before FetchJobs
	streamPublish func([]types.FetchJobsItem) // set by engine via SetStreamPublish before FetchJobs
}

// NewRodGoogle creates a RodGoogle adapter with the given config and logger.
func NewRodGoogle(cfg Config, log logger.Logger) *RodGoogle {
	return &RodGoogle{cfg: cfg, log: log}
}

// NewRodGoogleDefault creates a RodGoogle adapter with default config and no-op logger.
func NewRodGoogleDefault() *RodGoogle {
	return &RodGoogle{cfg: defaultConfig(), log: logger.NewNoop()}
}

// SetStreamPublish implements types.StreamPublisher.
// The engine calls this before FetchJobs. After each listing page the adapter
// calls fn with the new thin jobs so they enter Kafka without waiting for the
// full scan to complete.
func (s *RodGoogle) SetStreamPublish(fn func([]types.FetchJobsItem)) {
	s.streamPublish = fn
}

// SetKnownJobIDs implements types.KnownJobsProvider.
// The engine calls this before FetchJobs so detail page visits are skipped for
// jobs already in the pipeline ledger — only new job IDs get enriched.
func (s *RodGoogle) SetKnownJobIDs(ids map[string]struct{}) {
	s.knownIDs = ids
}

// CareersURL returns the filtered Google Careers starting URL.
func (s *RodGoogle) CareersURL(_ string) string { return careersURL }

// jsJob mirrors the shape extracted by the in-page JS evaluation.
type jsJob struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	URL         string `json:"url"`
	Location    string `json:"location"`
	Org         string `json:"org"`
	Description string `json:"description"` // populated after detail page visit
}

// FetchJobs launches Chrome (headed, via rod + stealth), paginates through Google
// Careers listing pages, and returns all collected job listings.
// Detail page enrichment is not performed here — kept separate for validation.
func (s *RodGoogle) FetchJobs(ctx context.Context, _ string) (types.FetchJobsResponse, error) {
	start := time.Now()

	if s.cfg.TimeoutMs > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(s.cfg.TimeoutMs)*time.Millisecond)
		defer cancel()
	}

	browser, closeBrowser, err := s.launchBrowser()
	if err != nil {
		return types.FetchJobsResponse{}, fmt.Errorf("google/rod: %w", err)
	}
	defer closeBrowser()

	page, err := stealth.Page(browser)
	if err != nil {
		return types.FetchJobsResponse{}, fmt.Errorf("google/rod: stealth page: %w", err)
	}
	defer func() { _ = page.Close() }()

	s.applyFingerprint(page)

	s.log.Info("google/rod: navigating to careers page")
	if err := page.Navigate(careersURL); err != nil {
		return types.FetchJobsResponse{}, fmt.Errorf("google/rod: navigate: %w", err)
	}
	if err := page.WaitLoad(); err != nil {
		s.log.Warn("google/rod: WaitLoad timed out, continuing anyway")
	}
	if err := s.sleep(ctx, 5000, 8000); err != nil {
		return types.FetchJobsResponse{}, err
	}

	var allJobs []jsJob
	seenIDs := make(map[string]struct{})
	pageNum := 1

	for {
		select {
		case <-ctx.Done():
			s.log.Warn("google/rod: context cancelled during pagination")
			goto done
		default:
		}

		pageJobs, err := s.extractListingJobs(page)
		if err != nil {
			s.log.Warn("google/rod: extraction error", logger.Str("error", err.Error()))
		}

		newCount := 0
		for _, job := range pageJobs {
			if _, seen := seenIDs[job.ID]; !seen {
				seenIDs[job.ID] = struct{}{}
				allJobs = append(allJobs, job)
				newCount++
			}
		}
		// No publish here — jobs are published after detail enrichment so Kafka
		// always receives full content, never empty thin jobs.

		s.log.Info("google/rod: page scraped",
			logger.Int("page", pageNum),
			logger.Int("on_page", len(pageJobs)),
			logger.Int("new", newCount),
			logger.Int("total", len(allJobs)),
		)

		if s.cfg.MaxPages > 0 && pageNum >= s.cfg.MaxPages {
			s.log.Info("google/rod: max pages reached", logger.Int("max_pages", s.cfg.MaxPages))
			break
		}

		hasNext, err := s.hasNextPage(page)
		if err != nil || !hasNext {
			s.log.Info("google/rod: pagination complete")
			break
		}

		if err := s.sleep(ctx, s.cfg.MinDelayMs, s.cfg.MaxDelayMs); err != nil {
			break
		}

		if err := s.clickNextPage(page); err != nil {
			s.log.Warn("google/rod: next page click failed", logger.Str("error", err.Error()))
			break
		}
		if err := page.WaitLoad(); err != nil {
			s.log.Warn("google/rod: WaitLoad after next page timed out, continuing")
		}
		if err := s.sleep(ctx, 4000, 6000); err != nil {
			break
		}
		pageNum++
	}

done:
	s.log.Info("google/rod: listing complete",
		logger.Int(logger.FieldFetched, len(allJobs)),
		logger.Dur(logger.FieldDuration, time.Since(start)),
	)

	// ── Phase 2: visit detail pages ───────────────────────────────────────────
	enriched := 0
	detailLimit := len(allJobs)
	if s.cfg.MaxDetailJobs > 0 && s.cfg.MaxDetailJobs < detailLimit {
		detailLimit = s.cfg.MaxDetailJobs
	}

	for i := range allJobs {
		if i >= detailLimit {
			break
		}

		select {
		case <-ctx.Done():
			s.log.Warn("google/rod: context cancelled during enrichment")
			goto enrich_done
		default:
		}

		// EnrichMaxMs == 0 signals skip enrichment entirely
		if s.cfg.EnrichMaxMs == 0 {
			break
		}

		if i > 0 {
			if err := s.sleep(ctx, s.cfg.EnrichMinMs, s.cfg.EnrichMaxMs); err != nil {
				goto enrich_done
			}
		}

		// Skip detail visit for jobs already in the pipeline ledger — they were
		// enriched on a previous scan. Only new job IDs need a detail page visit.
		if _, known := s.knownIDs[allJobs[i].ID]; known {
			continue
		}

		s.log.Info("google/rod: fetching detail page",
			logger.Int("index", i+1),
			logger.Int("total", len(allJobs)),
			logger.Str(logger.FieldJobID, allJobs[i].ID),
		)

		desc, err := s.fetchDetailPage(ctx, page, allJobs[i])
		if err != nil {
			s.log.Warn("google/rod: detail page failed",
				logger.Str(logger.FieldJobID, allJobs[i].ID),
				logger.Str("error", err.Error()),
			)
			continue
		}
		allJobs[i].Description = desc
		enriched++

		// Publish this job to Kafka immediately after enrichment.
		// Full content (description + qualifications) is always present here.
		if s.streamPublish != nil {
			s.streamPublish(toRodItems([]jsJob{allJobs[i]}))
		}
	}

enrich_done:
	s.log.Info("google/rod: enrichment complete",
		logger.Int("enriched", enriched),
		logger.Int("total", len(allJobs)),
		logger.Dur(logger.FieldDuration, time.Since(start)),
	)

	return types.FetchJobsResponse{Jobs: toRodItems(allJobs)}, nil
}

// extractListingJobs runs the same DOM extraction logic as scrape.js inside rod's page.Eval.
// Returns all job cards visible on the current listing page.
func (s *RodGoogle) extractListingJobs(page *rod.Page) ([]jsJob, error) {
	res, err := page.Eval(`() => {
		const jobs = [];
		const links = document.querySelectorAll('a[aria-label^="Learn more about"]');
		for (const linkEl of links) {
			try {
				const href = linkEl.getAttribute('href') || '';
				const idMatch = href.match(/jobs\/results\/(\d+)/);
				if (!idMatch) continue;
				const jobId = idMatch[1];
				const pathname = href.startsWith('/')
					? href.split('?')[0]
					: '/about/careers/applications/' + href.split('?')[0];
				const absoluteUrl = 'https://www.google.com' + pathname;
				const card = linkEl.closest('li');
				if (!card) continue;
				const titleEl = card.querySelector('h3');
				const title = titleEl ? titleEl.textContent.trim() : '';
				function getTextByIcon(name) {
					for (const el of card.querySelectorAll('i, span')) {
						if (el.textContent.trim() === name) {
							const parent = el.parentElement;
							if (!parent) continue;
							const text = parent.textContent.trim().replace(name, '').trim();
							if (text) return text;
						}
					}
					return '';
				}
				jobs.push({
					id: 'google-' + jobId,
					title,
					url: absoluteUrl,
					location: getTextByIcon('place'),
					org: getTextByIcon('corporate_fare') || getTextByIcon('business'),
				});
			} catch(_) {}
		}
		return JSON.stringify(jobs);
	}`)
	if err != nil {
		return nil, fmt.Errorf("page eval: %w", err)
	}
	var jobs []jsJob
	if err := json.Unmarshal([]byte(res.Value.Str()), &jobs); err != nil {
		return nil, fmt.Errorf("parse: %w (raw: %.200s)", err, res.Value.Str())
	}
	return jobs, nil
}

func (s *RodGoogle) hasNextPage(page *rod.Page) (bool, error) {
	res, err := page.Eval(`() => !!document.querySelector('a[aria-label="Go to next page"]')`)
	if err != nil {
		return false, err
	}
	return res.Value.Bool(), nil
}

// fetchDetailPage navigates to a job's detail URL and extracts the full description.
// Uses the same page (no new tab) to keep the browser session consistent.
func (s *RodGoogle) fetchDetailPage(ctx context.Context, page *rod.Page, job jsJob) (string, error) {
	if err := page.Navigate(job.URL); err != nil {
		return "", fmt.Errorf("navigate to detail: %w", err)
	}
	if err := page.WaitLoad(); err != nil {
		s.log.Warn("google/rod: detail WaitLoad timed out, continuing")
	}
	// Short wait for JS sections to render
	if err := s.sleep(ctx, 2000, 3000); err != nil {
		return "", err
	}
	return s.extractJobDetail(page, job.Title, job.Location)
}

// extractJobDetail extracts the full job description from a detail page.
// Ports the same section-extraction logic from scrape.js: finds h3 headings by
// text match, then collects bullet lists and paragraphs until the next h3.
func (s *RodGoogle) extractJobDetail(page *rod.Page, title, location string) (string, error) {
	res, err := page.Eval(`(title, location) => {
		const lines = [title, '', location, ''];

		function extractSection(sectionName) {
			const h3s = document.querySelectorAll('h3');
			for (const h3 of h3s) {
				if (h3.textContent.trim().toLowerCase().includes(sectionName.toLowerCase())) {
					const parts = [h3.textContent.trim()];
					let el = h3.nextElementSibling;
					while (el && el.tagName !== 'H3') {
						if (el.tagName === 'UL' || el.tagName === 'OL') {
							el.querySelectorAll('li').forEach(li => {
								const text = li.textContent.trim();
								if (text) parts.push('- ' + text);
							});
						} else {
							const nested = el.querySelectorAll('ul, ol');
							if (nested.length > 0) {
								nested.forEach(list => {
									list.querySelectorAll('li').forEach(li => {
										const text = li.textContent.trim();
										if (text) parts.push('- ' + text);
									});
								});
							} else {
								const text = (el.innerText || el.textContent || '').trim();
								if (text) parts.push(text);
							}
						}
						el = el.nextElementSibling;
					}
					return parts.join('\n');
				}
			}
			return '';
		}

		for (const section of ['Minimum qualifications', 'Preferred qualifications', 'Responsibilities']) {
			const content = extractSection(section);
			if (content) lines.push(content, '');
		}

		return lines.join('\n').trim();
	}`, title, location)
	if err != nil {
		return "", fmt.Errorf("detail eval: %w", err)
	}
	return res.Value.Str(), nil
}

// applyFingerprint sets realistic browser headers and properties before navigation.
//
// go-rod/stealth patches JS-level signals (navigator.webdriver, plugins, canvas, etc).
// This covers the remaining layer: actual HTTP headers sent to Google's servers,
// CDP-level timezone/locale, and screen properties — all of which fingerprinting
// checks can observe independently of JS.
//
// All values target a macOS user on Chrome 121 in New York — consistent across layers.
func (s *RodGoogle) applyFingerprint(page *rod.Page) {
	// Sets the HTTP User-Agent header AND navigator.userAgent at the CDP level.
	// Must match what stealth reports so both layers are consistent.
	if err := page.SetUserAgent(&proto.NetworkSetUserAgentOverride{
		UserAgent:      "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.0.0 Safari/537.36",
		AcceptLanguage: "en-US,en;q=0.9",
		Platform:       "MacIntel",
	}); err != nil {
		s.log.Warn("google/rod: set user agent failed", logger.Str("error", err.Error()))
	}

	// Timezone — must be set before navigation so JS Date APIs report correctly.
	if err := (proto.EmulationSetTimezoneOverride{TimezoneID: "America/New_York"}).Call(page); err != nil {
		s.log.Warn("google/rod: set timezone failed", logger.Str("error", err.Error()))
	}

	// Locale — affects Intl APIs and Accept-Language header negotiation.
	if err := (proto.EmulationSetLocaleOverride{Locale: "en-US"}).Call(page); err != nil {
		s.log.Warn("google/rod: set locale failed", logger.Str("error", err.Error()))
	}

	// Viewport — 1366x768 matches a common laptop resolution.
	if err := page.SetViewport(&proto.EmulationSetDeviceMetricsOverride{
		Width: 1366, Height: 768, DeviceScaleFactor: 1,
	}); err != nil {
		s.log.Warn("google/rod: set viewport failed", logger.Str("error", err.Error()))
	}

	// Screen properties — JS can read screen.width/height independently of the viewport.
	// Set to a full 1920x1080 monitor so the viewport looks like a non-maximised window.
	if _, err := page.Eval(`() => {
		const def = (obj, prop, val) =>
			Object.defineProperty(obj, prop, { get: () => val, configurable: true });
		def(screen, 'width',       1920);
		def(screen, 'height',      1080);
		def(screen, 'availWidth',  1920);
		def(screen, 'availHeight', 1040);
		def(screen, 'colorDepth',  24);
		def(screen, 'pixelDepth',  24);
	}`); err != nil {
		s.log.Warn("google/rod: screen patch failed", logger.Str("error", err.Error()))
	}
}

func (s *RodGoogle) clickNextPage(page *rod.Page) error {
	el, err := page.Element(`a[aria-label="Go to next page"]`)
	if err != nil {
		return fmt.Errorf("find next page: %w", err)
	}
	return el.Click(proto.InputMouseButtonLeft, 1)
}

func (s *RodGoogle) launchBrowser() (*rod.Browser, func(), error) {
	// Headless in containers (no display), headed on desktops for better fingerprint.
	headless := os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == ""
	l := launcher.New().Headless(headless)

	// Chromium binary resolution — priority: config field → CHROMIUM_PATH env → OS PATH.
	// In Docker the Dockerfile bakes CHROMIUM_PATH=/usr/bin/chromium so the binary is
	// always found without any runtime download or glob search.
	switch {
	case s.cfg.ChromiumPath != "":
		l = l.Bin(s.cfg.ChromiumPath)
	case os.Getenv("CHROMIUM_PATH") != "":
		l = l.Bin(os.Getenv("CHROMIUM_PATH"))
	default:
		if path, found := launcher.LookPath(); found {
			l = l.Bin(path)
		}
	}

	// A persistent profile accumulates cookies and session state across runs.
	// In K8s, mount a PersistentVolumeClaim at ProfileDir to survive pod restarts.
	if s.cfg.ProfileDir != "" {
		l = l.UserDataDir(s.cfg.ProfileDir)
		s.log.Info("google/rod: using persistent chrome profile",
			logger.Str("profile_dir", s.cfg.ProfileDir),
			logger.Bool("headless", headless),
		)
	}

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

func (s *RodGoogle) sleep(ctx context.Context, minMs, maxMs int) error {
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

func toRodItems(jobs []jsJob) []types.FetchJobsItem {
	items := make([]types.FetchJobsItem, len(jobs))
	for i, j := range jobs {
		var fc *types.FullJobContent
		if j.Description != "" {
			// Full-detail job: description was fetched from the detail page.
			fc = &types.FullJobContent{
				Description: j.Description,
				Department:  j.Org,
			}
		}
		// Thin job (no description): leave FullContent nil so the analyzer
		// falls through to its HTTP fetch fallback instead of fast-pathing
		// with an empty description field.
		items[i] = types.FetchJobsItem{
			ID:             j.ID,
			Title:          j.Title,
			URL:            j.URL,
			Location:       j.Location,
			EmploymentType: "full_time",
			FullContent:    fc,
		}
	}
	return items
}

// Compile-time interface checks.
var (
	_ types.Adapter          = (*RodGoogle)(nil)
	_ types.URLProvider      = (*RodGoogle)(nil)
	_ types.KnownJobsProvider = (*RodGoogle)(nil)
	_ types.StreamPublisher   = (*RodGoogle)(nil)
)
