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

	googleapi "github.com/trushilpatel/jobradar/apps/crawler-v2/adapters/ats/google/api"
	"github.com/trushilpatel/jobradar/apps/crawler-v2/common/logger"
	"github.com/trushilpatel/jobradar/apps/crawler-v2/core"
)

// defaultListingURL is the filtered careers search URL. Pagination is achieved
// by appending &page=N. These filters are intentional and not runtime-configurable.
const defaultListingURL = "https://www.google.com/about/careers/applications/jobs/results?" +
	"location=United%20States&target_level=EARLY&target_level=MID" +
	"&employment_type=FULL_TIME&q=Software%20Engineering"

// pageSize is the number of cards Google emits per listing page.
const pageSize = 20

const userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) " +
	"AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.0.0 Safari/537.36"

const acceptHeader = "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8"
const acceptLangHeader = "en-US,en;q=0.9"

// ── Regex patterns (RE2 — Go's regexp engine) ─────────────────────────────────

// reTotalCount captures the total job count from the "Showing X to Y of N" indicator.
var reTotalCount = regexp.MustCompile(`Showing\s+\d+\s+to\s+\d+\s+of\s+(\d+)`)

// reAriaTitle finds each card's "Learn more about <title>" anchor.
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
// next <i…> tag (the place icon that follows the org).
var reOrgBlock = regexp.MustCompile(`(?is)>corporate_fare</[^>]+>(.{0,400}?)<i\b`)

// reTag / reWS / rePlusMore are post-process helpers.
var (
	reTag      = regexp.MustCompile(`<[^>]+>`)
	reWS       = regexp.MustCompile(`\s+`)
	rePlusMore = regexp.MustCompile(`(?i);?\s*\+\d+\s*more`)
)

// ── Adapter struct ────────────────────────────────────────────────────────────

// Google is the crawler-v2 adapter for Google Careers pages.
// It uses plain HTTP + regex extraction — no headless browser required.
type Google struct {
	listingBaseURL string // overrides listing URL in tests
	log            logger.Logger
	httpClient     *http.Client
	minDelayMs     int // milliseconds between listing-page fetches; 0 in tests
	maxDelayMs     int // milliseconds between listing-page fetches; 0 in tests
}

// New constructs a Google adapter with production pacing defaults (20–40 s between listing pages).
func New(log logger.Logger, httpClient *http.Client) *Google {
	return &Google{
		log:        log.With(logger.Str(logger.FieldAdapter, "google")),
		httpClient: httpClient,
		minDelayMs: 20000,
		maxDelayMs: 40000,
	}
}

// ── ListingFetcher ────────────────────────────────────────────────────────────

// FetchJobListing walks every listing page (paginating via &page=N), parses
// listing cards, and returns a core.JobListing. The companyID parameter is
// ignored — Google Careers is a single global search result set.
func (g *Google) FetchJobListing(ctx context.Context, _ string) (core.JobListing, error) {
	log := g.log.WithContext(ctx)
	start := time.Now()

	var allJobs []core.JobSummary
	pageNum := 1

	for ctx.Err() == nil {
		listingURL := fmt.Sprintf("%s&page=%d", g.resolveListingBaseURL(), pageNum)
		log.Debug("fetching listing page",
			logger.Int("page", pageNum),
			logger.Str(logger.FieldURL, listingURL),
		)

		html, err := g.fetch(ctx, listingURL)
		if err != nil {
			log.Error("listing page fetch failed", err,
				logger.Int("page", pageNum),
				logger.Str(logger.FieldURL, listingURL),
			)
			return core.JobListing{}, fmt.Errorf("google: listing page %d: %w", pageNum, err)
		}

		parsed, total := parseListing(html)
		log.Info("listing page fetched",
			logger.Int("page", pageNum),
			logger.Int(logger.FieldJobCount, len(parsed)),
			logger.Int("total_in_search", total),
		)

		for _, p := range parsed {
			allJobs = append(allJobs, mapParsedJobToSummary(p))
		}

		// Stop when all pages covered.
		if total > 0 && pageNum*pageSize >= total {
			log.Info("covered all listing pages",
				logger.Int("page", pageNum),
				logger.Int("total", total),
			)
			break
		}
		// Stop if this page returned nothing and we have no total.
		if len(parsed) == 0 {
			break
		}

		if err := g.sleep(ctx, g.minDelayMs, g.maxDelayMs); err != nil {
			break
		}
		pageNum++
	}

	log.Info("listing fetch complete",
		logger.Int(logger.FieldJobCount, len(allJobs)),
		logger.Dur(logger.FieldDuration, time.Since(start)),
	)
	return core.JobListing{Jobs: allJobs}, nil
}

// ── PostFetcher ───────────────────────────────────────────────────────────────

// FetchJobPost fetches the detail page for the given job summary and extracts
// the description from the AF_initDataCallback(key:'ds:0') block.
func (g *Google) FetchJobPost(ctx context.Context, summary core.JobSummary) (core.JobPost, error) {
	log := g.log.WithContext(ctx).With(logger.Str(logger.FieldJobID, summary.ID))
	start := time.Now()
	log.Debug("fetching job post", logger.Str(logger.FieldURL, summary.URL))

	if summary.URL == "" {
		return core.JobPost{}, fmt.Errorf("google: empty URL for job %q", summary.ID)
	}

	html, err := g.fetch(ctx, summary.URL)
	if err != nil {
		log.Error("job post fetch failed", err, logger.Str(logger.FieldURL, summary.URL))
		return core.JobPost{}, fmt.Errorf("google: post fetch: %w", err)
	}

	detail := extractJobDetail(html)
	post := mapDetailToJobPost(summary, detail)
	log.Info("job post fetched",
		logger.Str(logger.FieldTitle, post.Title),
		logger.Dur(logger.FieldDuration, time.Since(start)),
	)
	return post, nil
}

// ── Parsing ───────────────────────────────────────────────────────────────────

// parseListing extracts every card from a listing-page HTML body and returns
// the parsed jobs along with the search's total job count.
//
// Slicing strategy: each "Learn more about" anchor sits at the END of its
// card's markup. Card[i]'s metadata lives between aria[i-1] and aria[i]
// (or [0..aria[0]] for the very first card).
func parseListing(html string) ([]googleapi.ParsedJob, int) {
	total := 0
	if m := reTotalCount.FindStringSubmatch(html); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil {
			total = n
		}
	}

	titleSubs := reAriaTitle.FindAllStringSubmatch(html, -1)
	titleIdx := reAriaTitle.FindAllStringSubmatchIndex(html, -1)

	cards := make([]googleapi.ParsedJob, 0, len(titleSubs))
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
			// Extra aria-label with no associated card content — skip.
			continue
		}
		// The last href in the block is this card's anchor target.
		last := hrefMatches[len(hrefMatches)-1]
		jobID, slug := last[1], last[2]
		jobURL := "https://www.google.com/about/careers/applications/jobs/results/" + jobID + "-" + slug

		var location, team string
		if m := reLocationBlock.FindStringSubmatch(block); m != nil {
			location = cleanText(m[1], true)
		}
		if m := reOrgBlock.FindStringSubmatch(block); m != nil {
			team = cleanText(m[1], false)
		}

		cards = append(cards, googleapi.ParsedJob{
			ID:       jobID,
			Slug:     slug,
			Title:    title,
			Location: location,
			Team:     team,
			URL:      jobURL,
		})
	}
	return cards, total
}

// extractJobDetail parses a Google Careers detail page and extracts fields
// from the AF_initDataCallback({key: 'ds:0'…}) block. The description is
// extracted by finding all JSON-quoted strings containing HTML-escaped `<`
// (the literal `<` six-byte sequence that Go's json.Marshal emits).
func extractJobDetail(html string) googleapi.JobDetail {
	return googleapi.JobDetail{
		Description: extractDescription(html),
	}
}

// extractDescription pulls the focal job's description out of the `ds:0`
// AF_initDataCallback block. It walks ds:0 with a JSON-string state machine,
// collects every quoted string that contains the unicode-escaped `<` marker,
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
			// Strip HTML tags and decode entities.
			plain := stripHTMLSimple(decoded)
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

// stripHTMLSimple removes all HTML tags and collapses whitespace.
// Mirrors the ats.StripHTML helper but defined locally to avoid the import cycle.
func stripHTMLSimple(s string) string {
	plain := reTag.ReplaceAllString(s, " ")
	return strings.TrimSpace(reWS.ReplaceAllString(plain, " "))
}

// cleanText strips tags, collapses whitespace, optionally removes the
// "+N more" responsive truncation indicators in multi-location fields.
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

// ── HTTP ──────────────────────────────────────────────────────────────────────

// fetch performs a single HTTP GET with pinned browser headers.
func (g *Google) fetch(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", acceptHeader)
	req.Header.Set("Accept-Language", acceptLangHeader)

	resp, err := g.httpClient.Do(req)
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

// sleep waits for a random duration in [minMs, maxMs) ms or returns
// ctx.Err() if the context is cancelled first. When both are 0, returns immediately.
func (g *Google) sleep(ctx context.Context, minMs, maxMs int) error {
	if minMs == 0 && maxMs == 0 {
		return nil
	}
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

func (g *Google) resolveListingBaseURL() string {
	if g.listingBaseURL != "" {
		return g.listingBaseURL
	}
	return defaultListingURL
}

// ── Interface checks ──────────────────────────────────────────────────────────

var (
	_ core.ListingFetcher = (*Google)(nil)
	_ core.PostFetcher    = (*Google)(nil)
)
