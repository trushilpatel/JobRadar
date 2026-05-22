package google

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/trushilpatel/jobradar/apps/crawler/ats/types"
	"github.com/trushilpatel/jobradar/common/logger"
)

// careersURL is the filtered search URL the crawler walks. Pagination is
// achieved by appending &page=N — Google Careers tracks pagination in the URL
// (verified end-to-end against live HTML).
const careersURL = "https://www.google.com/about/careers/applications/jobs/results?" +
	"location=United%20States&target_level=EARLY&target_level=MID" +
	"&employment_type=FULL_TIME&q=Software%20Engineering"

// detailBase is the canonical prefix for an individual job's URL.
// We always append "{id}-{slug}" without query parameters.
const detailBase = "https://www.google.com/about/careers/applications/jobs/results/"

// pageSize is the number of cards Google emits per listing page. Used to
// derive the last page from the "Showing X to Y of N" total.
const pageSize = 20

// userAgent — same Chrome UA the rod adapter used. Pinned so HTTP headers
// stay consistent across scans.
const userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) " +
	"AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.0.0 Safari/537.36"

const acceptHeader = "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8"
const acceptLangHeader = "en-US,en;q=0.9"

// GoogleAdapter implements types.Adapter using plain HTTP + regex extraction.
//
// Replaces the previous browser-based (rod) approach. Google Careers is fully
// server-side rendered — a plain HTTP GET returns the same DOM the browser
// sees, including the qualifications sections. No JavaScript execution is
// required at any step of the scan.
type GoogleAdapter struct {
	cfg           Config
	log           logger.Logger
	client        *http.Client
	streamPublish func([]types.FetchJobsItem)
	seenChecker   func(ctx context.Context, ids []string) (map[string]bool, error)
	// listingURLOverride replaces the production listing URL used by FetchJobs.
	// Production code leaves this empty. Tests set this to an httptest server
	// URL so FetchJobs can be exercised without hitting Google's live page.
	listingURLOverride string
}

// NewGoogleAdapter constructs a GoogleAdapter with the given config.
func NewGoogleAdapter(cfg Config, log logger.Logger) *GoogleAdapter {
	return &GoogleAdapter{
		cfg:    cfg,
		log:    log,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// NewGoogleAdapterDefault constructs a GoogleAdapter with production defaults
// and a no-op logger. Convenient for tests and simple wiring.
func NewGoogleAdapterDefault() *GoogleAdapter {
	return NewGoogleAdapter(defaultConfig(), logger.NewNoop())
}

// CareersURL implements types.URLProvider.
func (s *GoogleAdapter) CareersURL(_ string) string { return careersURL }

// SetStreamPublish implements types.StreamPublisher.
func (s *GoogleAdapter) SetStreamPublish(fn func([]types.FetchJobsItem)) {
	s.streamPublish = fn
}

// SetSeenChecker implements types.SeenChecker.
func (s *GoogleAdapter) SetSeenChecker(fn func(ctx context.Context, ids []string) (map[string]bool, error)) {
	s.seenChecker = fn
}

// ── Regex patterns (RE2 — Go's regexp engine) ──────────────────────────────

// reTotalCount captures the total job count from the "Showing X to Y of N"
// indicator, used to terminate pagination when the last page is reached.
var reTotalCount = regexp.MustCompile(`Showing\s+\d+\s+to\s+\d+\s+of\s+(\d+)`)

// reAriaTitle finds each card's "Learn more about <title>" anchor.
// On a 20-card listing page Google emits 20 of these (sometimes 21 — the
// extra one is a self-link with no associated card content; we detect those
// by absence of an href in the slice).
var reAriaTitle = regexp.MustCompile(`aria-label="Learn more about ([^"]+)"`)

// reHref pulls the canonical job ID and slug out of the card's anchor href.
// href shape is `jobs/results/{id}-{slug}?…query` — we capture id and slug
// up to the first `"` or `?`.
var reHref = regexp.MustCompile(`href="jobs/results/(\d+)-([^"?]+)`)

// reLocationBlock captures the wrapped location text. The icon close tag is
// followed by nested <span>/<div> wrappers; the natural terminator is the
// next <button> (the share/save action on every card). Strip tags after.
var reLocationBlock = regexp.MustCompile(`(?is)>place</[^>]+>(.{0,800}?)<button`)

// reOrgBlock captures the wrapped org text. Terminator is the start of the
// next <i…> tag (the place icon that follows the org). Stopping at ">place<"
// instead would leak the half-open <i…> attributes past the tag-strip.
var reOrgBlock = regexp.MustCompile(`(?is)>corporate_fare</[^>]+>(.{0,400}?)<i\b`)

// reTag / reWS / rePlusMore are post-process helpers: generic tag stripping,
// whitespace collapsing, and removal of "+N more" responsive truncation
// indicators in locations.
var (
	reTag      = regexp.MustCompile(`<[^>]+>`)
	reWS       = regexp.MustCompile(`\s+`)
	rePlusMore = regexp.MustCompile(`(?i);?\s*\+\d+\s*more`)
)

// parsedJob is the in-package representation of a parsed listing card.
// Detail-page content is fetched downstream by the analyzer.
type parsedJob struct {
	ID       string // e.g. "google-12345"
	Title    string
	URL      string
	Location string
	Org      string
}

// FetchJobs walks every listing page (paginating via &page=N), parses listing
// cards, and publishes thin items to Kafka. Detail-page fetching and LLM
// enrichment are handled downstream by the analyzer.
//
// Pacing:
//   - MinDelayMs/MaxDelayMs between listing-page fetches (default 20-40 s)
//   - TimeoutMs             overall scan budget          (default 2 h)
//
// Reliability:
//   - Seen-checker failures don't loop the page; they advance pagination
//     after treating every card as already-seen for that page.
func (s *GoogleAdapter) FetchJobs(ctx context.Context, _ string) (types.FetchJobsResponse, error) {
	start := time.Now()

	if s.cfg.TimeoutMs > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(s.cfg.TimeoutMs)*time.Millisecond)
		defer cancel()
	}

	seenIDs := make(map[string]struct{})
	var allPublished []types.FetchJobsItem
	pageNum := 1
	totalCommitted := 0

scanLoop:
	for {
		if ctx.Err() != nil {
			s.log.Warn("google/http: context cancelled")
			break
		}

		base := careersURL
		if s.listingURLOverride != "" {
			base = s.listingURLOverride
		}
		listingURL := fmt.Sprintf("%s&page=%d", base, pageNum)
		s.log.Info("google/http: scanning listing page", logger.Int("page", pageNum))

		html, err := s.fetch(ctx, listingURL)
		if err != nil {
			s.log.Warn("google/http: listing fetch failed",
				logger.Int("page", pageNum),
				logger.Str("error", err.Error()))
			break
		}

		cards, total := parseListing(html)
		s.log.Info("google/http: page scanned",
			logger.Int("page", pageNum),
			logger.Int(logger.FieldFetched, len(cards)),
			logger.Int("total_in_search", total),
		)

		// ── Filter via seenChecker ─────────────────────────────────────────
		ids := make([]string, len(cards))
		for i, c := range cards {
			ids[i] = c.ID
		}
		seenMap := map[string]bool{}
		if s.seenChecker != nil {
			seenMap, err = s.seenChecker(ctx, ids)
			if err != nil {
				s.log.Warn("google/http: seen check failed, skipping enrichment for this page",
					logger.Str("error", err.Error()))
				seenMap = make(map[string]bool, len(ids))
				for _, id := range ids {
					seenMap[id] = true
				}
			}
		}
		newCards := filterPageJobs(cards, seenMap, seenIDs)

		s.log.Info("google/http: filtered new",
			logger.Int(logger.FieldNew, len(newCards)),
			logger.Int("skipped", len(cards)-len(newCards)),
		)

		// Publish all new cards as thin items — the analyzer fetches detail
		// pages and runs LLM enrichment downstream.
		committed := 0
		for _, card := range newCards {
			if ctx.Err() != nil {
				s.log.Warn("google/http: context cancelled during publish")
				break scanLoop
			}
			item := toItem(card)
			if s.streamPublish != nil {
				s.streamPublish([]types.FetchJobsItem{item})
			}
			allPublished = append(allPublished, item)
			committed++
		}
		totalCommitted += committed

		s.log.Info("google/http: page committed",
			logger.Int("page", pageNum),
			logger.Int("committed", committed),
			logger.Int("total_committed", totalCommitted),
			logger.Dur(logger.FieldDuration, time.Since(start)),
		)

		// ── Termination checks ─────────────────────────────────────────────
		if s.cfg.MaxPages > 0 && pageNum >= s.cfg.MaxPages {
			s.log.Info("google/http: max pages reached", logger.Int("page", pageNum))
			break
		}
		if total > 0 && pageNum*pageSize >= total {
			s.log.Info("google/http: covered all pages",
				logger.Int("page", pageNum),
				logger.Int("total", total))
			break
		}

		// ── Inter-page pacing (configured), then advance ───────────────────
		if err := s.sleep(ctx, s.cfg.MinDelayMs, s.cfg.MaxDelayMs); err != nil {
			break
		}
		pageNum++
	}

	s.log.Info("google/http: scan complete",
		logger.Int("pages", pageNum),
		logger.Int("total_committed", totalCommitted),
		logger.Dur(logger.FieldDuration, time.Since(start)),
	)

	return types.FetchJobsResponse{Jobs: allPublished}, nil
}

// fetch performs a single HTTP GET with pinned headers and the configured
// client timeout. Returns the response body as a string for regex scanning.
func (s *GoogleAdapter) fetch(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", acceptHeader)
	req.Header.Set("Accept-Language", acceptLangHeader)

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("http do: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("http status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}
	return string(body), nil
}

// parseListing extracts every card from a listing-page HTML body and returns
// the parsed jobs along with the search's total job count.
//
// Slicing strategy: each "Learn more about" anchor sits at the END of its
// card's markup. Card[i]'s metadata (h3, place icon, corporate_fare icon,
// href) lives between aria[i-1] and aria[i] (or [0..aria[0]] for the very
// first card on the page).
func parseListing(html string) ([]parsedJob, int) {
	total := 0
	if m := reTotalCount.FindStringSubmatch(html); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil {
			total = n
		}
	}

	titleSubs := reAriaTitle.FindAllStringSubmatch(html, -1)
	titleIdx := reAriaTitle.FindAllStringSubmatchIndex(html, -1)

	cards := make([]parsedJob, 0, len(titleSubs))
	for i, idx := range titleIdx {
		title := strings.TrimSpace(titleSubs[i][1])

		var sliceStart int
		if i == 0 {
			sliceStart = 0
		} else {
			sliceStart = titleIdx[i-1][1] // end of previous aria-label
		}
		sliceEnd := idx[0]
		if sliceEnd <= sliceStart {
			continue
		}
		block := html[sliceStart:sliceEnd]

		hrefMatches := reHref.FindAllStringSubmatch(block, -1)
		if len(hrefMatches) == 0 {
			// The "extra" 21st aria-label (no associated card content) lands
			// here and is silently skipped.
			continue
		}
		// The last href in the block is this card's anchor target.
		last := hrefMatches[len(hrefMatches)-1]
		jobID, slug := last[1], last[2]
		jobURL := detailBase + jobID + "-" + slug

		var location, org string
		if m := reLocationBlock.FindStringSubmatch(block); m != nil {
			location = cleanText(m[1], true)
		}
		if m := reOrgBlock.FindStringSubmatch(block); m != nil {
			org = cleanText(m[1], false)
		}

		cards = append(cards, parsedJob{
			ID:       "google-" + jobID,
			Title:    title,
			URL:      jobURL,
			Location: location,
			Org:      org,
		})
	}
	return cards, total
}

// cleanText strips tags, collapses whitespace, optionally removes the
// "+N more" responsive truncation indicators that appear inside multi-
// location fields (e.g. "Kirkland, WA, USA; Sunnyvale, CA, USA; +2 more").
func cleanText(raw string, stripPlusMore bool) string {
	text := reTag.ReplaceAllString(raw, " ")
	if stripPlusMore {
		text = rePlusMore.ReplaceAllString(text, "")
	}
	text = reWS.ReplaceAllString(text, " ")
	text = strings.TrimSpace(text)
	text = strings.TrimRight(text, ";")
	return strings.TrimSpace(text)
}

// filterPageJobs returns only jobs that are not already in the pipeline
// ledger (per seenMap) and not already published earlier in this scan
// (per localSeen). Updates localSeen for every ID encountered so subsequent
// pages stay deduplicated. Same shape as the legacy implementation —
// preserved verbatim so the unit tests remain valid.
func filterPageJobs(pageJobs []parsedJob, seenMap map[string]bool, localSeen map[string]struct{}) []parsedJob {
	var result []parsedJob
	for _, job := range pageJobs {
		if _, local := localSeen[job.ID]; local {
			continue
		}
		localSeen[job.ID] = struct{}{}
		if seenMap[job.ID] {
			continue
		}
		result = append(result, job)
	}
	return result
}

// toItem maps a parsedJob to a thin FetchJobsItem. FullContent is always nil
// — the analyzer fetches the detail page and runs LLM enrichment downstream.
func toItem(j parsedJob) types.FetchJobsItem {
	return types.FetchJobsItem{
		ID:             j.ID,
		Title:          j.Title,
		URL:            j.URL,
		Location:       j.Location,
		EmploymentType: "full_time",
		LocationKnown:  true,
		AllLocations:   []string{j.Location},
	}
}

// sleep waits for a random duration in [minMs, maxMs) ms or returns ctx.Err()
// if the context is cancelled first.
func (s *GoogleAdapter) sleep(ctx context.Context, minMs, maxMs int) error {
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

// ── ContentFetcher implementation ──────────────────────────────────────────
//
// Google careers detail pages are server-rendered HTML (~1.2 MB) that embeds
// job data inside `AF_initDataCallback({key:'ds:N', hash:'…', data:[…]});`
// script blocks. The focal job lives in `ds:0`; `ds:1` carries ~20 sibling
// jobs from the same search. Inside the data array, description prose appears
// as JSON-quoted strings with HTML angle brackets unicode-escaped (`<` →
// `<`).
//
// The analyzer's generic HTTP fallback caps reads at 50 KB and runs StripHTML
// on the truncated body — well before reaching the description region. By
// implementing ContentFetcher, the Google adapter owns its own detail fetch
// and extraction, returning clean plain text the LLM enricher can consume.

// FetchJobContent fetches a single Google job's detail page and returns the
// plain-text Description extracted from the focal job's `ds:0` callback.
func (s *GoogleAdapter) FetchJobContent(ctx context.Context, job types.RawJob) (types.FetchJobContentResponse, error) {
	if job.URL == "" {
		return types.FetchJobContentResponse{}, fmt.Errorf("google: empty RawJob.URL for job %q", job.ID)
	}
	html, err := s.fetch(ctx, job.URL)
	if err != nil {
		return types.FetchJobContentResponse{}, fmt.Errorf("google: fetch detail: %w", err)
	}
	return types.FetchJobContentResponse{
		ID:          job.ID,
		Title:       job.Title,
		Location:    job.Location,
		URL:         job.URL,
		Description: extractDescription(html),
	}, nil
}

// extractDescription pulls the focal job's description out of the `ds:0`
// AF_initDataCallback block. It walks ds:0 with a JSON-string state machine,
// collects every quoted string that contains an encoded `<` (`<`),
// JSON-decodes it, strips HTML to plain text, dedupes by exact content, and
// joins the remaining parts on blank lines.
//
// Sibling jobs in `ds:1` are excluded by design — only ds:0 is scanned.
func extractDescription(html string) string {
	const ds0Marker = "AF_initDataCallback({key: 'ds:0'"
	const cbMarker = "AF_initDataCallback"
	// escapedLT is the literal six-byte sequence `<` (the JSON-escape form
	// of `<`) as it appears verbatim in Google's embedded data strings — NOT
	// the rendered `<` rune. Built as two halves so neither raw nor
	// interpreted string literals can collapse the backslash-u into a real `<`.
	const escapedLT = `\` + `u003c`

	start := strings.Index(html, ds0Marker)
	if start < 0 {
		return ""
	}
	end := len(html)
	if next := strings.Index(html[start+len(ds0Marker):], cbMarker); next >= 0 {
		end = start + len(ds0Marker) + next
	}
	block := html[start:end]

	var parts []string
	seen := make(map[string]struct{})

	var (
		inString bool
		escaped  bool
		qStart   int
	)
	for i := 0; i < len(block); i++ {
		c := block[i]
		if !inString {
			if c == '"' {
				inString = true
				escaped = false
				qStart = i
			}
			continue
		}
		if escaped {
			escaped = false
			continue
		}
		switch c {
		case '\\':
			escaped = true
		case '"':
			quoted := block[qStart : i+1]
			inString = false
			if !strings.Contains(quoted, escapedLT) {
				continue
			}
			var decoded string
			if err := json.Unmarshal([]byte(quoted), &decoded); err != nil {
				continue
			}
			plain := types.StripHTML(decoded)
			if plain == "" {
				continue
			}
			if _, dup := seen[plain]; dup {
				continue
			}
			seen[plain] = struct{}{}
			parts = append(parts, plain)
		}
	}
	return strings.Join(parts, "\n\n")
}

// Compile-time interface checks.
var (
	_ types.Adapter         = (*GoogleAdapter)(nil)
	_ types.URLProvider     = (*GoogleAdapter)(nil)
	_ types.StreamPublisher = (*GoogleAdapter)(nil)
	_ types.SeenChecker     = (*GoogleAdapter)(nil)
	_ types.ContentFetcher  = (*GoogleAdapter)(nil)
)
