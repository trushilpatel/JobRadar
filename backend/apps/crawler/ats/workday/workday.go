// Package workday implements the ATS adapter for Workday CXS-hosted careers
// boards — `<tenant>.wd{N}.myworkdayjobs.com/<site>`.
//
// Unlike Greenhouse or Lever (single endpoint, single slug), every Workday
// tenant lives on its own subdomain with its own site path; that information
// cannot be derived from the company slug alone. This adapter therefore
// implements ConfigurableAdapter and is driven by the per-company
// `company_scan_settings.adapter_config` JSON blob (`host`, `site`, optional
// `locale`, optional `max_jobs_per_scan`).
//
// Two-phase adapter: FetchJobs paginates the public CXS listing endpoint
// (POST .../wday/cxs/<tenant>/<site>/jobs) and emits minimal FetchJobsItem
// rows. The analyzer then calls FetchJobContent (GET .../wday/cxs/<tenant>/<site>/job<externalPath>)
// to retrieve the full job description.
package workday

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/trushilpatel/jobradar/apps/crawler/ats/types"
	companycommon "github.com/trushilpatel/jobradar/service/company/common"
	jobcommon "github.com/trushilpatel/jobradar/service/job/common"
)

const (
	// defaultPageSize is the limit Workday's CXS endpoint accepts; values
	// larger than 20 are silently capped server-side.
	defaultPageSize = 20

	// defaultMaxJobsPerScan caps the total number of listings returned by
	// FetchJobs when the caller does not set max_jobs_per_scan.
	defaultMaxJobsPerScan = 2000

	// hardPageCap is a defensive ceiling on the pagination loop — even if the
	// server lies about `total` and keeps returning full pages forever, we
	// stop after this many page calls.
	hardPageCap = 200

	// interPageDelay is the cooldown between paginated listing requests.
	// Workday's CXS endpoint is touchy under sustained traffic; a short pause
	// keeps us well under any rate threshold we've observed.
	interPageDelay = 250 * time.Millisecond

	// userAgent — Workday sits behind Cloudfront/Akamai variants that 403 the
	// default Go user agent. A real browser UA is required.
	userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 " +
		"(KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"
)

// hostPattern matches a canonical Workday tenant host such as
// `nvidia.wd5.myworkdayjobs.com` or `polar-semi.wd501.myworkdayjobs.com`.
var hostPattern = regexp.MustCompile(`^[a-z0-9-]+\.wd[0-9]+\.myworkdayjobs\.com$`)

// Workday implements types.Adapter, types.ConfigurableAdapter and
// types.ContentFetcher against Workday's public CXS endpoints.
type Workday struct {
	// httpClient is overridable in tests; nil falls back to http.DefaultClient.
	httpClient *http.Client

	// interPageDelay overrides the production cooldown in tests.
	interPageDelay time.Duration
}

// New constructs a Workday adapter with production defaults.
func New() *Workday {
	return &Workday{
		httpClient:     http.DefaultClient,
		interPageDelay: interPageDelay,
	}
}

// ── Config ─────────────────────────────────────────────────────────────────

// adapterConfig is the per-company JSON blob stored in
// `company_scan_settings.adapter_config` and supplied to FetchJobsWithConfig.
type adapterConfig struct {
	Host           string `json:"host"`                        // e.g. "nvidia.wd5.myworkdayjobs.com"
	Site           string `json:"site"`                        // e.g. "NVIDIAExternalCareerSite"
	Locale         string `json:"locale,omitempty"`            // optional, e.g. "en-US"
	MaxJobsPerScan int    `json:"max_jobs_per_scan,omitempty"` // optional cap; 0 → default
}

// validate enforces the host/site invariants. Returns a wrapped error suitable
// for surfacing through the engine.
func (c adapterConfig) validate() error {
	host := strings.TrimSpace(c.Host)
	if host == "" {
		return errors.New("host is required")
	}
	if !hostPattern.MatchString(host) {
		return fmt.Errorf("host %q does not match pattern <tenant>.wd<N>.myworkdayjobs.com", host)
	}
	site := strings.TrimSpace(c.Site)
	if site == "" {
		return errors.New("site is required")
	}
	if strings.ContainsRune(site, '/') {
		return fmt.Errorf("site %q must not contain '/'", site)
	}
	return nil
}

// tenant extracts the leftmost subdomain of host — e.g.
// "nvidia.wd5.myworkdayjobs.com" → "nvidia".
func (c adapterConfig) tenant() string {
	host := strings.TrimSpace(c.Host)
	idx := strings.Index(host, ".")
	if idx <= 0 {
		return ""
	}
	return host[:idx]
}

// ── CXS response types ─────────────────────────────────────────────────────
// Mirrors the full documented response so adding a downstream field never
// requires touching the HTTP/decode path (per ats/CLAUDE.md design rule).

type listingResponse struct {
	Total       int             `json:"total"`
	JobPostings []rawJobPosting `json:"jobPostings"`
}

type rawJobPosting struct {
	Title         string   `json:"title"`
	ExternalPath  string   `json:"externalPath"`
	LocationsText string   `json:"locationsText"`
	PostedOn      string   `json:"postedOn"`
	BulletFields  []string `json:"bulletFields"`
}

type detailResponse struct {
	JobPostingInfo rawJobPostingInfo `json:"jobPostingInfo"`
}

// rawJobPostingInfo mirrors the real CXS detail response. Field types and names
// here were validated against live Workday tenants (NVIDIA, Adobe, Salesforce);
// notably `country` is an OBJECT (`{descriptor, id}`) not a string, the
// requisition number lives under `jobReqId` (not `jobRequisitionId`), and the
// only ISO-shaped date Workday returns is `startDate` (`postedOn` is a display
// string like "Posted Today"). Fields we don't currently surface to the LLM
// are still captured per the "raw struct mirrors the platform" design rule in
// `apps/crawler/ats/CLAUDE.md`.
type rawJobPostingInfo struct {
	ID                     string          `json:"id"`
	Title                  string          `json:"title"`
	JobDescription         string          `json:"jobDescription"`
	Location               string          `json:"location"`
	AdditionalLocations    json.RawMessage `json:"additionalLocations"`
	PostedOn               string          `json:"postedOn"`
	StartDate              string          `json:"startDate"`
	TimeType               string          `json:"timeType"`
	JobReqID               string          `json:"jobReqId"`
	JobPostingID           string          `json:"jobPostingId"`
	JobPostingSiteID       string          `json:"jobPostingSiteId"`
	Country                json.RawMessage `json:"country"`
	CanApply               bool            `json:"canApply"`
	Posted                 bool            `json:"posted"`
	IncludeResumeParsing   bool            `json:"includeResumeParsing"`
	JobRequisitionLocation json.RawMessage `json:"jobRequisitionLocation"`
	ExternalURL            string          `json:"externalUrl"`
	QuestionnaireID        string          `json:"questionnaireId"`
}

// ── Adapter implementation ─────────────────────────────────────────────────

// FetchJobs is required by types.Adapter but cannot run without a per-company
// config — Workday tenants/sites cannot be inferred from the company slug. The
// engine should call FetchJobsWithConfig instead; this method always errors.
func (w *Workday) FetchJobs(_ context.Context, _ string) (types.FetchJobsResponse, error) {
	return types.FetchJobsResponse{}, errors.New("workday: requires adapter_config; use FetchJobsWithConfig")
}

// FetchJobsWithConfig paginates the CXS listings endpoint and returns every
// posting up to the configured max. Does NOT populate FullContent — Workday is
// a two-phase adapter; the analyzer calls FetchJobContent for each posting.
func (w *Workday) FetchJobsWithConfig(ctx context.Context, companyID string, raw json.RawMessage) (types.FetchJobsResponse, error) {
	if strings.TrimSpace(companyID) == "" {
		return types.FetchJobsResponse{}, errors.New("workday: empty companyID")
	}
	if len(raw) == 0 {
		return types.FetchJobsResponse{}, errors.New("workday: adapter_config is required")
	}

	var cfg adapterConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return types.FetchJobsResponse{}, fmt.Errorf("workday: decode config: %w", err)
	}
	if err := cfg.validate(); err != nil {
		return types.FetchJobsResponse{}, fmt.Errorf("workday: invalid config: %w", err)
	}

	maxJobs := cfg.MaxJobsPerScan
	if maxJobs <= 0 {
		maxJobs = defaultMaxJobsPerScan
	}

	tenant := cfg.tenant()
	listURL := fmt.Sprintf("https://%s/wday/cxs/%s/%s/jobs", cfg.Host, tenant, cfg.Site)

	items := make([]types.FetchJobsItem, 0, defaultPageSize)
	offset := 0
	for page := 0; page < hardPageCap; page++ {
		body, status, err := w.postJSON(ctx, listURL, map[string]any{
			"appliedFacets": map[string]any{},
			"limit":         defaultPageSize,
			"offset":        offset,
			"searchText":    "",
		})
		if err != nil {
			return types.FetchJobsResponse{}, fmt.Errorf("workday: listing fetch: %w", err)
		}
		if status == http.StatusNotFound {
			// Board has been retired; treat as empty result so the engine
			// records a clean zero-job scan rather than a hard error.
			return types.FetchJobsResponse{Jobs: items}, nil
		}
		if status != http.StatusOK {
			return types.FetchJobsResponse{}, fmt.Errorf("workday: listing status %d", status)
		}

		var resp listingResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return types.FetchJobsResponse{}, fmt.Errorf("workday: listing decode: %w", err)
		}

		if len(resp.JobPostings) == 0 {
			break
		}
		for _, jp := range resp.JobPostings {
			items = append(items, jobToItem(jp, cfg))
			if len(items) >= maxJobs {
				return types.FetchJobsResponse{Jobs: items}, nil
			}
		}

		offset += len(resp.JobPostings)
		if resp.Total > 0 && offset >= resp.Total {
			break
		}

		if w.interPageDelay > 0 {
			select {
			case <-ctx.Done():
				return types.FetchJobsResponse{}, ctx.Err()
			case <-time.After(w.interPageDelay):
			}
		}
	}

	return types.FetchJobsResponse{Jobs: items}, nil
}

// FetchJobContent fetches the structured detail JSON for one posting and
// returns a FetchJobContentResponse with the description plain-text stripped.
func (w *Workday) FetchJobContent(ctx context.Context, job types.RawJob) (types.FetchJobContentResponse, error) {
	host, site, externalPath, err := parseJobURL(job.URL)
	if err != nil {
		return types.FetchJobContentResponse{}, fmt.Errorf("workday: parse job URL: %w", err)
	}
	tenant := tenantFromHost(host)
	if tenant == "" {
		return types.FetchJobContentResponse{}, fmt.Errorf("workday: cannot derive tenant from host %q", host)
	}

	// externalPath already starts with `/job/…` — concatenate directly. Prepending
	// `/job` produces `/job/job/…` which the live CXS API rejects with HTTP 406.
	detailURL := fmt.Sprintf("https://%s/wday/cxs/%s/%s%s", host, tenant, site, externalPath)
	body, status, err := w.getJSON(ctx, detailURL)
	if err != nil {
		return types.FetchJobContentResponse{}, fmt.Errorf("workday: detail fetch: %w", err)
	}
	if status != http.StatusOK {
		return types.FetchJobContentResponse{}, fmt.Errorf("workday: detail status %d", status)
	}

	var d detailResponse
	if err := json.Unmarshal(body, &d); err != nil {
		return types.FetchJobContentResponse{}, fmt.Errorf("workday: detail decode: %w", err)
	}

	info := d.JobPostingInfo
	// PublishedDate prefers `startDate` (ISO yyyy-mm-dd) over `postedOn`
	// (display string like "Posted Today") so downstream LLM enrichment has
	// a real date to anchor recency on.
	publishedDate := info.StartDate
	if publishedDate == "" {
		publishedDate = info.PostedOn
	}
	return types.FetchJobContentResponse{
		ID:             buildID(info.JobReqID, externalPath),
		Title:          info.Title,
		Location:       info.Location,
		URL:            job.URL,
		Description:    types.StripHTML(info.JobDescription),
		EmploymentType: mapEmploymentType(info.TimeType),
		PublishedDate:  publishedDate,
	}, nil
}

// ── HTTP plumbing ──────────────────────────────────────────────────────────

func (w *Workday) postJSON(ctx context.Context, url string, payload any) ([]byte, int, error) {
	buf, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, fmt.Errorf("marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(buf))
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

func (w *Workday) getJSON(ctx context.Context, url string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
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

// ── Converters / helpers ───────────────────────────────────────────────────

// jobToItem converts one CXS listing row into a FetchJobsItem suitable for
// publishing on `job_listings.raw`. The CXS listing carries minimal data, so
// EmploymentType defaults to full_time and FullContent is left nil — the
// analyzer fetches the rest via FetchJobContent.
func jobToItem(jp rawJobPosting, cfg adapterConfig) types.FetchJobsItem {
	reqID := extractRequisitionID(jp)
	return types.FetchJobsItem{
		ID:             fmt.Sprintf("%s-%s", companycommon.AdapterWorkday, reqID),
		Title:          jp.Title,
		URL:            fmt.Sprintf("https://%s/%s%s", cfg.Host, cfg.Site, jp.ExternalPath),
		Location:       jp.LocationsText,
		EmploymentType: jobcommon.EmployFullTime,
	}
}

// extractRequisitionID resolves a stable identifier for a posting. Prefers
// bulletFields[0] (which carries the requisition number in every tenant we
// have seen), falls back to the trailing `_<id>` suffix on externalPath, and
// finally to externalPath itself.
func extractRequisitionID(jp rawJobPosting) string {
	for _, b := range jp.BulletFields {
		if s := strings.TrimSpace(b); s != "" {
			return s
		}
	}
	path := strings.TrimSpace(jp.ExternalPath)
	if idx := strings.LastIndex(path, "_"); idx >= 0 && idx+1 < len(path) {
		return path[idx+1:]
	}
	if path != "" {
		return path
	}
	return "unknown"
}

// tenantFromHost returns the leftmost subdomain of a Workday host. Returns
// empty string on a malformed host.
func tenantFromHost(host string) string {
	host = strings.TrimSpace(strings.ToLower(host))
	idx := strings.Index(host, ".")
	if idx <= 0 {
		return ""
	}
	return host[:idx]
}

// parseJobURL extracts (host, site, externalPath) from a Workday job page URL
// of the form `https://<host>/<site>/job<externalPath>`.
func parseJobURL(raw string) (host, site, externalPath string, err error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", "", "", fmt.Errorf("parse url: %w", err)
	}
	if u.Host == "" || !hostPattern.MatchString(u.Host) {
		return "", "", "", fmt.Errorf("host %q is not a Workday tenant", u.Host)
	}
	path := strings.TrimPrefix(u.Path, "/")
	idx := strings.Index(path, "/job/")
	if idx < 0 {
		return "", "", "", fmt.Errorf("path %q missing `/job/` segment", u.Path)
	}
	site = path[:idx]
	if site == "" || strings.ContainsRune(site, '/') {
		return "", "", "", fmt.Errorf("site segment %q invalid", site)
	}
	externalPath = path[idx:]
	return u.Host, site, externalPath, nil
}

// buildID composes the global job ID. Prefers an explicit requisition ID;
// falls back to the trailing path segment.
func buildID(requisitionID, externalPath string) string {
	id := strings.TrimSpace(requisitionID)
	if id == "" {
		if idx := strings.LastIndex(externalPath, "_"); idx >= 0 && idx+1 < len(externalPath) {
			id = externalPath[idx+1:]
		}
	}
	if id == "" {
		id = "unknown"
	}
	return fmt.Sprintf("%s-%s", companycommon.AdapterWorkday, id)
}

// mapEmploymentType normalises Workday's `timeType` strings ("Full time",
// "Part time", "Contract", "Intern") onto the internal jobcommon constants.
// Unknown values default to full_time, consistent with every other adapter.
func mapEmploymentType(t string) string {
	switch strings.ToLower(strings.TrimSpace(strings.ReplaceAll(t, "-", " "))) {
	case "full time", "full_time", "fulltime", "regular":
		return jobcommon.EmployFullTime
	case "part time", "part_time", "parttime":
		return jobcommon.EmployPartTime
	case "contract", "contractor", "consultant", "fixed term":
		return jobcommon.EmployContract
	case "intern", "internship":
		return jobcommon.EmployInternship
	default:
		return jobcommon.EmployFullTime
	}
}

// Compile-time interface checks.
var (
	_ types.Adapter             = (*Workday)(nil)
	_ types.ConfigurableAdapter = (*Workday)(nil)
	_ types.ContentFetcher      = (*Workday)(nil)
)
