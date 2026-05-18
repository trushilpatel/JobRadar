// Package eightfold implements the ATS adapter for Eightfold AI's SmartApply
// product — the JSON API that powers careers boards for HSBC, Bloomberg,
// Activision, Capital One, Bristol Myers Squibb, Vodafone, and many other
// large enterprises.
//
// Unlike Greenhouse or Lever (single endpoint, single slug), every Eightfold
// customer is served from its own vanity host (`explore.jobs.<company>.net`,
// `careers.<company>.com`, `<sub>.eightfold.ai`, …) and its own domain query
// parameter (e.g. `domain=netflix.com`). Neither can be derived from the
// company slug alone, so this adapter implements ConfigurableAdapter and is
// driven by the per-company `company_scan_settings.adapter_config` JSON blob.
//
// Two-phase adapter: FetchJobsWithConfig paginates `/api/apply/v2/jobs` and
// emits thin FetchJobsItem rows (FullContent left nil). The analyzer then
// calls FetchJobContent → `/api/apply/v2/jobs/{id}` to retrieve the full job
// description.
//
// Implementation note: the public Eightfold list endpoint is gated by host
// (`app.eightfold.ai` returns 403 unless the matching customer host is the
// SNI/Host header), which is why we keep `host` configurable rather than
// hardcoding a central endpoint.
package eightfold

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/trushilpatel/jobradar/apps/crawler/ats/types"
	companycommon "github.com/trushilpatel/jobradar/service/company/common"
	jobcommon "github.com/trushilpatel/jobradar/service/job/common"
)

const (
	// defaultPageSize matches Eightfold's documented max-per-page (10). Values
	// larger than 10 are silently capped server-side on every host we have
	// probed (netflix, hsbc, activision, bloomberg).
	defaultPageSize = 10

	// defaultMaxJobsPerScan caps the total number of listings returned by
	// FetchJobsWithConfig when the caller does not set max_jobs_per_scan. Most
	// Eightfold boards top out at a few thousand open postings; the cap is a
	// defensive ceiling rather than a true business limit.
	defaultMaxJobsPerScan = 5000

	// hardPageCap is a defensive ceiling on the pagination loop — even if the
	// server keeps returning full pages forever, we stop after this many
	// requests. At pageSize=10 → 1000 pages → 10 000 jobs.
	hardPageCap = 1000

	// interPageDelay is the cooldown between paginated listing requests.
	// Eightfold's CDN (CloudFront + custom) tolerates rapid pagination, but a
	// short pause keeps us well under any per-IP rate threshold we've observed
	// across hosts.
	interPageDelay = 150 * time.Millisecond

	// userAgent — Eightfold sits behind CloudFront on most customer hosts and
	// the default Go user agent gets 403'd intermittently. A real browser UA
	// is required for stable scans.
	userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 " +
		"(KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"
)

// Eightfold implements types.Adapter, types.ConfigurableAdapter and
// types.ContentFetcher against Eightfold AI's public `/api/apply/v2` endpoints.
type Eightfold struct {
	// httpClient is overridable in tests; nil falls back to http.DefaultClient.
	httpClient *http.Client

	// interPageDelay overrides the production cooldown in tests.
	interPageDelay time.Duration
}

// New constructs an Eightfold adapter with production defaults.
func New() *Eightfold {
	return &Eightfold{
		httpClient:     http.DefaultClient,
		interPageDelay: interPageDelay,
	}
}

// ── Config ─────────────────────────────────────────────────────────────────

// adapterConfig is the per-company JSON blob stored in
// `company_scan_settings.adapter_config` and supplied to FetchJobsWithConfig.
//
// Example:
//
//	{"host": "explore.jobs.netflix.net", "domain": "netflix.com"}
type adapterConfig struct {
	// Host is the FQDN serving the Eightfold API for this customer — e.g.
	// "explore.jobs.netflix.net", "careers.activision.com", "hsbc.eightfold.ai".
	// Must NOT include a scheme or trailing slash.
	Host string `json:"host"`

	// Domain is the value passed as `?domain=` to every API call — e.g.
	// "netflix.com". Required by Eightfold; the listing endpoint returns an
	// empty result set when the domain doesn't match the customer's branding.
	Domain string `json:"domain"`

	// MaxJobsPerScan optionally caps the total listings collected; 0 → default.
	MaxJobsPerScan int `json:"max_jobs_per_scan,omitempty"`

	// Locale optionally pins the `?hl=` query parameter (default "en").
	Locale string `json:"locale,omitempty"`
}

// validate enforces the host/domain invariants. Returns a wrapped error
// suitable for surfacing through the engine.
func (c adapterConfig) validate() error {
	host := strings.TrimSpace(c.Host)
	if host == "" {
		return errors.New("host is required")
	}
	if strings.ContainsAny(host, " /") || strings.Contains(host, "://") {
		return fmt.Errorf("host %q must be a bare host (no scheme, path, or whitespace)", host)
	}
	// Strip an optional ":port" suffix before requiring a dot. Ports are only
	// expected in tests (httptest servers run on 127.0.0.1:<port>); production
	// hosts are FQDNs without an explicit port.
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

// ── API response types ─────────────────────────────────────────────────────
// Mirrors only the documented fields we use across customers. Eightfold
// returns many additional fields (branding, candidate, signUpConfig, …) that
// we ignore by leaving them out of the struct — json.Decoder skips unknown
// keys by default.

// listResponse is the top-level JSON from `/api/apply/v2/jobs`.
type listResponse struct {
	Positions []position `json:"positions"`
	Count     int        `json:"count"`
}

// position mirrors a single entry in the Eightfold `positions` array. IDs and
// timestamps are numbers in the real API — not strings. Some hosts return
// `t_create` and `t_update` as 0 for unpublished/draft postings.
type position struct {
	ID                  int64    `json:"id"`
	Name                string   `json:"name"`
	PostingName         string   `json:"posting_name"`
	Location            string   `json:"location"`
	Locations           []string `json:"locations"`
	Department          string   `json:"department"`
	BusinessUnit        string   `json:"business_unit"`
	TCreate             int64    `json:"t_create"`
	TUpdate             int64    `json:"t_update"`
	DisplayJobID        string   `json:"display_job_id"`
	WorkLocationOption  string   `json:"work_location_option"`
	LocationFlexibility string   `json:"location_flexibility"`
	CanonicalURL        string   `json:"canonicalPositionUrl"`
	JobDescription      string   `json:"job_description"`
	Type                string   `json:"type"`
}

// positionDetail is the JSON returned by `/api/apply/v2/jobs/{id}`. The
// response is flat — position fields are at the top level — plus a handful of
// detail-only keys we ignore (custom_JD, preApplyInfoBanner, …).
type positionDetail = position

// ── Adapter implementation ─────────────────────────────────────────────────

// CareersURL returns the public careers landing page for the host encoded in
// the company URL. Because the adapter is ConfigurableAdapter-driven, the
// canonical careers URL is whatever is stored in `companies.careers_url` —
// this helper is only used by older code paths and degrades gracefully to a
// best-effort URL when called with a slug we cannot map.
func (e *Eightfold) CareersURL(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	// Best-effort fallback: assume the id is the configured `host` or a slug
	// embedded in the host. Callers that know the host should prefer
	// `companies.careers_url` directly.
	if strings.Contains(id, ".") {
		return "https://" + id + "/careers"
	}
	return "https://" + id + ".eightfold.ai/careers"
}

// FetchJobs is required by types.Adapter but cannot run without a per-company
// config — Eightfold host/domain pairs cannot be inferred from the company
// slug. The engine should call FetchJobsWithConfig instead; this method always
// errors.
func (e *Eightfold) FetchJobs(_ context.Context, _ string) (types.FetchJobsResponse, error) {
	return types.FetchJobsResponse{}, errors.New("eightfold: requires adapter_config; use FetchJobsWithConfig")
}

// FetchJobsWithConfig paginates the Eightfold listing API for the configured
// host/domain and returns every open position up to MaxJobsPerScan. Does NOT
// populate FullContent — Eightfold is a two-phase adapter; the analyzer calls
// FetchJobContent for each posting.
func (e *Eightfold) FetchJobsWithConfig(ctx context.Context, companyID string, raw json.RawMessage) (types.FetchJobsResponse, error) {
	if strings.TrimSpace(companyID) == "" {
		return types.FetchJobsResponse{}, errors.New("eightfold: empty companyID")
	}
	if len(raw) == 0 {
		return types.FetchJobsResponse{}, errors.New("eightfold: adapter_config is required")
	}

	var cfg adapterConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return types.FetchJobsResponse{}, fmt.Errorf("eightfold: decode config: %w", err)
	}
	if err := cfg.validate(); err != nil {
		return types.FetchJobsResponse{}, fmt.Errorf("eightfold: invalid config: %w", err)
	}

	maxJobs := cfg.MaxJobsPerScan
	if maxJobs <= 0 {
		maxJobs = defaultMaxJobsPerScan
	}

	items := make([]types.FetchJobsItem, 0, defaultPageSize)
	start := 0
	for page := 0; page < hardPageCap; page++ {
		listURL := buildListURL(cfg, start)
		body, status, err := e.getJSON(ctx, listURL)
		if err != nil {
			return types.FetchJobsResponse{}, fmt.Errorf("eightfold: list fetch start=%d: %w", start, err)
		}
		if status == http.StatusNotFound {
			// Board retired or domain rejected — treat as empty so the engine
			// records a clean zero-job scan rather than a hard error.
			return types.FetchJobsResponse{Jobs: items}, nil
		}
		if status != http.StatusOK {
			return types.FetchJobsResponse{}, fmt.Errorf("eightfold: list status %d at start=%d", status, start)
		}

		var resp listResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return types.FetchJobsResponse{}, fmt.Errorf("eightfold: list decode start=%d: %w", start, err)
		}

		for _, p := range resp.Positions {
			items = append(items, positionToItem(p, cfg))
			if len(items) >= maxJobs {
				return types.FetchJobsResponse{Jobs: items}, nil
			}
		}

		// Eightfold returns fewer than pageSize results on the last page and 0
		// results past the end — both are terminal signals.
		if len(resp.Positions) < defaultPageSize {
			break
		}
		start += defaultPageSize

		if e.interPageDelay > 0 {
			select {
			case <-ctx.Done():
				return types.FetchJobsResponse{}, ctx.Err()
			case <-time.After(e.interPageDelay):
			}
		}
	}

	return types.FetchJobsResponse{Jobs: items}, nil
}

// FetchJobContent fetches the structured detail JSON for one posting and
// returns a FetchJobContentResponse with the description plain-text stripped.
//
// The detail URL is reconstructed from RawJob.URL — the canonical position URL
// stored at scan time. We extract the host + position ID and the domain query
// parameter is recovered from RawJob.URL's `domain=` if present, otherwise
// derived from the host's apex (e.g. host="careers.activision.com" → apex
// "activision.com"). This keeps the analyzer pipeline independent of the
// per-company adapter_config blob.
func (e *Eightfold) FetchJobContent(ctx context.Context, job types.RawJob) (types.FetchJobContentResponse, error) {
	scheme, host, posID, domain, err := parseJobURL(job.URL)
	if err != nil {
		return types.FetchJobContentResponse{}, fmt.Errorf("eightfold: parse job URL: %w", err)
	}

	detailURL := fmt.Sprintf("%s://%s/api/apply/v2/jobs/%s?domain=%s", scheme, host, posID, url.QueryEscape(domain))
	body, status, err := e.getJSON(ctx, detailURL)
	if err != nil {
		return types.FetchJobContentResponse{}, fmt.Errorf("eightfold: detail fetch: %w", err)
	}
	if status != http.StatusOK {
		return types.FetchJobContentResponse{}, fmt.Errorf("eightfold: detail status %d for %s", status, posID)
	}

	var d positionDetail
	if err := json.Unmarshal(body, &d); err != nil {
		return types.FetchJobContentResponse{}, fmt.Errorf("eightfold: detail decode: %w", err)
	}

	return detailToContent(d, job.URL), nil
}

// ── HTTP plumbing ──────────────────────────────────────────────────────────

func (e *Eightfold) getJSON(ctx context.Context, target string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)

	resp, err := e.client().Do(req)
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

func (e *Eightfold) client() *http.Client {
	if e.httpClient != nil {
		return e.httpClient
	}
	return http.DefaultClient
}

// ── URL builders / parsers ─────────────────────────────────────────────────

// buildListURL composes the paginated list URL for a config + page offset.
func buildListURL(cfg adapterConfig, start int) string {
	q := url.Values{}
	q.Set("domain", cfg.Domain)
	q.Set("hl", cfg.resolveLocale())
	q.Set("start", fmt.Sprintf("%d", start))
	q.Set("num", fmt.Sprintf("%d", defaultPageSize))
	return fmt.Sprintf("https://%s/api/apply/v2/jobs?%s", cfg.Host, q.Encode())
}

// parseJobURL extracts (host, positionID, domain) from a canonical Eightfold
// job URL of the form `https://<host>/careers/job/<id>` or
// `https://<host>/careers/job/<id>?domain=<x>`. Returns an error for malformed
// inputs (no host, missing /careers/job/ segment, non-numeric id).
//
// When the URL does not carry a `domain=` query parameter, the apex of `host`
// is used (e.g. `careers.activision.com` → `activision.com`). This matches the
// pattern used by every Eightfold customer we have observed.
func parseJobURL(raw string) (scheme, host, positionID, domain string, err error) {
	u, perr := url.Parse(strings.TrimSpace(raw))
	if perr != nil {
		return "", "", "", "", fmt.Errorf("parse url: %w", perr)
	}
	if u.Scheme == "" || u.Host == "" {
		return "", "", "", "", fmt.Errorf("url %q must be absolute", raw)
	}

	// Strip leading slash and find `careers/job/<id>` segment. Eightfold also
	// emits `/careers/job/<id>/apply/...` variants; we tolerate any trailing
	// segments after the id.
	path := strings.TrimPrefix(u.Path, "/")
	const jobSeg = "careers/job/"
	idx := strings.Index(path, jobSeg)
	if idx < 0 {
		return "", "", "", "", fmt.Errorf("path %q missing `/careers/job/` segment", u.Path)
	}
	rest := path[idx+len(jobSeg):]
	if rest == "" {
		return "", "", "", "", fmt.Errorf("path %q has empty position id", u.Path)
	}
	// Truncate at next slash.
	if slash := strings.Index(rest, "/"); slash >= 0 {
		rest = rest[:slash]
	}
	if rest == "" {
		return "", "", "", "", fmt.Errorf("path %q has empty position id after truncation", u.Path)
	}

	domain = strings.TrimSpace(u.Query().Get("domain"))
	if domain == "" {
		domain = apexDomain(u.Host)
	}
	return u.Scheme, u.Host, rest, domain, nil
}

// apexDomain returns the apex (last two labels) of a hostname — e.g.
// `careers.activision.com` → `activision.com`, `explore.jobs.netflix.net` →
// `netflix.net`. For short hosts (one or two labels) the host is returned
// unchanged.
func apexDomain(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	parts := strings.Split(host, ".")
	if len(parts) < 2 {
		return host
	}
	return strings.Join(parts[len(parts)-2:], ".")
}

// ── Converters ─────────────────────────────────────────────────────────────

func positionToItem(p position, cfg adapterConfig) types.FetchJobsItem {
	loc := p.Location
	if loc == "" && len(p.Locations) > 0 {
		loc = strings.Join(p.Locations, "; ")
	}

	jobURL := p.CanonicalURL
	if jobURL == "" {
		jobURL = fmt.Sprintf("https://%s/careers/job/%d", cfg.Host, p.ID)
	}

	return types.FetchJobsItem{
		ID:             fmt.Sprintf("%s-%d", companycommon.AdapterEightfold, p.ID),
		Title:          firstNonEmpty(p.PostingName, p.Name),
		URL:            jobURL,
		Location:       loc,
		EmploymentType: mapEmploymentType(p.Type),
		FullContent:    nil, // thin adapter — detail fetched in phase two
	}
}

func detailToContent(p position, fallbackURL string) types.FetchJobContentResponse {
	loc := p.Location
	if loc == "" && len(p.Locations) > 0 {
		loc = strings.Join(p.Locations, "; ")
	}

	jobURL := p.CanonicalURL
	if jobURL == "" {
		jobURL = fallbackURL
	}

	return types.FetchJobContentResponse{
		ID:             fmt.Sprintf("%s-%d", companycommon.AdapterEightfold, p.ID),
		Title:          firstNonEmpty(p.PostingName, p.Name),
		Location:       loc,
		URL:            jobURL,
		Description:    types.StripHTML(p.JobDescription),
		EmploymentType: mapEmploymentType(p.Type),
		Department:     p.Department,
		WorkplaceType:  p.WorkLocationOption,
		PublishedDate:  unixToRFC3339(p.TCreate),
	}
}

// firstNonEmpty returns the first non-blank string. Eightfold sometimes
// populates `posting_name` (the user-facing title) and leaves `name` as the
// internal requisition title; we prefer the user-facing one.
func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if t := strings.TrimSpace(v); t != "" {
			return t
		}
	}
	return ""
}

// unixToRFC3339 converts a Unix timestamp (seconds) to an RFC 3339 string in
// UTC. Returns empty string for zero / unset values.
func unixToRFC3339(ts int64) string {
	if ts <= 0 {
		return ""
	}
	return time.Unix(ts, 0).UTC().Format(time.RFC3339)
}

// mapEmploymentType normalises Eightfold's free-form `type` field onto the
// internal jobcommon constants. Eightfold uses values like "Full Time",
// "Part Time", "Contract", "Internship", "Contingent Worker" — we map known
// values and fall through to full_time, consistent with every other adapter.
func mapEmploymentType(t string) string {
	switch strings.ToLower(strings.TrimSpace(strings.ReplaceAll(t, "-", " "))) {
	case "full time", "full_time", "fulltime", "regular", "regular full time", "permanent":
		return jobcommon.EmployFullTime
	case "part time", "part_time", "parttime", "regular part time":
		return jobcommon.EmployPartTime
	case "contract", "contractor", "consultant", "contingent worker", "fixed term", "temporary":
		return jobcommon.EmployContract
	case "intern", "internship", "trainee":
		return jobcommon.EmployInternship
	default:
		return jobcommon.EmployFullTime
	}
}

// Compile-time interface checks.
var (
	_ types.Adapter             = (*Eightfold)(nil)
	_ types.ConfigurableAdapter = (*Eightfold)(nil)
	_ types.ContentFetcher      = (*Eightfold)(nil)
	_ types.URLProvider         = (*Eightfold)(nil)
)
