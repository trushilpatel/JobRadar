package ashby

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	ashbyapi "github.com/trushilpatel/jobradar/apps/crawler-v2/adapters/ats/ashby/api"
	"github.com/trushilpatel/jobradar/apps/crawler-v2/common/logger"
	"github.com/trushilpatel/jobradar/common/ats/core"
)

const (
	defaultBaseURL = "https://jobs.ashbyhq.com"
	userAgent      = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

	listingQuery = `query ApiJobBoardWithTeams($organizationHostedJobsPageName: String!) {
	jobBoardWithTeams(organizationHostedJobsPageName: $organizationHostedJobsPageName) {
		jobPostings {
			id
			title
			locationName
			workplaceType
			employmentType
			compensationTierSummary
		}
	}
}`

	postingQuery = `query ApiJobPosting($organizationHostedJobsPageName: String!, $jobPostingId: String!) {
	jobPosting(
		organizationHostedJobsPageName: $organizationHostedJobsPageName
		jobPostingId: $jobPostingId
	) {
		id
		title
		locationName
		workplaceType
		employmentType
		descriptionHtml
		compensationTierSummary
		publishedDate
	}
}`
)

// ── Construction ──────────────────────────────────────────────────────────────

// Ashby fetches jobs from Ashby's public hosted jobs GraphQL API.
type Ashby struct {
	baseURL    string // overrides defaultBaseURL in tests
	log        logger.Logger
	httpClient *http.Client
}

// New creates an Ashby adapter. httpClient is used for all outgoing requests
// so callers can inject a test server client without touching the production URL.
func New(log logger.Logger, httpClient *http.Client) *Ashby {
	return &Ashby{
		log:        log.With(logger.Str(logger.FieldAdapter, "ashby")),
		httpClient: httpClient,
	}
}

// ── ListingFetcher ────────────────────────────────────────────────────────────

// FetchJobListing retrieves all open job postings for the given Ashby company slug.
func (a *Ashby) FetchJobListing(ctx context.Context, companyID string) (core.JobListing, error) {
	log := a.log.WithContext(ctx).With(logger.Str(logger.FieldCompany, companyID))
	start := time.Now()
	listURL := a.graphqlURL("ApiJobBoardWithTeams")
	log.Debug("fetching job listing", logger.Str(logger.FieldURL, listURL))

	payload := map[string]any{
		"operationName": "ApiJobBoardWithTeams",
		"variables": map[string]string{
			"organizationHostedJobsPageName": companyID,
		},
		"query": listingQuery,
	}

	var raw ashbyapi.ListingResponse
	if err := a.post(ctx, listURL, payload, &raw); err != nil {
		log.Error("listing fetch failed", err, logger.Str(logger.FieldURL, listURL))
		return core.JobListing{}, fmt.Errorf("ashby: listing: %w", err)
	}

	listing := mapJobListing(raw, companyID)
	log.Info("listing fetched",
		logger.Int(logger.FieldJobCount, len(listing.Jobs)),
		logger.Dur(logger.FieldDuration, time.Since(start)),
	)
	return listing, nil
}

// ── PostFetcher ───────────────────────────────────────────────────────────────

// FetchJobPost retrieves the full job posting detail for the given summary.
// summary.URL must be the Ashby hosted jobs URL: https://jobs.ashbyhq.com/{company}/{uuid}
func (a *Ashby) FetchJobPost(ctx context.Context, summary core.JobSummary) (core.JobPost, error) {
	log := a.log.WithContext(ctx).With(logger.Str(logger.FieldJobID, summary.ID))
	start := time.Now()
	log.Debug("fetching job post", logger.Str(logger.FieldURL, summary.URL))

	company, id, err := parseJobURL(summary.URL)
	if err != nil {
		return core.JobPost{}, fmt.Errorf("ashby: %w", err)
	}

	postURL := a.graphqlURL("ApiJobPosting")
	payload := map[string]any{
		"operationName": "ApiJobPosting",
		"variables": map[string]string{
			"organizationHostedJobsPageName": company,
			"jobPostingId":                   id,
		},
		"query": postingQuery,
	}

	var raw ashbyapi.PostingResponse
	if err := a.post(ctx, postURL, payload, &raw); err != nil {
		log.Error("post fetch failed", err, logger.Str(logger.FieldURL, postURL))
		return core.JobPost{}, fmt.Errorf("ashby: post: %w", err)
	}
	if raw.Data.JobPosting.ID == "" {
		return core.JobPost{}, fmt.Errorf("ashby: job not found for id %q", id)
	}

	post := mapJobPost(raw.Data.JobPosting)
	log.Info("job post fetched", logger.Dur(logger.FieldDuration, time.Since(start)))
	return post, nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// post marshals payload, sends a POST request with the required headers, and
// JSON-decodes the response body into dst.
func (a *Ashby) post(ctx context.Context, endpoint string, payload any, dst any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)

	resp, err := a.httpClient.Do(req)
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

// parseJobURL extracts the company slug and job posting UUID from an Ashby
// hosted jobs URL. Expected path: /{company}/{uuid}
func parseJobURL(rawURL string) (company, id string, err error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", "", fmt.Errorf("parse job URL: %w", err)
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("unexpected URL path %q (want /{company}/{uuid})", u.Path)
	}
	return parts[0], parts[1], nil
}

func (a *Ashby) graphqlURL(op string) string {
	base := defaultBaseURL
	if a.baseURL != "" {
		base = a.baseURL
	}
	return strings.TrimRight(base, "/") + "/api/non-user-graphql?op=" + op
}

// ── Interface checks ──────────────────────────────────────────────────────────

var (
	_ core.ListingFetcher = (*Ashby)(nil)
	_ core.PostFetcher    = (*Ashby)(nil)
)
