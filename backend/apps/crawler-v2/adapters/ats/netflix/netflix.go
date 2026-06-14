package netflix

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	nfapi "github.com/trushilpatel/jobradar/apps/crawler-v2/adapters/ats/netflix/api"
	"github.com/trushilpatel/jobradar/apps/crawler-v2/common/logger"
	"github.com/trushilpatel/jobradar/common/ats/core"
)

const (
	defaultBaseURL  = "https://explore.jobs.netflix.net"
	defaultDomain   = "netflix.com"
	defaultPageSize = 10

	// userAgent mimics a real browser to avoid CloudFront 403s on the default Go UA.
	userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"
)

// Netflix implements core.ListingFetcher and core.PostFetcher by calling the
// Eightfold AI SmartApply API that powers explore.jobs.netflix.net.
//
// FetchJobListing paginates through /api/apply/v2/jobs to collect all listings.
// FetchJobPost hits /api/apply/v2/jobs/{id} for full job details.
//
// companyID is ignored by FetchJobListing — this adapter always targets
// explore.jobs.netflix.net and does not support multi-tenant configuration.
type Netflix struct {
	baseURL    string // overrides defaultBaseURL in tests
	domain     string // overrides defaultDomain in tests
	pageSize   int    // overrides defaultPageSize in tests
	log        logger.Logger
	httpClient *http.Client
}

// New creates a Netflix adapter with production defaults.
func New(log logger.Logger, httpClient *http.Client) *Netflix {
	return &Netflix{
		log:        log.With(logger.Str(logger.FieldAdapter, "netflix")),
		httpClient: httpClient,
		pageSize:   defaultPageSize,
	}
}

// ── ListingFetcher ────────────────────────────────────────────────────────────

// FetchJobListing paginates through the Eightfold list API and returns all
// Netflix job summaries. companyID is unused — this adapter always targets the
// Netflix Eightfold host.
func (n *Netflix) FetchJobListing(ctx context.Context, _ string) (core.JobListing, error) {
	log := n.log.WithContext(ctx)
	start := time.Now()

	var allJobs []core.JobSummary
	offset := 0

	for {
		url := n.listURL(offset)
		log.Debug("fetching listing page", logger.Str(logger.FieldURL, url))

		var page nfapi.ListResponse
		if err := n.get(ctx, url, &page); err != nil {
			log.Error("listing page fetch failed", err,
				logger.Str(logger.FieldURL, url),
			)
			return core.JobListing{}, fmt.Errorf("netflix: listing page start=%d: %w", offset, err)
		}

		for _, p := range page.Positions {
			allJobs = append(allJobs, mapJobSummary(p))
		}

		if len(page.Positions) < n.resolvePageSize() {
			break
		}
		offset += n.resolvePageSize()
	}

	log.Info("listing fetched",
		logger.Int(logger.FieldJobCount, len(allJobs)),
		logger.Dur(logger.FieldDuration, time.Since(start)),
	)
	return core.JobListing{Jobs: allJobs}, nil
}

// ── PostFetcher ───────────────────────────────────────────────────────────────

// FetchJobPost fetches the full job description from the Eightfold detail API.
// summary.ID must be in "netflix-{eightfold_id}" format; the prefix is stripped
// to build the detail URL.
func (n *Netflix) FetchJobPost(ctx context.Context, summary core.JobSummary) (core.JobPost, error) {
	log := n.log.WithContext(ctx).With(logger.Str(logger.FieldJobID, summary.ID))
	start := time.Now()

	eightfoldID := stripPrefix(summary.ID)
	if eightfoldID == "" {
		return core.JobPost{}, fmt.Errorf("netflix: empty job ID after stripping prefix from %q", summary.ID)
	}

	url := n.detailURL(eightfoldID)
	log.Debug("fetching job post", logger.Str(logger.FieldURL, url))

	var pos nfapi.Position
	if err := n.get(ctx, url, &pos); err != nil {
		log.Error("post fetch failed", err, logger.Str(logger.FieldURL, url))
		return core.JobPost{}, fmt.Errorf("netflix: post: %w", err)
	}

	post := mapJobPost(pos)
	log.Info("job post fetched", logger.Dur(logger.FieldDuration, time.Since(start)))
	return post, nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// get performs a GET request with the required User-Agent header and
// JSON-decodes the response body into dst.
func (n *Netflix) get(ctx context.Context, url string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := n.httpClient.Do(req)
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

// stripPrefix removes the "netflix-" prefix from an ID to get the raw Eightfold
// numeric position ID as a string for use in the detail URL.
func stripPrefix(id string) string {
	const prefix = "netflix-"
	if strings.HasPrefix(id, prefix) {
		return id[len(prefix):]
	}
	return id
}

// ── URL builders ──────────────────────────────────────────────────────────────

func (n *Netflix) listURL(start int) string {
	return fmt.Sprintf("%s/api/apply/v2/jobs?domain=%s&hl=en&start=%d&num=%d",
		n.resolveBaseURL(), n.resolveDomain(), start, n.resolvePageSize())
}

func (n *Netflix) detailURL(id string) string {
	return fmt.Sprintf("%s/api/apply/v2/jobs/%s?domain=%s",
		n.resolveBaseURL(), id, n.resolveDomain())
}

func (n *Netflix) resolveBaseURL() string {
	if n.baseURL != "" {
		return n.baseURL
	}
	return defaultBaseURL
}

func (n *Netflix) resolveDomain() string {
	if n.domain != "" {
		return n.domain
	}
	return defaultDomain
}

func (n *Netflix) resolvePageSize() int {
	if n.pageSize > 0 {
		return n.pageSize
	}
	return defaultPageSize
}

// ── Interface checks ──────────────────────────────────────────────────────────

var (
	_ core.ListingFetcher = (*Netflix)(nil)
	_ core.PostFetcher    = (*Netflix)(nil)
)
