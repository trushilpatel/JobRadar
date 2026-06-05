// Package workday implements the ATS adapter for Workday CXS-hosted careers
// boards — `<tenant>.wd{N}.myworkdayjobs.com/<site>`.
//
// Unlike Greenhouse or Lever (single endpoint, single slug), every Workday
// tenant lives on its own subdomain with its own site path; that information
// cannot be derived from the company slug alone. This adapter therefore
// implements ConfigurableListingFetcher and is driven by per-company JSON
// config (`host`, `site`, optional `locale`, optional `max_jobs_per_scan`).
//
// Two-phase adapter: FetchJobListingWithConfig paginates the public CXS
// listing endpoint (POST .../wday/cxs/<tenant>/<site>/jobs) and emits minimal
// JobSummary rows. The enricher then calls FetchJobPost
// (GET .../wday/cxs/<tenant>/<site>/job<externalPath>) to retrieve the full
// job description.
package workday

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	wdapi "github.com/trushilpatel/jobradar/jobingest/adapters/ats/workday/api"
	"github.com/trushilpatel/jobradar/jobingest/common/logger"
	"github.com/trushilpatel/jobradar/jobingest/core"
)

const (
	// defaultPageSize is the limit the Workday CXS endpoint accepts; values
	// larger than 20 are silently capped server-side.
	defaultPageSize = 20

	// defaultMaxJobsPerScan caps the total number of listings returned by
	// FetchJobListingWithConfig when the caller does not set max_jobs_per_scan.
	defaultMaxJobsPerScan = 2000

	// hardPageCap is a defensive ceiling on the pagination loop — even if the
	// server lies about `total` and keeps returning full pages forever, we stop
	// after this many page calls.
	hardPageCap = 200

	// productionInterPageDelay is the cooldown between paginated listing
	// requests. Workday's CXS endpoint is touchy under sustained traffic; a
	// short pause keeps us well under any rate threshold we've observed.
	productionInterPageDelay = 250 * time.Millisecond

	// userAgent — Workday sits behind Cloudfront/Akamai variants that 403 the
	// default Go user agent. A real browser UA is required.
	userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 " +
		"(KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"
)

// Workday implements core.ListingFetcher (stub), core.ConfigurableListingFetcher
// (real), and core.PostFetcher.
type Workday struct {
	httpClient *http.Client

	// interPageDelay overrides the production cooldown in tests (set to 0).
	interPageDelay time.Duration

	log logger.Logger
}

// New constructs a Workday adapter with production defaults.
func New(log logger.Logger, httpClient *http.Client) *Workday {
	return &Workday{
		httpClient:     httpClient,
		interPageDelay: productionInterPageDelay,
		log:            log.With(logger.Str(logger.FieldAdapter, "workday")),
	}
}

// ── ListingFetcher (stub) ─────────────────────────────────────────────────────

// FetchJobListing always errors — Workday tenants/sites cannot be inferred
// from a company slug. Callers must use FetchJobListingWithConfig instead.
func (w *Workday) FetchJobListing(_ context.Context, _ string) (core.JobListing, error) {
	return core.JobListing{}, errors.New("workday: requires config; use FetchJobListingWithConfig")
}

// ── ConfigurableListingFetcher ────────────────────────────────────────────────

// FetchJobListingWithConfig paginates the CXS listings endpoint and returns
// every posting up to the configured max. Does NOT populate JobContent —
// Workday is a two-phase adapter; the enricher calls FetchJobPost for each.
func (w *Workday) FetchJobListingWithConfig(ctx context.Context, companyID string, raw json.RawMessage) (core.JobListing, error) {
	log := w.log.WithContext(ctx).With(logger.Str(logger.FieldCompany, companyID))
	start := time.Now()

	var cfg adapterConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return core.JobListing{}, fmt.Errorf("workday: decode config: %w", err)
	}
	if err := cfg.validate(); err != nil {
		return core.JobListing{}, fmt.Errorf("workday: invalid config: %w", err)
	}

	maxJobs := cfg.MaxJobsPerScan
	if maxJobs <= 0 {
		maxJobs = defaultMaxJobsPerScan
	}

	tenant := cfg.tenant()
	listURL := fmt.Sprintf("https://%s/wday/cxs/%s/%s/jobs", cfg.Host, tenant, cfg.Site)
	log.Debug("fetching job listing", logger.Str(logger.FieldURL, listURL))

	jobs := make([]core.JobSummary, 0, defaultPageSize)
	offset := 0
	for page := 0; page < hardPageCap; page++ {
		reqBody := wdapi.ListJobsRequest{
			AppliedFacets: map[string][]string{},
			Limit:         defaultPageSize,
			Offset:        offset,
			SearchText:    "",
		}
		body, status, err := w.postJSON(ctx, listURL, reqBody)
		if err != nil {
			return core.JobListing{}, fmt.Errorf("workday: listing fetch: %w", err)
		}
		// A 404 means the board has been retired; return a clean empty result.
		if status == http.StatusNotFound {
			log.Info("listing board retired (404)", logger.Str(logger.FieldURL, listURL))
			return core.JobListing{Jobs: jobs}, nil
		}
		if status != http.StatusOK {
			return core.JobListing{}, fmt.Errorf("workday: listing status %d", status)
		}

		var resp wdapi.ListJobsResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return core.JobListing{}, fmt.Errorf("workday: listing decode: %w", err)
		}

		if len(resp.JobPostings) == 0 {
			break
		}
		for _, jp := range resp.JobPostings {
			jobs = append(jobs, mapJobSummary(jp, cfg))
			if len(jobs) >= maxJobs {
				log.Info("listing fetched (max cap)",
					logger.Int(logger.FieldJobCount, len(jobs)),
					logger.Dur(logger.FieldDuration, time.Since(start)),
				)
				return core.JobListing{Jobs: jobs}, nil
			}
		}

		offset += len(resp.JobPostings)
		if resp.Total > 0 && offset >= resp.Total {
			break
		}

		if w.interPageDelay > 0 {
			select {
			case <-ctx.Done():
				return core.JobListing{}, ctx.Err()
			case <-time.After(w.interPageDelay):
			}
		}
	}

	log.Info("listing fetched",
		logger.Int(logger.FieldJobCount, len(jobs)),
		logger.Dur(logger.FieldDuration, time.Since(start)),
	)
	return core.JobListing{Jobs: jobs}, nil
}

// ── PostFetcher ───────────────────────────────────────────────────────────────

// FetchJobPost fetches the structured detail JSON for one posting and returns
// a JobPost with the description HTML-stripped.
func (w *Workday) FetchJobPost(ctx context.Context, summary core.JobSummary) (core.JobPost, error) {
	log := w.log.WithContext(ctx).With(logger.Str(logger.FieldJobID, summary.ID))
	start := time.Now()
	log.Debug("fetching job post", logger.Str(logger.FieldURL, summary.URL))

	host, site, externalPath, err := parseJobURL(summary.URL)
	if err != nil {
		return core.JobPost{}, fmt.Errorf("workday: parse job URL: %w", err)
	}
	tenant := tenantFromHost(host)
	if tenant == "" {
		return core.JobPost{}, fmt.Errorf("workday: cannot derive tenant from host %q", host)
	}

	// externalPath already starts with `/job/…` — concatenate directly.
	// Prepending `/job` produces `/job/job/…` which the CXS API rejects with HTTP 406.
	detailURL := fmt.Sprintf("https://%s/wday/cxs/%s/%s%s", host, tenant, site, externalPath)

	body, status, err := w.getJSON(ctx, detailURL)
	if err != nil {
		return core.JobPost{}, fmt.Errorf("workday: detail fetch: %w", err)
	}
	if status != http.StatusOK {
		return core.JobPost{}, fmt.Errorf("workday: detail status %d", status)
	}

	var d wdapi.DetailResponse
	if err := json.Unmarshal(body, &d); err != nil {
		return core.JobPost{}, fmt.Errorf("workday: detail decode: %w", err)
	}

	post := mapJobPost(d.JobPostingInfo, summary.URL)
	log.Info("job post fetched", logger.Dur(logger.FieldDuration, time.Since(start)))
	return post, nil
}

// ── HTTP plumbing ─────────────────────────────────────────────────────────────

func (w *Workday) postJSON(ctx context.Context, rawURL string, payload any) ([]byte, int, error) {
	buf, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, fmt.Errorf("marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, bytes.NewReader(buf))
	if err != nil {
		return nil, 0, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)

	resp, err := w.client().Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("do: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read: %w", err)
	}
	return body, resp.StatusCode, nil
}

func (w *Workday) getJSON(ctx context.Context, rawURL string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)

	resp, err := w.client().Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("do: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read: %w", err)
	}
	return body, resp.StatusCode, nil
}

func (w *Workday) client() *http.Client {
	if w.httpClient != nil {
		return w.httpClient
	}
	return http.DefaultClient
}

// ── Interface checks ──────────────────────────────────────────────────────────

var (
	_ core.ListingFetcher             = (*Workday)(nil)
	_ core.ConfigurableListingFetcher = (*Workday)(nil)
	_ core.PostFetcher                = (*Workday)(nil)
)
