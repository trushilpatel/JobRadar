package apple

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	appleapi "github.com/trushilpatel/jobradar/apps/crawler-v2/adapters/ats/apple/api"
	"github.com/trushilpatel/jobradar/common/ats/core"
	"github.com/trushilpatel/jobradar/common/logger"
)

const (
	defaultBaseURL = "https://jobs.apple.com"

	// listingPath is the search endpoint with US filter applied.
	listingPath = "/en-us/search?location=united-states-USA&page=%d"

	// userAgent mimics a real browser to avoid blocking.
	userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"
)

// ── Construction ──────────────────────────────────────────────────────────────

// Apple implements core.ListingFetcher and core.PostFetcher by scraping
// jobs.apple.com listing pages and detail pages.
//
// FetchJobListing:
//  1. Paginates through /en-us/search?location=united-states-USA&page={n}
//  2. Extracts window.__staticRouterHydrationData from each page's HTML
//  3. Stops when searchResults is empty
//
// FetchJobPost:
//  1. GETs summary.URL (the public detail page)
//  2. Extracts window.__staticRouterHydrationData
//  3. Reads loaderData.jobDetails.jobsData and builds the structured JobPost
type Apple struct {
	baseURL    string // overrides defaultBaseURL in tests
	log        logger.Logger
	httpClient *http.Client
}

// New creates an Apple adapter with the given logger and HTTP client.
func New(log logger.Logger, httpClient *http.Client) *Apple {
	return &Apple{
		log:        log.With(logger.Str(logger.FieldAdapter, "apple")),
		httpClient: httpClient,
	}
}

// ── ListingFetcher ────────────────────────────────────────────────────────────

// FetchJobListing returns all Apple job listings from the US careers search page.
// companyID is ignored — Apple adapter targets a fixed host.
func (a *Apple) FetchJobListing(ctx context.Context, _ string) (core.JobListing, error) {
	log := a.log.WithContext(ctx)
	start := time.Now()

	var allJobs []core.JobSummary
	page := 1

	for {
		select {
		case <-ctx.Done():
			return core.JobListing{}, ctx.Err()
		default:
		}

		pageJobs, err := a.fetchListingPage(ctx, page)
		if err != nil {
			log.Error("listing page fetch failed", err, logger.Int(logger.FieldPage, page))
			return core.JobListing{}, fmt.Errorf("apple: fetch page %d: %w", page, err)
		}
		if len(pageJobs) == 0 {
			break
		}
		allJobs = append(allJobs, pageJobs...)
		page++
	}

	log.Info("listing fetched",
		logger.Int(logger.FieldJobCount, len(allJobs)),
		logger.Int(logger.FieldTotalPages, page-1),
		logger.Dur(logger.FieldDurationMs, time.Since(start)),
	)
	return core.JobListing{Jobs: allJobs}, nil
}

// fetchListingPage fetches a single page and returns JobSummary items.
func (a *Apple) fetchListingPage(ctx context.Context, page int) ([]core.JobSummary, error) {
	url := a.resolveBaseURL() + fmt.Sprintf(listingPath, page)

	body, err := a.fetchHTML(ctx, url)
	if err != nil {
		return nil, err
	}

	hydrationStr, err := extractHydrationData(body)
	if err != nil {
		return nil, fmt.Errorf("extract hydration data: %w", err)
	}

	var data appleapi.HydrationData
	if err := json.Unmarshal([]byte(hydrationStr), &data); err != nil {
		return nil, fmt.Errorf("decode hydration JSON: %w", err)
	}

	results := data.LoaderData.Search.SearchResults
	items := make([]core.JobSummary, 0, len(results))
	for _, job := range results {
		items = append(items, mapJobSummary(job, a.resolveBaseURL()))
	}
	return items, nil
}

// ── PostFetcher ───────────────────────────────────────────────────────────────

// FetchJobPost GETs the public detail page (summary.URL), extracts the React
// Router hydration data, and returns the structured JobPost.
func (a *Apple) FetchJobPost(ctx context.Context, summary core.JobSummary) (core.JobPost, error) {
	log := a.log.WithContext(ctx).With(logger.Str(logger.FieldJobID, summary.ID))
	start := time.Now()
	log.Debug("fetching job post", logger.Str(logger.FieldURL, summary.URL))

	body, err := a.fetchHTML(ctx, summary.URL)
	if err != nil {
		log.Error("post fetch failed", err, logger.Str(logger.FieldURL, summary.URL))
		return core.JobPost{}, fmt.Errorf("apple: fetch detail %s: %w", summary.URL, err)
	}

	hydrationStr, err := extractHydrationData(body)
	if err != nil {
		return core.JobPost{}, fmt.Errorf("apple: extract detail hydration: %w", err)
	}

	var data appleapi.DetailHydrationData
	if err := json.Unmarshal([]byte(hydrationStr), &data); err != nil {
		return core.JobPost{}, fmt.Errorf("apple: decode detail JSON: %w", err)
	}

	post := mapJobPost(data.LoaderData.JobDetails.JobsData)
	log.Info("job post fetched", logger.Dur(logger.FieldDurationMs, time.Since(start)))
	return post, nil
}

// ── HTTP helpers ──────────────────────────────────────────────────────────────

// fetchHTML performs a browser-headers GET and returns the response body as a string.
func (a *Apple) fetchHTML(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	a.setHeaders(req)

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}
	return string(bodyBytes), nil
}

// setHeaders applies browser-like headers to every request.
func (a *Apple) setHeaders(req *http.Request) {
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")
}

// ── Hydration extraction ──────────────────────────────────────────────────────

// extractHydrationData finds and returns the JSON string from
// window.__staticRouterHydrationData. Handles three variants Apple has used:
//   - Direct object:           = {...};
//   - JSON.parse double-quote: = JSON.parse("{...}");
//   - JSON.parse single-quote: = JSON.parse('{...}');
func extractHydrationData(body string) (string, error) {
	const marker = "window.__staticRouterHydrationData = "
	_, after, found := strings.Cut(body, marker)
	if !found {
		return "", fmt.Errorf("window.__staticRouterHydrationData not found in page")
	}

	rest := strings.TrimLeft(after, " \t\r\n")

	const jsonParseCall = "JSON.parse("
	if strings.HasPrefix(rest, jsonParseCall) {
		arg := rest[len(jsonParseCall):]
		if len(arg) == 0 {
			return "", fmt.Errorf("empty JSON.parse() call")
		}
		quoteChar := arg[0]
		if quoteChar != '"' && quoteChar != '\'' {
			return "", fmt.Errorf("JSON.parse() argument has unexpected opening character %c", quoteChar)
		}
		inner, err := scanToClosingQuote(arg[1:], quoteChar)
		if err != nil {
			return "", fmt.Errorf("JSON.parse() argument: %w", err)
		}
		if quoteChar == '"' {
			var unescaped string
			if jsonErr := json.Unmarshal([]byte(`"`+inner+`"`), &unescaped); jsonErr != nil {
				return "", fmt.Errorf("decode JSON.parse string: %w", jsonErr)
			}
			return unescaped, nil
		}
		// Single-quote variant: unescape \' and \\
		unescaped := strings.ReplaceAll(inner, `\'`, "'")
		unescaped = strings.ReplaceAll(unescaped, `\\`, `\`)
		return unescaped, nil
	}

	// Direct object variant: trim to </script>
	before, _, found := strings.Cut(rest, "</script>")
	if !found {
		return "", fmt.Errorf("closing </script> not found after hydration marker")
	}
	return strings.TrimRight(before, " \t\r\n;"), nil
}

// scanToClosingQuote returns the raw content of a JS string literal up to (but
// not including) the first unescaped occurrence of quote.
func scanToClosingQuote(s string, quote byte) (string, error) {
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' {
			i++ // skip escaped character
			continue
		}
		if s[i] == quote {
			return s[:i], nil
		}
	}
	return "", fmt.Errorf("unclosed string — closing %c not found", quote)
}

// ── URL helper ────────────────────────────────────────────────────────────────

func (a *Apple) resolveBaseURL() string {
	if a.baseURL != "" {
		return a.baseURL
	}
	return defaultBaseURL
}

// ── Interface checks ──────────────────────────────────────────────────────────

var (
	_ core.ListingFetcher = (*Apple)(nil)
	_ core.PostFetcher    = (*Apple)(nil)
)
