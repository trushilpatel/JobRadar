package microsoft

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	msapi "github.com/trushilpatel/jobradar/apps/crawler-v2/adapters/ats/microsoft/api"
	"github.com/trushilpatel/jobradar/apps/crawler-v2/common/logger"
	"github.com/trushilpatel/jobradar/apps/crawler-v2/core"
)

const (
	defaultBaseURL  = "https://apply.careers.microsoft.com"
	defaultDomain   = "microsoft.com"
	defaultPageSize = 20

	// userAgent mimics a real browser to avoid 403s on Go-http-client UA.
	userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"
)

// Microsoft implements core.ListingFetcher and core.PostFetcher by calling
// the Microsoft careers API.
//
// FetchJobListing paginates through /api/pcsx/search to collect all US job listings.
// FetchJobPost hits /api/pcsx/position_details for full job details.
type Microsoft struct {
	baseURL    string // overrides defaultBaseURL in tests
	domain     string // overrides defaultDomain in tests
	pageSize   int    // overrides defaultPageSize in tests
	log        logger.Logger
	httpClient *http.Client
}

// New creates a Microsoft adapter with default production settings.
func New(log logger.Logger, httpClient *http.Client) *Microsoft {
	return &Microsoft{
		log:        log.With(logger.Str(logger.FieldAdapter, "microsoft")),
		httpClient: httpClient,
	}
}

// ── ListingFetcher ────────────────────────────────────────────────────────────

// FetchJobListing paginates through the Microsoft search API and returns all US
// job listings. companyID is unused since this adapter always targets
// careers.microsoft.com.
func (m *Microsoft) FetchJobListing(ctx context.Context, _ string) (core.JobListing, error) {
	log := m.log.WithContext(ctx)
	start := time.Now()
	log.Debug("fetching job listing", logger.Str(logger.FieldURL, m.searchURL(0)))

	var allSummaries []core.JobSummary
	offset := 0

	for {
		pageURL := m.searchURL(offset)
		var page msapi.SearchResponse
		if err := m.get(ctx, pageURL, &page); err != nil {
			log.Error("listing page fetch failed", err,
				logger.Str(logger.FieldURL, pageURL),
				logger.Int("start", offset),
			)
			return core.JobListing{}, fmt.Errorf("microsoft: listing page start=%d: %w", offset, err)
		}

		for _, p := range page.Data.Positions {
			allSummaries = append(allSummaries, mapJobSummary(p))
		}

		// Stop on empty page or once we've collected the total the API advertised.
		if len(page.Data.Positions) == 0 || len(allSummaries) >= page.Data.Count {
			break
		}
		offset += len(page.Data.Positions)
	}

	listing := core.JobListing{Jobs: allSummaries}
	log.Info("listing fetched",
		logger.Int(logger.FieldJobCount, len(listing.Jobs)),
		logger.Dur(logger.FieldDuration, time.Since(start)),
	)
	return listing, nil
}

// ── PostFetcher ───────────────────────────────────────────────────────────────

// FetchJobPost extracts the position ID from summary.URL's last path segment,
// calls /api/pcsx/position_details, and returns a structured JobPost.
func (m *Microsoft) FetchJobPost(ctx context.Context, summary core.JobSummary) (core.JobPost, error) {
	log := m.log.WithContext(ctx).With(logger.Str(logger.FieldJobID, summary.ID))
	start := time.Now()
	log.Debug("fetching job post", logger.Str(logger.FieldURL, summary.URL))

	positionID := extractPositionID(summary.URL)
	if positionID == 0 {
		return core.JobPost{}, fmt.Errorf("microsoft: could not extract position ID from URL %q", summary.URL)
	}

	detailURL := m.positionDetailsURL(positionID)
	var wrapper msapi.PositionDetailResponse
	if err := m.get(ctx, detailURL, &wrapper); err != nil {
		log.Error("post fetch failed", err, logger.Str(logger.FieldURL, detailURL))
		return core.JobPost{}, fmt.Errorf("microsoft: post position_id=%d: %w", positionID, err)
	}

	post := mapJobPost(wrapper.Data)
	log.Info("job post fetched", logger.Dur(logger.FieldDuration, time.Since(start)))
	return post, nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// get performs a GET request with User-Agent header and JSON-decodes the response.
func (m *Microsoft) get(ctx context.Context, url string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
		return fmt.Errorf("decode: %w", err)
	}
	return nil
}

// ── URL builders ──────────────────────────────────────────────────────────────

func (m *Microsoft) resolveBaseURL() string {
	if m.baseURL != "" {
		return m.baseURL
	}
	return defaultBaseURL
}

func (m *Microsoft) resolveDomain() string {
	if m.domain != "" {
		return m.domain
	}
	return defaultDomain
}

func (m *Microsoft) resolvePageSize() int {
	if m.pageSize > 0 {
		return m.pageSize
	}
	return defaultPageSize
}

func (m *Microsoft) searchURL(start int) string {
	// URL encode the location filter: "United States, Multiple Locations, Multiple Locations"
	// becomes "United+States%2C+Multiple+Locations%2C+Multiple+Locations"
	location := "United+States%2C+Multiple+Locations%2C+Multiple+Locations"
	return fmt.Sprintf("%s/api/pcsx/search?domain=%s&query=&location=%s&start=%d&num=%d",
		m.resolveBaseURL(), m.resolveDomain(), location, start, m.resolvePageSize())
}

func (m *Microsoft) positionDetailsURL(positionID int64) string {
	return fmt.Sprintf("%s/api/pcsx/position_details?position_id=%d&domain=%s&hl=en",
		m.resolveBaseURL(), positionID, m.resolveDomain())
}

// ── Interface checks ──────────────────────────────────────────────────────────

var (
	_ core.ListingFetcher = (*Microsoft)(nil)
	_ core.PostFetcher    = (*Microsoft)(nil)
)
