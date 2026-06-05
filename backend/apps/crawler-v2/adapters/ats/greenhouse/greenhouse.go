package greenhouse

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	ghapi "github.com/trushilpatel/jobradar/apps/crawler-v2/adapters/ats/greenhouse/api"
	"github.com/trushilpatel/jobradar/apps/crawler-v2/common/logger"
	"github.com/trushilpatel/jobradar/apps/crawler-v2/core"
)

const defaultBaseURL = "https://boards-api.greenhouse.io"

// ── Construction ──────────────────────────────────────────────────────────────

type Greenhouse struct {
	baseURL    string // overridden in tests
	log        logger.Logger
	httpClient *http.Client
}

func New(log logger.Logger, httpClient *http.Client) *Greenhouse {
	return &Greenhouse{
		log:        log.With(logger.Str(logger.FieldAdapter, "greenhouse")),
		httpClient: httpClient,
	}
}

// ── ListingFetcher ────────────────────────────────────────────────────────────

func (g *Greenhouse) FetchJobListing(ctx context.Context, companyID string) (core.JobListing, error) {
	log := g.log.WithContext(ctx).With(logger.Str(logger.FieldCompany, companyID))
	start := time.Now()
	log.Debug("fetching job listing", logger.Str(logger.FieldURL, g.jobListingURL(companyID)))

	var raw ghapi.JobListing
	if err := g.get(ctx, g.jobListingURL(companyID), &raw); err != nil {
		log.Error("listing fetch failed", err, logger.Str(logger.FieldURL, g.jobListingURL(companyID)))
		return core.JobListing{}, fmt.Errorf("greenhouse: listing: %w", err)
	}

	listing := mapJobListing(raw)
	log.Info("listing fetched",
		logger.Int(logger.FieldJobCount, len(listing.Jobs)),
		logger.Dur(logger.FieldDuration, time.Since(start)),
	)
	return listing, nil
}

// ── PostFetcher ───────────────────────────────────────────────────────────────

// FetchJobPost converts the hosted job URL (e.g. https://job-boards.greenhouse.io/{slug}/jobs/{id})
// to the JSON API endpoint and returns a structured JobPost.
func (g *Greenhouse) FetchJobPost(ctx context.Context, summary core.JobSummary) (core.JobPost, error) {
	log := g.log.WithContext(ctx).With(logger.Str(logger.FieldJobID, summary.ID))
	start := time.Now()
	log.Debug("fetching job post", logger.Str(logger.FieldURL, summary.URL))

	slug, id, err := parseJobURL(summary.URL)
	if err != nil {
		return core.JobPost{}, fmt.Errorf("greenhouse: %w", err)
	}
	jobPostURL := g.jobPostURL(slug, id)

	var raw ghapi.JobPost
	if err := g.get(ctx, jobPostURL, &raw); err != nil {
		log.Error("post fetch failed", err, logger.Str(logger.FieldURL, jobPostURL))
		return core.JobPost{}, fmt.Errorf("greenhouse: post: %w", err)
	}

	post := mapJobPost(raw)
	log.Info("job post fetched", logger.Dur(logger.FieldDuration, time.Since(start)))
	return post, nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// get performs a GET request and JSON-decodes the response body into dst.
func (g *Greenhouse) get(ctx context.Context, url string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
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

// parseJobURL extracts the company slug and job ID from a Greenhouse hosted URL.
// Expected path: /{slug}/jobs/{id}
func parseJobURL(rawURL string) (slug, id string, err error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", "", fmt.Errorf("parse job URL: %w", err)
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) != 3 || parts[1] != "jobs" {
		return "", "", fmt.Errorf("unexpected URL path %q", u.Path)
	}
	return parts[0], parts[2], nil
}

func (g *Greenhouse) jobListingURL(slug string) string {
	return fmt.Sprintf("%s/v1/boards/%s/jobs", g.resolveBaseURL(), slug)
}

func (g *Greenhouse) jobPostURL(slug, id string) string {
	return fmt.Sprintf("%s/v1/boards/%s/jobs/%s", g.resolveBaseURL(), slug, id)
}

func (g *Greenhouse) resolveBaseURL() string {
	if g.baseURL != "" {
		return g.baseURL
	}
	return defaultBaseURL
}

// ── Interface checks ──────────────────────────────────────────────────────────

var (
	_ core.ListingFetcher = (*Greenhouse)(nil)
	_ core.PostFetcher    = (*Greenhouse)(nil)
)
