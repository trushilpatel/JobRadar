package google

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"strings"
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
	browser       *rod.Browser                                                    // shared browser; nil = launch own per scan
	streamPublish func([]types.FetchJobsItem)                                     // set by engine via SetStreamPublish before FetchJobs
	seenChecker   func(ctx context.Context, ids []string) (map[string]bool, error) // set by engine via SetSeenChecker before FetchJobs
}

// NewRodGoogle creates a RodGoogle adapter with the given config and logger.
func NewRodGoogle(cfg Config, log logger.Logger) *RodGoogle {
	return &RodGoogle{cfg: cfg, log: log}
}

// NewRodGoogleDefault creates a RodGoogle adapter with default config and no-op logger.
func NewRodGoogleDefault() *RodGoogle {
	return &RodGoogle{cfg: defaultConfig(), log: logger.NewNoop()}
}

// WithBrowser sets a shared browser so FetchJobs opens a new tab instead of
// launching a fresh Chromium process. Pass the same *rod.Browser used by
// RodMeta so both adapters share one process. Returns the receiver for chaining.
func (s *RodGoogle) WithBrowser(b *rod.Browser) *RodGoogle {
	s.browser = b
	return s
}

// SetStreamPublish implements types.StreamPublisher.
// The engine calls this before FetchJobs. After each listing page the adapter
// calls fn with the new thin jobs so they enter Kafka without waiting for the
// full scan to complete.
func (s *RodGoogle) SetStreamPublish(fn func([]types.FetchJobsItem)) {
	s.streamPublish = fn
}

// SetSeenChecker implements types.SeenChecker.
// The engine injects HasSeenBatch so the adapter can do one live DB call per
// listing page to find which job IDs are already in the pipeline ledger.
func (s *RodGoogle) SetSeenChecker(fn func(ctx context.Context, ids []string) (map[string]bool, error)) {
	s.seenChecker = fn
}

// filterPageJobs returns only jobs that are not already seen (per seenMap) and
// not already processed earlier in this scan (per localSeen). It also updates
// localSeen with every ID it encounters so subsequent pages stay deduplicated.
func filterPageJobs(pageJobs []jsJob, seenMap map[string]bool, localSeen map[string]struct{}) []jsJob {
	var result []jsJob
	for _, job := range pageJobs {
		if _, local := localSeen[job.ID]; local {
			continue // already processed on a prior page this scan
		}
		localSeen[job.ID] = struct{}{}
		if seenMap[job.ID] {
			continue // already in pipeline ledger
		}
		result = append(result, job)
	}
	return result
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

// FetchJobs uses a single browser page throughout. Google Careers is a SPA —
// clicking a job card opens the detail panel inline on the right while the
// listing and pagination remain intact on the left.
//
// Per-page loop:
//  1. Extract job IDs from the current listing page.
//  2. Filter to only jobs not already in the pipeline ledger (knownIDs).
//  3. Click each new job card → detail panel renders → extract → publish to Kafka.
//  4. Wait the inter-page delay, then click next page.
func (s *RodGoogle) FetchJobs(ctx context.Context, _ string) (types.FetchJobsResponse, error) {
	start := time.Now()

	if s.cfg.TimeoutMs > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(s.cfg.TimeoutMs)*time.Millisecond)
		defer cancel()
	}

	var browser *rod.Browser
	var closeBrowser func()
	if s.browser != nil {
		browser, closeBrowser = s.browser, func() {}
	} else {
		var err error
		browser, closeBrowser, err = s.launchBrowser()
		if err != nil {
			return types.FetchJobsResponse{}, fmt.Errorf("google/rod: %w", err)
		}
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

	seenIDs := make(map[string]struct{})
	var allPublished []types.FetchJobsItem
	pageNum := 1
	totalCommitted := 0

	for {
		if ctx.Err() != nil {
			s.log.Warn("google/rod: context cancelled")
			break
		}

		s.log.Info("google/rod: scanning page", logger.Int("page", pageNum))

		// ── Step 1: extract job IDs from current listing page ─────────────────
		pageJobs, err := s.extractListingJobs(page)
		if err != nil {
			s.log.Warn("google/rod: extraction error", logger.Str("error", err.Error()))
		}

		// ── Step 2: one batch DB call — filter to only new, unvisited jobs ─────
		ids := make([]string, len(pageJobs))
		for i, j := range pageJobs {
			ids[i] = j.ID
		}
		seenMap, err := s.seenChecker(ctx, ids)
		if err != nil {
			s.log.Warn("google/rod: seen check failed, skipping page", logger.Str("error", err.Error()))
			continue
		}
		newJobs := filterPageJobs(pageJobs, seenMap, seenIDs)

		s.log.Info("google/rod: page scanned",
			logger.Int("page", pageNum),
			logger.Int(logger.FieldFetched, len(pageJobs)),
			logger.Int(logger.FieldNew, len(newJobs)),
			logger.Int("skipped", len(pageJobs)-len(newJobs)),
		)

		// ── Step 3: click each new job card, extract detail panel, publish ─────
		committed := 0
		detailLimit := len(newJobs)
		if s.cfg.MaxDetailJobs > 0 && s.cfg.MaxDetailJobs < detailLimit {
			detailLimit = s.cfg.MaxDetailJobs
		}

		for i, job := range newJobs {
			if i >= detailLimit || s.cfg.EnrichMaxMs == 0 {
				break
			}
			if ctx.Err() != nil {
				s.log.Warn("google/rod: context cancelled during enrichment")
				goto done
			}
			if i > 0 {
				if err := s.sleep(ctx, s.cfg.EnrichMinMs, s.cfg.EnrichMaxMs); err != nil {
					goto done
				}
			}

			desc, err := s.clickAndExtractPanel(ctx, page, job)
			if err != nil {
				s.log.Warn("google/rod: panel extraction failed",
					logger.Str(logger.FieldJobID, job.ID),
					logger.Str("error", err.Error()),
				)
				continue
			}
			newJobs[i].Description = desc

			item := toRodItems([]jsJob{newJobs[i]})
			if s.streamPublish != nil {
				s.streamPublish(item) // commit to Kafka + DB immediately
			}
			allPublished = append(allPublished, item...)
			committed++
		}

		totalCommitted += committed
		s.log.Info("google/rod: page committed",
			logger.Int("page", pageNum),
			logger.Int("committed", committed),
			logger.Int("total_committed", totalCommitted),
			logger.Dur(logger.FieldDuration, time.Since(start)),
		)

		// ── Step 4: check for next page ────────────────────────────────────────
		if s.cfg.MaxPages > 0 && pageNum >= s.cfg.MaxPages {
			s.log.Info("google/rod: max pages reached", logger.Int("page", pageNum))
			break
		}
		hasNext, err := s.hasNextPage(page)
		if err != nil || !hasNext {
			s.log.Info("google/rod: no more pages", logger.Int("page", pageNum))
			break
		}

		// ── Step 5: wait, then advance to next page ────────────────────────────
		s.log.Info("google/rod: advancing to next page", logger.Int("next", pageNum+1))
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
	s.log.Info("google/rod: scan complete",
		logger.Int("pages", pageNum),
		logger.Int("total_committed", totalCommitted),
		logger.Dur(logger.FieldDuration, time.Since(start)),
	)

	return types.FetchJobsResponse{Jobs: allPublished}, nil
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

// clickAndExtractPanel clicks the job card link on the listing page, waits for
// the SPA detail panel to render inline, then extracts the description.
// The listing and pagination remain intact — no navigation occurs.
//
// page.Element() polls until the element appears or its context deadline.
// A 15s per-card timeout prevents the loop from blocking for hours when
// Google changes DOM state (bot detection, panel overlay, SPA navigation).
func (s *RodGoogle) clickAndExtractPanel(ctx context.Context, page *rod.Page, job jsJob) (string, error) {
	idNum := strings.TrimPrefix(job.ID, "google-")
	sel := fmt.Sprintf(`a[href*="jobs/results/%s"]`, idNum)

	// Use a short deadline so a missing element fails fast rather than blocking
	// for the entire 5-hour scan timeout.
	el, err := page.Timeout(15 * time.Second).Element(sel)
	if err != nil {
		return "", fmt.Errorf("find card link: %w", err)
	}
	if err := el.Click(proto.InputMouseButtonLeft, 1); err != nil {
		return "", fmt.Errorf("click card: %w", err)
	}
	// Wait for the detail panel to render.
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
	el, err := page.Timeout(15 * time.Second).Element(`a[aria-label="Go to next page"]`)
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
	_ types.Adapter        = (*RodGoogle)(nil)
	_ types.URLProvider    = (*RodGoogle)(nil)
	_ types.StreamPublisher = (*RodGoogle)(nil)
	_ types.SeenChecker    = (*RodGoogle)(nil)
)
