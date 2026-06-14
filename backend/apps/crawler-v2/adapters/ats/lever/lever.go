package lever

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	leverapi "github.com/trushilpatel/jobradar/apps/crawler-v2/adapters/ats/lever/api"
	"github.com/trushilpatel/jobradar/apps/crawler-v2/common/logger"
	"github.com/trushilpatel/jobradar/common/ats/core"
)

const defaultBaseURL = "https://api.lever.co"

// ── Construction ──────────────────────────────────────────────────────────────

type Lever struct {
	baseURL    string // overridden in tests; empty = "https://api.lever.co"
	log        logger.Logger
	httpClient *http.Client
}

func New(log logger.Logger, httpClient *http.Client) *Lever {
	return &Lever{
		log:        log.With(logger.Str(logger.FieldAdapter, "lever")),
		httpClient: httpClient,
	}
}

// ── ListingFetcher ────────────────────────────────────────────────────────────

func (l *Lever) FetchJobListing(ctx context.Context, companyID string) (core.JobListing, error) {
	log := l.log.WithContext(ctx).With(logger.Str(logger.FieldCompany, companyID))
	start := time.Now()
	listURL := l.jobListingURL(companyID)
	log.Debug("fetching job listing", logger.Str(logger.FieldURL, listURL))

	var raw []leverapi.JobSummary
	if err := l.get(ctx, listURL, &raw); err != nil {
		log.Error("listing fetch failed", err, logger.Str(logger.FieldURL, listURL))
		return core.JobListing{}, fmt.Errorf("lever: listing: %w", err)
	}

	listing := mapJobListing(raw)
	log.Info("listing fetched",
		logger.Int(logger.FieldJobCount, len(listing.Jobs)),
		logger.Dur(logger.FieldDuration, time.Since(start)),
	)
	return listing, nil
}

// ── PostFetcher ───────────────────────────────────────────────────────────────

// FetchJobPost converts the hosted job URL (e.g. https://jobs.lever.co/{company}/{uuid})
// to the JSON API endpoint and returns a structured JobPost.
func (l *Lever) FetchJobPost(ctx context.Context, summary core.JobSummary) (core.JobPost, error) {
	log := l.log.WithContext(ctx).With(logger.Str(logger.FieldJobID, summary.ID))
	start := time.Now()
	log.Debug("fetching job post", logger.Str(logger.FieldURL, summary.URL))

	company, uuid, err := parseJobURL(summary.URL)
	if err != nil {
		return core.JobPost{}, fmt.Errorf("lever: %w", err)
	}
	jobPostURL := l.jobPostURL(company, uuid)

	var raw leverapi.JobPost
	if err := l.get(ctx, jobPostURL, &raw); err != nil {
		log.Error("post fetch failed", err, logger.Str(logger.FieldURL, jobPostURL))
		return core.JobPost{}, fmt.Errorf("lever: post: %w", err)
	}

	post := mapJobPost(raw)
	log.Info("job post fetched", logger.Dur(logger.FieldDuration, time.Since(start)))
	return post, nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// get performs a GET request and JSON-decodes the response body into dst.
func (l *Lever) get(ctx context.Context, rawURL string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	resp, err := l.httpClient.Do(req)
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

// parseJobURL extracts the company slug and job UUID from a Lever hosted URL.
// Expected path: /{company}/{uuid}
// Example: https://jobs.lever.co/netlify/abc-111-def → "netlify", "abc-111-def"
func parseJobURL(rawURL string) (company, uuid string, err error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", "", fmt.Errorf("parse job URL: %w", err)
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("unexpected URL path %q: expected /{company}/{uuid}", u.Path)
	}
	return parts[0], parts[1], nil
}

func (l *Lever) jobListingURL(companyID string) string {
	return fmt.Sprintf("%s/v0/postings/%s?mode=json", l.resolveBaseURL(), companyID)
}

func (l *Lever) jobPostURL(company, uuid string) string {
	return fmt.Sprintf("%s/v0/postings/%s/%s?mode=json", l.resolveBaseURL(), company, uuid)
}

func (l *Lever) resolveBaseURL() string {
	if l.baseURL != "" {
		return l.baseURL
	}
	return defaultBaseURL
}

// ── Interface checks ──────────────────────────────────────────────────────────

var (
	_ core.ListingFetcher = (*Lever)(nil)
	_ core.PostFetcher    = (*Lever)(nil)
)
