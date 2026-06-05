package rippling

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	rapi "github.com/trushilpatel/jobradar/jobingest/adapters/ats/rippling/api"
	"github.com/trushilpatel/jobradar/jobingest/common/logger"
	"github.com/trushilpatel/jobradar/jobingest/core"
)

const (
	defaultCareersBaseURL = "https://www.rippling.com"
	defaultATSBaseURL     = "https://ats.rippling.com"

	userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"
)

// Rippling fetches job listings from Rippling's public careers page and detail pages.
//
// Both pages are Next.js SSG/SSR; job data lives in a <script id="__NEXT_DATA__"> tag.
// Only US jobs are returned from the listing page (countryCode == "US" filter).
type Rippling struct {
	careersBaseURL string // overridden in tests
	atsBaseURL     string // overridden in tests
	log            logger.Logger
	httpClient     *http.Client
}

// New creates a Rippling adapter with production defaults.
func New(log logger.Logger, httpClient *http.Client) *Rippling {
	return &Rippling{
		log:        log.With(logger.Str(logger.FieldAdapter, "rippling")),
		httpClient: httpClient,
	}
}

// ── ListingFetcher ────────────────────────────────────────────────────────────

// FetchJobListing fetches the Rippling careers listing page and returns US-only jobs.
// companyID is ignored — Rippling uses a single global careers page.
func (r *Rippling) FetchJobListing(ctx context.Context, _ string) (core.JobListing, error) {
	listingURL := r.listingURL()
	log := r.log.WithContext(ctx).With(logger.Str(logger.FieldURL, listingURL))
	start := time.Now()
	log.Debug("fetching job listing")

	body, err := r.fetchHTML(ctx, listingURL)
	if err != nil {
		log.Error("listing fetch failed", err)
		return core.JobListing{}, fmt.Errorf("rippling: listing: %w", err)
	}

	raw, err := extractNextData(body)
	if err != nil {
		log.Error("extract next data failed", err)
		return core.JobListing{}, fmt.Errorf("rippling: listing: %w", err)
	}

	var data rapi.ListingNextData
	if err := json.Unmarshal(raw, &data); err != nil {
		log.Error("parse listing next data failed", err)
		return core.JobListing{}, fmt.Errorf("rippling: listing: parse: %w", err)
	}

	listing := mapJobListing(data.Props.PageProps.Jobs.Items, r.resolveATSBaseURL())
	log.Info("listing fetched",
		logger.Int(logger.FieldJobCount, len(listing.Jobs)),
		logger.Dur(logger.FieldDuration, time.Since(start)),
	)
	return listing, nil
}

// ── PostFetcher ───────────────────────────────────────────────────────────────

// FetchJobPost fetches the full job details from the Rippling ATS detail page.
// The job ID is extracted from the last path segment of summary.URL.
func (r *Rippling) FetchJobPost(ctx context.Context, summary core.JobSummary) (core.JobPost, error) {
	log := r.log.WithContext(ctx).With(logger.Str(logger.FieldJobID, summary.ID))
	start := time.Now()
	log.Debug("fetching job post", logger.Str(logger.FieldURL, summary.URL))

	jobID, err := extractJobID(summary.URL)
	if err != nil {
		return core.JobPost{}, fmt.Errorf("rippling: post: %w", err)
	}
	postURL := r.postURL(jobID)

	body, err := r.fetchHTML(ctx, postURL)
	if err != nil {
		log.Error("post fetch failed", err, logger.Str(logger.FieldURL, postURL))
		return core.JobPost{}, fmt.Errorf("rippling: post: %w", err)
	}

	raw, err := extractNextData(body)
	if err != nil {
		log.Error("extract detail next data failed", err)
		return core.JobPost{}, fmt.Errorf("rippling: post: %w", err)
	}

	var data rapi.DetailNextData
	if err := json.Unmarshal(raw, &data); err != nil {
		log.Error("parse detail next data failed", err)
		return core.JobPost{}, fmt.Errorf("rippling: post: parse: %w", err)
	}

	post := mapJobPost(data.Props.PageProps.APIData, postURL)
	log.Info("job post fetched", logger.Dur(logger.FieldDuration, time.Since(start)))
	return post, nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// fetchHTML performs a GET request with a browser User-Agent and returns the body bytes.
func (r *Rippling) fetchHTML(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	return data, nil
}

// extractNextData extracts the JSON from <script id="__NEXT_DATA__"...>...</script>.
// Returns the raw JSON bytes or an error if not found or malformed.
func extractNextData(body []byte) ([]byte, error) {
	s := string(body)

	start := strings.Index(s, `<script id="__NEXT_DATA__"`)
	if start == -1 {
		return nil, fmt.Errorf("__NEXT_DATA__ script not found")
	}

	// Find the opening > after the script tag attributes.
	openIdx := strings.Index(s[start:], ">")
	if openIdx == -1 {
		return nil, fmt.Errorf("malformed __NEXT_DATA__ script tag")
	}
	jsonStart := start + openIdx + 1

	// Find the closing </script> tag.
	endIdx := strings.Index(s[jsonStart:], "</script>")
	if endIdx == -1 {
		return nil, fmt.Errorf("__NEXT_DATA__ closing tag not found")
	}

	raw := strings.TrimSpace(s[jsonStart : jsonStart+endIdx])
	return []byte(raw), nil
}

// extractJobID extracts the last path segment from a Rippling ATS URL.
// Expected: https://ats.rippling.com/rippling/jobs/{id}
func extractJobID(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("parse job URL: %w", err)
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) == 0 || parts[len(parts)-1] == "" {
		return "", fmt.Errorf("no job ID in URL path %q", u.Path)
	}
	return parts[len(parts)-1], nil
}

// ── URL builders ──────────────────────────────────────────────────────────────

func (r *Rippling) listingURL() string {
	return fmt.Sprintf("%s/careers/open-roles", r.resolveCareersBaseURL())
}

func (r *Rippling) postURL(jobID string) string {
	return fmt.Sprintf("%s/rippling/jobs/%s", r.resolveATSBaseURL(), jobID)
}

func (r *Rippling) resolveCareersBaseURL() string {
	if r.careersBaseURL != "" {
		return r.careersBaseURL
	}
	return defaultCareersBaseURL
}

func (r *Rippling) resolveATSBaseURL() string {
	if r.atsBaseURL != "" {
		return r.atsBaseURL
	}
	return defaultATSBaseURL
}

// ── Interface checks ──────────────────────────────────────────────────────────

var (
	_ core.ListingFetcher = (*Rippling)(nil)
	_ core.PostFetcher    = (*Rippling)(nil)
)
