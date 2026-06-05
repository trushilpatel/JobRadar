package gem

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	gemapi "github.com/trushilpatel/jobradar/jobingest/adapters/ats/gem/api"
	"github.com/trushilpatel/jobradar/jobingest/common/logger"
	"github.com/trushilpatel/jobradar/jobingest/core"
)

const defaultBaseURL = "https://api.gem.com"

// Gem implements core.ListingFetcher against the Gem Job Board REST API.
//
// FetchJobListing issues a single GET to /job_board/v0/{slug}/job_posts/ which
// returns every open posting for the board, including the full description.
// Because the listing response already carries complete content, each
// JobSummary is emitted with JobContent populated — this is a fat adapter
// and no PostFetcher is implemented.
//
// Gem does not expose a per-job detail endpoint (GET .../job_posts/{id} returns 404).
type Gem struct {
	baseURL    string // overridden in tests to point at httptest server
	log        logger.Logger
	httpClient *http.Client
}

// New constructs a Gem adapter with production defaults.
func New(log logger.Logger, httpClient *http.Client) *Gem {
	return &Gem{
		log:        log.With(logger.Str(logger.FieldAdapter, "gem")),
		httpClient: httpClient,
	}
}

// FetchJobListing returns every open posting for the given Gem vanity slug.
// companyID is the board slug (e.g. "inception").
func (g *Gem) FetchJobListing(ctx context.Context, companyID string) (core.JobListing, error) {
	slug := strings.TrimSpace(companyID)
	if slug == "" {
		return core.JobListing{}, fmt.Errorf("gem: empty company slug")
	}

	log := g.log.WithContext(ctx).With(logger.Str(logger.FieldCompany, slug))
	start := time.Now()
	url := g.jobListingURL(slug)
	log.Debug("fetching job listing", logger.Str(logger.FieldURL, url))

	var raw []gemapi.JobPost
	if err := g.get(ctx, url, &raw); err != nil {
		log.Error("listing fetch failed", err, logger.Str(logger.FieldURL, url))
		return core.JobListing{}, fmt.Errorf("gem: listing: %w", err)
	}

	listing := mapJobListing(raw)
	log.Info("listing fetched",
		logger.Int(logger.FieldJobCount, len(listing.Jobs)),
		logger.Dur(logger.FieldDuration, time.Since(start)),
	)
	return listing, nil
}

// get performs a GET request with Accept: application/json and JSON-decodes
// the response body into dst.
func (g *Gem) get(ctx context.Context, url string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := g.httpClient.Do(req)
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

func (g *Gem) jobListingURL(slug string) string {
	return fmt.Sprintf("%s/job_board/v0/%s/job_posts/", g.resolveBaseURL(), slug)
}

func (g *Gem) resolveBaseURL() string {
	if g.baseURL != "" {
		return g.baseURL
	}
	return defaultBaseURL
}

// ── Interface checks ──────────────────────────────────────────────────────────

var _ core.ListingFetcher = (*Gem)(nil)

// No PostFetcher — Gem is a fat adapter; full content is included at listing time.
