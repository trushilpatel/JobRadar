// Package eightfold implements the ATS adapter for Eightfold AI's SmartApply
// product — the JSON API that powers careers boards for Netflix, Bloomberg,
// HSBC, Activision, Capital One, Vodafone, and many other large enterprises.
//
// Every Eightfold customer is served from its own vanity host (e.g.
// explore.jobs.netflix.net, careers.activision.com) with its own domain query
// parameter. Neither can be derived from a company slug alone, so this adapter
// implements ConfigurableListingFetcher and is driven by per-company JSON config.
//
// Two-phase adapter: FetchJobListingWithConfig paginates /api/apply/v2/jobs and
// returns thin JobSummary rows. The enricher then calls FetchJobPost →
// /api/apply/v2/jobs/{id} to retrieve the full job description.
package eightfold

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	efapi "github.com/trushilpatel/jobradar/apps/crawler-v2/adapters/ats/eightfold/api"
	"github.com/trushilpatel/jobradar/apps/crawler-v2/common/logger"
	"github.com/trushilpatel/jobradar/common/ats/core"
)

const (
	// defaultPageSize matches Eightfold's documented max per page (10). Values
	// larger than 10 are silently capped server-side.
	defaultPageSize = 10

	// defaultMaxJobsPerScan is the upper bound when max_jobs_per_scan is absent.
	defaultMaxJobsPerScan = 5000

	// hardPageCap prevents an infinite loop — even if the server returns full
	// pages indefinitely, the adapter stops after this many requests.
	hardPageCap = 1000

	// defaultInterPageDelay is the cooldown between paginated listing requests.
	// Eightfold's CDN (CloudFront + custom) tolerates rapid pagination, but a
	// short pause keeps the adapter under any per-IP rate threshold observed
	// across customer hosts.
	defaultInterPageDelay = 150 * time.Millisecond

	// userAgent — Eightfold sits behind CloudFront on most customer hosts;
	// the default Go user agent is intermittently 403'd. A real browser UA is
	// required for stable scans.
	userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 " +
		"(KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"
)

// ── Config ─────────────────────────────────────────────────────────────────────

// adapterConfig is the per-company JSON blob stored in adapter_config and
// supplied to FetchJobListingWithConfig.
//
// Example:
//
//	{"host": "explore.jobs.netflix.net", "domain": "netflix.com"}
type adapterConfig struct {
	// Host is the FQDN serving the Eightfold API for this customer — e.g.
	// "explore.jobs.netflix.net". Must NOT include a scheme or trailing slash.
	Host string `json:"host"`

	// Domain is the value passed as ?domain= to every API call — e.g. "netflix.com".
	// Required; the listing endpoint returns an empty result when the domain
	// doesn't match the customer's branding.
	Domain string `json:"domain"`

	// MaxJobsPerScan optionally caps the total listings collected; 0 → default.
	MaxJobsPerScan int `json:"max_jobs_per_scan,omitempty"`

	// Locale optionally pins the ?hl= query parameter; defaults to "en".
	Locale string `json:"locale,omitempty"`
}

// validate enforces host/domain invariants.
func (c adapterConfig) validate() error {
	host := strings.TrimSpace(c.Host)
	if host == "" {
		return errors.New("host is required")
	}
	if strings.Contains(host, "://") {
		return fmt.Errorf("host %q must be a bare FQDN with no scheme", host)
	}
	// Strip an optional ":port" suffix before requiring a dot. Ports are only
	// expected in tests (httptest servers run on 127.0.0.1:<port>).
	bare := host
	if i := strings.LastIndex(bare, ":"); i > 0 {
		bare = bare[:i]
	}
	if !strings.Contains(bare, ".") {
		return fmt.Errorf("host %q is not a valid FQDN", host)
	}
	if strings.TrimSpace(c.Domain) == "" {
		return errors.New("domain is required")
	}
	return nil
}

// resolveLocale falls back to "en" when the caller didn't pin one.
func (c adapterConfig) resolveLocale() string {
	if s := strings.TrimSpace(c.Locale); s != "" {
		return s
	}
	return "en"
}

// ── Adapter ────────────────────────────────────────────────────────────────────

// Eightfold implements core.ListingFetcher, core.ConfigurableListingFetcher,
// and core.PostFetcher against Eightfold AI's public /api/apply/v2 endpoints.
type Eightfold struct {
	httpClient     *http.Client
	interPageDelay time.Duration // 150ms in production; 0 in tests
	scheme         string        // "https" in production; "http" in tests
	log            logger.Logger
}

// New constructs an Eightfold adapter with production defaults.
func New(log logger.Logger, httpClient *http.Client) *Eightfold {
	return &Eightfold{
		log:            log.With(logger.Str(logger.FieldAdapter, "eightfold")),
		httpClient:     httpClient,
		interPageDelay: defaultInterPageDelay,
		scheme:         "https",
	}
}

// ── ListingFetcher (stub) ──────────────────────────────────────────────────────

// FetchJobListing satisfies core.ListingFetcher but always returns an error
// directing callers to FetchJobListingWithConfig. Eightfold host/domain pairs
// cannot be inferred from a company slug alone.
func (e *Eightfold) FetchJobListing(_ context.Context, _ string) (core.JobListing, error) {
	return core.JobListing{}, errors.New("eightfold: requires config; use FetchJobListingWithConfig")
}

// ── ConfigurableListingFetcher ─────────────────────────────────────────────────

// FetchJobListingWithConfig paginates the Eightfold listing API for the
// configured host/domain and returns every open position up to MaxJobsPerScan.
func (e *Eightfold) FetchJobListingWithConfig(
	ctx context.Context,
	companyID string,
	rawConfig json.RawMessage,
) (core.JobListing, error) {
	log := e.log.WithContext(ctx).With(logger.Str(logger.FieldCompany, companyID))
	start := time.Now()

	if len(rawConfig) == 0 {
		return core.JobListing{}, errors.New("eightfold: adapter config is required")
	}

	var cfg adapterConfig
	if err := json.Unmarshal(rawConfig, &cfg); err != nil {
		return core.JobListing{}, fmt.Errorf("eightfold: decode config: %w", err)
	}
	if err := cfg.validate(); err != nil {
		return core.JobListing{}, fmt.Errorf("eightfold: invalid config: %w", err)
	}

	maxJobs := cfg.MaxJobsPerScan
	if maxJobs <= 0 {
		maxJobs = defaultMaxJobsPerScan
	}

	jobs := make([]core.JobSummary, 0, defaultPageSize)
	offset := 0
	for page := 0; page < hardPageCap; page++ {
		listURL := e.buildListURL(cfg, offset)
		log.Debug("fetching listing page", logger.Str(logger.FieldURL, listURL))

		var resp efapi.ListResponse
		if err := e.get(ctx, listURL, &resp); err != nil {
			log.Error("listing page fetch failed", err, logger.Str(logger.FieldURL, listURL))
			return core.JobListing{}, fmt.Errorf("eightfold: listing start=%d: %w", offset, err)
		}

		for _, p := range resp.Positions {
			jobs = append(jobs, mapJobSummary(p, cfg.Host))
			if len(jobs) >= maxJobs {
				log.Info("listing fetched (cap reached)",
					logger.Int(logger.FieldJobCount, len(jobs)),
					logger.Dur(logger.FieldDuration, time.Since(start)),
				)
				return core.JobListing{Jobs: jobs}, nil
			}
		}

		// Eightfold returns fewer than pageSize results on the last page — this
		// is the stop signal.
		if len(resp.Positions) < defaultPageSize {
			break
		}
		offset += defaultPageSize

		if e.interPageDelay > 0 {
			select {
			case <-ctx.Done():
				return core.JobListing{}, ctx.Err()
			case <-time.After(e.interPageDelay):
			}
		}
	}

	log.Info("listing fetched",
		logger.Int(logger.FieldJobCount, len(jobs)),
		logger.Dur(logger.FieldDuration, time.Since(start)),
	)
	return core.JobListing{Jobs: jobs}, nil
}

// ── PostFetcher ────────────────────────────────────────────────────────────────

// FetchJobPost fetches the structured detail JSON for one posting.
// summary.URL must be a canonical Eightfold job URL of the form
// https://{host}/careers/job/{id}; the host and position ID are extracted to
// build the detail API URL. The domain is recovered from a ?domain= query
// parameter when present, otherwise inferred from the host's apex.
func (e *Eightfold) FetchJobPost(ctx context.Context, summary core.JobSummary) (core.JobPost, error) {
	log := e.log.WithContext(ctx).With(logger.Str(logger.FieldJobID, summary.ID))
	start := time.Now()
	log.Debug("fetching job post", logger.Str(logger.FieldURL, summary.URL))

	host, posID, domain, err := parseJobURL(summary.URL)
	if err != nil {
		return core.JobPost{}, fmt.Errorf("eightfold: parse job URL: %w", err)
	}

	detailURL := fmt.Sprintf("%s://%s/api/apply/v2/jobs/%s?domain=%s",
		e.resolveScheme(), host, posID, url.QueryEscape(domain))

	var pos efapi.Position
	if err := e.get(ctx, detailURL, &pos); err != nil {
		log.Error("post fetch failed", err, logger.Str(logger.FieldURL, detailURL))
		return core.JobPost{}, fmt.Errorf("eightfold: post: %w", err)
	}

	post := mapJobPost(pos)
	log.Info("job post fetched", logger.Dur(logger.FieldDuration, time.Since(start)))
	return post, nil
}

// ── HTTP helpers ───────────────────────────────────────────────────────────────

// get performs a GET request with the required User-Agent header and
// JSON-decodes the response body into dst.
func (e *Eightfold) get(ctx context.Context, target string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := e.httpClient.Do(req)
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

// ── URL builders / parsers ─────────────────────────────────────────────────────

// buildListURL composes the paginated list URL for a config + page offset.
func (e *Eightfold) buildListURL(cfg adapterConfig, start int) string {
	q := url.Values{}
	q.Set("domain", cfg.Domain)
	q.Set("hl", cfg.resolveLocale())
	q.Set("start", fmt.Sprintf("%d", start))
	q.Set("num", fmt.Sprintf("%d", defaultPageSize))
	return fmt.Sprintf("%s://%s/api/apply/v2/jobs?%s", e.resolveScheme(), cfg.Host, q.Encode())
}

// resolveScheme returns the URL scheme; defaults to "https" in production.
func (e *Eightfold) resolveScheme() string {
	if e.scheme != "" {
		return e.scheme
	}
	return "https"
}

// parseJobURL extracts (host, positionID, domain) from a canonical Eightfold
// job URL of the form https://<host>/careers/job/<id>. When the URL carries a
// ?domain= query parameter, that value is used as the domain; otherwise the
// apex of the host is inferred (e.g. explore.jobs.netflix.net → netflix.net).
func parseJobURL(raw string) (host, positionID, domain string, err error) {
	u, perr := url.Parse(strings.TrimSpace(raw))
	if perr != nil {
		return "", "", "", fmt.Errorf("parse url: %w", perr)
	}
	if u.Scheme == "" || u.Host == "" {
		return "", "", "", fmt.Errorf("url %q must be absolute", raw)
	}

	path := strings.TrimPrefix(u.Path, "/")
	const jobSeg = "careers/job/"
	idx := strings.Index(path, jobSeg)
	if idx < 0 {
		return "", "", "", fmt.Errorf("path %q missing /careers/job/ segment", u.Path)
	}
	rest := path[idx+len(jobSeg):]
	// Truncate at the next slash (e.g. trailing /apply/... variants).
	if slash := strings.Index(rest, "/"); slash >= 0 {
		rest = rest[:slash]
	}
	if rest == "" {
		return "", "", "", fmt.Errorf("path %q has empty position id", u.Path)
	}

	domain = strings.TrimSpace(u.Query().Get("domain"))
	if domain == "" {
		domain = apexDomain(u.Host)
	}
	return u.Host, rest, domain, nil
}

// apexDomain returns the last two labels of a hostname (the apex). For short
// hosts (one or two labels) the host is returned unchanged. A port suffix is
// stripped before computing labels.
//
// Examples:
//
//	explore.jobs.netflix.net  →  netflix.net
//	careers.activision.com    →  activision.com
//	127.0.0.1:12345           →  127.0.0.1
func apexDomain(host string) string {
	// Strip port.
	if i := strings.LastIndex(host, ":"); i > 0 {
		host = host[:i]
	}
	host = strings.ToLower(strings.TrimSpace(host))
	parts := strings.Split(host, ".")
	if len(parts) < 2 {
		return host
	}
	return strings.Join(parts[len(parts)-2:], ".")
}

// ── Interface checks ──────────────────────────────────────────────────────────

var (
	_ core.ListingFetcher             = (*Eightfold)(nil)
	_ core.ConfigurableListingFetcher = (*Eightfold)(nil)
	_ core.PostFetcher                = (*Eightfold)(nil)
)
